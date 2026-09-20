package app

import (
	"time"

	"github.com/angelmsger/jira-cli/pkg/apiclient"
	"github.com/spf13/cobra"
)

// worklogFlags holds the inputs shared by the worklog write commands.
type worklogFlags struct {
	timeSpent   string
	timeSeconds int64
	started     string
	comment     string
	commentFile string
	estimate    apiclient.EstimateAdjust
	dryRun      bool
}

// addEstimateFlags registers the remaining-estimate controls. increase reports
// whether the operation takes --increase-by (delete) or --reduce-by (add);
// update takes neither, because Jira's update endpoint has no manual mode.
func addEstimateFlags(cmd *cobra.Command, e *apiclient.EstimateAdjust, reduce, increase bool) {
	f := cmd.Flags()
	f.StringVar(&e.Mode, "adjust-estimate", "",
		`how to adjust the remaining estimate: auto, new, leave or manual (default "auto")`)
	f.StringVar(&e.NewEstimate, "new-estimate", "",
		`remaining estimate to set, e.g. "2d"; required with --adjust-estimate new`)
	if reduce {
		f.StringVar(&e.ReduceBy, "reduce-by", "",
			"reduce the remaining estimate by this amount; required with --adjust-estimate manual")
	}
	if increase {
		f.StringVar(&e.IncreaseBy, "increase-by", "",
			"increase the remaining estimate by this amount; required with --adjust-estimate manual")
	}
}

// addWorklogWriteFlags registers the inputs shared by `add` and `update`.
// required marks the `add` case, where a time quantity must be supplied.
func addWorklogWriteFlags(cmd *cobra.Command, w *worklogFlags, required bool) {
	f := cmd.Flags()
	usage := `time spent in Jira's own units, e.g. "3h 20m" or "1d 4h"`
	if required {
		usage += " (required unless --time-spent-seconds is given)"
	}
	f.StringVar(&w.timeSpent, "time-spent", "", usage)
	f.Int64Var(&w.timeSeconds, "time-spent-seconds", 0,
		"time spent in seconds (mutually exclusive with --time-spent)")
	f.StringVar(&w.started, "started", "",
		`when the work started: "now", an RFC3339 instant, a local wall clock `+
			`("2006-01-02T15:04:05") or a date ("2006-01-02", UTC); defaults to now`)
	f.StringVar(&w.comment, "comment", "", "worklog comment text")
	f.StringVar(&w.commentFile, "comment-file", "", "read the worklog comment from a file ('-' for stdin)")
	f.BoolVar(&w.dryRun, "dry-run", false, "preview the HTTP request without sending it")
}

// resolveComment reads the worklog comment only when one of its flags was
// given; the comment is optional on every worklog write.
func resolveComment(cmd *cobra.Command, w *worklogFlags) (string, error) {
	if !cmd.Flags().Changed("comment") && !cmd.Flags().Changed("comment-file") {
		return "", nil
	}
	return readBodyText(w.comment, w.commentFile, "WORKLOG_NO_COMMENT")
}

func newWorklogCmd(s *appState) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "worklog",
		Short: "Read and log work on issues (time tracking)",
		Long: "Read and log work on issues. Worklog IDs come from `worklog list`;\n" +
			"`worklog search` finds a user's entries across issues within a\n" +
			"bounded time window.",
		Aliases: []string{"worklogs"},
	}
	cmd.AddCommand(
		newWorklogListCmd(s), newWorklogSearchCmd(s), newWorklogAddCmd(s),
		newWorklogUpdateCmd(s), newWorklogDeleteCmd(s),
	)
	return cmd
}

func newWorklogListCmd(s *appState) *cobra.Command {
	var (
		limit  int
		all    bool
		cursor string
	)
	cmd := &cobra.Command{
		Use:   "list <issue-key|url>",
		Short: "List an issue's worklogs (oldest first)",
		Example: "  jira-cli worklog list PROJ-123\n" +
			"  jira-cli worklog list PROJ-123 --all",
		Args: cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			key, err := resolveIssueKey(args[0])
			if err != nil {
				return err
			}
			ctx, cancel := cmdContext(s)
			defer cancel()
			client, err := s.newClient(ctx)
			if err != nil {
				return err
			}
			fetch := func(c string) (apiclient.ListResult[apiclient.Worklog], error) {
				return client.ListWorklogs(ctx, apiclient.ListWorklogsOpts{
					ListOpts: apiclient.ListOpts{Limit: limit, Cursor: c},
					IssueKey: key,
				})
			}
			items, info, err := collectPage(fetch, cursor, all)
			if err != nil {
				return err
			}
			return s.emitList(items, info)
		},
	}
	addListFlags(cmd, &limit, &all, &cursor)
	return cmd
}

func newWorklogAddCmd(s *appState) *cobra.Command {
	var w worklogFlags
	cmd := &cobra.Command{
		Use:   "add <issue-key|url> --time-spent <duration>",
		Short: "Log work on an issue",
		Long: "Log work on an issue. --time-spent takes Jira's own units and is sent\n" +
			"verbatim, because only the server knows the instance's hours-per-day and\n" +
			"days-per-week settings. The comment is plain text on both flavors: on\n" +
			"Cloud it becomes ADF paragraphs, on Data Center it is sent as given.\n" +
			"By default Jira subtracts the logged time from the remaining estimate;\n" +
			"--adjust-estimate changes that.",
		Example: "  jira-cli worklog add PROJ-123 --time-spent \"3h 20m\" --comment \"Traced the retry loop\"\n" +
			"  jira-cli worklog add PROJ-123 --time-spent 45m --started 2026-09-19T09:00:00\n" +
			"  jira-cli worklog add PROJ-123 --time-spent 2h --adjust-estimate new --new-estimate 1d",
		Args: cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			key, err := resolveIssueKey(args[0])
			if err != nil {
				return err
			}
			started, err := resolveStarted(w.started, time.Time{})
			if err != nil {
				return err
			}
			comment, err := resolveComment(cmd, &w)
			if err != nil {
				return err
			}
			req := apiclient.AddWorklogReq{
				IssueKey:         key,
				TimeSpent:        w.timeSpent,
				TimeSpentSeconds: w.timeSeconds,
				Started:          started,
				Comment:          comment,
				Estimate:         w.estimate,
			}
			ctx, cancel := cmdContext(s)
			defer cancel()
			client, err := s.newClient(ctx)
			if err != nil {
				return err
			}
			if w.dryRun {
				return emitDryRun(s, client, ctx, req)
			}
			created, err := client.AddWorklog(ctx, req)
			if err != nil {
				return err
			}
			return s.emit(created)
		},
	}
	addWorklogWriteFlags(cmd, &w, true)
	addEstimateFlags(cmd, &w.estimate, true, false)
	return cmd
}

func newWorklogUpdateCmd(s *appState) *cobra.Command {
	var (
		w     worklogFlags
		issue string
	)
	cmd := &cobra.Command{
		Use:   "update <worklog-id> --issue <key>",
		Short: "Change an existing worklog",
		Long: "Change a worklog's time, start instant or comment. Unset flags are left\n" +
			"as they are. Jira's worklog update endpoint has no manual estimate mode,\n" +
			"so --adjust-estimate accepts auto, new and leave only.",
		Example: "  jira-cli worklog update 10042 --issue PROJ-123 --time-spent 2h\n" +
			"  jira-cli worklog update 10042 --issue PROJ-123 --comment \"Corrected the note\"",
		Aliases: []string{"edit"},
		Args:    cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			key, err := resolveIssueKey(issue)
			if err != nil {
				return err
			}
			started, err := resolveStarted(w.started, time.Time{})
			if err != nil {
				return err
			}
			comment, err := resolveComment(cmd, &w)
			if err != nil {
				return err
			}
			req := apiclient.UpdateWorklogReq{
				IssueKey:         key,
				ID:               args[0],
				TimeSpent:        w.timeSpent,
				TimeSpentSeconds: w.timeSeconds,
				Started:          started,
				Comment:          comment,
				Estimate:         w.estimate,
			}
			ctx, cancel := cmdContext(s)
			defer cancel()
			client, err := s.newClient(ctx)
			if err != nil {
				return err
			}
			if w.dryRun {
				return emitDryRun(s, client, ctx, req)
			}
			updated, err := client.UpdateWorklog(ctx, req)
			if err != nil {
				return err
			}
			return s.emit(updated)
		},
	}
	addWorklogWriteFlags(cmd, &w, false)
	addEstimateFlags(cmd, &w.estimate, false, false)
	cmd.Flags().StringVar(&issue, "issue", "", "the issue the worklog belongs to (required)")
	_ = cmd.MarkFlagRequired("issue")
	return cmd
}

func newWorklogDeleteCmd(s *appState) *cobra.Command {
	var (
		issue    string
		yes      bool
		dryRun   bool
		estimate apiclient.EstimateAdjust
	)
	cmd := &cobra.Command{
		Use:   "delete <worklog-id>... --issue <key>",
		Short: "Delete one or more worklogs",
		Long: "Delete a worklog by ID. Pass several IDs to delete them in one run, or\n" +
			"a single '-' to read newline-separated IDs from stdin. Deletion requires\n" +
			"--yes (or an interactive confirmation when stdin is a terminal); --yes\n" +
			"applies to the whole batch. By default Jira adds the deleted time back\n" +
			"to the remaining estimate; --adjust-estimate changes that.",
		Example: "  jira-cli worklog delete 10042 --issue PROJ-123 --yes\n" +
			"  jira-cli worklog delete 10042 10043 --issue PROJ-123 --yes --adjust-estimate leave",
		Aliases: []string{"remove", "rm"},
		Args:    cobra.MinimumNArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			key, err := resolveIssueKey(issue)
			if err != nil {
				return err
			}
			items, err := collectBatchArgs(args, cmd.InOrStdin())
			if err != nil {
				return err
			}
			ctx, cancel := cmdContext(s)
			defer cancel()
			client, err := s.newClient(ctx)
			if err != nil {
				return err
			}
			if !dryRun {
				if cerr := confirmDelete(deletePrompt("worklog", items, "on "+key), yes); cerr != nil {
					return cerr
				}
			}
			return runBatch(s, items, func(arg string) (any, error) {
				req := apiclient.DeleteWorklogReq{IssueKey: key, ID: arg, Estimate: estimate}
				if dryRun {
					return dryRunPlan(client, ctx, req)
				}
				if derr := client.DeleteWorklog(ctx, req); derr != nil {
					return nil, derr
				}
				return map[string]any{"id": arg, "issue": key, "status": "deleted"}, nil
			})
		},
	}
	f := cmd.Flags()
	f.StringVar(&issue, "issue", "", "the issue the worklogs belong to (required)")
	f.BoolVar(&yes, "yes", false, "skip the deletion confirmation")
	f.BoolVar(&dryRun, "dry-run", false, "preview the HTTP request without sending it")
	addEstimateFlags(cmd, &estimate, false, true)
	_ = cmd.MarkFlagRequired("issue")
	return cmd
}
