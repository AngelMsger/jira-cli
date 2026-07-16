package apiclient

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	cerrors "github.com/angelmsger/jira-cli/pkg/errors"
	"github.com/angelmsger/jira-cli/pkg/transport"
)

// newTestClient builds a client of the given flavor pointed at handler.
func newTestClient(t *testing.T, flavor Flavor, handler http.Handler) Client {
	t.Helper()
	srv := httptest.NewServer(handler)
	t.Cleanup(srv.Close)
	return New(Config{
		Flavor:    flavor,
		BaseURL:   srv.URL,
		Transport: transport.New(transport.Options{}),
	})
}

// silentHandler fails the test if any request arrives — used to prove
// DescribeWrite stays offline for ops that need no lookups.
func silentHandler(t *testing.T) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		t.Errorf("unexpected HTTP request: %s %s", r.Method, r.URL.Path)
	})
}

// TestDescribeWritePlansPerFlavor pins the method, path and payload dialect of
// every write op on both flavors — the parity table this family's recurring
// bug class (one flavor dropping an input) is guarded by.
func TestDescribeWritePlansPerFlavor(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	summary := "New title"
	desc := "line one\nline two"

	cases := []struct {
		name       string
		op         any
		wantMethod string
		wantPath   string // per flavor: {v} replaced by 3 (cloud) / 2 (dc)
		check      func(t *testing.T, flavor Flavor, payload []byte)
	}{
		{
			name:       "create issue",
			op:         CreateIssueReq{ProjectKey: "ENG", Type: "Task", Summary: "s", Description: desc, Labels: []string{"l1"}},
			wantMethod: "POST",
			wantPath:   "/rest/api/{v}/issue",
			check: func(t *testing.T, flavor Flavor, payload []byte) {
				var v struct {
					Fields struct {
						Description json.RawMessage `json:"description"`
						Labels      []string        `json:"labels"`
					} `json:"fields"`
				}
				if err := json.Unmarshal(payload, &v); err != nil {
					t.Fatalf("payload: %v", err)
				}
				if len(v.Fields.Labels) != 1 {
					t.Errorf("labels dropped: %s", payload)
				}
				assertBodyDialect(t, flavor, v.Fields.Description, desc)
			},
		},
		{
			name:       "edit issue description",
			op:         EditIssueReq{Key: "ENG-1", Summary: &summary, Description: &desc, AddLabels: []string{"a"}, RemoveLabels: []string{"b"}},
			wantMethod: "PUT",
			wantPath:   "/rest/api/{v}/issue/ENG-1",
			check: func(t *testing.T, flavor Flavor, payload []byte) {
				var v struct {
					Fields struct {
						Summary     string          `json:"summary"`
						Description json.RawMessage `json:"description"`
					} `json:"fields"`
					Update struct {
						Labels []map[string]string `json:"labels"`
					} `json:"update"`
				}
				if err := json.Unmarshal(payload, &v); err != nil {
					t.Fatalf("payload: %v", err)
				}
				if v.Fields.Summary != summary {
					t.Errorf("summary = %q", v.Fields.Summary)
				}
				if len(v.Update.Labels) != 2 {
					t.Errorf("label ops = %v, want add+remove", v.Update.Labels)
				}
				assertBodyDialect(t, flavor, v.Fields.Description, desc)
			},
		},
		{
			name:       "unassign",
			op:         AssignIssueReq{Key: "ENG-1", Unassign: true},
			wantMethod: "PUT",
			wantPath:   "/rest/api/{v}/issue/ENG-1/assignee",
			check: func(t *testing.T, flavor Flavor, payload []byte) {
				field := "name"
				if flavor == FlavorCloud {
					field = "accountId"
				}
				var v map[string]any
				if err := json.Unmarshal(payload, &v); err != nil {
					t.Fatalf("payload: %v", err)
				}
				if got, ok := v[field]; !ok || got != nil {
					t.Errorf("payload = %s, want {%q: null}", payload, field)
				}
			},
		},
		{
			name:       "transition with comment",
			op:         TransitionIssueReq{Key: "ENG-1", TransitionID: "21", Comment: "done"},
			wantMethod: "POST",
			wantPath:   "/rest/api/{v}/issue/ENG-1/transitions",
			check: func(t *testing.T, flavor Flavor, payload []byte) {
				var v struct {
					Transition struct {
						ID string `json:"id"`
					} `json:"transition"`
					Update struct {
						Comment []struct {
							Add struct {
								Body json.RawMessage `json:"body"`
							} `json:"add"`
						} `json:"comment"`
					} `json:"update"`
				}
				if err := json.Unmarshal(payload, &v); err != nil {
					t.Fatalf("payload: %v", err)
				}
				if v.Transition.ID != "21" {
					t.Errorf("transition id = %q", v.Transition.ID)
				}
				if len(v.Update.Comment) != 1 {
					t.Fatalf("comment dropped: %s", payload)
				}
				assertBodyDialect(t, flavor, v.Update.Comment[0].Add.Body, "done")
			},
		},
		{
			name:       "add comment",
			op:         AddCommentReq{IssueKey: "ENG-1", Body: "hello"},
			wantMethod: "POST",
			wantPath:   "/rest/api/{v}/issue/ENG-1/comment",
			check: func(t *testing.T, flavor Flavor, payload []byte) {
				var v struct {
					Body json.RawMessage `json:"body"`
				}
				if err := json.Unmarshal(payload, &v); err != nil {
					t.Fatalf("payload: %v", err)
				}
				assertBodyDialect(t, flavor, v.Body, "hello")
			},
		},
		{
			name:       "update comment",
			op:         UpdateCommentReq{IssueKey: "ENG-1", ID: "7", Body: "x"},
			wantMethod: "PUT",
			wantPath:   "/rest/api/{v}/issue/ENG-1/comment/7",
		},
		{
			name:       "delete comment",
			op:         DeleteCommentReq{IssueKey: "ENG-1", ID: "7"},
			wantMethod: "DELETE",
			wantPath:   "/rest/api/{v}/issue/ENG-1/comment/7",
		},
	}

	for _, flavor := range []Flavor{FlavorCloud, FlavorDataCenter} {
		flavor := flavor
		ver := "2"
		if flavor == FlavorCloud {
			ver = "3"
		}
		for _, tc := range cases {
			tc := tc
			t.Run(string(flavor)+"/"+tc.name, func(t *testing.T) {
				t.Parallel()
				c := newTestClient(t, flavor, silentHandler(t))
				plan, err := c.DescribeWrite(ctx, tc.op)
				if err != nil {
					t.Fatalf("DescribeWrite: %v", err)
				}
				if plan.Method != tc.wantMethod {
					t.Errorf("method = %s, want %s", plan.Method, tc.wantMethod)
				}
				wantPath := strings.ReplaceAll(tc.wantPath, "{v}", ver)
				if !strings.HasSuffix(plan.URL, wantPath) {
					t.Errorf("url = %s, want suffix %s", plan.URL, wantPath)
				}
				if tc.check != nil {
					raw, err := json.Marshal(plan.Payload)
					if err != nil {
						t.Fatalf("marshal payload: %v", err)
					}
					tc.check(t, flavor, raw)
				}
			})
		}
	}
}

// assertBodyDialect asserts a rich-text field took the flavor's wire shape:
// an ADF doc that flattens back to text on Cloud, the raw string on DC.
func assertBodyDialect(t *testing.T, flavor Flavor, raw json.RawMessage, wantText string) {
	t.Helper()
	if flavor == FlavorCloud {
		var doc struct {
			Type string `json:"type"`
		}
		if err := json.Unmarshal(raw, &doc); err != nil || doc.Type != "doc" {
			t.Fatalf("cloud body is not an ADF doc: %s", raw)
		}
		if got := ADFToText(raw); got != wantText {
			t.Errorf("cloud ADF round-trip = %q, want %q", got, wantText)
		}
		return
	}
	var s string
	if err := json.Unmarshal(raw, &s); err != nil || s != wantText {
		t.Errorf("dc body = %s, want %q verbatim", raw, wantText)
	}
}

// TestSearchDialects pins the flavor split of the JQL search endpoint: Cloud
// POSTs /search/jql with token pagination, DC GETs /search with startAt.
func TestSearchDialects(t *testing.T) {
	t.Parallel()
	ctx := context.Background()

	t.Run("cloud", func(t *testing.T) {
		t.Parallel()
		var gotMethod, gotPath string
		var gotBody struct {
			JQL           string   `json:"jql"`
			NextPageToken string   `json:"nextPageToken"`
			Fields        []string `json:"fields"`
		}
		c := newTestClient(t, FlavorCloud, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			gotMethod, gotPath = r.Method, r.URL.Path
			_ = json.NewDecoder(r.Body).Decode(&gotBody)
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write([]byte(`{"issues":[{"id":"1","key":"ENG-1","fields":{"summary":"s"}}],"nextPageToken":"tok2","isLast":false}`))
		}))
		res, err := c.SearchIssues(ctx, SearchOpts{JQL: "project = ENG", ListOpts: ListOpts{Cursor: "tok1"}})
		if err != nil {
			t.Fatal(err)
		}
		if gotMethod != "POST" || gotPath != "/rest/api/3/search/jql" {
			t.Errorf("request = %s %s, want POST /rest/api/3/search/jql", gotMethod, gotPath)
		}
		if gotBody.NextPageToken != "tok1" {
			t.Errorf("cursor not forwarded: %+v", gotBody)
		}
		if len(gotBody.Fields) == 0 {
			t.Error("default fields list not sent — Cloud returns bare IDs without it")
		}
		if res.Next != "tok2" {
			t.Errorf("next = %q, want tok2", res.Next)
		}
	})

	t.Run("datacenter", func(t *testing.T) {
		t.Parallel()
		var gotMethod, gotPath, gotQuery string
		c := newTestClient(t, FlavorDataCenter, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			gotMethod, gotPath, gotQuery = r.Method, r.URL.Path, r.URL.RawQuery
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write([]byte(`{"issues":[{"id":"1","key":"ENG-1","fields":{"summary":"s"}},{"id":"2","key":"ENG-2","fields":{"summary":"t"}}],"startAt":0,"total":5}`))
		}))
		res, err := c.SearchIssues(ctx, SearchOpts{JQL: "project = ENG", ListOpts: ListOpts{Limit: 2}})
		if err != nil {
			t.Fatal(err)
		}
		if gotMethod != "GET" || gotPath != "/rest/api/2/search" {
			t.Errorf("request = %s %s, want GET /rest/api/2/search", gotMethod, gotPath)
		}
		if !strings.Contains(gotQuery, "startAt=0") || !strings.Contains(gotQuery, "jql=") {
			t.Errorf("query = %s", gotQuery)
		}
		if res.Next != "2" {
			t.Errorf("next = %q, want offset 2", res.Next)
		}
	})
}

// TestResolveUserCloud pins Cloud assignee resolution: accountId passthrough,
// unique-match resolution, and the ambiguous / not-found errors.
func TestResolveUserCloud(t *testing.T) {
	t.Parallel()
	ctx := context.Background()

	t.Run("accountId passthrough stays offline", func(t *testing.T) {
		t.Parallel()
		c := newTestClient(t, FlavorCloud, silentHandler(t))
		u, err := c.ResolveUser(ctx, "712020:aaaa-bbbb")
		if err != nil {
			t.Fatal(err)
		}
		if u.AccountID != "712020:aaaa-bbbb" {
			t.Errorf("accountId = %q", u.AccountID)
		}
	})

	t.Run("unique match resolves", func(t *testing.T) {
		t.Parallel()
		c := newTestClient(t, FlavorCloud, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write([]byte(`[{"accountId":"712020:x","displayName":"Alice","active":true}]`))
		}))
		u, err := c.ResolveUser(ctx, "alice")
		if err != nil {
			t.Fatal(err)
		}
		if u.AccountID != "712020:x" {
			t.Errorf("accountId = %q", u.AccountID)
		}
	})

	t.Run("ambiguous lists candidates", func(t *testing.T) {
		t.Parallel()
		c := newTestClient(t, FlavorCloud, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write([]byte(`[{"accountId":"1","displayName":"Al A","active":true},{"accountId":"2","displayName":"Al B","active":true}]`))
		}))
		_, err := c.ResolveUser(ctx, "al")
		ce := cerrors.AsCLIError(err)
		if ce == nil || ce.Code != "USER_AMBIGUOUS" {
			t.Fatalf("err = %v, want USER_AMBIGUOUS", err)
		}
		if len(ce.NextSteps) != 2 {
			t.Errorf("candidates = %v", ce.NextSteps)
		}
	})

	t.Run("dc echoes username offline", func(t *testing.T) {
		t.Parallel()
		c := newTestClient(t, FlavorDataCenter, silentHandler(t))
		u, err := c.ResolveUser(ctx, "alice")
		if err != nil {
			t.Fatal(err)
		}
		if u.Username != "alice" {
			t.Errorf("username = %q", u.Username)
		}
	})
}

// TestExtractAPIMessage pins the Jira error envelope parsing.
func TestExtractAPIMessage(t *testing.T) {
	t.Parallel()
	got := extractAPIMessage([]byte(`{"errorMessages":["boom"],"errors":{"summary":"required"}}`))
	if !strings.Contains(got, "boom") || !strings.Contains(got, "summary: required") {
		t.Errorf("extractAPIMessage = %q", got)
	}
	if extractAPIMessage([]byte(`not json`)) != "" {
		t.Error("garbage should extract nothing")
	}
}
