package apiclient

import (
	"context"
	"errors"
	"maps"
	"net/http"
	"net/http/httptest"
	"reflect"
	"slices"
	"strings"
	"testing"

	cerrors "github.com/angelmsger/jira-cli/pkg/errors"
	"github.com/angelmsger/jira-cli/pkg/transport"
)

// newReadOnlyTestClient builds a Data Center flavored client wrapped in the
// read-only enforcement layer, pointed at handler. The handler is expected to
// never fire when the client's mutating methods are exercised.
func newReadOnlyTestClient(t *testing.T, handler http.Handler) Client {
	t.Helper()
	srv := httptest.NewServer(handler)
	t.Cleanup(srv.Close)
	inner := New(Config{
		Flavor:    FlavorDataCenter,
		BaseURL:   srv.URL,
		Transport: transport.New(transport.Options{}),
	})
	return NewReadOnly(inner)
}

// assertCoversMethods fails unless names is exactly the method set of iface,
// so a table test cannot silently skip a method added to the interface.
func assertCoversMethods(t *testing.T, iface reflect.Type, names []string) {
	t.Helper()
	var want []string
	for i := range iface.NumMethod() {
		want = append(want, iface.Method(i).Name)
	}
	got := slices.Sorted(slices.Values(names))
	if !slices.Equal(got, want) {
		t.Fatalf("table covers %v, %s declares %v", got, iface.Name(), want)
	}
}

// TestReadOnlyBlocksEveryMutator drives every mutating method on the wrapper
// and asserts each one returns a READONLY_BLOCKED permission error before any
// HTTP request is sent.
func TestReadOnlyBlocksEveryMutator(t *testing.T) {
	t.Parallel()
	c := newReadOnlyTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		t.Errorf("read-only wrapper sent an HTTP request: %s %s", r.Method, r.URL.Path)
	}))
	ctx := context.Background()

	cases := []struct {
		name string
		fn   func() error
	}{
		{"CreateIssue", func() error {
			_, err := c.CreateIssue(ctx, CreateIssueReq{ProjectKey: "ENG", Type: "Task", Summary: "x"})
			return err
		}},
		{"EditIssue", func() error {
			s := "x"
			_, err := c.EditIssue(ctx, EditIssueReq{Key: "ENG-1", Summary: &s})
			return err
		}},
		{"AssignIssue", func() error {
			return c.AssignIssue(ctx, AssignIssueReq{Key: "ENG-1", Assignee: "alice"})
		}},
		{"TransitionIssue", func() error {
			return c.TransitionIssue(ctx, TransitionIssueReq{Key: "ENG-1", TransitionID: "31"})
		}},
		{"AddComment", func() error {
			_, err := c.AddComment(ctx, AddCommentReq{IssueKey: "ENG-1", Body: "x"})
			return err
		}},
		{"UpdateComment", func() error {
			_, err := c.UpdateComment(ctx, UpdateCommentReq{IssueKey: "ENG-1", ID: "1", Body: "y"})
			return err
		}},
		{"DeleteComment", func() error {
			return c.DeleteComment(ctx, DeleteCommentReq{IssueKey: "ENG-1", ID: "1"})
		}},
		{"AddWorklog", func() error {
			_, err := c.AddWorklog(ctx, AddWorklogReq{IssueKey: "ENG-1", TimeSpent: "1h"})
			return err
		}},
		{"UpdateWorklog", func() error {
			_, err := c.UpdateWorklog(ctx, UpdateWorklogReq{IssueKey: "ENG-1", ID: "1", TimeSpent: "1h"})
			return err
		}},
		{"DeleteWorklog", func() error {
			return c.DeleteWorklog(ctx, DeleteWorklogReq{IssueKey: "ENG-1", ID: "1"})
		}},
	}
	var names []string
	for _, tc := range cases {
		names = append(names, tc.name)
	}
	assertCoversMethods(t, reflect.TypeFor[Writer](), names)

	for _, tc := range cases {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			err := tc.fn()
			if err == nil {
				t.Fatalf("%s: expected error, got nil", tc.name)
			}
			ce := cerrors.AsCLIError(err)
			if ce.Category != cerrors.CategoryPermission {
				t.Errorf("%s: category = %s, want permission", tc.name, ce.Category)
			}
			if ce.Code != "READONLY_BLOCKED" {
				t.Errorf("%s: code = %s, want READONLY_BLOCKED", tc.name, ce.Code)
			}
			if !strings.Contains(ce.Message, `"`+tc.name+`"`) {
				t.Errorf("%s: message %q does not name the operation", tc.name, ce.Message)
			}
			if !strings.Contains(strings.Join(ce.NextSteps, " "), "--allow-writes") {
				t.Errorf("%s: next_steps missing --allow-writes hint: %v", tc.name, ce.NextSteps)
			}
		})
	}
}

// readCall is one call a recordingReader received.
type readCall struct {
	method string
	args   []any
}

// errCannedRead is returned alongside every canned read result, so delegation
// tests also see errors pass through unchanged.
var errCannedRead = errors.New("canned read error")

// recordingReader answers every Reader method with a fixed non-zero result and
// records the call. It embeds Client only to satisfy NewReadOnly's parameter
// type: a write that reached it would panic on the nil embedded interface.
type recordingReader struct {
	Client
	calls []readCall
}

func (r *recordingReader) record(method string, args ...any) {
	r.calls = append(r.calls, readCall{method: method, args: args})
}

func (r *recordingReader) Flavor() Flavor {
	r.record("Flavor")
	return FlavorCloud
}

func (r *recordingReader) BaseURL() string {
	r.record("BaseURL")
	return "https://jira.example.test"
}

func (r *recordingReader) Ping(ctx context.Context) (ServerInfo, error) {
	r.record("Ping", ctx)
	return ServerInfo{Flavor: FlavorCloud, Reachable: true, Version: "1001.0.0"}, errCannedRead
}

func (r *recordingReader) CurrentUser(ctx context.Context) (*User, error) {
	r.record("CurrentUser", ctx)
	return &User{AccountID: "me"}, errCannedRead
}

func (r *recordingReader) GetIssue(ctx context.Context, opt GetIssueOpts) (*Issue, error) {
	r.record("GetIssue", ctx, opt)
	return &Issue{Key: "ENG-1"}, errCannedRead
}

func (r *recordingReader) SearchIssues(ctx context.Context, opt SearchOpts) (ListResult[Issue], error) {
	r.record("SearchIssues", ctx, opt)
	return ListResult[Issue]{Items: []Issue{{Key: "ENG-2"}}, Next: "2"}, errCannedRead
}

func (r *recordingReader) ListTransitions(ctx context.Context, issueKey string) (ListResult[Transition], error) {
	r.record("ListTransitions", ctx, issueKey)
	return ListResult[Transition]{Items: []Transition{{ID: "31", Name: "Done"}}}, errCannedRead
}

func (r *recordingReader) ListProjects(ctx context.Context, opt ProjectListOpts) (ListResult[Project], error) {
	r.record("ListProjects", ctx, opt)
	return ListResult[Project]{Items: []Project{{Key: "ENG"}}}, errCannedRead
}

func (r *recordingReader) GetProject(ctx context.Context, key string) (*Project, error) {
	r.record("GetProject", ctx, key)
	return &Project{Key: "ENG"}, errCannedRead
}

func (r *recordingReader) ListComponents(ctx context.Context, opt ProjectItemsOpts) (ListResult[Component], error) {
	r.record("ListComponents", ctx, opt)
	return ListResult[Component]{Items: []Component{{ID: "1", Name: "API"}}}, errCannedRead
}

func (r *recordingReader) ListVersions(ctx context.Context, opt ProjectItemsOpts) (ListResult[Version], error) {
	r.record("ListVersions", ctx, opt)
	return ListResult[Version]{Items: []Version{{ID: "2", Name: "1.0"}}}, errCannedRead
}

func (r *recordingReader) ListProjectIssueTypes(ctx context.Context, opt ProjectItemsOpts) (ListResult[IssueType], error) {
	r.record("ListProjectIssueTypes", ctx, opt)
	return ListResult[IssueType]{Items: []IssueType{{ID: "3", Name: "Task"}}}, errCannedRead
}

func (r *recordingReader) ListProjectStatuses(ctx context.Context, projectKey string) (ListResult[IssueTypeStatuses], error) {
	r.record("ListProjectStatuses", ctx, projectKey)
	return ListResult[IssueTypeStatuses]{Items: []IssueTypeStatuses{{IssueTypeID: "3", IssueType: "Task"}}}, errCannedRead
}

func (r *recordingReader) ListPriorities(ctx context.Context, opt ListOpts) (ListResult[Priority], error) {
	r.record("ListPriorities", ctx, opt)
	return ListResult[Priority]{Items: []Priority{{ID: "4", Name: "High"}}}, errCannedRead
}

func (r *recordingReader) ListLabels(ctx context.Context, opt ListOpts) (ListResult[string], error) {
	r.record("ListLabels", ctx, opt)
	return ListResult[string]{Items: []string{"backend"}}, errCannedRead
}

func (r *recordingReader) ListCreateFields(ctx context.Context, opt CreateFieldsOpts) (ListResult[FieldMeta], error) {
	r.record("ListCreateFields", ctx, opt)
	return ListResult[FieldMeta]{Items: []FieldMeta{{ID: "customfield_1", Name: "Team"}}}, errCannedRead
}

func (r *recordingReader) ListComments(ctx context.Context, opt ListCommentsOpts) (ListResult[Comment], error) {
	r.record("ListComments", ctx, opt)
	return ListResult[Comment]{Items: []Comment{{ID: "10", Body: "hi"}}}, errCannedRead
}

func (r *recordingReader) ListWorklogs(ctx context.Context, opt ListWorklogsOpts) (ListResult[Worklog], error) {
	r.record("ListWorklogs", ctx, opt)
	return ListResult[Worklog]{Items: []Worklog{{ID: "20", IssueKey: "ENG-1"}}}, errCannedRead
}

func (r *recordingReader) ResolveUser(ctx context.Context, selector string) (*User, error) {
	r.record("ResolveUser", ctx, selector)
	return &User{Username: "alice"}, errCannedRead
}

func (r *recordingReader) DescribeWrite(ctx context.Context, op any) (WriteRequestPlan, error) {
	r.record("DescribeWrite", ctx, op)
	return WriteRequestPlan{Method: "DELETE", URL: "https://jira.example.test/rest/api/3/issue/ENG-1/comment/10"}, errCannedRead
}

type delegationKey struct{}

// TestReadOnlyDelegatesEveryRead calls every Reader method through the
// wrapper and asserts the inner client received exactly that one call with the
// same arguments, and that its results and error came back unchanged.
func TestReadOnlyDelegatesEveryRead(t *testing.T) {
	t.Parallel()
	ctx := context.WithValue(context.Background(), delegationKey{}, "read-only delegation")
	page := ListOpts{Limit: 7, Cursor: "c1"}
	items := ProjectItemsOpts{ListOpts: page, ProjectKey: "ENG"}

	cases := map[string][]any{
		"Flavor":                nil,
		"BaseURL":               nil,
		"Ping":                  {ctx},
		"CurrentUser":           {ctx},
		"GetIssue":              {ctx, GetIssueOpts{Key: "ENG-1", Expand: "changelog"}},
		"SearchIssues":          {ctx, SearchOpts{ListOpts: page, JQL: "project = ENG", Fields: []string{"summary"}}},
		"ListTransitions":       {ctx, "ENG-1"},
		"ListProjects":          {ctx, ProjectListOpts{ListOpts: page, Query: "eng"}},
		"GetProject":            {ctx, "ENG"},
		"ListComponents":        {ctx, items},
		"ListVersions":          {ctx, items},
		"ListProjectIssueTypes": {ctx, items},
		"ListProjectStatuses":   {ctx, "ENG"},
		"ListPriorities":        {ctx, page},
		"ListLabels":            {ctx, page},
		"ListCreateFields":      {ctx, CreateFieldsOpts{ListOpts: page, ProjectKey: "ENG", IssueTypeID: "3"}},
		"ListComments":          {ctx, ListCommentsOpts{ListOpts: page, IssueKey: "ENG-1"}},
		"ListWorklogs":          {ctx, ListWorklogsOpts{ListOpts: page, IssueKey: "ENG-1"}},
		"ResolveUser":           {ctx, "alice"},
		"DescribeWrite":         {ctx, DeleteCommentReq{IssueKey: "ENG-1", ID: "10"}},
	}
	assertCoversMethods(t, reflect.TypeFor[Reader](), slices.Collect(maps.Keys(cases)))

	call := func(c Client, method string, args []any) []any {
		in := make([]reflect.Value, len(args))
		for i, a := range args {
			in[i] = reflect.ValueOf(a)
		}
		var out []any
		for _, v := range reflect.ValueOf(c).MethodByName(method).Call(in) {
			out = append(out, v.Interface())
		}
		return out
	}

	for method, args := range cases {
		t.Run(method, func(t *testing.T) {
			t.Parallel()
			inner := &recordingReader{}
			got := call(NewReadOnly(inner), method, args)
			if want := call(&recordingReader{}, method, args); !reflect.DeepEqual(got, want) {
				t.Errorf("results = %#v, want the inner client's %#v", got, want)
			}
			want := []readCall{{method: method, args: args}}
			if !reflect.DeepEqual(inner.calls, want) {
				t.Errorf("inner client received %#v, want %#v", inner.calls, want)
			}
		})
	}
}

// TestReadOnlyAllowsDescribeWrite verifies that --dry-run (DescribeWrite) is
// not blocked by the wrapper, even though the underlying op is a write.
func TestReadOnlyAllowsDescribeWrite(t *testing.T) {
	t.Parallel()
	c := newReadOnlyTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		t.Errorf("DescribeWrite should not send HTTP: %s %s", r.Method, r.URL.Path)
	}))
	plan, err := c.DescribeWrite(context.Background(), DeleteCommentReq{IssueKey: "ENG-1", ID: "42"})
	if err != nil {
		t.Fatalf("DescribeWrite under read-only failed: %v", err)
	}
	if plan.Method != "DELETE" {
		t.Errorf("plan.Method = %q, want DELETE", plan.Method)
	}
	if !strings.Contains(plan.URL, "/issue/ENG-1/comment/42") {
		t.Errorf("plan.URL = %q, want substring /issue/ENG-1/comment/42", plan.URL)
	}
}

// TestReadOnlyAllowsReads verifies a representative read passes through the
// wrapper to the network.
func TestReadOnlyAllowsReads(t *testing.T) {
	t.Parallel()
	hit := false
	c := newReadOnlyTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		hit = true
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"id":"1","key":"ENG-1","fields":{"summary":"s"}}`))
	}))
	if _, err := c.GetIssue(context.Background(), GetIssueOpts{Key: "ENG-1"}); err != nil {
		t.Fatalf("GetIssue through read-only wrapper failed: %v", err)
	}
	if !hit {
		t.Error("GetIssue never reached the server")
	}
}
