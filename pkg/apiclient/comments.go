package apiclient

import (
	"context"
	"encoding/json"
	"net/url"
	"strconv"

	cerrors "github.com/angelmsger/jira-cli/pkg/errors"
)

// rawComment is the wire shape of an issue comment. Body is ADF on Cloud and
// a plain string on Data Center; bodyToText normalizes both.
type rawComment struct {
	ID      string          `json:"id"`
	Author  *rawUser        `json:"author"`
	Body    json.RawMessage `json:"body"`
	Created string          `json:"created"`
	Updated string          `json:"updated"`
}

func mapComment(r rawComment, issueKey string) Comment {
	c := Comment{
		ID:       r.ID,
		IssueKey: issueKey,
		Body:     bodyToText(r.Body),
		Created:  r.Created,
		Updated:  r.Updated,
	}
	if r.Author != nil {
		c.Author = mapUser(*r.Author)
	}
	return c
}

// ListComments lists an issue's comments, oldest first.
//
//	Both flavors: GET /issue/{key}/comment?startAt&maxResults
func (c *apiClient) ListComments(ctx context.Context, opt ListCommentsOpts) (ListResult[Comment], error) {
	if opt.IssueKey == "" {
		return ListResult[Comment]{}, cerrors.New(cerrors.CategoryUsage, "ISSUE_NO_KEY",
			"an issue key is required")
	}
	limit := c.limitOf(opt.ListOpts)
	q := offsetQuery(opt.Cursor, limit)
	var raw struct {
		Comments []rawComment `json:"comments"`
		StartAt  int          `json:"startAt"`
		Total    int          `json:"total"`
	}
	path := c.apiBase() + "/issue/" + url.PathEscape(opt.IssueKey) + "/comment"
	if err := c.getJSON(ctx, path, q, &raw); err != nil {
		return ListResult[Comment]{}, err
	}
	res := ListResult[Comment]{
		Next: nextOffsetToken(strconv.Itoa(raw.StartAt), limit, len(raw.Comments), raw.Total),
	}
	for _, r := range raw.Comments {
		res.Items = append(res.Items, mapComment(r, opt.IssueKey))
	}
	return res, nil
}
