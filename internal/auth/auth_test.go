package auth

import (
	"errors"
	"os"
	"reflect"
	"testing"

	kit "github.com/angelmsger/agent-cli-kit/auth"
	"github.com/angelmsger/agent-cli-kit/credstore"
	"github.com/angelmsger/jira-cli/internal/config"
	"github.com/angelmsger/jira-cli/pkg/constants"
	cerrors "github.com/angelmsger/jira-cli/pkg/errors"
	"github.com/zalando/go-keyring"
)

// failingKeyring is a keychain backend whose every call fails with err.
type failingKeyring struct{ err error }

func (f failingKeyring) Get(string, string) (string, error) { return "", f.err }
func (f failingKeyring) Set(string, string, string) error   { return f.err }
func (f failingKeyring) Delete(string, string) error        { return f.err }

// newStoreWithKeyring returns jira-cli's Store over a substitute keychain
// backend. The store's own behavior is tested in agent-cli-kit/credstore.
func newStoreWithKeyring(dir string, backend credstore.Backend) *Store {
	return credstore.NewStoreWithBackend(dir, constants.KeychainService, backend)
}

func TestMain(m *testing.M) {
	// Use an in-memory keychain for all tests in this package.
	keyring.MockInit()
	os.Exit(m.Run())
}

// TestCredentialValidate checks jira-cli's two schemes and the codes and
// messages each reports when its secret is missing.
func TestCredentialValidate(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name    string
		cred    Credential
		code    string
		message string
	}{
		{"pat ok", Credential{Scheme: SchemePAT, Secret: "tok"}, "", ""},
		{"pat missing secret", Credential{Scheme: SchemePAT}, "AUTH_NO_TOKEN", "no Personal Access Token configured"},
		{"basic ok", Credential{Scheme: SchemeBasic, Username: "u", Secret: "p"}, "", ""},
		{"basic missing user", Credential{Scheme: SchemeBasic, Secret: "p"}, "AUTH_NO_BASIC", "basic auth requires both a username and a password/token"},
		{"basic missing secret", Credential{Scheme: SchemeBasic, Username: "u"}, "AUTH_NO_BASIC", "basic auth requires both a username and a password/token"},
		{"unknown scheme", Credential{Scheme: "oauth", Secret: "x"}, "AUTH_BAD_SCHEME", `unknown auth scheme "oauth"`},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			err := tc.cred.Validate()
			if tc.code == "" {
				if err != nil {
					t.Fatalf("Validate() = %v, want nil", err)
				}
				return
			}
			ce := cerrors.AsCLIError(err)
			if err == nil || ce.Code != tc.code || ce.Message != tc.message || ce.Category != cerrors.CategoryConfig {
				t.Fatalf("Validate() = %+v, want %s %q in config", ce, tc.code, tc.message)
			}
		})
	}
}

// TestSchemeHeaders checks which Authorization form each of jira-cli's schemes
// sends: a PAT as Bearer, basic as username:secret. The encoding itself is
// tested in agent-cli-kit/auth.
func TestSchemeHeaders(t *testing.T) {
	t.Parallel()
	if got := (Credential{Scheme: SchemePAT, Username: "alice", Secret: "abc"}).Header(); got != "Bearer abc" {
		t.Errorf("pat header = %q, want Bearer abc", got)
	}
	if got, want := (Credential{Scheme: SchemeBasic, Username: "alice", Secret: "pw"}).Header(), kit.BasicHeader("alice", "pw"); got != want {
		t.Errorf("basic header = %q, want %q", got, want)
	}
}

// TestAccountKey checks that stored secrets stay keyed by host alone, so a
// base URL with a context path finds the entry an earlier release wrote.
func TestAccountKey(t *testing.T) {
	t.Parallel()
	got := AccountKey("https://kms.example.com/wiki", SchemePAT)
	if got != "kms.example.com:pat" {
		t.Errorf("AccountKey = %q", got)
	}
}

// TestNewStoreKeepsJiraCredentialLocations checks the two names a stored
// credential is found by: the keychain service and the fallback file. The
// store's own behavior is tested in agent-cli-kit/credstore.
func TestNewStoreKeepsJiraCredentialLocations(t *testing.T) {
	t.Parallel()
	s := NewStore(t.TempDir())
	if _, err := s.Save("svc.example.com:pat", "tok-xyz"); err != nil {
		t.Fatal(err)
	}
	if got, err := keyring.Get(constants.KeychainService, "svc.example.com:pat"); err != nil || got != "tok-xyz" {
		t.Fatalf("keyring.Get(%q) = %q, %v; want the saved secret", constants.KeychainService, got, err)
	}
	if constants.CredentialsFileName != credstore.CredentialsFileName {
		t.Fatalf("CredentialsFileName = %q, kit uses %q", constants.CredentialsFileName, credstore.CredentialsFileName)
	}
}

func TestResolveDefaultsToPAT(t *testing.T) {
	t.Parallel()
	cfg := config.Config{BaseURL: "https://default.example.com"}
	cred, err := Resolve(cfg, config.Secrets{PAT: "tok"}, nil)
	if err != nil {
		t.Fatal(err)
	}
	if cred.Scheme != SchemePAT || cred.Secret != "tok" {
		t.Errorf("Resolve() = %+v, want the pat scheme with its token", cred)
	}
}

// TestResolveBasicSecret checks which transient secret basic auth uses: the
// password when set (Data Center), otherwise the API token (Cloud).
func TestResolveBasicSecret(t *testing.T) {
	t.Parallel()
	cfg := config.Config{
		BaseURL: "https://basic.example.com",
		Auth:    config.AuthConfig{Scheme: SchemeBasic, Username: "alice"},
	}
	tests := []struct {
		name    string
		secrets config.Secrets
		want    string
	}{
		{"password", config.Secrets{Password: "pw"}, "pw"},
		{"api token", config.Secrets{APIToken: "api"}, "api"},
		{"password wins", config.Secrets{Password: "pw", APIToken: "api"}, "pw"},
		{"pat ignored", config.Secrets{PAT: "tok", APIToken: "api"}, "api"},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			cred, err := Resolve(cfg, tc.secrets, nil)
			if err != nil {
				t.Fatal(err)
			}
			if cred.Scheme != SchemeBasic || cred.Username != "alice" || cred.Secret != tc.want {
				t.Errorf("Resolve() = %+v, want basic alice/%s", cred, tc.want)
			}
		})
	}
}

func TestResolvePrefersTransientSecret(t *testing.T) {
	t.Parallel()
	s := NewStore(t.TempDir())
	if _, err := s.Save("kms.example.com:pat", "stored-token"); err != nil {
		t.Fatal(err)
	}
	cfg := config.Config{
		BaseURL: "https://kms.example.com",
		Auth:    config.AuthConfig{Scheme: SchemePAT},
	}
	cred, err := Resolve(cfg, config.Secrets{PAT: "flag-token"}, s)
	if err != nil {
		t.Fatal(err)
	}
	if cred.Secret != "flag-token" {
		t.Errorf("Secret = %q, want transient flag-token", cred.Secret)
	}
}

func TestResolveFallsBackToStore(t *testing.T) {
	t.Parallel()
	s := NewStore(t.TempDir())
	if _, err := s.Save("kms2.example.com:pat", "stored-token"); err != nil {
		t.Fatal(err)
	}
	cfg := config.Config{
		BaseURL: "https://kms2.example.com",
		Auth:    config.AuthConfig{Scheme: SchemePAT},
	}
	cred, err := Resolve(cfg, config.Secrets{}, s)
	if err != nil {
		t.Fatal(err)
	}
	if cred.Secret != "stored-token" {
		t.Errorf("Secret = %q, want stored-token", cred.Secret)
	}
}

// TestResolveNoCredentialErrors checks the recovery jira-cli offers when the
// store holds no secret or cannot be read: Jira's wording, the doctor and
// config init steps, and, for a missing secret, the acquisition guide.
func TestResolveNoCredentialErrors(t *testing.T) {
	t.Parallel()
	cfg := config.Config{
		BaseURL: "https://nocreds.example.com",
		Auth:    config.AuthConfig{Scheme: SchemePAT},
	}
	retry := "Retry the same command with access to the host user environment (home directory and OS keychain)."
	tests := []struct {
		name    string
		err     error
		code    string
		message string
		steps   []string
	}{
		{"missing", keyring.ErrNotFound, "CREDENTIAL_NOT_VISIBLE_OR_MISSING",
			"stored Jira credentials are missing or not visible in this execution environment",
			[]string{retry, "jira-cli doctor",
				"Only if the host retry also reports missing credentials, run `jira-cli config init` in the user's terminal or set JIRA_* environment variables.",
				"jira-cli auth guide", "Credential page: https://nocreds.example.com"}},
		{"inaccessible", errors.New("sandbox denied keychain"), "CREDENTIAL_STORE_INACCESSIBLE",
			"stored Jira credentials cannot be read in this execution environment",
			[]string{retry, "jira-cli doctor",
				"Do not run `config init` unless the same check also fails in the host environment."}},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			s := newStoreWithKeyring(t.TempDir(), failingKeyring{err: tc.err})
			_, err := Resolve(cfg, config.Secrets{}, s)
			ce := cerrors.AsCLIError(err)
			if ce == nil || ce.Code != tc.code || ce.Message != tc.message {
				t.Fatalf("Resolve() error = %+v, want %s %q", ce, tc.code, tc.message)
			}
			if !reflect.DeepEqual(ce.NextSteps, tc.steps) {
				t.Errorf("NextSteps = %q, want %q", ce.NextSteps, tc.steps)
			}
			if ce.Recovery == nil || ce.Recovery.Scope != "host" || ce.Retryable {
				t.Fatalf("Resolve() recovery = %+v retryable=%v, want host and false", ce.Recovery, ce.Retryable)
			}
		})
	}
}

// TestResolveServiceMismatch checks that a credential stored for another
// complete service URL is not borrowed for this one.
func TestResolveServiceMismatch(t *testing.T) {
	t.Parallel()
	cfg := config.Config{
		BaseURL:           "https://jira.example.com/a",
		CredentialBaseURL: "https://jira.example.com/b",
		Auth:              config.AuthConfig{Scheme: SchemePAT},
	}
	_, err := Resolve(cfg, config.Secrets{}, NewStore(t.TempDir()))
	if ce := cerrors.AsCLIError(err); ce == nil || ce.Code != "CREDENTIAL_SERVICE_MISMATCH" ||
		ce.Message != "stored credential lookup does not match the complete service URL" {
		t.Fatalf("Resolve() error = %+v, want CREDENTIAL_SERVICE_MISMATCH", ce)
	}
}
