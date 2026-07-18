// Package apiclient is a flavor-agnostic Jira REST client. It supports
// Jira Cloud (REST v3) and Data Center / Server (REST v2) behind a single
// Client interface returning normalized models.
//
// This package backs the jira-cli command layer and is also importable as a
// standalone client library (e.g. by a GUI); see the repository README. Its
// exported surface — the Client interface, the normalized models, and the
// read-only / dry-run semantics — is a contract the CLI and its companion
// Skill depend on. Extend it additively and keep existing shapes and behavior
// stable; do not reshape the public API to suit a single local call site.
package apiclient

// Flavor identifies the Jira backend variant.
type Flavor string

const (
	FlavorCloud      Flavor = "cloud"
	FlavorDataCenter Flavor = "datacenter"
	FlavorAuto       Flavor = "auto"
)

// ServerInfo is the result of a connectivity probe.
type ServerInfo struct {
	Flavor    Flavor `json:"flavor"`
	BaseURL   string `json:"base_url"`
	Reachable bool   `json:"reachable"`
	// Version is the Jira version string reported by /serverInfo.
	Version string `json:"version,omitempty"`
	// DeploymentType is "Cloud" or "Server" as reported by /serverInfo.
	DeploymentType string `json:"deployment_type,omitempty"`
}

// User is a normalized Jira user. Data Center identifies users by Username,
// Cloud by AccountID; whichever the server returns is kept.
type User struct {
	AccountID   string `json:"account_id,omitempty"`
	Username    string `json:"username,omitempty"`
	DisplayName string `json:"display_name,omitempty"`
	Email       string `json:"email,omitempty"`
	Active      *bool  `json:"active,omitempty"`
}

// Identifier returns the flavor-appropriate stable identifier for the user:
// the Cloud accountId when present, otherwise the DC username.
func (u User) Identifier() string {
	if u.AccountID != "" {
		return u.AccountID
	}
	return u.Username
}

// Project is a normalized Jira project.
type Project struct {
	ID   string `json:"id"`
	Key  string `json:"key"`
	Name string `json:"name"`
	Type string `json:"type,omitempty"` // projectTypeKey: software, business, service_desk
	Lead string `json:"lead,omitempty"`
	URL  string `json:"url,omitempty"`
}

// Status is an issue's workflow status.
type Status struct {
	ID   string `json:"id,omitempty"`
	Name string `json:"name"`
	// Category is the status category key: new, indeterminate, done.
	Category string `json:"category,omitempty"`
}

// Issue is a normalized Jira issue. Description carries plain text: on Cloud
// the ADF document is flattened (see adf.go), on Data Center the raw string is
// passed through.
type Issue struct {
	ID          string   `json:"id"`
	Key         string   `json:"key"`
	ProjectKey  string   `json:"project_key,omitempty"`
	Type        string   `json:"type,omitempty"`
	Summary     string   `json:"summary"`
	Description string   `json:"description,omitempty"`
	Status      *Status  `json:"status,omitempty"`
	Priority    string   `json:"priority,omitempty"`
	Assignee    *User    `json:"assignee,omitempty"`
	Reporter    *User    `json:"reporter,omitempty"`
	Labels      []string `json:"labels,omitempty"`
	// Components, FixVersions and AffectsVersions carry the item names; use
	// `project components` / `project versions` to discover the valid values.
	Components      []string `json:"components,omitempty"`
	FixVersions     []string `json:"fix_versions,omitempty"`
	AffectsVersions []string `json:"affects_versions,omitempty"`
	ParentKey       string   `json:"parent_key,omitempty"`
	Created         string   `json:"created,omitempty"`
	Updated         string   `json:"updated,omitempty"`
	URL             string   `json:"url,omitempty"`
}

// Comment is a normalized issue comment. Body carries plain text (ADF is
// flattened on Cloud, the raw string is passed through on Data Center).
type Comment struct {
	ID       string `json:"id"`
	IssueKey string `json:"issue_key,omitempty"`
	Author   *User  `json:"author,omitempty"`
	Body     string `json:"body"`
	Created  string `json:"created,omitempty"`
	Updated  string `json:"updated,omitempty"`
}

// Transition is one workflow transition available on an issue.
type Transition struct {
	ID string `json:"id"`
	// Name is the transition's action name (e.g. "Start Progress").
	Name string `json:"name"`
	// To is the status the transition leads to.
	To *Status `json:"to,omitempty"`
}

// Component is a project component — a valid value for an issue's
// "components" field within that project.
type Component struct {
	ID          string `json:"id"`
	Name        string `json:"name"`
	Description string `json:"description,omitempty"`
	Lead        string `json:"lead,omitempty"`
}

// Version is a project version — a valid value for an issue's fixVersions /
// affects-versions fields within that project.
type Version struct {
	ID          string `json:"id"`
	Name        string `json:"name"`
	Description string `json:"description,omitempty"`
	Released    bool   `json:"released"`
	Archived    bool   `json:"archived"`
	StartDate   string `json:"start_date,omitempty"`
	ReleaseDate string `json:"release_date,omitempty"`
}

// IssueType is an issue type that can be created in a project.
type IssueType struct {
	ID          string `json:"id"`
	Name        string `json:"name"`
	Description string `json:"description,omitempty"`
	Subtask     bool   `json:"subtask"`
}

// Priority is an issue priority level.
type Priority struct {
	ID          string `json:"id"`
	Name        string `json:"name"`
	Description string `json:"description,omitempty"`
}

// IssueTypeStatuses groups the workflow statuses valid for one issue type of a
// project. The same status often appears under several issue types.
type IssueTypeStatuses struct {
	IssueTypeID string   `json:"issue_type_id"`
	IssueType   string   `json:"issue_type"`
	Statuses    []Status `json:"statuses"`
}

// FieldOption is one allowed value of a constrained field. Value carries the
// option's display value (the option "value" or "name", whichever the field
// type uses on the wire).
type FieldOption struct {
	ID    string `json:"id,omitempty"`
	Value string `json:"value"`
}

// FieldMeta describes one field on an issue type's create screen: whether it
// is required, its schema, and — for constrained fields such as components,
// versions, priority and select-list custom fields — its allowed values in
// this project + issue type context.
type FieldMeta struct {
	ID         string   `json:"id"`
	Name       string   `json:"name"`
	Required   bool     `json:"required"`
	Type       string   `json:"type,omitempty"`  // schema type, e.g. "array", "option", "priority"
	Items      string   `json:"items,omitempty"` // element type for arrays, e.g. "component"
	Custom     string   `json:"custom,omitempty"`
	Operations []string `json:"operations,omitempty"`
	HasDefault bool     `json:"has_default,omitempty"`
	// OptionsCount is len(AllowedValues); it survives when a presentation
	// layer trims the values themselves (see `field list`).
	OptionsCount  int           `json:"options_count,omitempty"`
	AllowedValues []FieldOption `json:"allowed_values,omitempty"`
}

// ProjectItemsOpts controls a listing of items scoped to one project
// (components, versions, issue types).
type ProjectItemsOpts struct {
	ListOpts
	ProjectKey string
}

// CreateFieldsOpts controls a create-screen field metadata listing for one
// project + issue type context.
type CreateFieldsOpts struct {
	ListOpts
	ProjectKey  string
	IssueTypeID string
}

// ListResult is one page of a paginated listing. Next is an opaque cursor for
// the following page, empty when the listing is exhausted.
type ListResult[T any] struct {
	Items []T    `json:"items"`
	Next  string `json:"next,omitempty"`
}

// ListOpts controls a paginated listing.
type ListOpts struct {
	// Limit is the page size; 0 uses the client default.
	Limit int
	// Cursor continues a previous listing; empty starts from the beginning.
	Cursor string
}

// GetIssueOpts controls an issue fetch.
type GetIssueOpts struct {
	Key string
	// Expand requests extra sections, comma-separated (e.g.
	// "renderedFields,changelog,transitions"). Passed through to the API.
	Expand string
}

// SearchOpts controls a JQL issue search.
type SearchOpts struct {
	ListOpts
	JQL string
	// Fields widens or narrows the fields returned per issue; empty uses the
	// client's default field set (see defaultSearchFields).
	Fields []string
}

// ProjectListOpts controls a project listing. Query filters by name/key
// substring (Cloud-native; applied client-side on Data Center).
type ProjectListOpts struct {
	ListOpts
	Query string
}

// ListCommentsOpts controls a comment listing for one issue.
type ListCommentsOpts struct {
	ListOpts
	IssueKey string
}

// CreateIssueReq is a request to create an issue. Assignee accepts a Cloud
// accountId, a DC username, or a display-name/email query that the client
// resolves to a unique user (see users.go).
type CreateIssueReq struct {
	ProjectKey  string
	Type        string // issue type name, e.g. "Task", "Bug"
	Summary     string
	Description string
	Assignee    string
	Priority    string // priority name, e.g. "High"
	Labels      []string
	ParentKey   string // for subtasks / issues under an epic
}

// EditIssueReq is a request to update issue fields. Nil pointers keep the
// current value; AddLabels/RemoveLabels adjust the label set incrementally.
type EditIssueReq struct {
	Key          string
	Summary      *string
	Description  *string
	Priority     string
	AddLabels    []string
	RemoveLabels []string
}

// AssignIssueReq is a request to change an issue's assignee. Unassign clears
// the assignee; otherwise Assignee is resolved like CreateIssueReq.Assignee.
type AssignIssueReq struct {
	Key      string
	Assignee string
	Unassign bool
}

// TransitionIssueReq is a request to move an issue through a workflow
// transition. TransitionID must be a transition ID (the command layer resolves
// names via ListTransitions first). Comment optionally adds a comment as part
// of the transition.
type TransitionIssueReq struct {
	Key          string
	TransitionID string
	Comment      string
}

// AddCommentReq is a request to add a comment to an issue.
type AddCommentReq struct {
	IssueKey string
	Body     string
}

// UpdateCommentReq is a request to replace a comment's body.
type UpdateCommentReq struct {
	IssueKey string
	ID       string
	Body     string
}

// DeleteCommentReq is a request to delete a comment.
type DeleteCommentReq struct {
	IssueKey string
	ID       string
}

// WriteRequestPlan describes the HTTP request a write operation would send,
// without sending it. It is used to render --dry-run previews.
type WriteRequestPlan struct {
	Method  string `json:"method"`
	URL     string `json:"url"`
	Payload any    `json:"payload,omitempty"`
}
