package app

import (
	"context"
	"strings"

	"github.com/angelmsger/jira-cli/pkg/apiclient"
	cerrors "github.com/angelmsger/jira-cli/pkg/errors"
	"github.com/spf13/cobra"
)

func newIssueCmd(s *appState) *cobra.Command {
	cmd := &cobra.Command{
		Use:     "issue",
		Short:   "Read, search and write Jira issues",
		Aliases: []string{"issues"},
	}
	cmd.AddCommand(
		newIssueGetCmd(s), newIssueSearchCmd(s),
		newIssueCreateCmd(s), newIssueEditCmd(s), newIssueAssignCmd(s),
		newIssueTransitionsCmd(s), newIssueTransitionCmd(s),
	)
	return cmd
}

func newIssueGetCmd(s *appState) *cobra.Command {
	var expand string
	cmd := &cobra.Command{
		Use:   "get <key|url>",
		Short: "Show one issue",
		Example: "  jira-cli issue get PROJ-123\n" +
			"  jira-cli issue get https://acme.atlassian.net/browse/PROJ-123",
		Aliases: []string{"view", "show"},
		Args:    cobra.ExactArgs(1),
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
			issue, err := client.GetIssue(ctx, apiclient.GetIssueOpts{Key: key, Expand: expand})
			if err != nil {
				return err
			}
			return s.emit(issue)
		},
	}
	cmd.Flags().StringVar(&expand, "expand", "", "extra sections to request, comma-separated (e.g. changelog)")
	return cmd
}

func newIssueSearchCmd(s *appState) *cobra.Command {
	var (
		params apiclient.JQLParams
		fields []string
		limit  int
		all    bool
		cursor string
	)
	cmd := &cobra.Command{
		Use:   "search [jql]",
		Short: "Search issues with JQL or filter flags",
		Long: "Search issues. Pass a raw JQL string, or compose one from filter flags\n" +
			"(--project, --assignee, --status, --type, --label, --text; AND-joined).\n" +
			"--assignee/--reporter accept \"me\" (the authenticated user) and\n" +
			"\"unassigned\".",
		Example: "  jira-cli issue search 'project = ENG AND status = \"In Progress\"'\n" +
			"  jira-cli issue search --project ENG --assignee me --order-by \"updated DESC\"\n" +
			"  jira-cli issue search --text \"login crash\" --all",
		Args: cobra.MaximumNArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			var jql string
			switch {
			case len(args) == 1 && args[0] != "":
				if !params.IsEmpty() {
					return cerrors.New(cerrors.CategoryUsage, "JQL_CONFLICT",
						"pass either a raw JQL string or filter flags, not both")
				}
				jql = args[0]
			default:
				// Fall back to the configured default project when the only
				// missing filter is the scope.
				if params.IsEmpty() && s.cfg().Defaults.Project != "" {
					params.Project = s.cfg().Defaults.Project
				}
				built, err := apiclient.BuildJQL(params)
				if err != nil {
					return err
				}
				jql = built
			}
			ctx, cancel := cmdContext(s)
			defer cancel()
			client, err := s.newClient(ctx)
			if err != nil {
				return err
			}
			fetch := func(c string) (apiclient.ListResult[apiclient.Issue], error) {
				return client.SearchIssues(ctx, apiclient.SearchOpts{
					ListOpts: apiclient.ListOpts{Limit: limit, Cursor: c},
					JQL:      jql,
					Fields:   fields,
				})
			}
			items, info, err := collectPage(fetch, cursor, all)
			if err != nil {
				return err
			}
			return s.emitList(items, info)
		},
	}
	f := cmd.Flags()
	f.StringVar(&params.Project, "project", "", "filter by project key")
	f.StringVar(&params.Assignee, "assignee", "", `filter by assignee ("me", "unassigned", or a user)`)
	f.StringVar(&params.Reporter, "reporter", "", `filter by reporter ("me" or a user)`)
	f.StringVar(&params.Status, "status", "", "filter by status name")
	f.StringVar(&params.Type, "type", "", "filter by issue type name")
	f.StringVar(&params.Label, "label", "", "filter by label")
	f.StringVar(&params.Text, "text", "", "free-text match")
	f.StringVar(&params.OrderBy, "order-by", "", `sort clause, e.g. "updated DESC"`)
	f.StringSliceVar(&fields, "field", nil, "issue fields to return (repeatable; default is a curated set)")
	addListFlags(cmd, &limit, &all, &cursor)
	return cmd
}

func newIssueCreateCmd(s *appState) *cobra.Command {
	var (
		req      apiclient.CreateIssueReq
		descFile string
		dryRun   bool
	)
	cmd := &cobra.Command{
		Use:   "create",
		Short: "Create an issue",
		Long: "Create an issue. --project falls back to defaults.project when\n" +
			"configured. Descriptions are plain text on both flavors; on Cloud the\n" +
			"text becomes ADF paragraphs, on Data Center it is sent verbatim (wiki\n" +
			"markup is rendered server-side).",
		Example: "  jira-cli issue create --project ENG --type Task --summary \"Fix login crash\"\n" +
			"  jira-cli issue create --project ENG --type Bug --summary \"...\" \\\n" +
			"      --description-file report.txt --assignee alice@example.com --label urgent",
		Args: cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			if req.ProjectKey == "" {
				req.ProjectKey = s.cfg().Defaults.Project
			}
			if descFile != "" {
				text, err := readBodyText(req.Description, descFile, "ISSUE_NO_DESCRIPTION")
				if err != nil {
					return err
				}
				req.Description = text
			}
			ctx, cancel := cmdContext(s)
			defer cancel()
			client, err := s.newClient(ctx)
			if err != nil {
				return err
			}
			if dryRun {
				return emitDryRun(s, client, ctx, req)
			}
			issue, err := client.CreateIssue(ctx, req)
			if err != nil {
				return err
			}
			return s.emit(issue)
		},
	}
	f := cmd.Flags()
	f.StringVar(&req.ProjectKey, "project", "", "project key (default from defaults.project)")
	f.StringVar(&req.Type, "type", "Task", "issue type name (e.g. Task, Bug, Story)")
	f.StringVar(&req.Summary, "summary", "", "issue summary (required)")
	f.StringVar(&req.Description, "description", "", "description text")
	f.StringVar(&descFile, "description-file", "", "read the description from a file ('-' for stdin)")
	f.StringVar(&req.Assignee, "assignee", "", "assignee (accountId, username, or display-name/email query)")
	f.StringVar(&req.Priority, "priority", "", "priority name (e.g. High)")
	f.StringSliceVar(&req.Labels, "label", nil, "label to set (repeatable)")
	f.StringVar(&req.ParentKey, "parent", "", "parent issue key (for subtasks / epic children)")
	f.BoolVar(&dryRun, "dry-run", false, "preview the HTTP request without sending it")
	return cmd
}

func newIssueEditCmd(s *appState) *cobra.Command {
	var (
		summary      string
		description  string
		descFile     string
		priority     string
		addLabels    []string
		removeLabels []string
		dryRun       bool
	)
	cmd := &cobra.Command{
		Use:   "edit <key|url>",
		Short: "Update issue fields",
		Long: "Update an issue's summary, description, priority or labels. Only the\n" +
			"flags you pass change; everything else keeps its value.",
		Example: "  jira-cli issue edit PROJ-123 --summary \"New title\"\n" +
			"  jira-cli issue edit PROJ-123 --add-label triaged --remove-label urgent",
		Aliases: []string{"update"},
		Args:    cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			key, err := resolveIssueKey(args[0])
			if err != nil {
				return err
			}
			req := apiclient.EditIssueReq{
				Key:          key,
				Priority:     priority,
				AddLabels:    addLabels,
				RemoveLabels: removeLabels,
			}
			if cmd.Flags().Changed("summary") {
				req.Summary = &summary
			}
			if cmd.Flags().Changed("description") || cmd.Flags().Changed("description-file") {
				text, err := readBodyText(description, descFile, "ISSUE_NO_DESCRIPTION")
				if err != nil {
					return err
				}
				req.Description = &text
			}
			ctx, cancel := cmdContext(s)
			defer cancel()
			client, err := s.newClient(ctx)
			if err != nil {
				return err
			}
			if dryRun {
				return emitDryRun(s, client, ctx, req)
			}
			issue, err := client.EditIssue(ctx, req)
			if err != nil {
				return err
			}
			return s.emit(issue)
		},
	}
	f := cmd.Flags()
	f.StringVar(&summary, "summary", "", "new summary")
	f.StringVar(&description, "description", "", "new description text")
	f.StringVar(&descFile, "description-file", "", "read the new description from a file ('-' for stdin)")
	f.StringVar(&priority, "priority", "", "new priority name")
	f.StringSliceVar(&addLabels, "add-label", nil, "label to add (repeatable)")
	f.StringSliceVar(&removeLabels, "remove-label", nil, "label to remove (repeatable)")
	f.BoolVar(&dryRun, "dry-run", false, "preview the HTTP request without sending it")
	return cmd
}

func newIssueAssignCmd(s *appState) *cobra.Command {
	var (
		to       string
		unassign bool
		dryRun   bool
	)
	cmd := &cobra.Command{
		Use:   "assign <key|url>",
		Short: "Change or clear an issue's assignee",
		Long: "Assign an issue. --to accepts a Cloud accountId, a Data Center\n" +
			"username, or a display-name/email query resolved to a unique user\n" +
			"(see `jira-cli user resolve`).",
		Example: "  jira-cli issue assign PROJ-123 --to alice@example.com\n" +
			"  jira-cli issue assign PROJ-123 --unassign",
		Args: cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			key, err := resolveIssueKey(args[0])
			if err != nil {
				return err
			}
			if to == "" && !unassign {
				return cerrors.New(cerrors.CategoryUsage, "ISSUE_NO_ASSIGNEE",
					"pass --to <user> or --unassign")
			}
			req := apiclient.AssignIssueReq{Key: key, Assignee: to, Unassign: unassign}
			ctx, cancel := cmdContext(s)
			defer cancel()
			client, err := s.newClient(ctx)
			if err != nil {
				return err
			}
			if dryRun {
				return emitDryRun(s, client, ctx, req)
			}
			if err := client.AssignIssue(ctx, req); err != nil {
				return err
			}
			return s.emit(map[string]any{"key": key, "status": "assigned", "to": to, "unassigned": unassign})
		},
	}
	f := cmd.Flags()
	f.StringVar(&to, "to", "", "the new assignee")
	f.BoolVar(&unassign, "unassign", false, "clear the assignee")
	f.BoolVar(&dryRun, "dry-run", false, "preview the HTTP request without sending it")
	return cmd
}

func newIssueTransitionsCmd(s *appState) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "transitions <key|url>",
		Short: "List the workflow transitions currently available on an issue",
		Long: "List the transitions the issue can take from its current status —\n" +
			"the values `issue transition --to` accepts.",
		Example: "  jira-cli issue transitions PROJ-123",
		Args:    cobra.ExactArgs(1),
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
			res, err := client.ListTransitions(ctx, key)
			if err != nil {
				return err
			}
			return s.emitList(res.Items, pageInfo{})
		},
	}
	return cmd
}

func newIssueTransitionCmd(s *appState) *cobra.Command {
	var (
		to      string
		comment string
		dryRun  bool
	)
	cmd := &cobra.Command{
		Use:   "transition <key|url> --to <name-or-id>",
		Short: "Move an issue through a workflow transition",
		Long: "Transition an issue. --to accepts a transition ID or name (matched\n" +
			"case-insensitively against the transitions currently available; see\n" +
			"`issue transitions`). Target status names also match when unambiguous.",
		Example: "  jira-cli issue transition PROJ-123 --to \"In Progress\"\n" +
			"  jira-cli issue transition PROJ-123 --to 31 --comment \"Deployed to staging.\"",
		Aliases: []string{"move"},
		Args:    cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			key, err := resolveIssueKey(args[0])
			if err != nil {
				return err
			}
			if to == "" {
				return cerrors.New(cerrors.CategoryUsage, "TRANSITION_NO_TARGET",
					"pass --to with a transition name or ID").
					WithNextSteps("jira-cli issue transitions " + key)
			}
			ctx, cancel := cmdContext(s)
			defer cancel()
			client, err := s.newClient(ctx)
			if err != nil {
				return err
			}
			id, err := resolveTransition(ctx, client, key, to)
			if err != nil {
				return err
			}
			req := apiclient.TransitionIssueReq{Key: key, TransitionID: id, Comment: comment}
			if dryRun {
				return emitDryRun(s, client, ctx, req)
			}
			if err := client.TransitionIssue(ctx, req); err != nil {
				return err
			}
			issue, err := client.GetIssue(ctx, apiclient.GetIssueOpts{Key: key})
			if err != nil {
				return err
			}
			return s.emit(issue)
		},
	}
	f := cmd.Flags()
	f.StringVar(&to, "to", "", "target transition name or ID")
	f.StringVar(&comment, "comment", "", "comment to add as part of the transition")
	f.BoolVar(&dryRun, "dry-run", false, "preview the HTTP request without sending it")
	return cmd
}

// resolveTransition maps a --to value onto a transition ID: an exact ID match
// wins, then a unique case-insensitive transition-name match, then a unique
// target-status-name match. Ambiguity and misses fail with the candidate list
// so the caller can pick.
func resolveTransition(ctx context.Context, client apiclient.Client, key, to string) (string, error) {
	res, err := client.ListTransitions(ctx, key)
	if err != nil {
		return "", err
	}
	var byName, byStatus []apiclient.Transition
	for _, t := range res.Items {
		if t.ID == to {
			return t.ID, nil
		}
		if strings.EqualFold(t.Name, to) {
			byName = append(byName, t)
		}
		if t.To != nil && strings.EqualFold(t.To.Name, to) {
			byStatus = append(byStatus, t)
		}
	}
	pick := byName
	if len(pick) == 0 {
		pick = byStatus
	}
	if len(pick) == 1 {
		return pick[0].ID, nil
	}
	candidates := make([]string, 0, len(res.Items))
	for _, t := range res.Items {
		label := t.Name + " (id " + t.ID
		if t.To != nil {
			label += ", to " + t.To.Name
		}
		label += ")"
		candidates = append(candidates, label)
	}
	if len(pick) > 1 {
		return "", cerrors.Newf(cerrors.CategoryUsage, "TRANSITION_AMBIGUOUS",
			"%q matches %d transitions on %s", to, len(pick), key).
			WithHint("Pass the transition ID instead.").
			WithNextSteps(candidates...)
	}
	return "", cerrors.Newf(cerrors.CategoryNotFound, "TRANSITION_NOT_FOUND",
		"no available transition on %s matches %q", key, to).
		WithHint("Transitions depend on the issue's current status and your permissions.").
		WithNextSteps(candidates...)
}
