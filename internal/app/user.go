package app

import (
	"github.com/spf13/cobra"
)

// newWhoamiCmd is the top-level convenience alias for `user me`. The
// stand-alone `whoami` is the universal Unix idiom and predates the `user`
// subtree, so the CLI keeps it.
func newWhoamiCmd(s *appState) *cobra.Command {
	return &cobra.Command{
		Use:     "whoami",
		Short:   "Print the user the configured credentials authenticate as",
		Example: "  jira-cli whoami",
		Args:    cobra.NoArgs,
		RunE:    runWhoami(s),
	}
}

func runWhoami(s *appState) func(*cobra.Command, []string) error {
	return func(cmd *cobra.Command, _ []string) error {
		ctx, cancel := cmdContext(s)
		defer cancel()
		client, err := s.newClient(ctx)
		if err != nil {
			return err
		}
		user, err := client.CurrentUser(ctx)
		if err != nil {
			return err
		}
		return s.emit(user)
	}
}

// newUserCmd is the discovery entry point for the user identifiers that the
// assignee flags (`issue assign --to`, `issue create --assignee`) and the
// `issue search --assignee` filter accept: a Cloud accountId, a Data Center
// username, or a display-name/email query resolved to a unique user.
func newUserCmd(s *appState) *cobra.Command {
	cmd := &cobra.Command{
		Use:     "user",
		Short:   "Discover Jira users — the values assignee flags accept",
		Aliases: []string{"users"},
	}
	cmd.AddCommand(newUserResolveCmd(s), newUserMeCmd(s))
	return cmd
}

func newUserResolveCmd(s *appState) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "resolve <selector>",
		Short: "Resolve a user selector to a unique user",
		Long: "Resolve a user selector to a unique user.\n\n" +
			"Cloud: an accountId is passed through; anything else is searched by\n" +
			"display name / email and must match exactly one active user.\n" +
			"DC:    the selector is the username and is echoed back verbatim.",
		Args: cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			ctx, cancel := cmdContext(s)
			defer cancel()
			client, err := s.newClient(ctx)
			if err != nil {
				return err
			}
			u, err := client.ResolveUser(ctx, args[0])
			if err != nil {
				return err
			}
			return s.emit(u)
		},
	}
	return cmd
}

func newUserMeCmd(s *appState) *cobra.Command {
	return &cobra.Command{
		Use:     "me",
		Short:   "Print the user the configured credentials authenticate as (alias for whoami)",
		Aliases: []string{"current"},
		Args:    cobra.NoArgs,
		RunE:    runWhoami(s),
	}
}
