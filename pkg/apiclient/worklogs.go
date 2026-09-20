package apiclient

import (
	"context"
	"encoding/json"
	"net/url"
	"strconv"

	cerrors "github.com/angelmsger/jira-cli/pkg/errors"
)

// rawWorklog is the wire shape of a worklog entry. Comment is ADF on Cloud and
// a plain string on Data Center; bodyToText normalizes both, exactly as it does
// for a comment body.
type rawWorklog struct {
	ID               string          `json:"id"`
	Author           *rawUser        `json:"author"`
	UpdateAuthor     *rawUser        `json:"updateAuthor"`
	Comment          json.RawMessage `json:"comment"`
	Started          string          `json:"started"`
	TimeSpent        string          `json:"timeSpent"`
	TimeSpentSeconds int64           `json:"timeSpentSeconds"`
	Created          string          `json:"created"`
	Updated          string          `json:"updated"`
}

func mapWorklog(r rawWorklog, issueKey string) Worklog {
	w := Worklog{
		ID:               r.ID,
		IssueKey:         issueKey,
		Comment:          bodyToText(r.Comment),
		Started:          r.Started,
		TimeSpent:        r.TimeSpent,
		TimeSpentSeconds: r.TimeSpentSeconds,
		Created:          r.Created,
		Updated:          r.Updated,
	}
	if r.Author != nil {
		w.Author = mapUser(*r.Author)
	}
	if r.UpdateAuthor != nil {
		w.UpdateAuthor = mapUser(*r.UpdateAuthor)
	}
	return w
}

// ListWorklogs lists an issue's worklogs, oldest first.
//
//	Both flavors: GET /issue/{key}/worklog?startAt&maxResults
func (c *apiClient) ListWorklogs(ctx context.Context, opt ListWorklogsOpts) (ListResult[Worklog], error) {
	if opt.IssueKey == "" {
		return ListResult[Worklog]{}, cerrors.New(cerrors.CategoryUsage, "ISSUE_NO_KEY",
			"an issue key is required")
	}
	limit := c.limitOf(opt.ListOpts)
	q := offsetQuery(opt.Cursor, limit)
	var raw struct {
		Worklogs []rawWorklog `json:"worklogs"`
		StartAt  int          `json:"startAt"`
		Total    int          `json:"total"`
	}
	path := c.apiBase() + "/issue/" + url.PathEscape(opt.IssueKey) + "/worklog"
	if err := c.getJSON(ctx, path, q, &raw); err != nil {
		return ListResult[Worklog]{}, err
	}
	res := ListResult[Worklog]{
		Next: nextOffsetToken(strconv.Itoa(raw.StartAt), limit, len(raw.Worklogs), raw.Total),
	}
	for _, r := range raw.Worklogs {
		res.Items = append(res.Items, mapWorklog(r, opt.IssueKey))
	}
	return res, nil
}
