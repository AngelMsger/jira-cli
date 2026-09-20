package app

import (
	"context"
	"errors"
	"strings"
	"testing"

	"github.com/angelmsger/jira-cli/pkg/apiclient"
	cerrors "github.com/angelmsger/jira-cli/pkg/errors"
)

// stubWorklogClient serves canned worklogs per issue. It embeds the Client
// interface so only the methods the fan-out actually calls need implementing;
// anything else would panic loudly rather than silently returning a zero value.
type stubWorklogClient struct {
	apiclient.Client
	flavor   apiclient.Flavor
	byIssue  map[string][]apiclient.Worklog
	failures map[string]bool
}

func (s stubWorklogClient) Flavor() apiclient.Flavor { return s.flavor }

func (s stubWorklogClient) ListWorklogs(_ context.Context, opt apiclient.ListWorklogsOpts) (apiclient.ListResult[apiclient.Worklog], error) {
	if s.failures[opt.IssueKey] {
		return apiclient.ListResult[apiclient.Worklog]{}, errors.New("boom")
	}
	return apiclient.ListResult[apiclient.Worklog]{Items: s.byIssue[opt.IssueKey]}, nil
}

func worklog(id, issue, started, accountID, username string) apiclient.Worklog {
	return apiclient.Worklog{
		ID: id, IssueKey: issue, Started: started,
		Author: &apiclient.User{AccountID: accountID, Username: username, DisplayName: "Someone"},
	}
}

// TestCollectWorklogsInWindow covers the fan-out: the half-open instant filter
// that narrows Jira's day-granular worklogDate prefilter, stable-identity actor
// matching, the coverage count for authors that cannot be matched, and
// per-issue failures being skipped rather than aborting the run.
func TestCollectWorklogsInWindow(t *testing.T) {
	t.Parallel()
	window, err := resolveWorklogWindow("", "2026-09-01", "2026-09-08", fixedNow)
	if err != nil {
		t.Fatalf("resolveWorklogWindow: %v", err)
	}
	issues := []apiclient.Issue{{Key: "ENG-1"}, {Key: "ENG-2"}, {Key: "ENG-3"}}

	t.Run("filters by window and stable identity", func(t *testing.T) {
		t.Parallel()
		client := stubWorklogClient{
			flavor: apiclient.FlavorCloud,
			byIssue: map[string][]apiclient.Worklog{
				"ENG-1": {
					worklog("1", "ENG-1", "2026-09-02T10:00:00.000+0000", "me", ""),
					worklog("2", "ENG-1", "2026-08-31T10:00:00.000+0000", "me", ""), // before the window
				},
				"ENG-2": {
					worklog("3", "ENG-2", "2026-09-08T00:00:00.000+0000", "me", ""),    // `to` is exclusive
					worklog("4", "ENG-2", "2026-09-07T23:59:59.000+0000", "me", ""),    // last instant inside
					worklog("5", "ENG-2", "2026-09-03T10:00:00.000+0000", "other", ""), // another author
				},
				"ENG-3": {
					worklog("6", "ENG-3", "not-a-timestamp", "me", ""),
				},
			},
		}
		actor := &apiclient.User{AccountID: "me"}
		items, report := collectWorklogsInWindow(context.Background(), client, issues, window, actor)

		var ids []string
		for _, w := range items {
			ids = append(ids, w.ID)
		}
		if got := strings.Join(ids, ","); got != "1,4" {
			t.Errorf("ids = %q, want \"1,4\"", got)
		}
		if report.missingActor != 0 || len(report.failedIssues) != 0 {
			t.Errorf("report = %+v, want no coverage gaps", report)
		}
	})

	t.Run("counts authors without a stable identity", func(t *testing.T) {
		t.Parallel()
		client := stubWorklogClient{
			flavor: apiclient.FlavorCloud,
			byIssue: map[string][]apiclient.Worklog{
				// A display name is output, never identity evidence, so a
				// Cloud worklog without an accountId cannot be matched.
				"ENG-1": {worklog("1", "ENG-1", "2026-09-02T10:00:00.000+0000", "", "fred")},
			},
		}
		items, report := collectWorklogsInWindow(context.Background(), client,
			[]apiclient.Issue{{Key: "ENG-1"}}, window, &apiclient.User{AccountID: "me"})
		if len(items) != 0 {
			t.Errorf("items = %v, want none", items)
		}
		if report.missingActor != 1 {
			t.Errorf("missingActor = %d, want 1 — the gap must be reported, not hidden", report.missingActor)
		}
	})

	t.Run("matches by username on data center", func(t *testing.T) {
		t.Parallel()
		client := stubWorklogClient{
			flavor: apiclient.FlavorDataCenter,
			byIssue: map[string][]apiclient.Worklog{
				"ENG-1": {worklog("1", "ENG-1", "2026-09-02T10:00:00.000+0000", "", "Fred")},
			},
		}
		items, report := collectWorklogsInWindow(context.Background(), client,
			[]apiclient.Issue{{Key: "ENG-1"}}, window, &apiclient.User{Username: "fred"})
		if len(items) != 1 {
			t.Fatalf("items = %v, want the username match (case-insensitive)", items)
		}
		if report.missingActor != 0 {
			t.Errorf("missingActor = %d, want 0", report.missingActor)
		}
	})

	t.Run("skips issues that cannot be read", func(t *testing.T) {
		t.Parallel()
		client := stubWorklogClient{
			flavor:   apiclient.FlavorCloud,
			failures: map[string]bool{"ENG-2": true},
			byIssue: map[string][]apiclient.Worklog{
				"ENG-1": {worklog("1", "ENG-1", "2026-09-02T10:00:00.000+0000", "me", "")},
			},
		}
		items, report := collectWorklogsInWindow(context.Background(), client, issues, window,
			&apiclient.User{AccountID: "me"})
		if len(items) != 1 {
			t.Errorf("items = %v, want the readable issue's worklog", items)
		}
		if len(report.failedIssues) != 1 || report.failedIssues[0] != "ENG-2" {
			t.Errorf("failedIssues = %v, want [ENG-2]", report.failedIssues)
		}
		if report.fatal() {
			t.Error("one failure out of three is not fatal")
		}
	})

	t.Run("every issue failing is fatal", func(t *testing.T) {
		t.Parallel()
		client := stubWorklogClient{
			flavor:   apiclient.FlavorCloud,
			failures: map[string]bool{"ENG-1": true, "ENG-2": true, "ENG-3": true},
		}
		_, report := collectWorklogsInWindow(context.Background(), client, issues, window, nil)
		if !report.fatal() {
			t.Error("all candidates failing must not be reported as an empty result")
		}
	})

	t.Run("no actor keeps every author", func(t *testing.T) {
		t.Parallel()
		client := stubWorklogClient{
			flavor: apiclient.FlavorCloud,
			byIssue: map[string][]apiclient.Worklog{
				"ENG-1": {
					worklog("1", "ENG-1", "2026-09-02T10:00:00.000+0000", "me", ""),
					worklog("2", "ENG-1", "2026-09-03T10:00:00.000+0000", "other", ""),
				},
			},
		}
		items, _ := collectWorklogsInWindow(context.Background(), client,
			[]apiclient.Issue{{Key: "ENG-1"}}, window, nil)
		if len(items) != 2 {
			t.Errorf("items = %d, want both", len(items))
		}
	})
}

// TestWorklogSearchRequiresBoundedWindow enforces the multi-resource contract:
// the window is validated before any client is built, so the rejection needs
// neither credentials nor network.
func TestWorklogSearchRequiresBoundedWindow(t *testing.T) {
	err := runRoot(t, "worklog", "search")
	if err == nil {
		t.Fatal("worklog search without a window should fail")
	}
	ce := cerrors.AsCLIError(err)
	if ce.Code != "WORKLOG_TIME_REQUIRED" {
		t.Fatalf("code = %s, want WORKLOG_TIME_REQUIRED", ce.Code)
	}
	if cerrors.ExitCode(ce) != cerrors.ExitUsage {
		t.Errorf("exit code = %d, want %d", cerrors.ExitCode(ce), cerrors.ExitUsage)
	}
	if len(ce.NextSteps) == 0 || !strings.Contains(strings.Join(ce.NextSteps, " "), "--since") {
		t.Errorf("next_steps = %v, want a bounded-window example", ce.NextSteps)
	}
}

// TestWorklogSearchRejectsAnInvalidWindow keeps the shared window contract
// reachable from the command surface.
func TestWorklogSearchRejectsAnInvalidWindow(t *testing.T) {
	err := runRoot(t, "worklog", "search", "--since", "7d", "--from", "2026-09-01")
	if err == nil {
		t.Fatal("--since with --from should fail")
	}
	if code := cerrors.AsCLIError(err).Code; code != "BAD_TIME_RANGE" {
		t.Errorf("code = %s, want BAD_TIME_RANGE", code)
	}
}

// TestWorklogWriteCommandsRequireTheIssueFlag guards the addressing rule: a
// worklog ID is meaningless without its issue.
func TestWorklogWriteCommandsRequireTheIssueFlag(t *testing.T) {
	for _, verb := range []string{"update", "delete"} {
		t.Run(verb, func(t *testing.T) {
			err := runRoot(t, "worklog", verb, "10042")
			if err == nil {
				t.Fatalf("worklog %s without --issue should fail", verb)
			}
			if !strings.Contains(err.Error(), "issue") {
				t.Errorf("error = %q, want it to name --issue", err)
			}
		})
	}
}
