// Reuse a verified native credential by associating it with an existing team
// context. agent-cli-kit/reuse decides; this file describes jira-cli's
// contexts to it and supplies resolution, verification and persistence.
// Secrets stay in their original store; public setup stays offline.
package app

import (
	"context"
	"reflect"
	"time"

	"github.com/angelmsger/agent-cli-kit/credstore"
	"github.com/angelmsger/agent-cli-kit/reuse"
	"github.com/angelmsger/jira-cli/internal/auth"
	"github.com/angelmsger/jira-cli/internal/config"
	"github.com/angelmsger/jira-cli/pkg/apiclient"
	cerrors "github.com/angelmsger/jira-cli/pkg/errors"
	"github.com/spf13/cobra"
)

type reuseServices struct {
	Resolve func(config.Config) (auth.Credential, credstore.Lookup, error)
	Lookup  func(key string) credstore.Lookup
	// Verify authenticates cred and returns the user it signed in as.
	Verify func(context.Context, config.Config, auth.Credential) (apiclient.User, error)
	Read   func(string) (config.File, bool, error)
	Write  func(string, config.File) error
}

type reuseOptions struct {
	From       string
	FromScheme string
	DryRun     bool
}

func newAuthReuseCmd(s *appState) *cobra.Command {
	var opts reuseOptions
	cmd := &cobra.Command{
		Use:   "reuse",
		Short: "Reuse an existing login in the selected context without signing in again",
		Long: "Find a login this machine already verified for the same service (complete URL\n" +
			"and deployment flavor) and point the selected context at it. Contexts under the\n" +
			"context's own scheme come first, then a credential under the other scheme (a\n" +
			"personal access token, or a password or API token, including one stored for\n" +
			"this service that no context points at). A working login is never replaced\n" +
			"automatically; a username recorded under the pat scheme is only a label. The\n" +
			"result names the account the login authenticates as. Never copy secrets,\n" +
			"activate a context, or read credentials from environment variables. Dry-run\n" +
			"performs read-only verification and reports the same proposed association. An\n" +
			"unchanged or unavailable result is not proof of authentication; use whoami to\n" +
			"check.",
		Args: cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			result, err := reuseAuthentication(s, opts, s.reuseServices())
			if err != nil {
				return err
			}
			return s.emit(result)
		},
	}
	cmd.Flags().BoolVar(&opts.DryRun, "dry-run", false, "verify and preview the association without writing")
	cmd.Flags().StringVar(&opts.From, "from-context", "", "choose the source context when several logins match; list names with 'config get-contexts'")
	cmd.Flags().StringVar(&opts.FromScheme, "from-scheme", "", "use the credential stored for this service under this scheme (pat or basic); with --from-context, paired with that context's username")
	return cmd
}

func (s *appState) reuseServices() reuseServices {
	return reuseServices{
		Resolve: func(cfg config.Config) (auth.Credential, credstore.Lookup, error) {
			return auth.ResolveLookup(cfg, config.Secrets{}, s.store)
		},
		Lookup: func(key string) credstore.Lookup { return s.store.Lookup(key) },
		Verify: func(ctx context.Context, cfg config.Config, cred auth.Credential) (apiclient.User, error) {
			return verifyCredentialWithin(ctx, s, cfg, cred)
		},
		Read: config.ReadFile, Write: config.WriteFile,
	}
}

func reuseAuthentication(s *appState, opts reuseOptions, services reuseServices) (reuse.Result, error) {
	started := time.Now()
	file, _, err := services.Read(s.cfgDir)
	if err != nil {
		return reuse.Result{Context: s.resolved.ActiveContext, State: reuse.StateUnavailable, DryRun: opts.DryRun},
			cerrors.Wrap(err, cerrors.CategoryConfig, "CONFIG_READ", "failed to read authentication contexts")
	}
	target, _ := file.Context(s.resolved.ActiveContext)
	// Every source matched the destination's complete service, flavor included,
	// so each is resolved and verified with the destination's configuration
	// under the flavor the match compared.
	targetCfg := config.StoredContext(target, file.Defaults)
	targetCfg.Flavor = config.ReuseFlavor(targetCfg)
	withCredential := func(spec reuse.Spec) config.Config {
		cfg := targetCfg
		cfg.BaseURL, cfg.Auth.Scheme, cfg.Auth.Username = spec.KeyBase, spec.Scheme, spec.Username
		return cfg
	}
	// Jira Cloud accepts only basic authentication.
	validate := func(scheme string) error {
		cfg := targetCfg
		cfg.Auth.Scheme = scheme
		return config.ValidateService(cfg)
	}
	req := reuse.Request{
		Target:         s.resolved.ActiveContext,
		Contexts:       config.ReuseContexts(file),
		Runtime:        reuse.Context{Service: config.ReuseService(s.cfg()), Scheme: s.cfg().Auth.Scheme},
		SchemeSupplied: config.SchemeSupplied(s.resolved.Sources),
		From:           opts.From,
		FromScheme:     opts.FromScheme,
		DryRun:         opts.DryRun,
		Start:          started,
		Timeout:        s.timeout(),
		Check:          "whoami",
		Key:            auth.AccountKey,
		Valid:          func(scheme string) bool { return validate(scheme) == nil },
		Resolve: func(spec reuse.Spec) (auth.Credential, credstore.Lookup, error) {
			return services.Resolve(withCredential(spec))
		},
		Lookup: services.Lookup,
		Verify: func(ctx context.Context, spec reuse.Spec, cred auth.Credential) (reuse.Account, error) {
			// Valid already keeps such sources out of the automatic steps; an
			// explicitly selected one is refused here, before any request.
			if spec.Scheme != targetCfg.Auth.Scheme {
				if err := validate(spec.Scheme); err != nil {
					return reuse.Account{}, cerrors.Wrap(err, cerrors.CategoryConflict, "AUTH_REUSE_SOURCE_MISMATCH",
						"the destination cannot use the selected scheme").
						WithHint(cerrors.AsCLIError(err).Message).
						WithNextSteps(config.ListContextsStep())
				}
			}
			user, err := services.Verify(ctx, withCredential(spec), cred)
			if err != nil {
				return reuse.Account{}, err
			}
			// The Cloud accountId, or the Data Center username.
			return reuse.Account{ID: user.Identifier(), Name: user.DisplayName}, nil
		},
		Persist: func(u reuse.Update) error {
			current, _, err := services.Read(s.cfgDir)
			if err != nil {
				return cerrors.Wrap(err, cerrors.CategoryConfig, "CONFIG_READ", "failed to recheck authentication contexts")
			}
			if !reflect.DeepEqual(file, current) {
				return reuse.ErrConfigChanged
			}
			next := file
			next.Contexts = append([]config.NamedContext(nil), file.Contexts...)
			for i, c := range next.Contexts {
				if c.Name != target.Name {
					continue
				}
				c.Auth.Scheme = u.Scheme
				if u.SetUsername {
					c.Auth.Username = u.Username
				}
				if u.BaseURL != "" {
					c.BaseURL = u.BaseURL
				}
				next.Contexts[i] = c
			}
			return services.Write(s.cfgDir, next)
		},
	}
	return reuse.Run(req)
}

// withReuseRecovery leads a rejected-credential error (HTTP 401) with
// `auth reuse` when a stored login could replace the rejected one.
func withReuseRecovery(ce *cerrors.CLIError, s *appState, cmd *cobra.Command) {
	if ce.Category != cerrors.CategoryAuth || ce.HTTPStatus != 401 || s == nil || s.resolved == nil || cmd == nil {
		return
	}
	if cmd.Name() == "reuse" || cmd.Name() == "login" {
		return
	}
	if step := s.cfg().ReuseStep; step != "" {
		ce.NextSteps = append([]string{step}, ce.NextSteps...)
	}
}
