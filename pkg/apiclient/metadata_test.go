package apiclient

import (
	"context"
	"encoding/json"
	"net/http"
	"reflect"
	"testing"

	cerrors "github.com/angelmsger/jira-cli/pkg/errors"
)

// TestListComponentsPerFlavor pins the per-flavor endpoint and envelope:
// Cloud reads the paginated /component PageBean, DC the full-list /components
// array presented as a single page.
func TestListComponentsPerFlavor(t *testing.T) {
	t.Parallel()
	ctx := context.Background()

	t.Run("cloud paginates", func(t *testing.T) {
		t.Parallel()
		c := newTestClient(t, FlavorCloud, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if r.URL.Path != "/rest/api/3/project/ENG/component" {
				t.Errorf("path = %s", r.URL.Path)
			}
			if got := r.URL.Query().Get("maxResults"); got != "2" {
				t.Errorf("maxResults = %s, want 2", got)
			}
			writeTestJSON(t, w, map[string]any{
				"values": []any{
					map[string]any{"id": "10", "name": "PaaS", "description": "platform",
						"lead": map[string]any{"displayName": "Alice"}},
					map[string]any{"id": "11", "name": "IaaS"},
				},
				"startAt": 0, "total": 3, "isLast": false,
			})
		}))
		res, err := c.ListComponents(ctx, ProjectItemsOpts{ListOpts: ListOpts{Limit: 2}, ProjectKey: "ENG"})
		if err != nil {
			t.Fatal(err)
		}
		want := []Component{
			{ID: "10", Name: "PaaS", Description: "platform", Lead: "Alice"},
			{ID: "11", Name: "IaaS"},
		}
		if !reflect.DeepEqual(res.Items, want) {
			t.Errorf("items = %+v, want %+v", res.Items, want)
		}
		if res.Next != "2" {
			t.Errorf("next = %q, want \"2\"", res.Next)
		}
	})

	t.Run("dc full list is one page", func(t *testing.T) {
		t.Parallel()
		c := newTestClient(t, FlavorDataCenter, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if r.URL.Path != "/rest/api/2/project/ENG/components" {
				t.Errorf("path = %s", r.URL.Path)
			}
			writeTestJSON(t, w, []any{map[string]any{"id": "10", "name": "PaaS"}})
		}))
		res, err := c.ListComponents(ctx, ProjectItemsOpts{ProjectKey: "ENG"})
		if err != nil {
			t.Fatal(err)
		}
		if len(res.Items) != 1 || res.Items[0].Name != "PaaS" {
			t.Errorf("items = %+v", res.Items)
		}
		if res.Next != "" {
			t.Errorf("next = %q, want empty (single page)", res.Next)
		}
	})
}

// TestListVersionsPerFlavor pins the version listing dialects and the
// released/archived/date mapping.
func TestListVersionsPerFlavor(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	version := map[string]any{
		"id": "20", "name": "1.0.0", "released": true, "archived": false,
		"releaseDate": "2026-01-15",
	}

	t.Run("cloud", func(t *testing.T) {
		t.Parallel()
		c := newTestClient(t, FlavorCloud, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if r.URL.Path != "/rest/api/3/project/ENG/version" {
				t.Errorf("path = %s", r.URL.Path)
			}
			writeTestJSON(t, w, map[string]any{"values": []any{version}, "isLast": true})
		}))
		res, err := c.ListVersions(ctx, ProjectItemsOpts{ProjectKey: "ENG"})
		if err != nil {
			t.Fatal(err)
		}
		want := Version{ID: "20", Name: "1.0.0", Released: true, ReleaseDate: "2026-01-15"}
		if len(res.Items) != 1 || res.Items[0] != want {
			t.Errorf("items = %+v, want [%+v]", res.Items, want)
		}
		if res.Next != "" {
			t.Errorf("next = %q, want empty (isLast)", res.Next)
		}
	})

	t.Run("dc", func(t *testing.T) {
		t.Parallel()
		c := newTestClient(t, FlavorDataCenter, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if r.URL.Path != "/rest/api/2/project/ENG/versions" {
				t.Errorf("path = %s", r.URL.Path)
			}
			writeTestJSON(t, w, []any{version})
		}))
		res, err := c.ListVersions(ctx, ProjectItemsOpts{ProjectKey: "ENG"})
		if err != nil {
			t.Fatal(err)
		}
		if len(res.Items) != 1 || res.Items[0].Name != "1.0.0" {
			t.Errorf("items = %+v", res.Items)
		}
	})
}

// TestListProjectIssueTypesEnvelopes proves the createmeta issue-type page is
// decoded whichever array key the server uses: Cloud's current `issueTypes`,
// its legacy alias, or DC's `values`.
func TestListProjectIssueTypesEnvelopes(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	items := []any{
		map[string]any{"id": "1", "name": "Bug", "subtask": false},
		map[string]any{"id": "5", "name": "Sub-task", "subtask": true},
	}

	cases := []struct {
		name   string
		flavor Flavor
		path   string
		body   map[string]any
	}{
		{"cloud issueTypes key", FlavorCloud, "/rest/api/3/issue/createmeta/ENG/issuetypes",
			map[string]any{"issueTypes": items, "startAt": 0, "maxResults": 50, "total": 2}},
		{"cloud legacy alias key", FlavorCloud, "/rest/api/3/issue/createmeta/ENG/issuetypes",
			map[string]any{"createMetaIssueType": items, "total": 2}},
		{"dc values key with isLast", FlavorDataCenter, "/rest/api/2/issue/createmeta/ENG/issuetypes",
			map[string]any{"values": items, "isLast": true}},
		{"dc values key with last", FlavorDataCenter, "/rest/api/2/issue/createmeta/ENG/issuetypes",
			map[string]any{"values": items, "last": true}},
	}
	for _, tc := range cases {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			c := newTestClient(t, tc.flavor, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				if r.URL.Path != tc.path {
					t.Errorf("path = %s, want %s", r.URL.Path, tc.path)
				}
				writeTestJSON(t, w, tc.body)
			}))
			res, err := c.ListProjectIssueTypes(ctx, ProjectItemsOpts{ProjectKey: "ENG"})
			if err != nil {
				t.Fatal(err)
			}
			want := []IssueType{{ID: "1", Name: "Bug"}, {ID: "5", Name: "Sub-task", Subtask: true}}
			if !reflect.DeepEqual(res.Items, want) {
				t.Errorf("items = %+v, want %+v", res.Items, want)
			}
			if res.Next != "" {
				t.Errorf("next = %q, want empty", res.Next)
			}
		})
	}
}

// TestMetaNextToken covers the createmeta paging fallbacks: explicit last-page
// flag first, then total, then the page-full heuristic.
func TestMetaNextToken(t *testing.T) {
	t.Parallel()
	yes, no := true, false
	total10 := 10
	cases := []struct {
		name   string
		cursor string
		limit  int
		size   int
		isLast *bool
		total  *int
		want   string
	}{
		{"isLast true wins", "", 2, 2, &yes, &total10, ""},
		{"isLast false continues", "2", 2, 2, &no, nil, "4"},
		{"isLast false short page still continues", "", 2, 1, &no, nil, "1"},
		{"total exhausted", "8", 2, 2, nil, &total10, ""},
		{"total remaining", "0", 2, 2, nil, &total10, "2"},
		{"no metadata full page", "", 2, 2, nil, nil, "2"},
		{"no metadata short page", "", 2, 1, nil, nil, ""},
	}
	for _, tc := range cases {
		if got := metaNextToken(tc.cursor, tc.limit, tc.size, tc.isLast, tc.total); got != tc.want {
			t.Errorf("%s: metaNextToken = %q, want %q", tc.name, got, tc.want)
		}
	}
}

// TestListProjectStatuses verifies the per-issue-type status grouping shared
// by both flavors.
func TestListProjectStatuses(t *testing.T) {
	t.Parallel()
	c := newTestClient(t, FlavorDataCenter, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/rest/api/2/project/ENG/statuses" {
			t.Errorf("path = %s", r.URL.Path)
		}
		writeTestJSON(t, w, []any{
			map[string]any{"id": "1", "name": "Bug", "statuses": []any{
				map[string]any{"id": "3", "name": "In Progress",
					"statusCategory": map[string]any{"key": "indeterminate"}},
			}},
		})
	}))
	res, err := c.ListProjectStatuses(context.Background(), "ENG")
	if err != nil {
		t.Fatal(err)
	}
	want := []IssueTypeStatuses{{
		IssueTypeID: "1", IssueType: "Bug",
		Statuses: []Status{{ID: "3", Name: "In Progress", Category: "indeterminate"}},
	}}
	if !reflect.DeepEqual(res.Items, want) {
		t.Errorf("items = %+v, want %+v", res.Items, want)
	}
}

// TestListPrioritiesPerFlavor pins the endpoint split: Cloud must use
// /priority/search (the plain listing is deprecated there), DC the plain
// /priority array.
func TestListPrioritiesPerFlavor(t *testing.T) {
	t.Parallel()
	ctx := context.Background()

	t.Run("cloud search endpoint", func(t *testing.T) {
		t.Parallel()
		c := newTestClient(t, FlavorCloud, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if r.URL.Path != "/rest/api/3/priority/search" {
				t.Errorf("path = %s", r.URL.Path)
			}
			writeTestJSON(t, w, map[string]any{
				"values": []any{map[string]any{"id": "2", "name": "High"}},
				"isLast": true,
			})
		}))
		res, err := c.ListPriorities(ctx, ListOpts{})
		if err != nil {
			t.Fatal(err)
		}
		if len(res.Items) != 1 || res.Items[0].Name != "High" {
			t.Errorf("items = %+v", res.Items)
		}
	})

	t.Run("dc plain listing", func(t *testing.T) {
		t.Parallel()
		c := newTestClient(t, FlavorDataCenter, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if r.URL.Path != "/rest/api/2/priority" {
				t.Errorf("path = %s", r.URL.Path)
			}
			writeTestJSON(t, w, []any{map[string]any{"id": "2", "name": "High"}})
		}))
		res, err := c.ListPriorities(ctx, ListOpts{})
		if err != nil {
			t.Fatal(err)
		}
		if len(res.Items) != 1 || res.Items[0].Name != "High" {
			t.Errorf("items = %+v", res.Items)
		}
	})
}

// TestListLabels verifies the Cloud string PageBean and the structured DC
// unsupported-capability error (no HTTP request may be sent there).
func TestListLabels(t *testing.T) {
	t.Parallel()
	ctx := context.Background()

	t.Run("cloud", func(t *testing.T) {
		t.Parallel()
		c := newTestClient(t, FlavorCloud, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if r.URL.Path != "/rest/api/3/label" {
				t.Errorf("path = %s", r.URL.Path)
			}
			writeTestJSON(t, w, map[string]any{"values": []any{"infra", "urgent"}, "isLast": true})
		}))
		res, err := c.ListLabels(ctx, ListOpts{})
		if err != nil {
			t.Fatal(err)
		}
		if !reflect.DeepEqual(res.Items, []string{"infra", "urgent"}) {
			t.Errorf("items = %v", res.Items)
		}
	})

	t.Run("dc is a structured unsupported error", func(t *testing.T) {
		t.Parallel()
		c := newTestClient(t, FlavorDataCenter, silentHandler(t))
		_, err := c.ListLabels(ctx, ListOpts{})
		if err == nil {
			t.Fatal("expected error on DC")
		}
		ce := cerrors.AsCLIError(err)
		if ce.Code != "LABEL_LIST_DC" {
			t.Errorf("code = %s, want LABEL_LIST_DC", ce.Code)
		}
	})
}

// TestListCreateFields proves the field-meta decode across envelope variants
// and the allowedValues normalization: custom options carry `value`,
// components/priorities carry `name`, and ids may be strings or numbers.
func TestListCreateFields(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	fields := []any{
		map[string]any{
			"fieldId": "components", "name": "Component/s", "required": false,
			"schema":        map[string]any{"type": "array", "items": "component"},
			"operations":    []any{"add", "set", "remove"},
			"allowedValues": []any{map[string]any{"id": "10", "name": "PaaS"}},
		},
		map[string]any{
			"fieldId": "customfield_10010", "name": "Severity", "required": true,
			"schema": map[string]any{"type": "option",
				"custom": "com.atlassian.jira.plugin.system.customfieldtypes:select"},
			"allowedValues": []any{
				map[string]any{"id": 1, "value": "Critical"},
				map[string]any{"id": 2, "value": "Minor"},
			},
		},
	}

	cases := []struct {
		name   string
		flavor Flavor
		path   string
		body   map[string]any
	}{
		{"cloud fields key", FlavorCloud, "/rest/api/3/issue/createmeta/ENG/issuetypes/1",
			map[string]any{"fields": fields, "total": 2}},
		{"cloud legacy results key", FlavorCloud, "/rest/api/3/issue/createmeta/ENG/issuetypes/1",
			map[string]any{"results": fields, "total": 2}},
		{"dc values key", FlavorDataCenter, "/rest/api/2/issue/createmeta/ENG/issuetypes/1",
			map[string]any{"values": fields, "isLast": true}},
	}
	for _, tc := range cases {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			c := newTestClient(t, tc.flavor, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				if r.URL.Path != tc.path {
					t.Errorf("path = %s, want %s", r.URL.Path, tc.path)
				}
				writeTestJSON(t, w, tc.body)
			}))
			res, err := c.ListCreateFields(ctx, CreateFieldsOpts{ProjectKey: "ENG", IssueTypeID: "1"})
			if err != nil {
				t.Fatal(err)
			}
			if len(res.Items) != 2 {
				t.Fatalf("items = %d, want 2", len(res.Items))
			}
			comp := res.Items[0]
			if comp.ID != "components" || comp.Type != "array" || comp.Items != "component" {
				t.Errorf("components meta = %+v", comp)
			}
			if !reflect.DeepEqual(comp.AllowedValues, []FieldOption{{ID: "10", Value: "PaaS"}}) {
				t.Errorf("components allowed = %+v", comp.AllowedValues)
			}
			sev := res.Items[1]
			if !sev.Required || sev.OptionsCount != 2 {
				t.Errorf("severity meta = %+v", sev)
			}
			wantSev := []FieldOption{{ID: "1", Value: "Critical"}, {ID: "2", Value: "Minor"}}
			if !reflect.DeepEqual(sev.AllowedValues, wantSev) {
				t.Errorf("severity allowed = %+v, want %+v", sev.AllowedValues, wantSev)
			}
		})
	}
}

// TestMapIssueOptionFields verifies the normalized issue keeps components and
// fix/affects versions — the discovery gap that motivated metadata.go (an
// explicitly requested field must not be dropped by the mapper).
func TestMapIssueOptionFields(t *testing.T) {
	t.Parallel()
	raw := rawIssue{ID: "1", Key: "ENG-1"}
	raw.Fields.Summary = "s"
	raw.Fields.Components = []namedRef{{Name: "PaaS"}, {Name: "IaaS"}}
	raw.Fields.FixVersions = []namedRef{{Name: "1.1.0"}}
	raw.Fields.Versions = []namedRef{{Name: "1.0.0"}}
	issue := mapIssue(raw, "")
	if !reflect.DeepEqual(issue.Components, []string{"PaaS", "IaaS"}) {
		t.Errorf("components = %v", issue.Components)
	}
	if !reflect.DeepEqual(issue.FixVersions, []string{"1.1.0"}) {
		t.Errorf("fix_versions = %v", issue.FixVersions)
	}
	if !reflect.DeepEqual(issue.AffectsVersions, []string{"1.0.0"}) {
		t.Errorf("affects_versions = %v", issue.AffectsVersions)
	}
}

// writeTestJSON encodes v as the response body.
func writeTestJSON(t *testing.T, w http.ResponseWriter, v any) {
	t.Helper()
	w.Header().Set("Content-Type", "application/json")
	if err := json.NewEncoder(w).Encode(v); err != nil {
		t.Errorf("encode response: %v", err)
	}
}
