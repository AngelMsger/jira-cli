package apiclient

import (
	"context"

	cerrors "github.com/angelmsger/jira-cli/pkg/errors"
)

// NewReadOnly wraps inner so that every mutating method returns a
// READONLY_BLOCKED error before any HTTP request is sent. Reads (every other
// method on Client) and DescribeWrite (the --dry-run preview path) pass
// straight through, so safe inspection still works.
func NewReadOnly(inner Client) Client { return &readOnlyClient{Client: inner} }

// readOnlyClient is the read-only enforcement layer. It embeds Client so every
// non-mutating method is inherited unchanged; mutating methods are overridden
// to return READONLY_BLOCKED.
type readOnlyClient struct{ Client }

// blocked returns the structured error for a blocked write. op names the
// operation (e.g. "CreateIssue") so the error message is precise about which
// call was refused.
func blocked(op string) *cerrors.CLIError {
	return cerrors.Newf(cerrors.CategoryPermission, "READONLY_BLOCKED",
		"operation %q blocked: read-only mode is enabled", op).
		WithHint("Re-run with --allow-writes to permit writes for this invocation, "+
			"or unset JIRA_CLI_READ_ONLY / defaults.read_only.").
		WithNextSteps(
			"Add --allow-writes to the command line",
			"unset JIRA_CLI_READ_ONLY",
			"Set defaults.read_only=false in ~/.angelmsger/jira/config.yaml",
		)
}

// Issue writes.
func (r *readOnlyClient) CreateIssue(_ context.Context, _ CreateIssueReq) (*Issue, error) {
	return nil, blocked("CreateIssue")
}
func (r *readOnlyClient) EditIssue(_ context.Context, _ EditIssueReq) (*Issue, error) {
	return nil, blocked("EditIssue")
}
func (r *readOnlyClient) AssignIssue(_ context.Context, _ AssignIssueReq) error {
	return blocked("AssignIssue")
}
func (r *readOnlyClient) TransitionIssue(_ context.Context, _ TransitionIssueReq) error {
	return blocked("TransitionIssue")
}

// Comment writes.
func (r *readOnlyClient) AddComment(_ context.Context, _ AddCommentReq) (*Comment, error) {
	return nil, blocked("AddComment")
}
func (r *readOnlyClient) UpdateComment(_ context.Context, _ UpdateCommentReq) (*Comment, error) {
	return nil, blocked("UpdateComment")
}
func (r *readOnlyClient) DeleteComment(_ context.Context, _ DeleteCommentReq) error {
	return blocked("DeleteComment")
}
