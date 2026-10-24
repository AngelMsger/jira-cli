package app

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"github.com/angelmsger/agent-cli-kit/credstore"
	"github.com/angelmsger/jira-cli/internal/auth"
	"github.com/angelmsger/jira-cli/internal/config"
	"github.com/angelmsger/jira-cli/pkg/apiclient"
	"github.com/angelmsger/jira-cli/pkg/constants"
	cerrors "github.com/angelmsger/jira-cli/pkg/errors"
	"github.com/spf13/cobra"
	"github.com/zalando/go-keyring"
)

// The decision itself is agent-cli-kit/reuse's and tested there. These tests
// cover jira-cli's wiring: how contexts are described (the service is the URL
// plus the effective flavor), how the destination is rewritten, the Cloud rule
// against personal access tokens, the account verification reports, and the
// native store, verification and command.

const reuseBase = "https://service.example.test/deploy"

// Other spellings of reuseBase: the same service under another credential key.
const (
	reuseUpper     = "https://SERVICE.example.test/deploy"
	reuseUpperPort = "https://SERVICE.example.test:443/deploy"
)

// fakeStore answers lookups from keychain entries keyed like the real store.
type fakeStore map[string]string

func (f fakeStore) lookup(key string) credstore.Lookup {
	if v, ok := f[key]; ok {
		return credstore.Lookup{Secret: v, Keychain: credstore.Hit}
	}
	return credstore.Lookup{Keychain: credstore.Miss, File: credstore.Miss}
}

func (f fakeStore) resolve(cfg config.Config) (auth.Credential, credstore.Lookup, error) {
	l := f.lookup(auth.AccountKey(cfg.BaseURL, cfg.Auth.Scheme))
	if !l.Found() {
		return auth.Credential{}, l, cerrors.New(cerrors.CategoryConfig, "CREDENTIAL_NOT_VISIBLE_OR_MISSING", "missing")
	}
	cred := auth.Credential{Scheme: cfg.Auth.Scheme, Username: cfg.Auth.Username, Secret: l.Secret}
	return cred, l, cred.Validate()
}

// logins maps "username\x00secret" to the user the service signs it in as;
// anything else is rejected with HTTP 401.
type logins map[string]apiclient.User

func (l logins) verify(_ context.Context, _ config.Config, cred auth.Credential) (apiclient.User, error) {
	if u, ok := l[cred.Username+"\x00"+cred.Secret]; ok {
		return u, nil
	}
	return apiclient.User{}, cerrors.New(cerrors.CategoryAuth, "HTTP_UNAUTHORIZED", "rejected").WithHTTPStatus(401)
}

var existingUser = apiclient.User{Username: "existing-user", DisplayName: "Existing User"}

func basicContext(name, user string) config.NamedContext {
	return config.NamedContext{Name: name, BaseURL: reuseBase, Auth: config.AuthConfig{Scheme: "basic", Username: user}}
}

func patContext(name, base, label string) config.NamedContext {
	return config.NamedContext{Name: name, BaseURL: base, Auth: config.AuthConfig{Scheme: "pat", Username: label}}
}

// reuseFixture writes contexts (by default a personal basic login and a team
// context without one) and selects the last as the destination. The store
// holds the basic secret for reuseBase.
func reuseFixture(t *testing.T, contexts ...config.NamedContext) (*appState, fakeStore, reuseServices) {
	t.Helper()
	dir := t.TempDir()
	if len(contexts) == 0 {
		contexts = []config.NamedContext{basicContext("personal", "existing-user"), basicContext("team", "")}
	}
	file := config.File{CurrentContext: contexts[0].Name, Contexts: contexts}
	if err := config.WriteFile(dir, file); err != nil {
		t.Fatal(err)
	}
	target := contexts[len(contexts)-1]
	s := &appState{cfgDir: dir, resolved: &config.Resolved{Config: config.StoredContext(target, file.Defaults), ActiveContext: target.Name}}
	store := fakeStore{auth.AccountKey(reuseBase, "basic"): "test-secret"}
	services := reuseServices{Read: config.ReadFile, Write: config.WriteFile, Resolve: store.resolve, Lookup: store.lookup,
		Verify: logins{"existing-user\x00test-secret": existingUser}.verify}
	return s, store, services
}

// pairingFixture is a token destination whose service records two usernames,
// both only as labels of token contexts under other spellings, and whose basic
// secret signs either of them in.
func pairingFixture(t *testing.T) (*appState, reuseServices) {
	t.Helper()
	s, _, services := reuseFixture(t, patContext("x", reuseUpper, "user-a"), patContext("y", reuseUpperPort, "user-b"), patContext("team", reuseBase, ""))
	services.Verify = logins{"user-a\x00test-secret": {Username: "user-a"}, "user-b\x00test-secret": {Username: "user-b"}}.verify
	return s, services
}

func targetOf(t *testing.T, s *appState) config.NamedContext {
	t.Helper()
	file, _, err := config.ReadFile(s.cfgDir)
	if err != nil {
		t.Fatal(err)
	}
	c, _ := file.Context(s.resolved.ActiveContext)
	return c
}

func errorCode(err error) string {
	if err == nil {
		return ""
	}
	return cerrors.AsCLIError(err).Code
}

func TestAuthReusePreviewApplyAndIdempotence(t *testing.T) {
	s, _, services := reuseFixture(t)
	before, _, _ := config.ReadFile(s.cfgDir)
	preview, err := reuseAuthentication(s, reuseOptions{DryRun: true}, services)
	if err != nil || !preview.Changed || preview.State != "available" || !preview.Verified || preview.Scheme != "basic" || preview.Account != "existing-user" {
		t.Fatalf("preview: %+v %v", preview, err)
	}
	if after, _, _ := config.ReadFile(s.cfgDir); !reflect.DeepEqual(before, after) {
		t.Fatal("preview changed config")
	}
	applied, err := reuseAuthentication(s, reuseOptions{}, services)
	if err != nil || applied.State != "reused" || applied.SourceContext != "personal" || applied.Username != "existing-user" ||
		applied.Account != "existing-user" || applied.AccountName != "Existing User" {
		t.Fatalf("apply: %+v %v", applied, err)
	}
	after, _, _ := config.ReadFile(s.cfgDir)
	if got := targetOf(t, s); got.Auth.Username != "existing-user" || after.CurrentContext != before.CurrentContext || !reflect.DeepEqual(after.Contexts[0], before.Contexts[0]) {
		t.Fatalf("did not preserve the source or the current context: %+v", after)
	}
	services.Write = func(string, config.File) error { t.Fatal("repeated write"); return nil }
	services.Resolve = func(config.Config) (auth.Credential, credstore.Lookup, error) {
		t.Fatal("a configured basic identity must not be looked up")
		return auth.Credential{}, credstore.Lookup{}, nil
	}
	repeated, err := reuseAuthentication(s, reuseOptions{}, services)
	if err != nil || repeated.Changed || repeated.State != "unchanged" {
		t.Fatalf("repeat: %+v %v", repeated, err)
	}
	raw, _ := json.Marshal(applied)
	if strings.Contains(string(raw), "test-secret") || !strings.Contains(string(raw), `"account":"existing-user"`) {
		t.Fatalf("output %s", raw)
	}
	if len(applied.NextSteps) != 1 || applied.NextSteps[0] != "jira-cli --use-context 'team' whoami" {
		t.Fatalf("next steps: %v", applied.NextSteps)
	}
}

// A personal-access-token team context adopts the personal basic login on
// the same service once its own token is gone.
func TestAuthReusePATContextAdoptsBasicLogin(t *testing.T) {
	s, _, services := reuseFixture(t, basicContext("personal", "existing-user"), patContext("team", reuseBase, ""))
	r, err := reuseAuthentication(s, reuseOptions{}, services)
	if err != nil || r.State != "reused" || r.SourceContext != "personal" || r.Scheme != "basic" || r.Username != "existing-user" || r.Account != "existing-user" {
		t.Fatalf("got %+v %v", r, err)
	}
	if got := targetOf(t, s); got.Auth.Scheme != "basic" || got.Auth.Username != "existing-user" {
		t.Fatalf("destination %+v", got)
	}
}

// A basic team context without a username adopts a personal access token on
// Data Center, and never on Cloud, which accepts only basic authentication.
func TestAuthReuseBasicContextAdoptsPATOnlyWhereValid(t *testing.T) {
	for _, flavor := range []string{"datacenter", "cloud"} {
		t.Run(flavor, func(t *testing.T) {
			personal, team := patContext("personal", reuseBase, ""), basicContext("team", "")
			personal.Flavor, team.Flavor = flavor, flavor
			s, store, services := reuseFixture(t, personal, team)
			store[auth.AccountKey(reuseBase, "pat")] = "pat-secret"
			services.Verify = logins{"\x00pat-secret": existingUser}.verify
			resolve := services.Resolve
			services.Resolve = func(cfg config.Config) (auth.Credential, credstore.Lookup, error) {
				if flavor == "cloud" && cfg.Auth.Scheme == "pat" {
					t.Fatal("read a personal access token for a Cloud destination")
				}
				return resolve(cfg)
			}
			r, err := reuseAuthentication(s, reuseOptions{}, services)
			if flavor == "datacenter" {
				if err != nil || r.State != "reused" || r.Scheme != "pat" || r.Username != "" || r.SourceContext != "personal" || r.Account != "existing-user" {
					t.Fatalf("datacenter: %+v %v", r, err)
				}
				if got := targetOf(t, s); got.Auth.Scheme != "pat" || got.Auth.Username != "" {
					t.Fatalf("destination %+v", got)
				}
				return
			}
			if err != nil || r.State != "unavailable" || r.Changed {
				t.Fatalf("cloud: %+v %v", r, err)
			}
			// An explicit choice of a token is refused before any request.
			services.Resolve = resolve
			services.Verify = func(context.Context, config.Config, auth.Credential) (apiclient.User, error) {
				t.Fatal("verified a personal access token for a Cloud destination")
				return apiclient.User{}, nil
			}
			for _, opts := range []reuseOptions{{From: "personal"}, {FromScheme: "pat"}} {
				if _, err = reuseAuthentication(s, opts, services); errorCode(err) != "AUTH_REUSE_SOURCE_MISMATCH" {
					t.Fatalf("explicit %+v: %v", opts, err)
				}
			}
			if got := targetOf(t, s); got.Auth.Scheme != "basic" {
				t.Fatalf("destination %+v", got)
			}
		})
	}
}

// A token context whose username is only a label adopts the basic secret
// stored for its service, which no context points at any more.
func TestAuthReuseLabelledPATAdoptsStoredSecret(t *testing.T) {
	s, _, services := reuseFixture(t, patContext("team", reuseBase, "existing-user"))
	r, err := reuseAuthentication(s, reuseOptions{}, services)
	if err != nil || r.State != "reused" || r.SourceContext != "" || r.Username != "existing-user" || !strings.Contains(r.Reason, "stored for this service") {
		t.Fatalf("got %+v %v", r, err)
	}
	if got := targetOf(t, s); got.Auth.Scheme != "basic" || got.Auth.Username != "existing-user" {
		t.Fatalf("destination %+v", got)
	}
}

// A working token is never replaced, not even by an explicitly selected
// password: only a browser session gives way to an explicit choice.
func TestAuthReuseKeepsAWorkingToken(t *testing.T) {
	s, store, services := reuseFixture(t, patContext("team", reuseBase, "existing-user"))
	store[auth.AccountKey(reuseBase, "pat")] = "pat-secret"
	services.Verify = logins{"existing-user\x00test-secret": existingUser, "existing-user\x00pat-secret": existingUser}.verify
	for _, opts := range []reuseOptions{{}, {FromScheme: "basic"}} {
		r, err := reuseAuthentication(s, opts, services)
		if err != nil || r.State != "unchanged" || !r.Verified || r.Changed {
			t.Fatalf("%+v: %+v %v", opts, r, err)
		}
	}
	if got := targetOf(t, s); got.Auth.Scheme != "pat" {
		t.Fatalf("destination %+v", got)
	}
}

func TestAuthReuseMatchesCompleteServiceBeforeCredentialAccess(t *testing.T) {
	for _, kind := range []string{"host", "path", "protocol", "port", "flavor"} {
		t.Run(kind, func(t *testing.T) {
			personal := basicContext("personal", "existing-user")
			switch kind {
			case "host":
				personal.BaseURL = "https://other.example.test/deploy"
			case "path":
				personal.BaseURL = "https://service.example.test/other"
			case "protocol":
				personal.BaseURL = "http://service.example.test/deploy"
			case "port":
				personal.BaseURL = "https://service.example.test:8443/deploy"
			case "flavor":
				personal.Flavor = "cloud"
			}
			s, _, services := reuseFixture(t, personal, basicContext("team", ""))
			resolve := services.Resolve
			services.Resolve = func(cfg config.Config) (auth.Credential, credstore.Lookup, error) {
				if cfg.Auth.Username == "existing-user" {
					t.Fatal("read another service's credential")
				}
				return resolve(cfg)
			}
			result, err := reuseAuthentication(s, reuseOptions{DryRun: true}, services)
			if err != nil || result.Changed || result.State != "unavailable" {
				t.Fatalf("mismatch: %+v %v", result, err)
			}
			_, err = reuseAuthentication(s, reuseOptions{From: "personal", DryRun: true}, services)
			if errorCode(err) != "AUTH_REUSE_SOURCE_MISMATCH" {
				t.Fatalf("explicit mismatch: %v", err)
			}
		})
	}
}

// A scheme the invocation supplies keeps reuse within that scheme.
func TestAuthReuseSuppliedSchemeStaysWithinIt(t *testing.T) {
	s, _, services := reuseFixture(t, basicContext("personal", "existing-user"), patContext("team", reuseBase, ""))
	s.resolved.Sources = map[string]string{config.FieldAuthScheme: "flag"}
	r, err := reuseAuthentication(s, reuseOptions{}, services)
	if err != nil || r.State != "unavailable" || r.Changed {
		t.Fatalf("pinned: %+v %v", r, err)
	}
	for _, opts := range []reuseOptions{{From: "personal"}, {FromScheme: "basic"}} {
		if _, err = reuseAuthentication(s, opts, services); errorCode(err) != "AUTH_REUSE_SOURCE_MISMATCH" {
			t.Fatalf("pinned explicit %+v: %v", opts, err)
		}
	}
}

func TestAuthReusePreservesDestinationIdentity(t *testing.T) {
	s, _, services := reuseFixture(t, basicContext("personal", "existing-user"), basicContext("team", "other-user"))
	services.Resolve = func(config.Config) (auth.Credential, credstore.Lookup, error) {
		t.Fatal("must not replace an existing identity")
		return auth.Credential{}, credstore.Lookup{}, nil
	}
	r, err := reuseAuthentication(s, reuseOptions{}, services)
	if err != nil || r.State != "unchanged" || r.Changed {
		t.Fatalf("existing identity: %+v %v", r, err)
	}
}

// Two tokens are one login when the service signs both in as one account,
// and a choice when they belong to different accounts.
func TestAuthReuseAccountsDecideAmbiguity(t *testing.T) {
	fixture := func(t *testing.T, a, b apiclient.User) (*appState, reuseServices) {
		s, store, services := reuseFixture(t, patContext("first", reuseUpper, ""), patContext("second", reuseUpperPort, ""), patContext("team", reuseBase, ""))
		store[auth.AccountKey(reuseUpper, "pat")] = "first-secret"
		store[auth.AccountKey(reuseUpperPort, "pat")] = "second-secret"
		services.Verify = logins{"\x00first-secret": a, "\x00second-secret": b}.verify
		return s, services
	}

	s, services := fixture(t, apiclient.User{AccountID: "acct-a"}, apiclient.User{AccountID: "acct-b"})
	_, err := reuseAuthentication(s, reuseOptions{DryRun: true}, services)
	if errorCode(err) != "AUTH_REUSE_AMBIGUOUS" {
		t.Fatalf("ambiguity: %v", err)
	}
	details, _ := json.Marshal(cerrors.AsCLIError(err).Details)
	if !strings.Contains(string(details), `"account":"acct-a"`) || !strings.Contains(string(details), `"account":"acct-b"`) {
		t.Fatalf("candidates %s", details)
	}
	r, err := reuseAuthentication(s, reuseOptions{From: "SECOND"}, services)
	if err != nil || r.SourceContext != "second" || r.State != "reused" || r.Account != "acct-b" {
		t.Fatalf("explicit choice: %+v %v", r, err)
	}
	if got := targetOf(t, s); got.BaseURL != reuseUpperPort {
		t.Fatalf("destination does not resolve the chosen key: %+v", got)
	}

	s, services = fixture(t, apiclient.User{AccountID: "acct-a"}, apiclient.User{AccountID: "acct-a"})
	r, err = reuseAuthentication(s, reuseOptions{}, services)
	if err != nil || r.State != "reused" || r.SourceContext != "first" || r.Account != "acct-a" {
		t.Fatalf("one account: %+v %v", r, err)
	}
}

// A stored password pairs with every username the service's contexts record,
// labels included; --from-context names the one to pair it with. The token
// contexts' own secrets are reliably absent, so they do not stop the search.
func TestAuthReuseStoredSecretPairsWithSelectedUsername(t *testing.T) {
	s, services := pairingFixture(t)
	for _, opts := range []reuseOptions{{DryRun: true}, {FromScheme: "basic", DryRun: true}} {
		_, err := reuseAuthentication(s, opts, services)
		if errorCode(err) != "AUTH_REUSE_AMBIGUOUS" {
			t.Fatalf("%+v: %v", opts, err)
		}
	}
	r, err := reuseAuthentication(s, reuseOptions{FromScheme: "basic", From: "x"}, services)
	if err != nil || r.State != "reused" || r.Username != "user-a" || r.SourceContext != "" || r.Account != "user-a" {
		t.Fatalf("paired: %+v %v", r, err)
	}
	if got := targetOf(t, s); got.Auth.Scheme != "basic" || got.Auth.Username != "user-a" || got.BaseURL != reuseBase {
		t.Fatalf("destination %+v", got)
	}
}

func TestAuthReuseDuplicateAliasesDoNotRequireChoice(t *testing.T) {
	s, _, services := reuseFixture(t, basicContext("personal", "existing-user"), basicContext("alias", "existing-user"), basicContext("team", ""))
	r, err := reuseAuthentication(s, reuseOptions{DryRun: true}, services)
	if err != nil || !r.Changed {
		t.Fatalf("same native credential: %+v %v", r, err)
	}
}

func TestAuthReuseRetainsOperationalFailures(t *testing.T) {
	for _, kind := range []string{"store", "network", "permission", "identity"} {
		t.Run(kind, func(t *testing.T) {
			s, _, services := reuseFixture(t)
			before, _, _ := config.ReadFile(s.cfgDir)
			failure := cerrors.New(cerrors.CategoryNetwork, "NETWORK_ERROR", "network unavailable")
			switch kind {
			case "store":
				failure = cerrors.New(cerrors.CategoryConfig, "CREDENTIAL_STORE_INACCESSIBLE", "host keychain unavailable")
				services.Resolve = func(config.Config) (auth.Credential, credstore.Lookup, error) {
					return auth.Credential{}, credstore.Lookup{Keychain: credstore.Failed}, failure
				}
			case "permission":
				failure = cerrors.New(cerrors.CategoryPermission, "FORBIDDEN", "access denied")
			case "identity":
				failure = cerrors.New(cerrors.CategoryAuth, "AUTH_IDENTITY_UNAVAILABLE", "authenticated identity could not be established")
			}
			if kind != "store" {
				services.Verify = func(context.Context, config.Config, auth.Credential) (apiclient.User, error) {
					return apiclient.User{}, failure
				}
			}
			_, err := reuseAuthentication(s, reuseOptions{}, services)
			if !errors.Is(err, failure) {
				t.Fatalf("lost diagnostic: %v", err)
			}
			if after, _, _ := config.ReadFile(s.cfgDir); !reflect.DeepEqual(before, after) {
				t.Fatal("failure mutated config")
			}
		})
	}
}

func TestAuthReuseExpiredIdentityIsUnavailable(t *testing.T) {
	s, _, services := reuseFixture(t)
	services.Verify = logins{}.verify
	r, err := reuseAuthentication(s, reuseOptions{DryRun: true}, services)
	if err != nil || r.State != "unavailable" || r.Changed {
		t.Fatalf("expired credential: %+v %v", r, err)
	}
	// A basic context without a username has no login of its own to check.
	if len(r.NextSteps) != 2 || !strings.HasSuffix(r.NextSteps[0], "auth guide") || !strings.HasSuffix(r.NextSteps[1], "auth login") {
		t.Fatalf("next steps: %v", r.NextSteps)
	}
}

func TestAuthReuseSelfConfigurationInReadOnlyMode(t *testing.T) {
	s, _, services := reuseFixture(t)
	s.resolved.Config.Defaults.ReadOnly = true
	r, err := reuseAuthentication(s, reuseOptions{}, services)
	if err != nil || r.State != "reused" {
		t.Fatalf("native self-configuration should remain available: %+v %v", r, err)
	}
}

func TestAuthReuseConcurrentChangeAndRotation(t *testing.T) {
	s, _, services := reuseFixture(t)
	verify := services.Verify
	services.Verify = func(ctx context.Context, cfg config.Config, cred auth.Credential) (apiclient.User, error) {
		f, _, _ := config.ReadFile(s.cfgDir)
		f.CurrentContext = "changed-by-user"
		if err := config.WriteFile(s.cfgDir, f); err != nil {
			return apiclient.User{}, err
		}
		return verify(ctx, cfg, cred)
	}
	_, err := reuseAuthentication(s, reuseOptions{}, services)
	if errorCode(err) != "AUTH_REUSE_CONFIG_CHANGED" {
		t.Fatalf("concurrent change: %v", err)
	}

	s, store, services := reuseFixture(t)
	before, _, _ := config.ReadFile(s.cfgDir)
	resolve := services.Resolve
	calls := 0
	services.Resolve = func(cfg config.Config) (auth.Credential, credstore.Lookup, error) {
		calls++
		if calls > 1 {
			store[auth.AccountKey(reuseBase, "basic")] = "rotated-secret"
		}
		return resolve(cfg)
	}
	_, err = reuseAuthentication(s, reuseOptions{}, services)
	if errorCode(err) != "AUTH_REUSE_CREDENTIAL_CHANGED" {
		t.Fatalf("credential rotation: %v", err)
	}
	if after, _, _ := config.ReadFile(s.cfgDir); !reflect.DeepEqual(before, after) {
		t.Fatal("rotation changed destination")
	}
}

func TestAuthReuseRejectsTargetOverridesAndUnknownSource(t *testing.T) {
	s, _, services := reuseFixture(t)
	s.resolved.Config.BaseURL = "https://service.example.test/other"
	_, err := reuseAuthentication(s, reuseOptions{DryRun: true}, services)
	if errorCode(err) != "AUTH_REUSE_TARGET_MISMATCH" {
		t.Fatal(err)
	}
	s.resolved.Config.BaseURL = reuseBase
	_, err = reuseAuthentication(s, reuseOptions{From: "missing", DryRun: true}, services)
	if errorCode(err) != "AUTH_REUSE_SOURCE_NOT_FOUND" {
		t.Fatal(err)
	}
}

// commandPath drops the program, the context selection and everything from
// the first flag on, leaving the command path a step names.
func commandPath(step string) []string {
	words := strings.Fields(step)[1:]
	if len(words) > 1 && words[0] == "--use-context" {
		words = words[2:]
	}
	for i, w := range words {
		if strings.HasPrefix(w, "-") {
			return words[:i]
		}
	}
	return words
}

// Recovery names the context listing and other commands whose names differ
// across the CLI family, so resolve every command and flag a step advertises
// against this command tree instead of trusting its spelling.
func TestAuthReuseRecoveryNamesRealCommands(t *testing.T) {
	steps := map[string][]string{}

	s, _, services := reuseFixture(t)
	_, err := reuseAuthentication(s, reuseOptions{From: "missing", DryRun: true}, services)
	steps["AUTH_REUSE_SOURCE_NOT_FOUND"] = cerrors.AsCLIError(err).NextSteps

	s, services = pairingFixture(t)
	_, err = reuseAuthentication(s, reuseOptions{DryRun: true}, services)
	steps["AUTH_REUSE_AMBIGUOUS"] = cerrors.AsCLIError(err).NextSteps

	personal := basicContext("personal", "existing-user")
	personal.BaseURL = "https://other.example.test/deploy"
	s, _, services = reuseFixture(t, personal, basicContext("team", ""))
	_, err = reuseAuthentication(s, reuseOptions{From: "personal", DryRun: true}, services)
	steps["AUTH_REUSE_SOURCE_MISMATCH"] = cerrors.AsCLIError(err).NextSteps

	s, _, services = reuseFixture(t, basicContext("personal", "existing-user"), patContext("team", reuseBase, ""))
	services.Verify = logins{}.verify
	r, _ := reuseAuthentication(s, reuseOptions{DryRun: true}, services)
	steps["unavailable"] = r.NextSteps

	s, _, services = reuseFixture(t)
	r, _ = reuseAuthentication(s, reuseOptions{DryRun: true}, services)
	steps["available"] = r.NextSteps
	r, _ = reuseAuthentication(s, reuseOptions{}, services)
	steps["reused"] = r.NextSteps

	if !strings.Contains(strings.Join(steps["AUTH_REUSE_AMBIGUOUS"], "\n"), "--from-scheme basic") {
		t.Fatalf("ambiguity does not offer the stored secret: %v", steps["AUTH_REUSE_AMBIGUOUS"])
	}
	for what, list := range steps {
		if len(list) == 0 {
			t.Fatalf("%s: no recovery command", what)
		}
		for _, step := range list {
			if !strings.HasPrefix(step, constants.AppName+" ") {
				t.Fatalf("%s: recovery %q is not a %s command", what, step, constants.AppName)
			}
			cmd, rest, findErr := NewRootCmd().Find(commandPath(step))
			if findErr != nil || len(rest) != 0 || !cmd.Runnable() {
				t.Errorf("%s: recovery %q does not resolve to a command (unresolved %v, %v)", what, step, rest, findErr)
				continue
			}
			for _, w := range strings.Fields(step) {
				name, isFlag := strings.CutPrefix(w, "--")
				if isFlag && cmd.Flags().Lookup(name) == nil && cmd.InheritedFlags().Lookup(name) == nil {
					t.Errorf("%s: recovery %q names an unknown flag %s", what, step, w)
				}
			}
		}
	}
}

func TestAuthReuseUsesPersistedDefaultsWithoutEnvironmentIdentity(t *testing.T) {
	// No stored scheme: both contexts resolve the built-in default (pat).
	s, store, services := reuseFixture(t, config.NamedContext{Name: "personal", BaseURL: reuseUpper}, config.NamedContext{Name: "team", BaseURL: reuseBase})
	store[auth.AccountKey(reuseUpper, "pat")] = "stored-secret"
	s.resolved.Secrets = config.Secrets{PAT: "environment-secret", Password: "environment-password"}
	services.Verify = func(ctx context.Context, cfg config.Config, cred auth.Credential) (apiclient.User, error) {
		if strings.HasPrefix(cred.Secret, "environment") {
			t.Fatal("verified an environment secret")
		}
		return logins{"\x00stored-secret": existingUser}.verify(ctx, cfg, cred)
	}
	r, err := reuseAuthentication(s, reuseOptions{}, services)
	if err != nil || r.State != "reused" || r.SourceContext != "personal" || r.Scheme != "pat" {
		t.Fatalf("stored defaults: %+v %v", r, err)
	}
	if got := targetOf(t, s); got.Auth.Scheme != "pat" || got.BaseURL != reuseUpper {
		t.Fatalf("destination %+v", got)
	}
}

func TestAuthReusePreservesWorkingTokenWithoutUsername(t *testing.T) {
	keyring.MockInit()
	alt := reuseUpperPort + "/"
	s, _, services := reuseFixture(t, patContext("personal", alt, "existing-user"), patContext("team", reuseBase, ""))
	store := auth.NewStore(s.cfgDir)
	existing := auth.Credential{Scheme: "pat", Secret: "destination-secret"}
	if _, err := auth.Save(reuseBase, existing, store); err != nil {
		t.Fatal(err)
	}
	if _, err := auth.Save(alt, auth.Credential{Scheme: "pat", Secret: "other-source-secret"}, store); err != nil {
		t.Fatal(err)
	}
	before, _, _ := config.ReadFile(s.cfgDir)
	services.Resolve = func(cfg config.Config) (auth.Credential, credstore.Lookup, error) {
		return auth.ResolveLookup(cfg, config.Secrets{}, store)
	}
	services.Lookup = store.Lookup
	services.Verify = func(_ context.Context, _ config.Config, credential auth.Credential) (apiclient.User, error) {
		if credential.Secret != existing.Secret {
			t.Fatal("attempted to switch an already valid destination login")
		}
		return existingUser, nil
	}
	r, err := reuseAuthentication(s, reuseOptions{}, services)
	if err != nil || r.Changed || !r.Verified || r.State != "unchanged" {
		t.Fatalf("existing token: %+v %v", r, err)
	}
	if after, _, _ := config.ReadFile(s.cfgDir); !reflect.DeepEqual(before, after) {
		t.Fatal("modified a valid token-only context")
	}
}

// A source with flavor auto stands for the flavor a probe detected, and it is
// verified under that flavor.
func TestAuthReuseMatchesPreviouslyDetectedFlavor(t *testing.T) {
	personal, team := basicContext("personal", "existing-user"), basicContext("team", "")
	personal.Flavor, personal.DetectedFlavor = "auto", "datacenter"
	team.Flavor = "datacenter"
	s, _, services := reuseFixture(t, personal, team)
	verify := services.Verify
	services.Verify = func(ctx context.Context, cfg config.Config, cred auth.Credential) (apiclient.User, error) {
		if cfg.Flavor != "datacenter" {
			t.Fatal("ignored verified deployment flavor")
		}
		return verify(ctx, cfg, cred)
	}
	r, err := reuseAuthentication(s, reuseOptions{}, services)
	if err != nil || r.State != "reused" {
		t.Fatalf("detected flavor: %+v %v", r, err)
	}
}

// The command verifies through the native client and reports the canonical
// account: the accountId on Cloud, the username on Data Center. Two logins the
// service signs in as one account are one login, not a choice.
func TestAuthReuseCommandVerifiesNativeStoredCredential(t *testing.T) {
	for _, flavor := range []string{"cloud", "datacenter"} {
		t.Run(flavor, func(t *testing.T) {
			keyring.MockInit()
			requests := 0
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				requests++
				user, password, ok := r.BasicAuth()
				if r.Method != http.MethodGet || !ok || (user != "existing-user" && user != "existing-user@example.test") || password != "retained-secret" {
					w.WriteHeader(http.StatusUnauthorized)
					return
				}
				w.Header().Set("Content-Type", "application/json")
				switch r.URL.Path {
				case "/deploy/rest/api/3/myself":
					_, _ = w.Write([]byte(`{"accountId":"5b10ac8d82e05b22cc7d4ef5","displayName":"Existing User","emailAddress":"existing-user@example.test"}`))
				case "/deploy/rest/api/2/myself":
					_, _ = w.Write([]byte(`{"name":"existing-user","key":"JIRAUSER10100","displayName":"Existing User","emailAddress":"existing-user@example.test"}`))
				default:
					w.WriteHeader(http.StatusNotFound)
				}
			}))
			defer server.Close()
			base := server.URL + "/deploy"
			contexts := []config.NamedContext{basicContext("personal", "existing-user"), basicContext("email", "existing-user@example.test"), basicContext("team", "")}
			for i := range contexts {
				contexts[i].BaseURL, contexts[i].Flavor = base, flavor
			}
			s, _, _ := reuseFixture(t, contexts...)
			s.store = auth.NewStore(s.cfgDir)
			credential := auth.Credential{Scheme: "basic", Username: "existing-user", Secret: "retained-secret"}
			if _, err := auth.Save(base, credential, s.store); err != nil {
				t.Fatal(err)
			}
			preview, err := reuseAuthentication(s, reuseOptions{DryRun: true}, s.reuseServices())
			want := map[string]string{"cloud": "5b10ac8d82e05b22cc7d4ef5", "datacenter": "existing-user"}[flavor]
			if err != nil || preview.State != "available" || preview.Account != want || preview.AccountName != "Existing User" {
				t.Fatalf("preview: %+v %v", preview, err)
			}
			command := newAuthReuseCmd(s)
			if err := command.RunE(command, nil); err != nil {
				t.Fatal(err)
			}
			if requests == 0 {
				t.Fatal("did not verify with the native API client")
			}
			after, _, _ := config.ReadFile(s.cfgDir)
			target, _ := after.Context("team")
			actual, err := auth.Resolve(config.StoredContext(target, after.Defaults), config.Secrets{}, auth.NewStore(s.cfgDir))
			if err != nil || actual.Username != preview.Username || actual.Secret != credential.Secret {
				t.Fatalf("native reuse unavailable after reload: %+v %v", actual, err)
			}
		})
	}
}

func TestAuthReusePreservesLegacyStoreKeyUnderEquivalentOverride(t *testing.T) {
	keyring.MockInit()
	legacy := reuseUpperPort + "/"
	personal := basicContext("personal", "existing-user")
	personal.BaseURL = legacy
	s, _, services := reuseFixture(t, personal, basicContext("team", ""))
	store := auth.NewStore(s.cfgDir)
	credential := auth.Credential{Scheme: "basic", Username: "existing-user", Secret: "retained-secret"}
	if _, err := auth.Save(legacy, credential, store); err != nil {
		t.Fatal(err)
	}
	services.Resolve = func(cfg config.Config) (auth.Credential, credstore.Lookup, error) {
		return auth.ResolveLookup(cfg, config.Secrets{}, store)
	}
	services.Lookup = store.Lookup
	services.Verify = func(context.Context, config.Config, auth.Credential) (apiclient.User, error) {
		return existingUser, nil
	}
	if _, err := reuseAuthentication(s, reuseOptions{}, services); err != nil {
		t.Fatal(err)
	}
	fresh, err := config.Load(config.LoadOptions{ConfigDir: s.cfgDir, Context: "team", DotenvPath: filepath.Join(t.TempDir(), "absent"), Flags: config.FlagValues{BaseURL: reuseBase}})
	if err != nil {
		t.Fatal(err)
	}
	got, err := auth.Resolve(fresh.Config, config.Secrets{}, auth.NewStore(s.cfgDir))
	if err != nil || got.Secret != credential.Secret || got.Username != credential.Username {
		t.Fatalf("equivalent override lost native credential: %v", err)
	}
	if fresh.Config.BaseURL != reuseBase {
		t.Fatal("changed request destination")
	}
	if err := auth.ForgetForConfig(fresh.Config, fresh.Config.Auth.Scheme, store); err != nil {
		t.Fatal(err)
	}
	if _, err := store.Load(auth.AccountKey(legacy, credential.Scheme)); err == nil {
		t.Fatal("logout left the original credential active")
	}
	fresh.Config.BaseURL = "https://service.example.test/other"
	_, err = auth.Resolve(fresh.Config, config.Secrets{}, store)
	if errorCode(err) != "CREDENTIAL_SERVICE_MISMATCH" {
		t.Fatalf("foreign deployment reused stored key: %v", err)
	}
}

// A rejected credential leads with reuse when a stored login could replace
// it; other errors, and reuse and login themselves, keep their steps.
func TestRejectedCredentialRecoveryLeadsWithReuse(t *testing.T) {
	step := "jira-cli --use-context 'team' auth reuse"
	s := &appState{resolved: &config.Resolved{Config: config.Config{BaseURL: reuseBase, Auth: config.AuthConfig{Scheme: "pat"}, ReuseStep: step}}}
	rejected := func() *cerrors.CLIError {
		return cerrors.New(cerrors.CategoryAuth, "HTTP_UNAUTHORIZED", "rejected").WithHTTPStatus(401)
	}
	ce := rejected()
	withReuseRecovery(ce, s, &cobra.Command{Use: "search"})
	if len(ce.NextSteps) < 2 || ce.NextSteps[0] != step {
		t.Fatalf("next steps %v", ce.NextSteps)
	}
	for _, name := range []string{"reuse", "login"} {
		ce := rejected()
		steps := append([]string(nil), ce.NextSteps...)
		withReuseRecovery(ce, s, &cobra.Command{Use: name})
		if !reflect.DeepEqual(steps, ce.NextSteps) {
			t.Fatalf("%s gained a reuse step", name)
		}
	}
	forbidden := cerrors.New(cerrors.CategoryPermission, "FORBIDDEN", "denied").WithHTTPStatus(403)
	steps := append([]string(nil), forbidden.NextSteps...)
	withReuseRecovery(forbidden, s, &cobra.Command{Use: "search"})
	if !reflect.DeepEqual(steps, forbidden.NextSteps) {
		t.Fatal("a permission error gained a reuse step")
	}
}

// doctor offers reuse when the context's credential is missing, after the host
// retry and before the step that configures credentials.
func TestDoctorOffersReuseForAMissingCredential(t *testing.T) {
	keyring.MockInit()
	step := "jira-cli --use-context 'team' auth reuse"
	s, _, _ := reuseFixture(t, basicContext("personal", "existing-user"), patContext("team", reuseBase, ""))
	s.store = auth.NewStore(s.cfgDir)
	s.resolved.Config.ReuseStep = step
	report := runDoctor(s, true)
	if report.Healthy || !reflect.DeepEqual(report.NextSteps, []string{step}) {
		t.Fatalf("report %+v", report)
	}
	s.resolved.Config.ReuseStep = ""
	if report = runDoctor(s, true); report.NextSteps != nil {
		t.Fatalf("offered reuse without a reuse step: %+v", report.NextSteps)
	}
}
