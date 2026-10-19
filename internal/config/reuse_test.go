// An external test package, so it can register jira-cli's auth schemes by
// importing internal/auth, which imports this package.
package config_test

import (
	"path/filepath"
	"strings"
	"testing"

	_ "github.com/angelmsger/jira-cli/internal/auth" // registers the auth schemes
	"github.com/angelmsger/jira-cli/internal/config"
)

// cleanReuseEnv clears every variable that could supply an identity or a
// service override.
func cleanReuseEnv(t *testing.T) {
	t.Helper()
	for _, key := range []string{"JIRA_SERVER", "JIRA_FLAVOR", "JIRA_CREDENTIAL_URL", "JIRA_AUTH_SCHEME", "JIRA_USERNAME",
		"JIRA_PERSONAL_ACCESS_TOKEN", "JIRA_TOKEN", "JIRA_PASSWORD", "JIRA_API_TOKEN", "JIRA_CONTEXT"} {
		t.Setenv(key, "")
	}
}

func TestLoadOffersReuseBeforeSignIn(t *testing.T) {
	cleanReuseEnv(t)
	dir := t.TempDir()
	file := config.File{CurrentContext: "team", Contexts: []config.NamedContext{
		{Name: "personal", BaseURL: "https://jira.example.test", Flavor: config.FlavorDataCenter, Auth: config.AuthConfig{Scheme: config.SchemeBasic, Username: "me"}},
		{Name: "other", BaseURL: "https://JIRA.example.test", Flavor: config.FlavorDataCenter, Auth: config.AuthConfig{Scheme: config.SchemePAT}},
		{Name: "team", BaseURL: "https://jira.example.test", Flavor: config.FlavorDataCenter, Auth: config.AuthConfig{Scheme: config.SchemePAT}},
	}}
	if err := config.WriteFile(dir, file); err != nil {
		t.Fatal(err)
	}
	load := func(opt config.LoadOptions) *config.Resolved {
		t.Helper()
		opt.ConfigDir, opt.DotenvPath = dir, filepath.Join(t.TempDir(), "absent.env")
		r, err := config.Load(opt)
		if err != nil {
			t.Fatal(err)
		}
		return r
	}
	r := load(config.LoadOptions{})
	if r.Config.ReuseStep != "jira-cli --use-context 'team' auth reuse" {
		t.Fatalf("reuse step %q", r.Config.ReuseStep)
	}
	g, err := config.Guide(r.Config, r.Sources)
	if err != nil || g.NextSteps[0] != r.Config.ReuseStep || !strings.Contains(g.Instructions[0], "auth reuse") {
		t.Fatalf("guide %+v %v", g, err)
	}
	if r = load(config.LoadOptions{Context: "personal"}); r.Config.ReuseStep != "" {
		t.Fatalf("a context with an identity was offered reuse: %q", r.Config.ReuseStep)
	}
	// A supplied scheme counts only sources under that scheme, and the step
	// keeps the flag.
	if r = load(config.LoadOptions{Flags: config.FlagValues{AuthScheme: config.SchemePAT}}); r.Config.ReuseStep != "jira-cli --use-context 'team' auth reuse --auth-scheme pat" {
		t.Fatalf("supplied scheme: %q", r.Config.ReuseStep)
	}
	if r = load(config.LoadOptions{Context: "other", Flags: config.FlagValues{AuthScheme: config.SchemePAT, BaseURL: "https://JIRA.example.test"}}); r.Config.ReuseStep != "jira-cli --use-context 'other' auth reuse --auth-scheme pat" {
		t.Fatalf("supplied scheme from the other spelling: %q", r.Config.ReuseStep)
	}
	if r = load(config.LoadOptions{Flags: config.FlagValues{BaseURL: "https://other.example.test"}}); r.Config.ReuseStep != "" {
		t.Fatalf("an override was offered reuse: %q", r.Config.ReuseStep)
	}
	t.Setenv("JIRA_USERNAME", "someone")
	if r = load(config.LoadOptions{}); r.Config.ReuseStep != "" {
		t.Fatalf("a runtime username was offered reuse: %q", r.Config.ReuseStep)
	}
}

func TestReuseServiceComparesEffectiveFlavor(t *testing.T) {
	base := config.Config{BaseURL: "https://jira.example.test/", Flavor: config.FlavorAuto, DetectedFlavor: config.FlavorDataCenter}
	pinned := config.Config{BaseURL: "https://JIRA.example.test:443", Flavor: config.FlavorDataCenter}
	if config.ReuseService(base) == "" || config.ReuseService(base) != config.ReuseService(pinned) {
		t.Fatalf("auto with a detected flavor: %q vs %q", config.ReuseService(base), config.ReuseService(pinned))
	}
	pinned.Flavor = config.FlavorCloud
	if config.ReuseService(base) == config.ReuseService(pinned) {
		t.Fatal("different flavors matched")
	}
	if config.ReuseService(config.Config{BaseURL: "://bad"}) != "" {
		t.Fatal("an unusable URL identified a service")
	}
}

func TestSetupPlanLeadsWithReuse(t *testing.T) {
	cleanReuseEnv(t)
	resolved := &config.Resolved{Config: config.Config{BaseURL: "https://jira.example.test", Flavor: config.FlavorDataCenter, Auth: config.AuthConfig{Scheme: config.SchemePAT}},
		Sources: map[string]string{config.FieldServer: "flag", config.FieldFlavor: "flag", config.FieldAuthScheme: "flag"}}
	plan, err := config.PlanServiceContext(config.File{}, "team", resolved, false, false)
	if err != nil || len(plan.NextSteps) == 0 || plan.NextSteps[0] != "jira-cli --use-context 'team' auth reuse" {
		t.Fatalf("plan %+v %v", plan.NextSteps, err)
	}
}
