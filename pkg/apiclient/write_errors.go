package apiclient

import (
	"context"
	"fmt"
	"strings"

	cerrors "github.com/angelmsger/jira-cli/pkg/errors"
)

type writeTarget struct {
	description string
	readSteps   []string
}

func issueWriteTarget(baseURL, key string) writeTarget {
	return writeTarget{
		description: fmt.Sprintf("issue %s (%s/browse/%s)", key, baseURL, key),
		readSteps:   []string{"jira-cli issue get " + shellArgument(key)},
	}
}

func (c *apiClient) commentWriteTarget(key, id string) writeTarget {
	target := issueWriteTarget(c.baseURL, key)
	target.description = "comment on " + target.description
	if id != "" {
		target.description = "comment " + id + " on " + issueWriteTarget(c.baseURL, key).description
	}
	target.readSteps = []string{"jira-cli comment list " + shellArgument(key) + " --limit 25"}
	return target
}

func createWriteTarget(req CreateIssueReq) writeTarget {
	return writeTarget{
		description: fmt.Sprintf("new issue %q in project %s", req.Summary, req.ProjectKey),
		readSteps: []string{"jira-cli issue search --project " + shellArgument(req.ProjectKey) +
			" --order-by 'created DESC' --limit 25"},
	}
}

func shellArgument(value string) string {
	return "'" + strings.ReplaceAll(value, "'", "'\"'\"'") + "'"
}

func writeResultError(err error, code, message string, target writeTarget) *cerrors.CLIError {
	cause := cerrors.AsCLIError(err)
	out := cerrors.Wrap(err, cause.Category, code, message+": "+cause.Message).
		WithHTTPStatus(cause.HTTPStatus).
		WithHint("Do not repeat the write. Inspect the remote state using the read command; follow pagination if needed. An empty page is not proof that the write failed.").
		WithNextSteps(target.readSteps...)
	out.Retryable = false
	return out
}

// doWriteJSON keeps uncertain write failures distinct from safely retryable reads.
func (c *apiClient) doWriteJSON(ctx context.Context, method, path string, body, out any, target writeTarget) error {
	err := c.doJSON(ctx, method, path, nil, body, out)
	if err == nil {
		return nil
	}
	ce := cerrors.AsCLIError(err)
	switch ce.Category {
	case cerrors.CategoryParse:
		if ce.HTTPStatus >= 200 && ce.HTTPStatus < 300 {
			return writeResultError(err, "WRITE_SUCCEEDED_RESPONSE_INVALID",
				"Write succeeded for "+target.description+", but its response could not be decoded", target)
		}
		return writeResultError(err, ce.Code, "Write outcome is unknown for "+target.description, target)
	case cerrors.CategoryNetwork, cerrors.CategoryServer, cerrors.CategoryRateLimit:
		return writeResultError(err, ce.Code,
			"Write outcome is unknown for "+target.description, target)
	default:
		return err
	}
}

func missingWriteIdentity(target writeTarget) error {
	err := cerrors.New(cerrors.CategoryParse, "WRITE_RESPONSE_MISSING_ID", "the successful write response omitted its resource identifier")
	return writeResultError(err, "WRITE_SUCCEEDED_RESPONSE_INVALID",
		"Write succeeded for "+target.description+", but its response was incomplete", target)
}

// GetIssueAfterWrite hydrates a confirmed write without turning a failed read
// into permission to replay the mutation. It never returns a partial Issue.
func GetIssueAfterWrite(ctx context.Context, client Client, key string) (*Issue, error) {
	issue, err := client.GetIssue(ctx, GetIssueOpts{Key: key})
	if err == nil && (issue == nil || issue.Key == "") {
		err = cerrors.New(cerrors.CategoryParse, "ISSUE_RESPONSE_MISSING_KEY", "the issue response omitted its key")
	}
	if err != nil {
		target := issueWriteTarget(client.BaseURL(), key)
		return nil, writeResultError(err, "WRITE_SUCCEEDED_READ_FAILED",
			"Write succeeded for "+target.description+", but reading the updated issue failed", target)
	}
	return issue, nil
}
