package app

import (
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
	cmd.AddCommand(newProjectListCmd(s), newProjectGetCmd(s))
	return cmd
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

func newProjectGetCmd(s *appState) *cobra.Command {
	cmd := &cobra.Command{
		Use:               "get <key|url>",
		Short:             "Show one project",
		Example:           "  jira-cli project get ENG",
		Aliases:           []string{"view", "show"},
		Args:              cobra.ExactArgs(1),
		ValidArgsFunction: completeProjectKeys(s),
		RunE: func(cmd *cobra.Command, args []string) error {
			key := args[0]
			if ref := urlref.Parse(key); ref.ProjectKey != "" {
				key = ref.ProjectKey
			}
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
