package apiclient

import (
	"context"
	"net/url"
	"regexp"
	"strings"

	cerrors "github.com/angelmsger/jira-cli/pkg/errors"
)

// rawUser is the wire shape of a user. Data Center populates name/key, Cloud
// populates accountId; displayName is set by both.
type rawUser struct {
	AccountID    string `json:"accountId"`
	Name         string `json:"name"`
	Key          string `json:"key"`
	DisplayName  string `json:"displayName"`
	EmailAddress string `json:"emailAddress"`
	Active       *bool  `json:"active"`
}

func mapUser(r rawUser) *User {
	u := &User{
		AccountID:   r.AccountID,
		Username:    r.Name,
		DisplayName: r.DisplayName,
		Email:       r.EmailAddress,
		Active:      r.Active,
	}
	if u.Username == "" {
		u.Username = r.Key
	}
	return u
}

// CurrentUser returns the user the configured credentials authenticate as.
//
//	Both flavors: GET /myself
func (c *apiClient) CurrentUser(ctx context.Context) (*User, error) {
	var raw rawUser
	if err := c.getJSON(ctx, c.apiBase()+"/myself", nil, &raw); err != nil {
		return nil, err
	}
	return mapUser(raw), nil
}

// cloudAccountIDPattern matches Cloud account IDs: either the modern
// "712020:uuid" form or a bare 24+ character hex/opaque ID. Values that match
// are passed through without a lookup.
var cloudAccountIDPattern = regexp.MustCompile(`^[0-9a-zA-Z]+:[0-9a-fA-F-]+$|^[0-9a-f]{24,}$`)

// ResolveUser resolves a user selector to a unique user. On Cloud, a value
// that does not look like an accountId is searched by display name / email
// and must match exactly one user. On Data Center the selector is treated as
// the username and echoed back without a lookup (assignment APIs take the
// name directly).
func (c *apiClient) ResolveUser(ctx context.Context, selector string) (*User, error) {
	if selector == "" {
		return nil, cerrors.New(cerrors.CategoryUsage, "USER_NO_SELECTOR",
			"a user selector is required (accountId on Cloud, username on Data Center, or a display-name/email query)")
	}
	if c.flavor != FlavorCloud {
		return &User{Username: selector}, nil
	}
	if cloudAccountIDPattern.MatchString(selector) {
		return &User{AccountID: selector}, nil
	}

	// GET /rest/api/3/user/search?query= matches display name and email.
	q := url.Values{}
	q.Set("query", selector)
	q.Set("maxResults", "10")
	var raw []rawUser
	if err := c.getJSON(ctx, c.apiBase()+"/user/search", q, &raw); err != nil {
		return nil, err
	}
	var matches []*User
	for _, r := range raw {
		if r.Active != nil && !*r.Active {
			continue
		}
		matches = append(matches, mapUser(r))
	}
	switch len(matches) {
	case 1:
		return matches[0], nil
	case 0:
		return nil, cerrors.Newf(cerrors.CategoryNotFound, "USER_NOT_FOUND",
			"no active user matches %q", selector).
			WithHint("On Jira Cloud users are matched by display name or email; " +
				"the search needs the Browse Users permission.")
	default:
		names := make([]string, 0, len(matches))
		for _, m := range matches {
			label := m.DisplayName
			if m.Email != "" {
				label += " <" + m.Email + ">"
			}
			names = append(names, label+" ("+m.AccountID+")")
		}
		return nil, cerrors.Newf(cerrors.CategoryUsage, "USER_AMBIGUOUS",
			"%q matches %d users", selector, len(matches)).
			WithHint("Pass the accountId of the intended user.").
			WithNextSteps(names...)
	}
}

// resolveAssignee resolves an assignee selector into the flavor's assignment
// identifier field. It returns nil when the selector is empty.
func (c *apiClient) resolveAssignee(ctx context.Context, selector string) (map[string]any, error) {
	if selector == "" {
		return nil, nil
	}
	u, err := c.ResolveUser(ctx, selector)
	if err != nil {
		return nil, err
	}
	if c.flavor == FlavorCloud {
		return map[string]any{"accountId": u.AccountID}, nil
	}
	return map[string]any{"name": u.Username}, nil
}

func escapeQuotes(s string) string {
	return strings.ReplaceAll(s, `"`, `\"`)
}
