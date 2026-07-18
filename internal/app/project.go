package app

import (
	"context"

	"github.com/angelmsger/jira-cli/pkg/apiclient"
	"github.com/angelmsger/jira-cli/pkg/urlref"
	"github.com/spf13/cobra"
)

func newProjectCmd(s *appState) *cobra.Command {
	cmd := &cobra.Command{
		Use:     "project",
		Short:   "Browse Jira projects",
		Aliases: []string{"projects"},
	}
	cmd.AddCommand(
		newProjectListCmd(s), newProjectGetCmd(s),
		newProjectComponentsCmd(s), newProjectVersionsCmd(s),
		newProjectIssueTypesCmd(s), newProjectStatusesCmd(s),
	)
	return cmd
}

// projectKeyArg extracts a project key from a bare key or a Jira URL (project
// browse URLs and issue keys/URLs both carry one).
func projectKeyArg(arg string) string {
	if ref := urlref.Parse(arg); ref.ProjectKey != "" {
		return ref.ProjectKey
	}
	return arg
}

func newProjectListCmd(s *appState) *cobra.Command {
	var (
		query  string
		limit  int
		all    bool
		cursor string
	)
	cmd := &cobra.Command{
		Use:   "list",
		Short: "List projects visible to the authenticated user",
		Long: "List projects. On Jira Cloud the listing paginates and --query filters\n" +
			"server-side; on Data Center the API returns every project in one\n" +
			"response (a single page) and --query filters client-side.",
		Example: "  jira-cli project list\n" +
			"  jira-cli project list --query platform",
		Args: cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			ctx, cancel := cmdContext(s)
			defer cancel()
			client, err := s.newClient(ctx)
			if err != nil {
				return err
			}
			fetch := func(c string) (apiclient.ListResult[apiclient.Project], error) {
				return client.ListProjects(ctx, apiclient.ProjectListOpts{
					ListOpts: apiclient.ListOpts{Limit: limit, Cursor: c},
					Query:    query,
				})
			}
			items, info, err := collectPage(fetch, cursor, all)
			if err != nil {
				return err
			}
			return s.emitList(items, info)
		},
	}
	cmd.Flags().StringVar(&query, "query", "", "filter by name/key substring")
	addListFlags(cmd, &limit, &all, &cursor)
	return cmd
}

// newProjectItemsCmd builds one project-scoped metadata listing command
// (components / versions / issue types share the exact same shape).
func newProjectItemsCmd[T any](
	s *appState, use, short, long, example string, aliases []string,
	list func(client apiclient.Client, ctx context.Context, opt apiclient.ProjectItemsOpts) (apiclient.ListResult[T], error),
) *cobra.Command {
	var (
		limit  int
		all    bool
		cursor string
	)
	cmd := &cobra.Command{
		Use:               use + " <key|url>",
		Short:             short,
		Long:              long,
		Example:           example,
		Aliases:           aliases,
		Args:              cobra.ExactArgs(1),
		ValidArgsFunction: completeProjectKeys(s),
		RunE: func(cmd *cobra.Command, args []string) error {
			key := projectKeyArg(args[0])
			ctx, cancel := cmdContext(s)
			defer cancel()
			client, err := s.newClient(ctx)
			if err != nil {
				return err
			}
			fetch := func(c string) (apiclient.ListResult[T], error) {
				return list(client, ctx, apiclient.ProjectItemsOpts{
					ListOpts:   apiclient.ListOpts{Limit: limit, Cursor: c},
					ProjectKey: key,
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

func newProjectComponentsCmd(s *appState) *cobra.Command {
	return newProjectItemsCmd(s, "components",
		"List a project's components",
		"List the components defined in a project — the valid values for the\n"+
			"issue \"components\" field there. On Jira Cloud the listing paginates;\n"+
			"on Data Center the API returns the full list in one response.",
		"  jira-cli project components ENG",
		[]string{"component"},
		func(client apiclient.Client, ctx context.Context, opt apiclient.ProjectItemsOpts) (apiclient.ListResult[apiclient.Component], error) {
			return client.ListComponents(ctx, opt)
		})
}

func newProjectVersionsCmd(s *appState) *cobra.Command {
	return newProjectItemsCmd(s, "versions",
		"List a project's versions",
		"List the versions defined in a project — the valid values for the issue\n"+
			"\"fixVersions\" and affects-versions fields there. On Jira Cloud the\n"+
			"listing paginates; on Data Center the API returns the full list in one\n"+
			"response.",
		"  jira-cli project versions ENG",
		[]string{"version"},
		func(client apiclient.Client, ctx context.Context, opt apiclient.ProjectItemsOpts) (apiclient.ListResult[apiclient.Version], error) {
			return client.ListVersions(ctx, opt)
		})
}

func newProjectIssueTypesCmd(s *appState) *cobra.Command {
	return newProjectItemsCmd(s, "issuetypes",
		"List the issue types creatable in a project",
		"List the issue types that can be created in a project (from the create\n"+
			"metadata, so permissions apply) — the valid values for\n"+
			"`issue create --type` there.",
		"  jira-cli project issuetypes ENG",
		[]string{"issue-types", "types"},
		func(client apiclient.Client, ctx context.Context, opt apiclient.ProjectItemsOpts) (apiclient.ListResult[apiclient.IssueType], error) {
			return client.ListProjectIssueTypes(ctx, opt)
		})
}

func newProjectStatusesCmd(s *appState) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "statuses <key|url>",
		Short: "List a project's workflow statuses per issue type",
		Long: "List the workflow statuses valid in a project, grouped by issue type\n" +
			"(different issue types can use different workflows). These are the\n" +
			"values a `status = ...` JQL clause can match in that project.",
		Example:           "  jira-cli project statuses ENG",
		Aliases:           []string{"status"},
		Args:              cobra.ExactArgs(1),
		ValidArgsFunction: completeProjectKeys(s),
		RunE: func(cmd *cobra.Command, args []string) error {
			key := projectKeyArg(args[0])
			ctx, cancel := cmdContext(s)
			defer cancel()
			client, err := s.newClient(ctx)
			if err != nil {
				return err
			}
			res, err := client.ListProjectStatuses(ctx, key)
			if err != nil {
				return err
			}
			return s.emitList(res.Items, pageInfo{})
		},
	}
	return cmd
}

func newProjectGetCmd(s *appState) *cobra.Command {
	cmd := &cobra.Command{
		Use:               "get <key|url>",
		Short:             "Show one project",
		Example:           "  jira-cli project get ENG",
		Aliases:           []string{"view", "show"},
		Args:              cobra.ExactArgs(1),
		ValidArgsFunction: completeProjectKeys(s),
		RunE: func(cmd *cobra.Command, args []string) error {
			key := projectKeyArg(args[0])
			ctx, cancel := cmdContext(s)
			defer cancel()
			client, err := s.newClient(ctx)
			if err != nil {
				return err
			}
			p, err := client.GetProject(ctx, key)
			if err != nil {
				return err
			}
			return s.emit(p)
		},
	}
	return cmd
}
