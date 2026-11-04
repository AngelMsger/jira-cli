package apiclient

import (
	"context"

	cerrors "github.com/angelmsger/jira-cli/pkg/errors"
)

// NewReadOnly wraps inner so that every mutating method returns a
// READONLY_BLOCKED error before any HTTP request is sent. Reads (the Reader
// half of Client, including DescribeWrite, the --dry-run preview path) pass
// straight through, so safe inspection still works.
func NewReadOnly(inner Client) Client { return &readOnlyClient{Reader: inner} }

// readOnlyClient is the read-only enforcement layer. It keeps inner only as a
// Reader, so reads are delegated unchanged and no write can reach inner. Every
// Writer method is overridden below to return READONLY_BLOCKED; the assertion
// that follows stops compilation when a new Writer method lacks its override.
type readOnlyClient struct{ Reader }

var _ Client = (*readOnlyClient)(nil)

// blocked returns the family's READONLY_BLOCKED error for a refused write. op
// names the operation (e.g. "CreateIssue") so the message says which call was
// refused. Turning read-only mode off is the user's decision, so the error
// names the user's switches without offering to change them.
func blocked(op string) *cerrors.CLIError {
	return cerrors.ReadOnlyBlocked(op, cerrors.ReadOnlyMode{
		Env:        "JIRA_CLI_READ_ONLY",
		ConfigFile: "~/.angelmsger/jira/config.yaml",
		DryRun:     true,
	})
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

// Worklog writes.
func (r *readOnlyClient) AddWorklog(_ context.Context, _ AddWorklogReq) (*Worklog, error) {
	return nil, blocked("AddWorklog")
}
func (r *readOnlyClient) UpdateWorklog(_ context.Context, _ UpdateWorklogReq) (*Worklog, error) {
	return nil, blocked("UpdateWorklog")
}
func (r *readOnlyClient) DeleteWorklog(_ context.Context, _ DeleteWorklogReq) error {
	return blocked("DeleteWorklog")
}
