package app

import (
	"context"
	"fmt"
	"os"
	"strings"
	"time"

	"github.com/angelmsger/agent-cli-kit/output"
	"github.com/angelmsger/jira-cli/pkg/apiclient"
	cerrors "github.com/angelmsger/jira-cli/pkg/errors"
	"github.com/spf13/cobra"
)

// worklog_search.go implements the cross-issue worklog query. It is a
// multi-resource event query, so it follows the family contract in
// docs/agent-facing-cli-best-practices.md §3.5: a bounded time window is
// mandatory, the actor is matched on a stable backend identity, each item
// carries its source issue, and coverage gaps are reported rather than hidden.

// worklogSearchFields is the minimal field set the candidate search needs.
// Cloud requires an explicit list, and only the issue keys are used — the
// issues themselves are never emitted.
var worklogSearchFields = []string{"summary"}

func newWorklogSearchCmd(s *appState) *cobra.Command {
	var (
		author  string
		project string
		since   string
		from    string
		to      string
		limit   int
		all     bool
		cursor  string
	)
	cmd := &cobra.Command{
		Use:   "search --since <duration> | --from <instant> [--to <instant>]",
		Short: "Find a user's worklogs across issues within a time window",
		Long: "Find worklogs across issues. A bounded window is required so the query\n" +
			"cannot scan every issue you can see.\n\n" +
			"Candidate issues come from a JQL search on worklogAuthor/worklogDate.\n" +
			"Jira's worklogDate has day granularity and is evaluated in the instance's\n" +
			"timezone, so that clause deliberately over-selects; the exact [--from,\n" +
			"--to) bound is then applied to each worklog's own `started` instant.\n\n" +
			"--limit, --cursor and --all page the candidate issue search, not the\n" +
			"worklogs, so the number of items per page varies. Only issues you can\n" +
			"browse are visible, and Jira withholds worklogs whose visibility is\n" +
			"restricted, so a result is not proof that nothing else exists.",
		Example: "  jira-cli worklog search --since 7d\n" +
			"  jira-cli worklog search --project ENG --since 24h\n" +
			"  jira-cli worklog search --author alice --from 2026-09-01T00:00:00+08:00 --to 2026-09-08T00:00:00+08:00",
		Args: cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			window, err := resolveWorklogWindow(since, from, to, time.Now())
			if err != nil {
				return err
			}
			if !window.set {
				return cerrors.New(cerrors.CategoryUsage, "WORKLOG_TIME_REQUIRED",
					"searching worklogs requires a bounded time window").
					WithHint("Bound the window so the command does not scan every issue you can see.").
					WithNextSteps("jira-cli worklog search --since 7d",
						"jira-cli worklog search --from 2026-09-01 --to 2026-09-08")
			}
			ctx, cancel := cmdContext(s)
			defer cancel()
			client, err := s.newClient(ctx)
			if err != nil {
				return err
			}

			actor, err := resolveWorklogActor(ctx, client, author)
			if err != nil {
				return err
			}
			jql, err := apiclient.BuildJQL(apiclient.JQLParams{
				Project:         project,
				WorklogAuthor:   author,
				WorklogDateFrom: window.from.Format("2006-01-02"),
				WorklogDateTo:   window.to.Format("2006-01-02"),
				OrderBy:         "updated DESC",
			})
			if err != nil {
				return err
			}

			fetch := func(c string) (apiclient.ListResult[apiclient.Issue], error) {
				return client.SearchIssues(ctx, apiclient.SearchOpts{
					ListOpts: apiclient.ListOpts{Limit: limit, Cursor: c},
					JQL:      jql,
					Fields:   worklogSearchFields,
				})
			}
			issues, info, err := collectPage(fetch, cursor, all)
			if err != nil {
				return err
			}

			items, report := collectWorklogsInWindow(ctx, client, issues, window, actor)
			report.emit()
			if report.fatal() {
				return cerrors.Newf(cerrors.CategoryServer, "WORKLOG_SOURCE_FAILED",
					"could not read worklogs for any of the %d candidate issues", len(issues)).
					WithNextSteps("jira-cli worklog list <issue-key>")
			}
			return s.emitList(items, info)
		},
	}
	f := cmd.Flags()
	f.StringVar(&author, "author", "me",
		`only worklogs by this user selector; "me" is the authenticated user`)
	f.StringVar(&project, "project", "", "restrict the candidate issue search to this project key")
	f.StringVar(&since, "since", "", "only worklogs started within this recent duration, such as 24h or 7d")
	f.StringVar(&from, "from", "", "worklogs started at or after this RFC3339 instant or UTC date")
	f.StringVar(&to, "to", "", "worklogs started before this RFC3339 instant or UTC date (defaults to now)")
	addListFlags(cmd, &limit, &all, &cursor)
	return cmd
}

// resolveWorklogActor resolves the --author selector to a stable backend
// identity — a Cloud accountId or a Data Center username. A display name is
// output, never identity evidence, so it is not used for matching.
func resolveWorklogActor(ctx context.Context, client apiclient.Client, selector string) (*apiclient.User, error) {
	if strings.TrimSpace(selector) == "" {
		return nil, nil
	}
	if strings.EqualFold(selector, "me") {
		return client.CurrentUser(ctx)
	}
	return client.ResolveUser(ctx, selector)
}

// worklogSearchReport accumulates the coverage gaps a fan-out can produce, so
// they are reported once on stderr instead of silently shrinking the result.
type worklogSearchReport struct {
	candidates   int
	failedIssues []string
	missingActor int
}

func (r worklogSearchReport) fatal() bool {
	return r.candidates > 0 && len(r.failedIssues) == r.candidates
}

func (r worklogSearchReport) emit() {
	if len(r.failedIssues) > 0 && !r.fatal() {
		output.EmitNotice(os.Stderr, map[string]any{"_notice": map[string]any{
			"worklog_source_failed": map[string]any{
				"code":    "WORKLOG_SOURCE_FAILED",
				"message": fmt.Sprintf("%d of %d candidate issues could not be read and were skipped", len(r.failedIssues), r.candidates),
				"issues":  r.failedIssues,
				"hint":    "Results are incomplete. Read the listed issues directly to cover the gap.",
			},
		}})
	}
	if r.missingActor > 0 {
		output.EmitNotice(os.Stderr, map[string]any{"_notice": map[string]any{
			"worklog_actor_coverage": map[string]any{
				"code":    "WORKLOG_ACTOR_COVERAGE",
				"message": fmt.Sprintf("%d worklogs carry no stable author identity and were excluded", r.missingActor),
				"hint":    "An author without an accountId (Cloud) or username (Data Center) cannot be matched; a display name is not identity evidence.",
			},
		}})
	}
}

// collectWorklogsInWindow fans out over the candidate issues, keeping only the
// worklogs that fall inside the half-open window and belong to the actor.
func collectWorklogsInWindow(
	ctx context.Context,
	client apiclient.Client,
	issues []apiclient.Issue,
	window worklogWindow,
	actor *apiclient.User,
) ([]apiclient.Worklog, worklogSearchReport) {
	report := worklogSearchReport{candidates: len(issues)}
	items := []apiclient.Worklog{}
	flavor := client.Flavor()

	for _, issue := range issues {
		fetch := func(c string) (apiclient.ListResult[apiclient.Worklog], error) {
			return client.ListWorklogs(ctx, apiclient.ListWorklogsOpts{
				ListOpts: apiclient.ListOpts{Cursor: c},
				IssueKey: issue.Key,
			})
		}
		worklogs, _, err := collectPage(fetch, "", true)
		if err != nil {
			report.failedIssues = append(report.failedIssues, issue.Key)
			continue
		}
		for _, w := range worklogs {
			started, perr := parseWindowInstant(w.Started)
			if perr != nil || !window.contains(started) {
				continue
			}
			if actor != nil {
				if !hasStableUserID(flavor, w.Author) {
					report.missingActor++
					continue
				}
				if !sameStableUser(actor, w.Author, flavor) {
					continue
				}
			}
			items = append(items, w)
		}
	}
	return items, report
}

// hasStableUserID reports whether a user carries the identifier this flavor
// treats as stable.
func hasStableUserID(flavor apiclient.Flavor, u *apiclient.User) bool {
	if u == nil {
		return false
	}
	if flavor == apiclient.FlavorCloud {
		return u.AccountID != ""
	}
	return u.Username != ""
}

// sameStableUser compares two users by stable identity only.
func sameStableUser(left, right *apiclient.User, flavor apiclient.Flavor) bool {
	if left == nil || right == nil {
		return false
	}
	if flavor == apiclient.FlavorCloud {
		return left.AccountID != "" && right.AccountID != "" &&
			strings.EqualFold(left.AccountID, right.AccountID)
	}
	return left.Username != "" && right.Username != "" &&
		strings.EqualFold(left.Username, right.Username)
}
