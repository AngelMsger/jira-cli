package apiclient

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/url"
	"strings"

	cerrors "github.com/angelmsger/jira-cli/pkg/errors"
	"github.com/angelmsger/jira-cli/pkg/transport"
)

// NormalizeBaseURL trims a trailing slash so the dialect can append the REST
// prefix for either flavor.
func NormalizeBaseURL(raw string) string {
	return strings.TrimRight(raw, "/")
}

// Detect probes baseURL to determine the Jira flavor. The order of attempts is
// chosen to be fast and resilient:
//
//  1. Hostname shortcut. `*.atlassian.net` is the only host suffix Atlassian
//     uses for Cloud tenants, so we can answer Cloud with zero network calls.
//     Custom-domain tenants are rare and still covered by the probe below.
//  2. `<base>/rest/api/2/serverInfo` — present on both flavors, anonymous by
//     default, and self-describing: its deploymentType field is "Cloud" or
//     "Server".
func Detect(ctx context.Context, tc *transport.Client, baseURL string) (Flavor, error) {
	if isAtlassianCloudHost(baseURL) {
		return FlavorCloud, nil
	}
	base := NormalizeBaseURL(baseURL)
	if info, ok := probeServerInfo(ctx, tc, base+"/rest/api/2/serverInfo"); ok {
		if strings.EqualFold(info.DeploymentType, "Cloud") {
			return FlavorCloud, nil
		}
		return FlavorDataCenter, nil
	}
	return FlavorAuto, cerrors.New(cerrors.CategoryNetwork, "DETECT_FAILED",
		"could not determine the Jira flavor; the serverInfo endpoint did not respond").
		WithNextSteps("Set the flavor explicitly with --flavor cloud|datacenter.",
			"jira-cli doctor")
}

// isAtlassianCloudHost reports whether rawURL points at an `*.atlassian.net`
// tenant — the host suffix Atlassian reserves for Cloud instances. Inputs
// without a scheme are tolerated so a user typing `acme.atlassian.net` in the
// wizard is still recognized.
func isAtlassianCloudHost(rawURL string) bool {
	parsed, err := url.Parse(rawURL)
	host := ""
	if err == nil && parsed.Host != "" {
		host = parsed.Hostname()
	} else {
		// Likely missing a scheme; pull the host portion out manually so the
		// fast path still fires for `acme.atlassian.net` and friends.
		s := strings.TrimSpace(rawURL)
		s = strings.TrimPrefix(s, "//")
		if i := strings.IndexAny(s, "/?#"); i >= 0 {
			s = s[:i]
		}
		host = s
	}
	host = strings.ToLower(strings.TrimSpace(host))
	return strings.HasSuffix(host, ".atlassian.net")
}

// serverInfoResponse is the subset of /serverInfo the client reads.
type serverInfoResponse struct {
	Version        string `json:"version"`
	DeploymentType string `json:"deploymentType"`
}

// probeServerInfo fetches and parses a serverInfo endpoint. A 200 JSON answer
// with a deploymentType counts as a hit; 401/403 counts as a reachable server
// that hides serverInfo, treated as Data Center (Cloud serves it anonymously).
func probeServerInfo(ctx context.Context, tc *transport.Client, endpoint string) (serverInfoResponse, bool) {
	var info serverInfoResponse
	req, err := http.NewRequest(http.MethodGet, endpoint, nil)
	if err != nil {
		return info, false
	}
	req.Header.Set("Accept", "application/json")
	resp, err := tc.Do(ctx, req)
	if err != nil {
		return info, false
	}
	defer resp.Body.Close()
	if resp.StatusCode == http.StatusUnauthorized || resp.StatusCode == http.StatusForbidden {
		info.DeploymentType = "Server"
		return info, true
	}
	if resp.StatusCode != http.StatusOK || !strings.Contains(resp.Header.Get("Content-Type"), "json") {
		return info, false
	}
	raw, err := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
	if err != nil || json.Unmarshal(raw, &info) != nil || info.DeploymentType == "" {
		return info, false
	}
	return info, true
}

// Ping verifies connectivity against the configured flavor via serverInfo.
// It does not require credentials; use CurrentUser to verify authentication.
func (c *apiClient) Ping(ctx context.Context) (ServerInfo, error) {
	info := ServerInfo{Flavor: c.flavor, BaseURL: c.baseURL}
	var raw serverInfoResponse
	if err := c.getJSON(ctx, c.apiBase()+"/serverInfo", nil, &raw); err != nil {
		return info, err
	}
	info.Reachable = true
	info.Version = raw.Version
	info.DeploymentType = raw.DeploymentType
	return info, nil
}
