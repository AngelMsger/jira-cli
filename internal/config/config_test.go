package config

import (
	"os"
	"path/filepath"
	"reflect"
	"testing"
	"time"

	cerrors "github.com/angelmsger/jira-cli/pkg/errors"
)

func writeFile(t *testing.T, path, content string) {
	t.Helper()
	if err := os.WriteFile(path, []byte(content), 0o600); err != nil {
		t.Fatal(err)
	}
}

func TestLoadDefaults(t *testing.T) {
	cleanSetupEnv(t)
	dir := t.TempDir()
	got, err := Load(LoadOptions{ConfigDir: dir, DotenvPath: filepath.Join(dir, "absent.env")})
	if err != nil {
		t.Fatal(err)
	}
	want := Config{Flavor: FlavorAuto, Auth: AuthConfig{Scheme: SchemePAT},
		Defaults: Defaults{Format: "json", PageSize: 25, Timeout: 30 * time.Second, MaxRetries: 3}}
	if !reflect.DeepEqual(got.Config, want) || got.Sources[fieldAuthScheme] != "default" {
		t.Errorf("defaults = %+v (%v)", got.Config, got.Sources)
	}
}

// TestSchema pins jira-cli's environment variables and scheme inference.
func TestSchema(t *testing.T) {
	cleanSetupEnv(t)
	dir := t.TempDir()
	load := func() *Resolved {
		t.Helper()
		r, err := Load(LoadOptions{ConfigDir: dir, DotenvPath: filepath.Join(dir, "absent.env")})
		if err != nil {
			t.Fatal(err)
		}
		return r
	}
	t.Setenv("JIRA_SERVER", "https://jira.example.com")
	t.Setenv("JIRA_FLAVOR", FlavorDataCenter)
	t.Setenv("JIRA_USERNAME", "alice")
	t.Setenv("JIRA_CREDENTIAL_URL", "https://help.example.com/token")
	t.Setenv("JIRA_FORMAT", "table")
	t.Setenv("JIRA_DEFAULT_PROJECT", "ENG")
	t.Setenv("JIRA_CLI_READ_ONLY", "yes")
	t.Setenv("JIRA_API_TOKEN", "api")
	r := load()
	if r.Config.BaseURL != "https://jira.example.com" || r.Config.Flavor != FlavorDataCenter || r.Config.Auth.Username != "alice" ||
		r.Config.Auth.CredentialURL != "https://help.example.com/token" || r.Config.Defaults.Format != "table" ||
		r.Config.Defaults.Project != "ENG" || !r.Config.Defaults.ReadOnly ||
		r.Config.Auth.Scheme != SchemeBasic || r.Secrets != (Secrets{APIToken: "api"}) {
		t.Fatalf("environment = %+v %+v", r.Config, r.Secrets)
	}
	t.Setenv("JIRA_PASSWORD", "pw")
	if r = load(); r.Config.Auth.Scheme != SchemeBasic || r.Secrets.Password != "pw" {
		t.Errorf("a password selects basic: %+v", r)
	}
	t.Setenv("JIRA_TOKEN", "short")
	if r = load(); r.Config.Auth.Scheme != SchemePAT || r.Secrets.PAT != "short" {
		t.Errorf("a PAT wins over a password: %q %q", r.Config.Auth.Scheme, r.Secrets.PAT)
	}
	t.Setenv("JIRA_PERSONAL_ACCESS_TOKEN", "long")
	if r = load(); r.Secrets.PAT != "long" {
		t.Errorf("JIRA_PERSONAL_ACCESS_TOKEN wins over JIRA_TOKEN: %q", r.Secrets.PAT)
	}
	t.Setenv("JIRA_AUTH_SCHEME", SchemeBasic)
	if r = load(); r.Config.Auth.Scheme != SchemeBasic {
		t.Errorf("an explicit scheme wins: %q", r.Config.Auth.Scheme)
	}
}

// TestCloudRules pins the hooks jira-cli registers: an unset scheme is basic
// on Cloud, a cached flavor is dropped for another server, and setup rejects a
// bad flavor and a PAT on Cloud.
func TestCloudRules(t *testing.T) {
	cleanSetupEnv(t)
	dir := t.TempDir()
	writeFile(t, ConfigFilePath(dir), `contexts:
  - name: cloud
    server: https://acme.atlassian.net
  - name: cached
    server: https://Jira.Example.com:443/
    detected_flavor: cloud
current_context: cloud
`)
	load := func(opt LoadOptions) *Resolved {
		t.Helper()
		opt.ConfigDir, opt.DotenvPath = dir, filepath.Join(dir, "absent.env")
		r, err := Load(opt)
		if err != nil {
			t.Fatal(err)
		}
		return r
	}
	if r := load(LoadOptions{}); r.Config.Auth.Scheme != SchemeBasic || r.Sources[fieldAuthScheme] != "default" {
		t.Errorf("Cloud default scheme = %q (%s)", r.Config.Auth.Scheme, r.Sources[fieldAuthScheme])
	}
	if r := load(LoadOptions{Flags: FlagValues{AuthScheme: SchemePAT}}); r.Config.Auth.Scheme != SchemePAT {
		t.Errorf("an explicit scheme stays: %q", r.Config.Auth.Scheme)
	}
	r := load(LoadOptions{Context: "cached", Flags: FlagValues{BaseURL: "https://jira.example.com"}})
	if r.Config.DetectedFlavor != FlavorCloud || r.Config.Auth.Scheme != SchemeBasic ||
		r.Config.CredentialBaseURL != "https://Jira.Example.com:443/" {
		t.Errorf("an equivalent spelling keeps the cache and the stored key: %+v", r.Config)
	}
	r = load(LoadOptions{Context: "cached", Flags: FlagValues{BaseURL: "https://other.example.com"}})
	if r.Config.DetectedFlavor != "" || r.Config.Auth.Scheme != SchemePAT || r.Config.CredentialBaseURL != "" {
		t.Errorf("another server drops the cache: %+v", r.Config)
	}
	if got := StoredContext(NamedContext{Name: "x", BaseURL: "https://acme.atlassian.net"}, Defaults{}); got.Auth.Scheme != SchemeBasic {
		t.Errorf("stored Cloud context scheme = %q", got.Auth.Scheme)
	}

	for _, tc := range []struct {
		cfg  Config
		code string
	}{
		{Config{BaseURL: "https://jira.example.com", Flavor: "server", Auth: AuthConfig{Scheme: SchemePAT}}, "BAD_FLAVOR"},
		{Config{BaseURL: "https://acme.atlassian.net", Auth: AuthConfig{Scheme: SchemePAT}}, "AUTH_CLOUD_NEEDS_BASIC"},
		{Config{BaseURL: "https://jira.example.com", Flavor: FlavorCloud, Auth: AuthConfig{Scheme: SchemePAT}}, "AUTH_CLOUD_NEEDS_BASIC"},
		{Config{BaseURL: "https://jira.example.com", Auth: AuthConfig{Scheme: "bearer"}}, "AUTH_BAD_SCHEME"},
		{Config{BaseURL: "https://jira.example.com", Flavor: FlavorDataCenter, Auth: AuthConfig{Scheme: SchemePAT}}, ""},
	} {
		err := ValidateService(tc.cfg)
		if code := cerrors.AsCLIError(err); (code == nil) != (tc.code == "") || (code != nil && code.Code != tc.code) {
			t.Errorf("ValidateService(%+v) = %v, want %q", tc.cfg, err, tc.code)
		}
	}
	if ce := cerrors.AsCLIError(ValidateService(Config{BaseURL: "https://jira.example.com"})); ce.Hint != "Use one of: pat, basic." {
		t.Errorf("scheme hint = %q", ce.Hint)
	}
}

// TestFileFormat pins the bytes of config.yaml. The engine is shared; the
// shape is jira-cli's (project first under defaults), and existing files must
// keep reading and writing the same way.
func TestFileFormat(t *testing.T) {
	dir := filepath.Join(t.TempDir(), "cfg")
	in := File{
		CurrentContext: "prod",
		Contexts: []NamedContext{
			{Name: "default", BaseURL: "https://dc.example.com", Flavor: FlavorDataCenter, Auth: AuthConfig{Scheme: SchemePAT}},
			{Name: "prod", BaseURL: "https://acme.atlassian.net", Flavor: FlavorAuto, DetectedFlavor: FlavorCloud,
				Auth: AuthConfig{Scheme: SchemeBasic, Username: "alice@acme.com", CredentialURL: "https://help.example.com/t"}},
		},
		Defaults: Defaults{Format: "table", PageSize: 50, Timeout: 90 * time.Second, MaxRetries: 5, Project: "ENG", ReadOnly: true},
	}
	if err := WriteFile(dir, in); err != nil {
		t.Fatal(err)
	}
	raw, err := os.ReadFile(ConfigFilePath(dir))
	if err != nil {
		t.Fatal(err)
	}
	const want = `current_context: prod
contexts:
    - name: default
      server: https://dc.example.com
      flavor: datacenter
      auth:
        scheme: pat
    - name: prod
      server: https://acme.atlassian.net
      flavor: auto
      detected_flavor: cloud
      auth:
        credential_url: https://help.example.com/t
        scheme: basic
        username: alice@acme.com
defaults:
    project: ENG
    format: table
    page_size: 50
    timeout: 1m30s
    max_retries: 5
    read_only: true
`
	if string(raw) != want {
		t.Errorf("config.yaml changed:\n%s", raw)
	}
	got, ok, err := ReadFile(dir)
	if err != nil || !ok || !reflect.DeepEqual(got, in) {
		t.Errorf("read back %+v, %v, %v", got, ok, err)
	}
}

// TestLegacyFlatFile pins the pre-context format: root fields become one
// context named "default", and the next write uses the context format.
func TestLegacyFlatFile(t *testing.T) {
	cleanSetupEnv(t)
	dir := t.TempDir()
	writeFile(t, ConfigFilePath(dir),
		"server: https://legacy.example.com\nflavor: datacenter\ndetected_flavor: datacenter\nauth:\n  scheme: basic\n  username: bob\ndefaults:\n  page_size: 10\n")
	got, exists, err := ReadFile(dir)
	if err != nil || !exists {
		t.Fatalf("ReadFile: exists=%v err=%v", exists, err)
	}
	want := File{CurrentContext: DefaultContextName, Defaults: Defaults{PageSize: 10}, Contexts: []NamedContext{{
		Name: DefaultContextName, BaseURL: "https://legacy.example.com", Flavor: FlavorDataCenter,
		DetectedFlavor: FlavorDataCenter, Auth: AuthConfig{Scheme: SchemeBasic, Username: "bob"}}}}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("legacy file = %+v", got)
	}
	r, err := Load(LoadOptions{ConfigDir: dir, DotenvPath: filepath.Join(dir, "absent.env")})
	if err != nil {
		t.Fatal(err)
	}
	if r.Config.BaseURL != "https://legacy.example.com" || r.Sources[fieldServer] != "file" ||
		r.ActiveContext != DefaultContextName || r.ContextSource != ContextSourceCurrent {
		t.Errorf("legacy load = %+v", r)
	}
	if err := WriteFile(dir, got); err != nil {
		t.Fatal(err)
	}
	raw, _ := os.ReadFile(ConfigFilePath(dir))
	const rewritten = `current_context: default
contexts:
    - name: default
      server: https://legacy.example.com
      flavor: datacenter
      detected_flavor: datacenter
      auth:
        scheme: basic
        username: bob
defaults:
    page_size: 10
`
	if string(raw) != rewritten {
		t.Errorf("rewritten legacy file:\n%s", raw)
	}
	writeFile(t, ConfigFilePath(dir), "flavor: cloud\nauth:\n  scheme: basic\n")
	if got, _, _ := ReadFile(dir); len(got.Contexts) != 0 || got.CurrentContext != "" {
		t.Errorf("a flat file without a server has no context: %+v", got)
	}
}

// TestUnknownContextNamesTheListing pins the wording of UNKNOWN_CONTEXT.
func TestUnknownContextNamesTheListing(t *testing.T) {
	cleanSetupEnv(t)
	dir := t.TempDir()
	writeFile(t, ConfigFilePath(dir), "contexts:\n  - name: alpha\n  - name: beta\n")
	_, err := Load(LoadOptions{ConfigDir: dir, DotenvPath: filepath.Join(dir, "absent.env"), Context: "ghost"})
	ce := cerrors.AsCLIError(err)
	if ce == nil || ce.Code != "UNKNOWN_CONTEXT" || ce.Message != `context "ghost" (from --use-context) is not defined in the config file` ||
		ce.Hint != "Available contexts: alpha, beta." || !reflect.DeepEqual(ce.NextSteps, []string{"jira-cli config get-contexts"}) {
		t.Fatalf("UNKNOWN_CONTEXT = %+v", ce)
	}
	if hint := UnknownContextHint("ghost", nil); hint != "Run `jira-cli config get-contexts` to list defined contexts." {
		t.Errorf("empty hint = %q", hint)
	}
}

// TestResolveConfigDir covers the default config-dir resolution used when
// --config was not supplied. It overrides $HOME with a temp directory so the
// real user's config is never touched.
func TestResolveConfigDir(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("USERPROFILE", home)
	dir, err := ResolveConfigDir()
	if err != nil {
		t.Fatal(err)
	}
	if want := filepath.Join(home, ".angelmsger", "jira"); dir != want {
		t.Errorf("dir = %q, want %q", dir, want)
	}
}
