package apiclient

import (
	"context"
	"net/url"

	cerrors "github.com/angelmsger/jira-cli/pkg/errors"
)

// comments_write.go holds the comment write operations. Each has a build*
// helper shared with DescribeWrite so --dry-run previews the exact request.

// buildAddComment assembles the POST request for adding a comment.
func (c *apiClient) buildAddComment(req AddCommentReq) (method, path string, payload any, err error) {
	if req.IssueKey == "" {
		return "", "", nil, cerrors.New(cerrors.CategoryUsage, "ISSUE_NO_KEY",
			"an issue key is required to add a comment")
	}
	if req.Body == "" {
		return "", "", nil, cerrors.New(cerrors.CategoryUsage, "COMMENT_NO_BODY",
			"comment body must not be empty")
	}
	path = c.apiBase() + "/issue/" + url.PathEscape(req.IssueKey) + "/comment"
	return "POST", path, map[string]any{"body": c.textToBody(req.Body)}, nil
}

// AddComment adds a comment to an issue.
func (c *apiClient) AddComment(ctx context.Context, req AddCommentReq) (*Comment, error) {
	method, path, payload, err := c.buildAddComment(req)
	if err != nil {
		return nil, err
	}
	var raw rawComment
	if err := c.doJSON(ctx, method, path, nil, payload, &raw); err != nil {
		return nil, err
	}
	out := mapComment(raw, req.IssueKey)
	return &out, nil
}

// buildUpdateComment assembles the PUT request for replacing a comment body.
func (c *apiClient) buildUpdateComment(req UpdateCommentReq) (method, path string, payload any, err error) {
	if req.IssueKey == "" {
		return "", "", nil, cerrors.New(cerrors.CategoryUsage, "ISSUE_NO_KEY",
			"an issue key is required to update a comment")
	}
	if req.ID == "" {
		return "", "", nil, cerrors.New(cerrors.CategoryUsage, "COMMENT_NO_ID",
			"a comment ID is required to update a comment")
	}
	if req.Body == "" {
		return "", "", nil, cerrors.New(cerrors.CategoryUsage, "COMMENT_NO_BODY",
			"comment body must not be empty")
	}
	path = c.apiBase() + "/issue/" + url.PathEscape(req.IssueKey) + "/comment/" + url.PathEscape(req.ID)
	return "PUT", path, map[string]any{"body": c.textToBody(req.Body)}, nil
}

// UpdateComment replaces a comment's body.
func (c *apiClient) UpdateComment(ctx context.Context, req UpdateCommentReq) (*Comment, error) {
	method, path, payload, err := c.buildUpdateComment(req)
	if err != nil {
		return nil, err
	}
	var raw rawComment
	if err := c.doJSON(ctx, method, path, nil, payload, &raw); err != nil {
		return nil, err
	}
	out := mapComment(raw, req.IssueKey)
	return &out, nil
}

// buildDeleteComment assembles the DELETE request for a comment.
func (c *apiClient) buildDeleteComment(req DeleteCommentReq) (method, path string, err error) {
	if req.IssueKey == "" {
		return "", "", cerrors.New(cerrors.CategoryUsage, "ISSUE_NO_KEY",
			"an issue key is required to delete a comment")
	}
	if req.ID == "" {
		return "", "", cerrors.New(cerrors.CategoryUsage, "COMMENT_NO_ID",
			"a comment ID is required to delete a comment")
	}
	path = c.apiBase() + "/issue/" + url.PathEscape(req.IssueKey) + "/comment/" + url.PathEscape(req.ID)
	return "DELETE", path, nil
}

// DeleteComment deletes a comment.
func (c *apiClient) DeleteComment(ctx context.Context, req DeleteCommentReq) error {
	method, path, err := c.buildDeleteComment(req)
	if err != nil {
		return err
	}
	return c.doJSON(ctx, method, path, nil, nil, nil)
}
