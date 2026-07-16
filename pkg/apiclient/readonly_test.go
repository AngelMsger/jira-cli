package apiclient

import (
	"context"
	"net/http"
	"net/http/httptest"
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
	}
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
			if !strings.Contains(strings.Join(ce.NextSteps, " "), "--allow-writes") {
				t.Errorf("%s: next_steps missing --allow-writes hint: %v", tc.name, ce.NextSteps)
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
