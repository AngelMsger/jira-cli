package apiclient

import (
	"context"
	"encoding/json"
	"net/url"

	cerrors "github.com/angelmsger/jira-cli/pkg/errors"
)

// rawIssue is the wire shape of an issue, shared by the get and search paths.
// Rich-text fields are kept raw and normalized via bodyToText (ADF on Cloud,
// plain string on Data Center).
type rawIssue struct {
	ID     string `json:"id"`
	Key    string `json:"key"`
	Fields struct {
		Summary     string          `json:"summary"`
		Description json.RawMessage `json:"description"`
		Status      *struct {
			ID             string `json:"id"`
			Name           string `json:"name"`
			StatusCategory struct {
				Key string `json:"key"`
			} `json:"statusCategory"`
		} `json:"status"`
		IssueType *struct {
			Name string `json:"name"`
		} `json:"issuetype"`
		Priority *struct {
			Name string `json:"name"`
		} `json:"priority"`
		Assignee *rawUser `json:"assignee"`
		Reporter *rawUser `json:"reporter"`
		Labels   []string `json:"labels"`
		Project  *struct {
			Key string `json:"key"`
		} `json:"project"`
		Parent *struct {
			Key string `json:"key"`
		} `json:"parent"`
		Created string `json:"created"`
		Updated string `json:"updated"`
	} `json:"fields"`
}

// mapIssue normalizes a rawIssue. baseURL builds the human /browse URL.
func mapIssue(r rawIssue, baseURL string) Issue {
	issue := Issue{
		ID:          r.ID,
		Key:         r.Key,
		Summary:     r.Fields.Summary,
		Description: bodyToText(r.Fields.Description),
		Labels:      r.Fields.Labels,
		Created:     r.Fields.Created,
		Updated:     r.Fields.Updated,
	}
	if r.Key != "" && baseURL != "" {
		issue.URL = baseURL + "/browse/" + r.Key
	}
	if r.Fields.Status != nil {
		issue.Status = &Status{
			ID:       r.Fields.Status.ID,
			Name:     r.Fields.Status.Name,
			Category: r.Fields.Status.StatusCategory.Key,
		}
	}
	if r.Fields.IssueType != nil {
		issue.Type = r.Fields.IssueType.Name
	}
	if r.Fields.Priority != nil {
		issue.Priority = r.Fields.Priority.Name
	}
	if r.Fields.Assignee != nil {
		issue.Assignee = mapUser(*r.Fields.Assignee)
	}
	if r.Fields.Reporter != nil {
		issue.Reporter = mapUser(*r.Fields.Reporter)
	}
	if r.Fields.Project != nil {
		issue.ProjectKey = r.Fields.Project.Key
	}
	if r.Fields.Parent != nil {
		issue.ParentKey = r.Fields.Parent.Key
	}
	return issue
}

// GetIssue fetches a single issue by key.
//
//	Cloud: GET /rest/api/3/issue/{key}
//	DC:    GET /rest/api/2/issue/{key}
func (c *apiClient) GetIssue(ctx context.Context, opt GetIssueOpts) (*Issue, error) {
	if opt.Key == "" {
		return nil, cerrors.New(cerrors.CategoryUsage, "ISSUE_NO_KEY", "an issue key is required")
	}
	q := url.Values{}
	if opt.Expand != "" {
		q.Set("expand", opt.Expand)
	}
	var raw rawIssue
	if err := c.getJSON(ctx, c.apiBase()+"/issue/"+url.PathEscape(opt.Key), q, &raw); err != nil {
		return nil, err
	}
	issue := mapIssue(raw, c.baseURL)
	return &issue, nil
}
