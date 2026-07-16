package app

import (
	"github.com/angelmsger/jira-cli/pkg/apiclient"
	"github.com/spf13/cobra"
)

func newCommentCmd(s *appState) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "comment",
		Short: "Read and post issue comments",
	}
	cmd.AddCommand(
		newCommentListCmd(s), newCommentAddCmd(s),
		newCommentUpdateCmd(s), newCommentDeleteCmd(s),
	)
	return cmd
}

func newCommentListCmd(s *appState) *cobra.Command {
	var (
		limit  int
		all    bool
		cursor string
	)
	cmd := &cobra.Command{
		Use:   "list <issue-key|url>",
		Short: "List an issue's comments (oldest first)",
		Example: "  jira-cli comment list PROJ-123\n" +
			"  jira-cli comment list PROJ-123 --all",
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
			fetch := func(c string) (apiclient.ListResult[apiclient.Comment], error) {
				return client.ListComments(ctx, apiclient.ListCommentsOpts{
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

func newCommentAddCmd(s *appState) *cobra.Command {
	var (
		body     string
		bodyFile string
		dryRun   bool
	)
	cmd := &cobra.Command{
		Use:   "add <issue-key|url>",
		Short: "Post a comment on an issue",
		Long: "Post a comment. Bodies are plain text on both flavors: on Cloud the\n" +
			"text becomes ADF paragraphs, on Data Center it is sent verbatim (wiki\n" +
			"markup is rendered server-side).",
		Example: "  jira-cli comment add PROJ-123 --body \"Deployed to staging.\"\n" +
			"  echo \"Done.\" | jira-cli comment add PROJ-123 --body-file -",
		Args: cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			key, err := resolveIssueKey(args[0])
			if err != nil {
				return err
			}
			text, err := readBodyText(body, bodyFile, "COMMENT_NO_BODY")
			if err != nil {
				return err
			}
			req := apiclient.AddCommentReq{IssueKey: key, Body: text}
			ctx, cancel := cmdContext(s)
			defer cancel()
			client, err := s.newClient(ctx)
			if err != nil {
				return err
			}
			if dryRun {
				return emitDryRun(s, client, ctx, req)
			}
			created, err := client.AddComment(ctx, req)
			if err != nil {
				return err
			}
			return s.emit(created)
		},
	}
	f := cmd.Flags()
	f.StringVar(&body, "body", "", "comment body text")
	f.StringVar(&bodyFile, "body-file", "", "read body from a file ('-' for stdin)")
	f.BoolVar(&dryRun, "dry-run", false, "preview the HTTP request without sending it")
	return cmd
}

func newCommentUpdateCmd(s *appState) *cobra.Command {
	var (
		issue    string
		body     string
		bodyFile string
		dryRun   bool
	)
	cmd := &cobra.Command{
		Use:   "update <comment-id> --issue <key>",
		Short: "Replace a comment's body",
		Example: "  jira-cli comment update 10042 --issue PROJ-123 --body \"Revised.\"\n" +
			"  echo \"Revised.\" | jira-cli comment update 10042 --issue PROJ-123 --body-file -",
		Aliases: []string{"edit"},
		Args:    cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			key, err := resolveIssueKey(issue)
			if err != nil {
				return err
			}
			text, err := readBodyText(body, bodyFile, "COMMENT_NO_BODY")
			if err != nil {
				return err
			}
			req := apiclient.UpdateCommentReq{IssueKey: key, ID: args[0], Body: text}
			ctx, cancel := cmdContext(s)
			defer cancel()
			client, err := s.newClient(ctx)
			if err != nil {
				return err
			}
			if dryRun {
				return emitDryRun(s, client, ctx, req)
			}
			updated, err := client.UpdateComment(ctx, req)
			if err != nil {
				return err
			}
			return s.emit(updated)
		},
	}
	f := cmd.Flags()
	f.StringVar(&issue, "issue", "", "the issue the comment belongs to (required)")
	f.StringVar(&body, "body", "", "new comment body text")
	f.StringVar(&bodyFile, "body-file", "", "read body from a file ('-' for stdin)")
	f.BoolVar(&dryRun, "dry-run", false, "preview the HTTP request without sending it")
	_ = cmd.MarkFlagRequired("issue")
	return cmd
}

func newCommentDeleteCmd(s *appState) *cobra.Command {
	var (
		issue  string
		yes    bool
		dryRun bool
	)
	cmd := &cobra.Command{
		Use:   "delete <comment-id>... --issue <key>",
		Short: "Delete one or more comments",
		Long: "Delete a comment by ID. Pass several IDs to delete them in one run, or\n" +
			"a single '-' to read newline-separated IDs from stdin. Deletion requires\n" +
			"--yes (or an interactive confirmation when stdin is a terminal); --yes\n" +
			"applies to the whole batch.",
		Example: "  jira-cli comment delete 10042 --issue PROJ-123 --yes\n" +
			"  jira-cli comment delete 10042 10043 --issue PROJ-123 --yes",
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
				if cerr := confirmDelete(deletePrompt("comment", items, "on "+key), yes); cerr != nil {
					return cerr
				}
			}
			return runBatch(s, items, func(arg string) (any, error) {
				req := apiclient.DeleteCommentReq{IssueKey: key, ID: arg}
				if dryRun {
					return dryRunPlan(client, ctx, req)
				}
				if derr := client.DeleteComment(ctx, req); derr != nil {
					return nil, derr
				}
				return map[string]any{"id": arg, "issue": key, "status": "deleted"}, nil
			})
		},
	}
	f := cmd.Flags()
	f.StringVar(&issue, "issue", "", "the issue the comments belong to (required)")
	f.BoolVar(&yes, "yes", false, "skip the deletion confirmation")
	f.BoolVar(&dryRun, "dry-run", false, "preview the HTTP request without sending it")
	_ = cmd.MarkFlagRequired("issue")
	return cmd
}
