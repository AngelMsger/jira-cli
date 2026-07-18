package apiclient

import (
	"context"
	"strconv"

	cerrors "github.com/angelmsger/jira-cli/pkg/errors"
)

// defaultSearchFields is the field set requested per issue when the caller
// does not widen it. Cloud's /search/jql endpoint returns only issue IDs
// unless fields are named explicitly, so the client always sends this list.
var defaultSearchFields = []string{
	"summary", "status", "assignee", "reporter", "issuetype",
	"priority", "labels", "components", "fixVersions",
	"project", "parent", "created", "updated",
}

// SearchIssues runs a JQL search. The endpoints differ per flavor:
//
//	Cloud: POST /rest/api/3/search/jql  (token pagination: nextPageToken/isLast;
//	       the legacy startAt-based /search was removed from Cloud in 2025)
//	DC:    GET  /rest/api/2/search      (startAt/maxResults/total)
//
// Both collapse into the opaque ListResult.Next cursor.
func (c *apiClient) SearchIssues(ctx context.Context, opt SearchOpts) (ListResult[Issue], error) {
	if opt.JQL == "" {
		return ListResult[Issue]{}, cerrors.New(cerrors.CategoryUsage, "JQL_EMPTY",
			"a JQL query is required")
	}
	fields := opt.Fields
	if len(fields) == 0 {
		fields = defaultSearchFields
	}
	limit := c.limitOf(opt.ListOpts)

	if c.flavor == FlavorCloud {
		body := map[string]any{
			"jql":        opt.JQL,
			"maxResults": limit,
			"fields":     fields,
		}
		if opt.Cursor != "" {
			body["nextPageToken"] = opt.Cursor
		}
		var raw struct {
			Issues        []rawIssue `json:"issues"`
			NextPageToken string     `json:"nextPageToken"`
			IsLast        bool       `json:"isLast"`
		}
		if err := c.doJSON(ctx, "POST", c.apiBase()+"/search/jql", nil, body, &raw); err != nil {
			return ListResult[Issue]{}, err
		}
		res := ListResult[Issue]{}
		if !raw.IsLast && raw.NextPageToken != "" {
			res.Next = raw.NextPageToken
		}
		for _, r := range raw.Issues {
			res.Items = append(res.Items, mapIssue(r, c.baseURL))
		}
		return res, nil
	}

	q := offsetQuery(opt.Cursor, limit)
	q.Set("jql", opt.JQL)
	q.Set("fields", joinFields(fields))
	var raw struct {
		Issues  []rawIssue `json:"issues"`
		StartAt int        `json:"startAt"`
		Total   int        `json:"total"`
	}
	if err := c.getJSON(ctx, c.apiBase()+"/search", q, &raw); err != nil {
		return ListResult[Issue]{}, err
	}
	res := ListResult[Issue]{
		Next: nextOffsetToken(strconv.Itoa(raw.StartAt), limit, len(raw.Issues), raw.Total),
	}
	for _, r := range raw.Issues {
		res.Items = append(res.Items, mapIssue(r, c.baseURL))
	}
	return res, nil
}

// joinFields renders a fields list as the comma-separated query form the DC
// search endpoint expects.
func joinFields(fields []string) string {
	out := ""
	for i, f := range fields {
		if i > 0 {
			out += ","
		}
		out += f
	}
	return out
}
