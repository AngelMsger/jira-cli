package apiclient

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"

	"github.com/angelmsger/jira-cli/pkg/constants"
	cerrors "github.com/angelmsger/jira-cli/pkg/errors"
	"github.com/angelmsger/jira-cli/pkg/transport"
)

// Client is the flavor-agnostic Jira API surface. All methods return
// normalized models; flavor-specific request shapes are hidden.
type Client interface {
	Flavor() Flavor
	BaseURL() string
	Ping(ctx context.Context) (ServerInfo, error)

	CurrentUser(ctx context.Context) (*User, error)

	GetIssue(ctx context.Context, opt GetIssueOpts) (*Issue, error)
	SearchIssues(ctx context.Context, opt SearchOpts) (ListResult[Issue], error)
	CreateIssue(ctx context.Context, req CreateIssueReq) (*Issue, error)
	EditIssue(ctx context.Context, req EditIssueReq) (*Issue, error)
	AssignIssue(ctx context.Context, req AssignIssueReq) error
	ListTransitions(ctx context.Context, issueKey string) (ListResult[Transition], error)
	TransitionIssue(ctx context.Context, req TransitionIssueReq) error

	ListProjects(ctx context.Context, opt ProjectListOpts) (ListResult[Project], error)
	GetProject(ctx context.Context, key string) (*Project, error)

	ListComments(ctx context.Context, opt ListCommentsOpts) (ListResult[Comment], error)
	AddComment(ctx context.Context, req AddCommentReq) (*Comment, error)
	UpdateComment(ctx context.Context, req UpdateCommentReq) (*Comment, error)
	DeleteComment(ctx context.Context, req DeleteCommentReq) error

	// ResolveUser resolves a user selector (Cloud accountId, DC username, or a
	// display-name/email query) to a unique user. It backs assignee flags.
	ResolveUser(ctx context.Context, selector string) (*User, error)

	// DescribeWrite reports the HTTP request a write op would send, for
	// --dry-run previews. It never sends the write itself.
	DescribeWrite(ctx context.Context, op any) (WriteRequestPlan, error)
}

// apiClient is the single Client implementation. Per-flavor behaviour is
// selected by the flavor field and the helpers in dialect.go / adf.go.
type apiClient struct {
	flavor   Flavor
	baseURL  string // site root, no trailing slash
	pageSize int
	http     *transport.Client
}

// Config configures a Client.
type Config struct {
	Flavor    Flavor
	BaseURL   string
	PageSize  int
	Transport *transport.Client
}

// New builds a Client. The transport must already carry the auth decorator.
func New(cfg Config) Client {
	ps := cfg.PageSize
	if ps <= 0 {
		ps = constants.DefaultPageSize
	}
	if ps > constants.MaxPageSize {
		ps = constants.MaxPageSize
	}
	return &apiClient{
		flavor:   cfg.Flavor,
		baseURL:  strings.TrimRight(cfg.BaseURL, "/"),
		pageSize: ps,
		http:     cfg.Transport,
	}
}

func (c *apiClient) Flavor() Flavor  { return c.flavor }
func (c *apiClient) BaseURL() string { return c.baseURL }

// limitOf returns the effective page size for a ListOpts.
func (c *apiClient) limitOf(opt ListOpts) int {
	if opt.Limit > 0 {
		if opt.Limit > constants.MaxPageSize {
			return constants.MaxPageSize
		}
		return opt.Limit
	}
	return c.pageSize
}

// getJSON performs a GET and decodes the JSON body into out.
func (c *apiClient) getJSON(ctx context.Context, path string, query url.Values, out any) error {
	return c.doJSON(ctx, http.MethodGet, path, query, nil, out)
}

// doJSON performs an HTTP request and decodes a JSON response into out.
// Non-2xx responses are converted into structured *errors.CLIError values.
func (c *apiClient) doJSON(ctx context.Context, method, path string, query url.Values, body any, out any) error {
	endpoint := c.baseURL + path
	if len(query) > 0 {
		endpoint += "?" + query.Encode()
	}

	var reqBody io.Reader
	if body != nil {
		raw, err := json.Marshal(body)
		if err != nil {
			return cerrors.Wrap(err, cerrors.CategoryInternal, "ENCODE", "failed to encode request body")
		}
		reqBody = bytes.NewReader(raw)
	}

	req, err := http.NewRequest(method, endpoint, reqBody)
	if err != nil {
		return cerrors.Wrap(err, cerrors.CategoryUsage, "BAD_REQUEST", "failed to build request")
	}
	req.Header.Set("Accept", "application/json")
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}

	resp, err := c.http.Do(ctx, req)
	if err != nil {
		return cerrors.Wrap(err, cerrors.CategoryNetwork, "NETWORK",
			fmt.Sprintf("request to %s failed", endpoint))
	}
	defer resp.Body.Close()

	if resp.StatusCode >= 400 {
		return c.httpError(resp)
	}
	if out == nil {
		_, _ = io.Copy(io.Discard, resp.Body)
		return nil
	}
	rawResp, _ := io.ReadAll(resp.Body)
	return decodeJSON(rawResp, out)
}

// decodeJSON unmarshals a server response body into out. On failure it surfaces
// the underlying parser error and a snippet of the body, so a shape mismatch is
// diagnosable rather than an opaque "failed to decode".
func decodeJSON(body []byte, out any) error {
	if err := json.Unmarshal(body, out); err != nil {
		snippet := strings.TrimSpace(string(body))
		if len(snippet) > 200 {
			snippet = snippet[:200] + "…"
		}
		return cerrors.Wrap(err, cerrors.CategoryParse, "DECODE",
			fmt.Sprintf("could not decode the server response: %v", err)).
			WithHint("The server's JSON did not match what jira-cli expected; "+
				"this is likely a client bug, not a failed request.").
			WithNextSteps(
				"The operation may well have succeeded — verify with a read command.",
				"Report it with this snippet: "+snippet)
	}
	return nil
}

// httpError turns a non-2xx response into a classified CLIError.
func (c *apiClient) httpError(resp *http.Response) error {
	cat := cerrors.FromHTTPStatus(resp.StatusCode)
	snippet, _ := io.ReadAll(io.LimitReader(resp.Body, 1024))
	msg := fmt.Sprintf("Jira returned HTTP %d", resp.StatusCode)
	if detail := extractAPIMessage(snippet); detail != "" {
		msg += ": " + detail
	}
	return cerrors.New(cat, "HTTP_"+http.StatusText(resp.StatusCode), msg).
		WithHTTPStatus(resp.StatusCode)
}

// extractAPIMessage best-effort extracts a human message from a Jira JSON
// error body. Jira reports errors as {"errorMessages": [...], "errors":
// {"field": "message"}} on both flavors.
func extractAPIMessage(raw []byte) string {
	var v struct {
		ErrorMessages []string          `json:"errorMessages"`
		Errors        map[string]string `json:"errors"`
	}
	if json.Unmarshal(raw, &v) != nil {
		return ""
	}
	parts := make([]string, 0, len(v.ErrorMessages)+len(v.Errors))
	parts = append(parts, v.ErrorMessages...)
	// Field errors get their field name so "Specify a summary" style messages
	// stay actionable. Order is not guaranteed by the map, but multi-field
	// failures are rare and every message is included.
	for field, m := range v.Errors {
		parts = append(parts, field+": "+m)
	}
	return strings.Join(parts, "; ")
}
