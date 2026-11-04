package apiclient

import (
	"context"
	"fmt"

	cerrors "github.com/angelmsger/jira-cli/pkg/errors"
)

// Write targets for the family's write-outcome rule (cerrors.FromWrite): what a
// write changes, for the message, and the bounded read that shows its state.

func issueWriteTarget(baseURL, key string) cerrors.WriteTarget {
	return cerrors.WriteTarget{
		Description: fmt.Sprintf("issue %s (%s/browse/%s)", key, baseURL, key),
		ReadSteps:   []string{"jira-cli issue get " + cerrors.ShellArg(key)},
	}
}

func (c *apiClient) commentWriteTarget(key, id string) cerrors.WriteTarget {
	target := issueWriteTarget(c.baseURL, key)
	target.Description = "comment on " + target.Description
	if id != "" {
		target.Description = "comment " + id + " on " + issueWriteTarget(c.baseURL, key).Description
	}
	target.ReadSteps = []string{"jira-cli comment list " + cerrors.ShellArg(key) + " --limit 25"}
	return target
}

func (c *apiClient) worklogWriteTarget(key, id string) cerrors.WriteTarget {
	target := issueWriteTarget(c.baseURL, key)
	target.Description = "worklog on " + target.Description
	if id != "" {
		target.Description = "worklog " + id + " on " + issueWriteTarget(c.baseURL, key).Description
	}
	target.ReadSteps = []string{"jira-cli worklog list " + cerrors.ShellArg(key) + " --limit 25"}
	return target
}

func createWriteTarget(req CreateIssueReq) cerrors.WriteTarget {
	return cerrors.WriteTarget{
		Description: fmt.Sprintf("new issue %q in project %s", req.Summary, req.ProjectKey),
		ReadSteps: []string{"jira-cli issue search --project " + cerrors.ShellArg(req.ProjectKey) +
			" --order-by 'created DESC' --limit 25"},
	}
}

// doWriteJSON keeps uncertain write failures distinct from safely retryable reads.
func (c *apiClient) doWriteJSON(ctx context.Context, method, path string, body, out any, target cerrors.WriteTarget) error {
	return cerrors.FromWrite(c.doJSON(ctx, method, path, nil, body, out), target)
}

// GetIssueAfterWrite hydrates a confirmed write without turning a failed read
// into permission to replay the mutation. It never returns a partial Issue.
func GetIssueAfterWrite(ctx context.Context, client Client, key string) (*Issue, error) {
	issue, err := client.GetIssue(ctx, GetIssueOpts{Key: key})
	if err == nil && (issue == nil || issue.Key == "") {
		err = cerrors.New(cerrors.CategoryParse, "ISSUE_RESPONSE_MISSING_KEY", "the issue response omitted its key")
	}
	if err != nil {
		return nil, cerrors.WriteSucceeded(err, cerrors.CodeWriteSucceededReadFailed,
			"reading the updated issue failed", issueWriteTarget(client.BaseURL(), key))
	}
	return issue, nil
}
