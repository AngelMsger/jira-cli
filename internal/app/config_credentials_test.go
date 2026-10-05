package app

import (
	"errors"
	"os"
	"runtime"
	"testing"

	"github.com/angelmsger/jira-cli/internal/auth"
	"github.com/angelmsger/jira-cli/internal/config"
	"github.com/zalando/go-keyring"
)

// A stored secret is keyed by the server's host and the authentication scheme,
// so a team preset and a personal context on one Jira site share it — `auth
// reuse` associates an identity on exactly that basis, without copying the
// secret. Removing a context, or re-spelling its URL, must therefore never
// take away a secret another context still resolves.

func storedSecret(t *testing.T, cfgDir, baseURL, scheme string) (string, error) {
	t.Helper()
	return auth.NewStore(cfgDir).Load(auth.AccountKey(baseURL, scheme))
}

func tokenContext(name, baseURL string) config.NamedContext {
	return config.NamedContext{Name: name, BaseURL: baseURL, Flavor: config.FlavorDataCenter,
		Auth: config.AuthConfig{Scheme: config.SchemePAT}}
}

func saveToken(t *testing.T, s *appState, baseURL, secret string) {
	t.Helper()
	if _, err := auth.Save(baseURL, auth.Credential{Scheme: auth.SchemePAT, Secret: secret}, s.store); err != nil {
		t.Fatal(err)
	}
}

// A team preset stores the URL without a trailing slash; a URL pasted into the
// wizard often carries one. Re-running the wizard across that difference must
// not delete the credential it has just saved.
func TestConfigInitKeepsTheCredentialWhenOnlyTheURLSpellingChanges(t *testing.T) {
	keyring.MockInit()
	s, _, _ := loginStateForTest(t)
	existing := config.File{CurrentContext: "default", Contexts: []config.NamedContext{tokenContext("default", "https://jira.example.test")}}
	edited := tokenContext("default", "https://jira.example.test/")
	result := &config.WizardResult{
		File:  config.File{CurrentContext: "default", Contexts: []config.NamedContext{edited}},
		Creds: []config.ContextResult{{Context: edited, Secrets: config.Secrets{PAT: "personal-token"}}},
	}
	if _, err := persistInitResult(s, result, existing); err != nil {
		t.Fatal(err)
	}
	if got, err := storedSecret(t, s.cfgDir, edited.BaseURL, auth.SchemePAT); err != nil || got != "personal-token" {
		t.Fatalf("the wizard deleted the credential it had just saved: %q %v", got, err)
	}
}

// Moving a context to another server still clears the secret nothing uses any
// more, and leaves one that a second context on the old server resolves.
func TestConfigInitForgetsOnlyCredentialsNoContextUses(t *testing.T) {
	for _, shared := range []bool{false, true} {
		keyring.MockInit()
		s, _, _ := loginStateForTest(t)
		old := tokenContext("default", "https://old.example.test/jira")
		saveToken(t, s, old.BaseURL, "old-token")
		existing := config.File{CurrentContext: "default", Contexts: []config.NamedContext{old}}
		moved := tokenContext("default", "https://new.example.test/jira")
		file := config.File{CurrentContext: "default", Contexts: []config.NamedContext{moved}}
		if shared {
			other := tokenContext("other", "https://old.example.test/")
			existing.Contexts = append(existing.Contexts, other)
			file.Contexts = append([]config.NamedContext{other}, moved)
		}
		result := &config.WizardResult{File: file, Creds: []config.ContextResult{{Context: moved, Secrets: config.Secrets{PAT: "new-token"}}}}
		if _, err := persistInitResult(s, result, existing); err != nil {
			t.Fatal(err)
		}
		if got, err := storedSecret(t, s.cfgDir, moved.BaseURL, auth.SchemePAT); err != nil || got != "new-token" {
			t.Fatalf("shared=%v: new credential missing: %q %v", shared, got, err)
		}
		got, err := storedSecret(t, s.cfgDir, old.BaseURL, auth.SchemePAT)
		if shared && (err != nil || got != "old-token") {
			t.Fatalf("a credential another context still uses was deleted: %q %v", got, err)
		}
		if !shared && !errors.Is(err, auth.ErrSecretNotFound) {
			t.Fatalf("an unused credential was left behind: %q %v", got, err)
		}
	}
}

// Switching a context's scheme leaves the old scheme's secret orphaned unless
// another context on that server still authenticates with it.
func TestConfigInitSchemeChangeForgetsOnlyTheUnusedSecret(t *testing.T) {
	for _, shared := range []bool{false, true} {
		keyring.MockInit()
		s, _, _ := loginStateForTest(t)
		old := tokenContext("default", "https://jira.example.test")
		saveToken(t, s, old.BaseURL, "old-token")
		existing := config.File{CurrentContext: "default", Contexts: []config.NamedContext{old}}
		edited := old
		edited.Auth = config.AuthConfig{Scheme: config.SchemeBasic, Username: "alice"}
		file := config.File{CurrentContext: "default", Contexts: []config.NamedContext{edited}}
		if shared {
			other := tokenContext("team", "https://jira.example.test/")
			existing.Contexts = append(existing.Contexts, other)
			file.Contexts = append(file.Contexts, other)
		}
		result := &config.WizardResult{File: file, Creds: []config.ContextResult{{Context: edited, Secrets: config.Secrets{Password: "new-password"}}}}
		if _, err := persistInitResult(s, result, existing); err != nil {
			t.Fatal(err)
		}
		if got, err := storedSecret(t, s.cfgDir, edited.BaseURL, auth.SchemeBasic); err != nil || got != "new-password" {
			t.Fatalf("shared=%v: new credential missing: %q %v", shared, got, err)
		}
		got, err := storedSecret(t, s.cfgDir, old.BaseURL, auth.SchemePAT)
		if shared && (err != nil || got != "old-token") {
			t.Fatalf("a credential another context still uses was deleted: %q %v", got, err)
		}
		if !shared && !errors.Is(err, auth.ErrSecretNotFound) {
			t.Fatalf("an unused credential was left behind: %q %v", got, err)
		}
	}
}

func TestDeleteContextKeepsACredentialAnotherContextUses(t *testing.T) {
	keyring.MockInit()
	s, _, _ := loginStateForTest(t)
	file := config.File{CurrentContext: "default", Contexts: []config.NamedContext{
		tokenContext("default", "https://jira.example.test/"),
		// A preset carries no scheme of its own; on Data Center it resolves to
		// the token scheme and so to the personal context's secret.
		{Name: "team", BaseURL: "https://jira.example.test", Flavor: config.FlavorDataCenter},
		tokenContext("elsewhere", "https://elsewhere.example.test/jira"),
	}}
	if err := config.WriteFile(s.cfgDir, file); err != nil {
		t.Fatal(err)
	}
	for _, c := range []config.NamedContext{file.Contexts[0], file.Contexts[2]} {
		saveToken(t, s, c.BaseURL, "token-for-"+c.Name)
	}
	// The preset shares the personal context's server, and therefore its secret.
	if _, err := runCLIFile(t, s.cfgDir, "config", "delete-context", "team"); err != nil {
		t.Fatal(err)
	}
	if got, err := storedSecret(t, s.cfgDir, "https://jira.example.test/", auth.SchemePAT); err != nil || got != "token-for-default" {
		t.Fatalf("deleting the preset removed the personal context's credential: %q %v", got, err)
	}
	// A context that is the only user of its server takes its secret with it.
	if _, err := runCLIFile(t, s.cfgDir, "config", "delete-context", "elsewhere"); err != nil {
		t.Fatal(err)
	}
	if got, err := storedSecret(t, s.cfgDir, "https://elsewhere.example.test/jira", auth.SchemePAT); !errors.Is(err, auth.ErrSecretNotFound) {
		t.Fatalf("an unused credential was left behind: %q %v", got, err)
	}
	after, _, _ := config.ReadFile(s.cfgDir)
	if len(after.Contexts) != 1 || after.Contexts[0].Name != "default" {
		t.Fatalf("unexpected contexts: %+v", after)
	}
}

// A context on the same host that authenticates with another scheme keeps its
// own secret, and does not keep the deleted context's secret alive.
func TestDeleteContextScopesTheSharedCredentialByScheme(t *testing.T) {
	keyring.MockInit()
	s, _, _ := loginStateForTest(t)
	basic := config.NamedContext{Name: "basic", BaseURL: "https://jira.example.test", Flavor: config.FlavorDataCenter,
		Auth: config.AuthConfig{Scheme: config.SchemeBasic, Username: "alice"}}
	file := config.File{CurrentContext: "basic", Contexts: []config.NamedContext{basic, tokenContext("token", "https://jira.example.test")}}
	if err := config.WriteFile(s.cfgDir, file); err != nil {
		t.Fatal(err)
	}
	saveToken(t, s, basic.BaseURL, "the-token")
	if _, err := auth.Save(basic.BaseURL, auth.Credential{Scheme: auth.SchemeBasic, Username: "alice", Secret: "the-password"}, s.store); err != nil {
		t.Fatal(err)
	}
	if _, err := runCLIFile(t, s.cfgDir, "config", "delete-context", "token"); err != nil {
		t.Fatal(err)
	}
	if got, err := storedSecret(t, s.cfgDir, basic.BaseURL, auth.SchemePAT); !errors.Is(err, auth.ErrSecretNotFound) {
		t.Fatalf("the deleted context's secret was left behind: %q %v", got, err)
	}
	if got, err := storedSecret(t, s.cfgDir, basic.BaseURL, auth.SchemeBasic); err != nil || got != "the-password" {
		t.Fatalf("a secret under another scheme was deleted: %q %v", got, err)
	}
}

// A failed config write must not cost the credential: the context is still in
// the file and still resolves it.
func TestDeleteContextKeepsTheCredentialWhenTheConfigWriteFails(t *testing.T) {
	if runtime.GOOS == "windows" || os.Geteuid() == 0 {
		t.Skip("a read-only directory does not block file creation for this user")
	}
	keyring.MockInit()
	s, _, _ := loginStateForTest(t)
	target := tokenContext("target", "https://target.example.test")
	file := config.File{CurrentContext: "default", Contexts: []config.NamedContext{tokenContext("default", "https://jira.example.test"), target}}
	if err := config.WriteFile(s.cfgDir, file); err != nil {
		t.Fatal(err)
	}
	saveToken(t, s, target.BaseURL, "target-token")
	// The config is replaced through a temporary file in the same directory,
	// so a read-only directory fails the write while the read still succeeds.
	if err := os.Chmod(s.cfgDir, 0o500); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.Chmod(s.cfgDir, 0o700) })
	if _, err := runCLIFile(t, s.cfgDir, "config", "delete-context", "target"); err == nil {
		t.Fatal("a failed config write was reported as success")
	}
	if got, err := storedSecret(t, s.cfgDir, target.BaseURL, auth.SchemePAT); err != nil || got != "target-token" {
		t.Fatalf("a failed delete removed a credential the context still names: %q %v", got, err)
	}
}
