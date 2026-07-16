// Package urlref parses Jira issue references. A reference may be a bare
// issue key (PROJ-123) or a full Jira URL (Cloud or Data Center, in several
// layouts).
package urlref

import (
	"net/url"
	"regexp"
	"strings"
)

// FlavorHint is a best-effort guess of the backend flavor derived from a URL.
type FlavorHint string

const (
	// FlavorUnknown means the reference carried no flavor signal (e.g. a bare key).
	FlavorUnknown FlavorHint = ""
	// FlavorCloud indicates a Jira Cloud URL.
	FlavorCloud FlavorHint = "cloud"
)

// Ref is a parsed Jira reference.
type Ref struct {
	// IssueKey is the issue key (PROJ-123), empty if not resolvable from the input.
	IssueKey string
	// ProjectKey is the project key when the URL names a project rather than
	// an issue (or derived from the issue key).
	ProjectKey string
	// CommentID is the comment ID, set when a URL carries a
	// focusedCommentId query parameter.
	CommentID string
	// BaseURL is the site root (scheme://host[/context]) when input was a URL.
	BaseURL string
	// Flavor is the best-effort backend flavor guess.
	Flavor FlavorHint
	// IsURL reports whether the input was a URL rather than a bare key.
	IsURL bool
}

var (
	issueKeyRe    = regexp.MustCompile(`^[A-Za-z][A-Za-z0-9_]*-\d+$`)
	browsePathRe  = regexp.MustCompile(`/browse/([A-Za-z][A-Za-z0-9_]*-\d+)`)
	projectPathRe = regexp.MustCompile(`/(?:browse|projects)/([A-Za-z][A-Za-z0-9_]+)(?:[/?#]|$)`)
)

// Parse interprets s as either a bare issue key or a Jira URL.
// It never errors: an unrecognised input yields a Ref with empty fields.
func Parse(s string) Ref {
	s = strings.TrimSpace(s)
	if s == "" {
		return Ref{}
	}
	if issueKeyRe.MatchString(s) {
		key := strings.ToUpper(s)
		return Ref{IssueKey: key, ProjectKey: projectOf(key)}
	}
	if !strings.Contains(s, "://") {
		// Not a URL and not a key shape; pass through verbatim as a key so the
		// server produces the authoritative error.
		return Ref{IssueKey: s}
	}

	u, err := url.Parse(s)
	if err != nil {
		return Ref{}
	}
	ref := Ref{IsURL: true}
	ref.Flavor = flavorOf(u)
	ref.BaseURL = baseURLOf(u)

	// .../browse/PROJ-123 — the canonical issue URL on both flavors.
	if m := browsePathRe.FindStringSubmatch(u.Path); m != nil {
		ref.IssueKey = strings.ToUpper(m[1])
	}
	// Board / backlog URLs carry the issue in ?selectedIssue=PROJ-123.
	if ref.IssueKey == "" {
		if k := u.Query().Get("selectedIssue"); issueKeyRe.MatchString(k) {
			ref.IssueKey = strings.ToUpper(k)
		}
	}
	// .../browse/PROJ or .../projects/PROJ — a project reference.
	if ref.IssueKey == "" {
		if m := projectPathRe.FindStringSubmatch(u.Path); m != nil {
			ref.ProjectKey = strings.ToUpper(m[1])
		}
	}
	if ref.IssueKey != "" {
		ref.ProjectKey = projectOf(ref.IssueKey)
	}
	if id := u.Query().Get("focusedCommentId"); id != "" {
		ref.CommentID = id
	}
	return ref
}

// projectOf derives the project key from an issue key.
func projectOf(issueKey string) string {
	if i := strings.LastIndex(issueKey, "-"); i > 0 {
		return issueKey[:i]
	}
	return ""
}

// flavorOf guesses the flavor from the host.
func flavorOf(u *url.URL) FlavorHint {
	host := strings.ToLower(u.Hostname())
	if strings.HasSuffix(host, ".atlassian.net") || strings.HasSuffix(host, ".jira.com") {
		return FlavorCloud
	}
	return FlavorUnknown
}

// baseURLOf returns the site root. Data Center may serve Jira under a context
// path (e.g. /jira); everything before /browse|/projects|/secure is kept.
func baseURLOf(u *url.URL) string {
	root := u.Scheme + "://" + u.Host
	path := u.Path
	for _, marker := range []string{"/jira/software/", "/browse/", "/projects/", "/secure/"} {
		if i := strings.Index(path, marker); i >= 0 {
			path = path[:i]
			break
		}
	}
	return root + strings.TrimRight(path, "/")
}
