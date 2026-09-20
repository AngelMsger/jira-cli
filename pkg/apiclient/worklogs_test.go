package apiclient

import (
	"context"
	"encoding/json"
	"net/http"
	"testing"

	cerrors "github.com/angelmsger/jira-cli/pkg/errors"
)

// TestEstimateAdjustValidation pins the --adjust-estimate matrix: which
// companion flag is legal with which mode on which operation. The validation
// lives in the build* helpers, so --dry-run and the live write share it.
func TestEstimateAdjustValidation(t *testing.T) {
	t.Parallel()
	cases := []struct {
		name      string
		op        string
		estimate  EstimateAdjust
		wantCode  string // "" means the combination is valid
		wantQuery string
	}{
		{name: "default sends nothing", op: "add"},
		{name: "auto", op: "add", estimate: EstimateAdjust{Mode: "auto"}, wantQuery: "adjustEstimate=auto"},
		{name: "leave", op: "delete", estimate: EstimateAdjust{Mode: "leave"}, wantQuery: "adjustEstimate=leave"},
		{name: "mode is case-insensitive", op: "add", estimate: EstimateAdjust{Mode: "LEAVE"}, wantQuery: "adjustEstimate=leave"},
		{
			name: "new carries the estimate", op: "update",
			estimate:  EstimateAdjust{Mode: "new", NewEstimate: "2d"},
			wantQuery: "adjustEstimate=new&newEstimate=2d",
		},
		{
			name: "manual reduces on add", op: "add",
			estimate:  EstimateAdjust{Mode: "manual", ReduceBy: "30m"},
			wantQuery: "adjustEstimate=manual&reduceBy=30m",
		},
		{
			name: "manual increases on delete", op: "delete",
			estimate:  EstimateAdjust{Mode: "manual", IncreaseBy: "1h"},
			wantQuery: "adjustEstimate=manual&increaseBy=1h",
		},

		{name: "unknown mode", op: "add", estimate: EstimateAdjust{Mode: "shrink"}, wantCode: "WORKLOG_BAD_ADJUST_ESTIMATE"},
		{name: "new without an estimate", op: "add", estimate: EstimateAdjust{Mode: "new"}, wantCode: "WORKLOG_ESTIMATE_FLAG_MISSING"},
		{name: "manual without a reduction", op: "add", estimate: EstimateAdjust{Mode: "manual"}, wantCode: "WORKLOG_ESTIMATE_FLAG_MISSING"},
		{name: "manual without an increase", op: "delete", estimate: EstimateAdjust{Mode: "manual"}, wantCode: "WORKLOG_ESTIMATE_FLAG_MISSING"},
		{
			// Jira's PUT .../worklog/{id} takes only adjustEstimate+newEstimate.
			name: "manual is unavailable on update", op: "update",
			estimate: EstimateAdjust{Mode: "manual", ReduceBy: "1h"},
			wantCode: "WORKLOG_ESTIMATE_FLAG_UNSUPPORTED",
		},
		{
			name: "companion flag without a mode", op: "add",
			estimate: EstimateAdjust{NewEstimate: "2d"},
			wantCode: "WORKLOG_ESTIMATE_FLAG_UNSUPPORTED",
		},
		{
			name: "reduce-by with the wrong mode", op: "add",
			estimate: EstimateAdjust{Mode: "leave", ReduceBy: "1h"},
			wantCode: "WORKLOG_ESTIMATE_FLAG_UNSUPPORTED",
		},
		{
			name: "increase-by on add", op: "add",
			estimate: EstimateAdjust{Mode: "manual", ReduceBy: "1h", IncreaseBy: "1h"},
			wantCode: "WORKLOG_ESTIMATE_FLAG_UNSUPPORTED",
		},
		{
			name: "malformed estimate duration", op: "add",
			estimate: EstimateAdjust{Mode: "new", NewEstimate: "two days"},
			wantCode: "WORKLOG_BAD_TIME_SPENT",
		},
	}

	for _, tc := range cases {
		tc := tc
		t.Run(tc.op+"/"+tc.name, func(t *testing.T) {
			t.Parallel()
			q, err := estimateQuery(tc.op, tc.estimate)
			if tc.wantCode != "" {
				if err == nil {
					t.Fatalf("estimateQuery(%q, %+v) = %v, want error %s", tc.op, tc.estimate, q, tc.wantCode)
				}
				if got := cerrors.AsCLIError(err).Code; got != tc.wantCode {
					t.Errorf("code = %s, want %s", got, tc.wantCode)
				}
				return
			}
			if err != nil {
				t.Fatalf("estimateQuery: %v", err)
			}
			if got := q.Encode(); got != tc.wantQuery {
				t.Errorf("query = %q, want %q", got, tc.wantQuery)
			}
		})
	}
}

// TestTimeSpentValidation covers the time-quantity contract, including the
// space in Jira's grammar that time.ParseDuration cannot handle.
func TestTimeSpentValidation(t *testing.T) {
	t.Parallel()
	cases := []struct {
		name     string
		spent    string
		seconds  int64
		required bool
		wantCode string
		wantKey  string
		wantVal  any
	}{
		{name: "jira grammar with a space", spent: "3h 20m", required: true, wantKey: "timeSpent", wantVal: "3h 20m"},
		{name: "weeks and days", spent: "1w 2d", required: true, wantKey: "timeSpent", wantVal: "1w 2d"},
		{name: "fractional", spent: "1.5h", required: true, wantKey: "timeSpent", wantVal: "1.5h"},
		{name: "uppercase", spent: "2H", required: true, wantKey: "timeSpent", wantVal: "2H"},
		{name: "seconds", seconds: 1800, required: true, wantKey: "timeSpentSeconds", wantVal: int64(1800)},
		{name: "optional and absent", required: false},

		{name: "missing when required", required: true, wantCode: "WORKLOG_NO_TIME_SPENT"},
		{name: "both given", spent: "1h", seconds: 60, required: true, wantCode: "WORKLOG_TIME_SPENT_CONFLICT"},
		{name: "go duration syntax is rejected", spent: "1h30m0s", required: true, wantCode: "WORKLOG_BAD_TIME_SPENT"},
		{name: "prose", spent: "about an hour", required: true, wantCode: "WORKLOG_BAD_TIME_SPENT"},
		{name: "bare number", spent: "90", required: true, wantCode: "WORKLOG_BAD_TIME_SPENT"},
		{name: "negative seconds", seconds: -60, required: true, wantCode: "WORKLOG_BAD_TIME_SPENT"},
		{name: "negative seconds when optional", seconds: -60, wantCode: "WORKLOG_BAD_TIME_SPENT"},
	}
	for _, tc := range cases {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			payload := map[string]any{}
			err := timeSpentPayload(tc.spent, tc.seconds, tc.required, payload)
			if tc.wantCode != "" {
				if err == nil {
					t.Fatalf("timeSpentPayload = nil, want %s", tc.wantCode)
				}
				if got := cerrors.AsCLIError(err).Code; got != tc.wantCode {
					t.Errorf("code = %s, want %s", got, tc.wantCode)
				}
				return
			}
			if err != nil {
				t.Fatalf("timeSpentPayload: %v", err)
			}
			if tc.wantKey == "" {
				if len(payload) != 0 {
					t.Errorf("payload = %v, want empty", payload)
				}
				return
			}
			if got := payload[tc.wantKey]; got != tc.wantVal {
				t.Errorf("payload[%q] = %v, want %v", tc.wantKey, got, tc.wantVal)
			}
		})
	}
}

// TestEstimateCompanionErrorIsDeterministic guards against reporting a
// different flag on each run when several illegal companions are supplied.
func TestEstimateCompanionErrorIsDeterministic(t *testing.T) {
	t.Parallel()
	e := EstimateAdjust{NewEstimate: "2d", ReduceBy: "1h", IncreaseBy: "1h"}
	var first string
	for i := 0; i < 20; i++ {
		_, err := estimateQuery("add", e)
		if err == nil {
			t.Fatal("estimateQuery = nil, want an error")
		}
		msg := err.Error()
		if i == 0 {
			first = msg
			continue
		}
		if msg != first {
			t.Fatalf("message varies between runs: %q then %q", first, msg)
		}
	}
}

// TestUpdateWorklogRequiresAChange keeps a no-op PUT from reaching Jira, which
// rejects an empty worklog update anyway.
func TestUpdateWorklogRequiresAChange(t *testing.T) {
	t.Parallel()
	c := newTestClient(t, FlavorDataCenter, silentHandler(t))
	_, err := c.DescribeWrite(context.Background(), UpdateWorklogReq{IssueKey: "ENG-1", ID: "7"})
	if err == nil {
		t.Fatal("DescribeWrite = nil, want WORKLOG_NO_UPDATE_FIELDS")
	}
	if got := cerrors.AsCLIError(err).Code; got != "WORKLOG_NO_UPDATE_FIELDS" {
		t.Errorf("code = %s, want WORKLOG_NO_UPDATE_FIELDS", got)
	}
}

// TestWorklogIDErrorNamesItsDiscoveryCommand enforces the no-dead-end rule: a
// missing worklog ID must point at the command that lists them.
func TestWorklogIDErrorNamesItsDiscoveryCommand(t *testing.T) {
	t.Parallel()
	c := newTestClient(t, FlavorCloud, silentHandler(t))
	_, err := c.DescribeWrite(context.Background(), UpdateWorklogReq{IssueKey: "ENG-1", TimeSpent: "1h"})
	ce := cerrors.AsCLIError(err)
	if ce.Code != "WORKLOG_NO_ID" {
		t.Fatalf("code = %s, want WORKLOG_NO_ID", ce.Code)
	}
	found := false
	for _, step := range ce.NextSteps {
		if step == "jira-cli worklog list <issue-key> --limit 25" {
			found = true
		}
	}
	if !found {
		t.Errorf("next_steps = %v, want the worklog list discovery command", ce.NextSteps)
	}
}

// TestListWorklogsNormalizesBothFlavors covers the read path: offset
// pagination, the issue key stamped onto every item, and the ADF-vs-plain
// comment asymmetry collapsing to the same plain text.
func TestListWorklogsNormalizesBothFlavors(t *testing.T) {
	t.Parallel()
	for _, flavor := range []Flavor{FlavorCloud, FlavorDataCenter} {
		flavor := flavor
		t.Run(string(flavor), func(t *testing.T) {
			t.Parallel()
			comment := json.RawMessage(`"I did some work here."`)
			if flavor == FlavorCloud {
				comment, _ = json.Marshal(TextToADF("I did some work here."))
			}
			handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				if got := r.URL.Query().Get("startAt"); got != "0" {
					t.Errorf("startAt = %q, want 0", got)
				}
				body := map[string]any{
					"startAt": 0, "maxResults": 2, "total": 4,
					"worklogs": []map[string]any{
						{
							"id": "100028", "started": "2026-09-19T09:00:00.000+0800",
							"timeSpent": "3h 20m", "timeSpentSeconds": 12000,
							"comment": comment,
							"author":  map[string]any{"accountId": "abc", "name": "fred", "displayName": "Fred"},
						},
						{"id": "100029", "timeSpentSeconds": 600},
					},
				}
				w.Header().Set("Content-Type", "application/json")
				_ = json.NewEncoder(w).Encode(body)
			})
			c := newTestClient(t, flavor, handler)
			res, err := c.ListWorklogs(context.Background(), ListWorklogsOpts{
				ListOpts: ListOpts{Limit: 2}, IssueKey: "ENG-1",
			})
			if err != nil {
				t.Fatalf("ListWorklogs: %v", err)
			}
			if len(res.Items) != 2 {
				t.Fatalf("items = %d, want 2", len(res.Items))
			}
			if res.Next != "2" {
				t.Errorf("next = %q, want 2 (4 total, page size 2)", res.Next)
			}
			got := res.Items[0]
			if got.IssueKey != "ENG-1" {
				t.Errorf("issue_key = %q, want the source context on every item", got.IssueKey)
			}
			if got.Comment != "I did some work here." {
				t.Errorf("comment = %q, want it normalized to plain text on both flavors", got.Comment)
			}
			if got.TimeSpent != "3h 20m" || got.TimeSpentSeconds != 12000 {
				t.Errorf("time = %q/%d, want the human and machine pair preserved", got.TimeSpent, got.TimeSpentSeconds)
			}
			if got.Author == nil {
				t.Fatal("author dropped")
			}
		})
	}
}

// TestListWorklogsRequiresAnIssueKey guards the library entry point.
func TestListWorklogsRequiresAnIssueKey(t *testing.T) {
	t.Parallel()
	c := newTestClient(t, FlavorCloud, silentHandler(t))
	if _, err := c.ListWorklogs(context.Background(), ListWorklogsOpts{}); err == nil {
		t.Fatal("ListWorklogs = nil, want ISSUE_NO_KEY")
	}
}
