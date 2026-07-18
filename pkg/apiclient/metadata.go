package apiclient

// metadata.go implements metadata discovery: the valid values of issue fields.
// Project-scoped listings (components, versions, issue types, statuses) read
// one project's option sets; priorities and labels are instance-wide. The
// create-metadata endpoints additionally expose per-context allowed values for
// any constrained field, including custom select fields.
//
// The wire shapes diverge more than usual here (see the capability table):
// Cloud paginates through PageBean envelopes (`values` + `isLast`) while DC
// returns full-list arrays, and the granular createmeta endpoints name their
// item array differently per flavor and version — Cloud `issueTypes`/`fields`
// (with legacy aliases), DC `values`. Decoding is tolerant of all of them.

import (
	"context"
	"encoding/json"
	"net/url"
	"strconv"

	cerrors "github.com/angelmsger/jira-cli/pkg/errors"
)

// cloudPage is the Cloud PageBean envelope shared by paginated metadata
// listings (component, version, priority/search, label).
type cloudPage[T any] struct {
	Values  []T  `json:"values"`
	StartAt int  `json:"startAt"`
	Total   int  `json:"total"`
	IsLast  bool `json:"isLast"`
}

// next returns the continuation cursor for the page, or "" on the last page.
func (p cloudPage[T]) next(limit int) string {
	if p.IsLast {
		return ""
	}
	return nextOffsetToken(strconv.Itoa(p.StartAt), limit, len(p.Values), p.Total)
}

// metaNextToken computes the continuation cursor for a granular createmeta
// page. The paging metadata differs across flavors and DC versions, so it
// trusts, in order: an explicit last-page flag, a reported total, and finally
// the page-full heuristic.
func metaNextToken(cursor string, limit, size int, isLast *bool, total *int) string {
	if isLast != nil {
		if *isLast || size == 0 {
			return ""
		}
		start := 0
		if cursor != "" {
			if n, err := strconv.Atoi(cursor); err == nil {
				start = n
			}
		}
		return strconv.Itoa(start + size)
	}
	t := -1
	if total != nil {
		t = *total
	}
	return nextOffsetToken(cursor, limit, size, t)
}

func requireProjectKey(key string) error {
	if key == "" {
		return cerrors.New(cerrors.CategoryUsage, "PROJECT_NO_KEY", "a project key is required")
	}
	return nil
}

func (c *apiClient) projectPath(key string) string {
	return c.apiBase() + "/project/" + url.PathEscape(key)
}

// --- Components ---

type rawComponent struct {
	ID          string `json:"id"`
	Name        string `json:"name"`
	Description string `json:"description"`
	Lead        *struct {
		DisplayName string `json:"displayName"`
	} `json:"lead"`
}

func mapComponent(r rawComponent) Component {
	comp := Component{ID: r.ID, Name: r.Name, Description: r.Description}
	if r.Lead != nil {
		comp.Lead = r.Lead.DisplayName
	}
	return comp
}

// ListComponents lists a project's components.
//
//	Cloud: GET /rest/api/3/project/{key}/component  (paginated PageBean)
//	DC:    GET /rest/api/2/project/{key}/components — full list, one response
func (c *apiClient) ListComponents(ctx context.Context, opt ProjectItemsOpts) (ListResult[Component], error) {
	if err := requireProjectKey(opt.ProjectKey); err != nil {
		return ListResult[Component]{}, err
	}
	limit := c.limitOf(opt.ListOpts)
	if c.flavor == FlavorCloud {
		var raw cloudPage[rawComponent]
		if err := c.getJSON(ctx, c.projectPath(opt.ProjectKey)+"/component", offsetQuery(opt.Cursor, limit), &raw); err != nil {
			return ListResult[Component]{}, err
		}
		res := ListResult[Component]{Next: raw.next(limit)}
		for _, r := range raw.Values {
			res.Items = append(res.Items, mapComponent(r))
		}
		return res, nil
	}
	var raw []rawComponent
	if err := c.getJSON(ctx, c.projectPath(opt.ProjectKey)+"/components", nil, &raw); err != nil {
		return ListResult[Component]{}, err
	}
	res := ListResult[Component]{}
	for _, r := range raw {
		res.Items = append(res.Items, mapComponent(r))
	}
	return res, nil
}

// --- Versions ---

type rawVersion struct {
	ID          string `json:"id"`
	Name        string `json:"name"`
	Description string `json:"description"`
	Released    bool   `json:"released"`
	Archived    bool   `json:"archived"`
	StartDate   string `json:"startDate"`
	ReleaseDate string `json:"releaseDate"`
}

func mapVersion(r rawVersion) Version {
	return Version{
		ID: r.ID, Name: r.Name, Description: r.Description,
		Released: r.Released, Archived: r.Archived,
		StartDate: r.StartDate, ReleaseDate: r.ReleaseDate,
	}
}

// ListVersions lists a project's versions.
//
//	Cloud: GET /rest/api/3/project/{key}/version  (paginated PageBean)
//	DC:    GET /rest/api/2/project/{key}/versions — full list, one response
func (c *apiClient) ListVersions(ctx context.Context, opt ProjectItemsOpts) (ListResult[Version], error) {
	if err := requireProjectKey(opt.ProjectKey); err != nil {
		return ListResult[Version]{}, err
	}
	limit := c.limitOf(opt.ListOpts)
	if c.flavor == FlavorCloud {
		var raw cloudPage[rawVersion]
		if err := c.getJSON(ctx, c.projectPath(opt.ProjectKey)+"/version", offsetQuery(opt.Cursor, limit), &raw); err != nil {
			return ListResult[Version]{}, err
		}
		res := ListResult[Version]{Next: raw.next(limit)}
		for _, r := range raw.Values {
			res.Items = append(res.Items, mapVersion(r))
		}
		return res, nil
	}
	var raw []rawVersion
	if err := c.getJSON(ctx, c.projectPath(opt.ProjectKey)+"/versions", nil, &raw); err != nil {
		return ListResult[Version]{}, err
	}
	res := ListResult[Version]{}
	for _, r := range raw {
		res.Items = append(res.Items, mapVersion(r))
	}
	return res, nil
}

// --- Issue types (createmeta) ---

type rawIssueTypeMeta struct {
	ID          string `json:"id"`
	Name        string `json:"name"`
	Description string `json:"description"`
	Subtask     bool   `json:"subtask"`
}

func mapIssueType(r rawIssueTypeMeta) IssueType {
	return IssueType{ID: r.ID, Name: r.Name, Description: r.Description, Subtask: r.Subtask}
}

// ListProjectIssueTypes lists the issue types creatable in a project, from the
// granular create metadata (so create permission applies).
//
//	Cloud: GET /rest/api/3/issue/createmeta/{key}/issuetypes — items under
//	       `issueTypes` (legacy alias `createMetaIssueType`)
//	DC:    GET /rest/api/2/issue/createmeta/{key}/issuetypes (DC >= 8.4) —
//	       items under `values`
func (c *apiClient) ListProjectIssueTypes(ctx context.Context, opt ProjectItemsOpts) (ListResult[IssueType], error) {
	if err := requireProjectKey(opt.ProjectKey); err != nil {
		return ListResult[IssueType]{}, err
	}
	limit := c.limitOf(opt.ListOpts)
	var raw struct {
		IssueTypes []rawIssueTypeMeta `json:"issueTypes"`          // Cloud (current)
		Alias      []rawIssueTypeMeta `json:"createMetaIssueType"` // Cloud (legacy alias)
		Values     []rawIssueTypeMeta `json:"values"`              // DC
		Total      *int               `json:"total"`
		IsLast     *bool              `json:"isLast"`
		Last       *bool              `json:"last"` // DC 9.x doc naming
	}
	path := c.apiBase() + "/issue/createmeta/" + url.PathEscape(opt.ProjectKey) + "/issuetypes"
	if err := c.getJSON(ctx, path, offsetQuery(opt.Cursor, limit), &raw); err != nil {
		return ListResult[IssueType]{}, err
	}
	items := raw.IssueTypes
	if len(items) == 0 {
		items = raw.Alias
	}
	if len(items) == 0 {
		items = raw.Values
	}
	isLast := raw.IsLast
	if isLast == nil {
		isLast = raw.Last
	}
	res := ListResult[IssueType]{Next: metaNextToken(opt.Cursor, limit, len(items), isLast, raw.Total)}
	for _, r := range items {
		res.Items = append(res.Items, mapIssueType(r))
	}
	return res, nil
}

// --- Statuses ---

// ListProjectStatuses lists a project's workflow statuses grouped by issue
// type. The endpoint is unpaginated, so the result is always a single page.
//
//	Both flavors: GET /project/{key}/statuses
func (c *apiClient) ListProjectStatuses(ctx context.Context, projectKey string) (ListResult[IssueTypeStatuses], error) {
	if err := requireProjectKey(projectKey); err != nil {
		return ListResult[IssueTypeStatuses]{}, err
	}
	var raw []struct {
		ID       string `json:"id"`
		Name     string `json:"name"`
		Statuses []struct {
			ID             string `json:"id"`
			Name           string `json:"name"`
			StatusCategory struct {
				Key string `json:"key"`
			} `json:"statusCategory"`
		} `json:"statuses"`
	}
	if err := c.getJSON(ctx, c.projectPath(projectKey)+"/statuses", nil, &raw); err != nil {
		return ListResult[IssueTypeStatuses]{}, err
	}
	res := ListResult[IssueTypeStatuses]{}
	for _, it := range raw {
		group := IssueTypeStatuses{IssueTypeID: it.ID, IssueType: it.Name}
		for _, st := range it.Statuses {
			group.Statuses = append(group.Statuses, Status{
				ID: st.ID, Name: st.Name, Category: st.StatusCategory.Key,
			})
		}
		res.Items = append(res.Items, group)
	}
	return res, nil
}

// --- Priorities ---

type rawPriority struct {
	ID          string `json:"id"`
	Name        string `json:"name"`
	Description string `json:"description"`
}

func mapPriority(r rawPriority) Priority {
	return Priority{ID: r.ID, Name: r.Name, Description: r.Description}
}

// ListPriorities lists the instance's issue priorities.
//
//	Cloud: GET /rest/api/3/priority/search  (paginated PageBean; the plain
//	       /priority listing is deprecated on Cloud)
//	DC:    GET /rest/api/2/priority — full list, one response
func (c *apiClient) ListPriorities(ctx context.Context, opt ListOpts) (ListResult[Priority], error) {
	limit := c.limitOf(opt)
	if c.flavor == FlavorCloud {
		var raw cloudPage[rawPriority]
		if err := c.getJSON(ctx, c.apiBase()+"/priority/search", offsetQuery(opt.Cursor, limit), &raw); err != nil {
			return ListResult[Priority]{}, err
		}
		res := ListResult[Priority]{Next: raw.next(limit)}
		for _, r := range raw.Values {
			res.Items = append(res.Items, mapPriority(r))
		}
		return res, nil
	}
	var raw []rawPriority
	if err := c.getJSON(ctx, c.apiBase()+"/priority", nil, &raw); err != nil {
		return ListResult[Priority]{}, err
	}
	res := ListResult[Priority]{}
	for _, r := range raw {
		res.Items = append(res.Items, mapPriority(r))
	}
	return res, nil
}

// --- Labels ---

// ListLabels lists every label in use on the instance.
//
//	Cloud: GET /rest/api/3/label  (paginated PageBean of plain strings)
//	DC:    unsupported — REST v2 has no label-listing endpoint
func (c *apiClient) ListLabels(ctx context.Context, opt ListOpts) (ListResult[string], error) {
	if sup := c.supportFor(CapLabelList); !sup.Supported() {
		return ListResult[string]{}, cerrors.New(cerrors.CategoryUsage, "LABEL_LIST_DC",
			"label list is not available on this backend: "+sup.Reason).
			WithHint("Labels still appear on issues; discover the ones in use from search results.").
			WithNextSteps(`jira-cli issue search --project <key> --fields items.key,items.labels`)
	}
	limit := c.limitOf(opt)
	var raw cloudPage[string]
	if err := c.getJSON(ctx, c.apiBase()+"/label", offsetQuery(opt.Cursor, limit), &raw); err != nil {
		return ListResult[string]{}, err
	}
	return ListResult[string]{Items: raw.Values, Next: raw.next(limit)}, nil
}

// --- Create-screen field metadata ---

type rawFieldMeta struct {
	FieldID  string `json:"fieldId"`
	Name     string `json:"name"`
	Required bool   `json:"required"`
	Schema   struct {
		Type   string `json:"type"`
		Items  string `json:"items"`
		Custom string `json:"custom"`
	} `json:"schema"`
	Operations      []string          `json:"operations"`
	HasDefaultValue bool              `json:"hasDefaultValue"`
	AllowedValues   []json.RawMessage `json:"allowedValues"`
}

func mapFieldMeta(r rawFieldMeta) FieldMeta {
	fm := FieldMeta{
		ID: r.FieldID, Name: r.Name, Required: r.Required,
		Type: r.Schema.Type, Items: r.Schema.Items, Custom: r.Schema.Custom,
		Operations: r.Operations, HasDefault: r.HasDefaultValue,
	}
	for _, raw := range r.AllowedValues {
		fm.AllowedValues = append(fm.AllowedValues, mapFieldOption(raw))
	}
	fm.OptionsCount = len(fm.AllowedValues)
	return fm
}

// mapFieldOption normalizes one allowedValues entry. The entries are the
// serialized beans of the field's type and are officially untyped: custom
// select options carry `value`, components/versions/priorities/issue types
// carry `name`, and ids may arrive as strings or numbers.
func mapFieldOption(raw json.RawMessage) FieldOption {
	var v struct {
		ID    any    `json:"id"`
		Value string `json:"value"`
		Name  string `json:"name"`
	}
	if err := json.Unmarshal(raw, &v); err != nil {
		// Not an object (some field types list bare strings).
		var s string
		if json.Unmarshal(raw, &s) == nil {
			return FieldOption{Value: s}
		}
		return FieldOption{Value: string(raw)}
	}
	opt := FieldOption{Value: v.Value}
	if opt.Value == "" {
		opt.Value = v.Name
	}
	switch id := v.ID.(type) {
	case string:
		opt.ID = id
	case float64:
		opt.ID = strconv.FormatFloat(id, 'f', -1, 64)
	}
	return opt
}

// ListCreateFields reports the create-screen field metadata for one project +
// issue type context, including each constrained field's allowed values.
//
//	Cloud: GET /rest/api/3/issue/createmeta/{key}/issuetypes/{typeId} — items
//	       under `fields` (legacy alias `results`)
//	DC:    GET /rest/api/2/issue/createmeta/{key}/issuetypes/{typeId}
//	       (DC >= 8.4) — items under `values`
func (c *apiClient) ListCreateFields(ctx context.Context, opt CreateFieldsOpts) (ListResult[FieldMeta], error) {
	if err := requireProjectKey(opt.ProjectKey); err != nil {
		return ListResult[FieldMeta]{}, err
	}
	if opt.IssueTypeID == "" {
		return ListResult[FieldMeta]{}, cerrors.New(cerrors.CategoryUsage, "FIELD_NO_ISSUETYPE",
			"an issue type ID is required (see ListProjectIssueTypes)")
	}
	limit := c.limitOf(opt.ListOpts)
	var raw struct {
		Fields  []rawFieldMeta `json:"fields"`  // Cloud (current)
		Results []rawFieldMeta `json:"results"` // Cloud (legacy alias)
		Values  []rawFieldMeta `json:"values"`  // DC
		Total   *int           `json:"total"`
		IsLast  *bool          `json:"isLast"`
		Last    *bool          `json:"last"` // DC 9.x doc naming
	}
	path := c.apiBase() + "/issue/createmeta/" + url.PathEscape(opt.ProjectKey) +
		"/issuetypes/" + url.PathEscape(opt.IssueTypeID)
	if err := c.getJSON(ctx, path, offsetQuery(opt.Cursor, limit), &raw); err != nil {
		return ListResult[FieldMeta]{}, err
	}
	items := raw.Fields
	if len(items) == 0 {
		items = raw.Results
	}
	if len(items) == 0 {
		items = raw.Values
	}
	isLast := raw.IsLast
	if isLast == nil {
		isLast = raw.Last
	}
	res := ListResult[FieldMeta]{Next: metaNextToken(opt.Cursor, limit, len(items), isLast, raw.Total)}
	for _, r := range items {
		res.Items = append(res.Items, mapFieldMeta(r))
	}
	return res, nil
}
