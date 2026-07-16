package apiclient

import (
	"context"
	"net/url"

	cerrors "github.com/angelmsger/jira-cli/pkg/errors"
)

// issues_write.go holds the issue write operations. Each write has a build*
// helper shared between the real call and DescribeWrite (the --dry-run path),
// so the previewed request can never diverge from the sent one. Read-only
// lookups needed to compute a payload (assignee resolution on Cloud) still
// run under --dry-run.

// buildCreateIssue assembles the POST request for creating an issue.
func (c *apiClient) buildCreateIssue(ctx context.Context, req CreateIssueReq) (method, path string, payload any, err error) {
	if req.ProjectKey == "" {
		return "", "", nil, cerrors.New(cerrors.CategoryUsage, "ISSUE_NO_PROJECT",
			"a project key is required to create an issue").
			WithNextSteps("jira-cli project list")
	}
	if req.Type == "" {
		return "", "", nil, cerrors.New(cerrors.CategoryUsage, "ISSUE_NO_TYPE",
			"an issue type is required (e.g. --type Task)")
	}
	if req.Summary == "" {
		return "", "", nil, cerrors.New(cerrors.CategoryUsage, "ISSUE_NO_SUMMARY",
			"a summary is required to create an issue")
	}
	fields := map[string]any{
		"project":   map[string]any{"key": req.ProjectKey},
		"issuetype": map[string]any{"name": req.Type},
		"summary":   req.Summary,
	}
	if req.Description != "" {
		fields["description"] = c.textToBody(req.Description)
	}
	if req.Priority != "" {
		fields["priority"] = map[string]any{"name": req.Priority}
	}
	if len(req.Labels) > 0 {
		fields["labels"] = req.Labels
	}
	if req.ParentKey != "" {
		fields["parent"] = map[string]any{"key": req.ParentKey}
	}
	if req.Assignee != "" {
		assignee, err := c.resolveAssignee(ctx, req.Assignee)
		if err != nil {
			return "", "", nil, err
		}
		fields["assignee"] = assignee
	}
	return "POST", c.apiBase() + "/issue", map[string]any{"fields": fields}, nil
}

// CreateIssue creates an issue and returns it fully populated (the create
// response is only an id/key skeleton, so the issue is re-read).
func (c *apiClient) CreateIssue(ctx context.Context, req CreateIssueReq) (*Issue, error) {
	method, path, payload, err := c.buildCreateIssue(ctx, req)
	if err != nil {
		return nil, err
	}
	var created struct {
		Key string `json:"key"`
	}
	if err := c.doJSON(ctx, method, path, nil, payload, &created); err != nil {
		return nil, err
	}
	return c.GetIssue(ctx, GetIssueOpts{Key: created.Key})
}

// buildEditIssue assembles the PUT request for updating issue fields.
func (c *apiClient) buildEditIssue(req EditIssueReq) (method, path string, payload any, err error) {
	if req.Key == "" {
		return "", "", nil, cerrors.New(cerrors.CategoryUsage, "ISSUE_NO_KEY",
			"an issue key is required to edit an issue")
	}
	fields := map[string]any{}
	if req.Summary != nil {
		fields["summary"] = *req.Summary
	}
	if req.Description != nil {
		fields["description"] = c.textToBody(*req.Description)
	}
	if req.Priority != "" {
		fields["priority"] = map[string]any{"name": req.Priority}
	}
	var labelOps []map[string]any
	for _, l := range req.AddLabels {
		labelOps = append(labelOps, map[string]any{"add": l})
	}
	for _, l := range req.RemoveLabels {
		labelOps = append(labelOps, map[string]any{"remove": l})
	}
	if len(fields) == 0 && len(labelOps) == 0 {
		return "", "", nil, cerrors.New(cerrors.CategoryUsage, "ISSUE_NO_CHANGES",
			"nothing to change").
			WithNextSteps("Pass at least one of --summary, --description, --priority, --add-label, --remove-label.")
	}
	body := map[string]any{}
	if len(fields) > 0 {
		body["fields"] = fields
	}
	if len(labelOps) > 0 {
		body["update"] = map[string]any{"labels": labelOps}
	}
	return "PUT", c.apiBase() + "/issue/" + url.PathEscape(req.Key), body, nil
}

// EditIssue updates issue fields (the PUT returns 204, so the issue is
// re-read and returned).
func (c *apiClient) EditIssue(ctx context.Context, req EditIssueReq) (*Issue, error) {
	method, path, payload, err := c.buildEditIssue(req)
	if err != nil {
		return nil, err
	}
	if err := c.doJSON(ctx, method, path, nil, payload, nil); err != nil {
		return nil, err
	}
	return c.GetIssue(ctx, GetIssueOpts{Key: req.Key})
}

// buildAssignIssue assembles the PUT request for changing an issue's
// assignee. Unassigning sends a null identifier, which both flavors accept.
func (c *apiClient) buildAssignIssue(ctx context.Context, req AssignIssueReq) (method, path string, payload any, err error) {
	if req.Key == "" {
		return "", "", nil, cerrors.New(cerrors.CategoryUsage, "ISSUE_NO_KEY",
			"an issue key is required to assign an issue")
	}
	path = c.apiBase() + "/issue/" + url.PathEscape(req.Key) + "/assignee"
	if req.Unassign {
		field := "name"
		if c.flavor == FlavorCloud {
			field = "accountId"
		}
		return "PUT", path, map[string]any{field: nil}, nil
	}
	if req.Assignee == "" {
		return "", "", nil, cerrors.New(cerrors.CategoryUsage, "ISSUE_NO_ASSIGNEE",
			"an assignee is required (or pass --unassign)")
	}
	assignee, err := c.resolveAssignee(ctx, req.Assignee)
	if err != nil {
		return "", "", nil, err
	}
	return "PUT", path, assignee, nil
}

// AssignIssue changes or clears an issue's assignee.
func (c *apiClient) AssignIssue(ctx context.Context, req AssignIssueReq) error {
	method, path, payload, err := c.buildAssignIssue(ctx, req)
	if err != nil {
		return err
	}
	return c.doJSON(ctx, method, path, nil, payload, nil)
}

// buildTransitionIssue assembles the POST request for a workflow transition.
// TransitionID must already be resolved (the command layer maps names to IDs
// via ListTransitions).
func (c *apiClient) buildTransitionIssue(req TransitionIssueReq) (method, path string, payload any, err error) {
	if req.Key == "" {
		return "", "", nil, cerrors.New(cerrors.CategoryUsage, "ISSUE_NO_KEY",
			"an issue key is required to transition an issue")
	}
	if req.TransitionID == "" {
		return "", "", nil, cerrors.New(cerrors.CategoryUsage, "TRANSITION_NO_ID",
			"a transition is required").
			WithNextSteps("jira-cli issue transitions " + req.Key)
	}
	body := map[string]any{
		"transition": map[string]any{"id": req.TransitionID},
	}
	if req.Comment != "" {
		body["update"] = map[string]any{
			"comment": []map[string]any{
				{"add": map[string]any{"body": c.textToBody(req.Comment)}},
			},
		}
	}
	return "POST", c.apiBase() + "/issue/" + url.PathEscape(req.Key) + "/transitions", body, nil
}

// TransitionIssue moves an issue through a workflow transition.
func (c *apiClient) TransitionIssue(ctx context.Context, req TransitionIssueReq) error {
	method, path, payload, err := c.buildTransitionIssue(req)
	if err != nil {
		return err
	}
	return c.doJSON(ctx, method, path, nil, payload, nil)
}

// DescribeWrite returns the HTTP request a write operation would send, without
// sending it. The op must be one of the *Req types. Read-only GETs needed to
// compute a payload (user resolution) are still performed.
func (c *apiClient) DescribeWrite(ctx context.Context, op any) (WriteRequestPlan, error) {
	var (
		method, path string
		payload      any
		err          error
	)
	switch v := op.(type) {
	case CreateIssueReq:
		method, path, payload, err = c.buildCreateIssue(ctx, v)
	case EditIssueReq:
		method, path, payload, err = c.buildEditIssue(v)
	case AssignIssueReq:
		method, path, payload, err = c.buildAssignIssue(ctx, v)
	case TransitionIssueReq:
		method, path, payload, err = c.buildTransitionIssue(v)
	case AddCommentReq:
		method, path, payload, err = c.buildAddComment(v)
	case UpdateCommentReq:
		method, path, payload, err = c.buildUpdateComment(v)
	case DeleteCommentReq:
		method, path, err = c.buildDeleteComment(v)
	default:
		return WriteRequestPlan{}, cerrors.Newf(cerrors.CategoryInternal, "UNKNOWN_WRITE_OP",
			"DescribeWrite: unsupported operation type %T", op)
	}
	if err != nil {
		return WriteRequestPlan{}, err
	}
	return WriteRequestPlan{Method: method, URL: c.baseURL + path, Payload: payload}, nil
}
