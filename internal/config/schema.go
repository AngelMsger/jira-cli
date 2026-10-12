package config

import (
	"path/filepath"
	"strconv"

	kit "github.com/angelmsger/agent-cli-kit/config"
	"github.com/angelmsger/jira-cli/pkg/constants"
	cerrors "github.com/angelmsger/jira-cli/pkg/errors"
)

// EnvNoContextHint opts out of the runtime multi-context notice.
const EnvNoContextHint = "JIRA_CLI_NO_CONTEXT_HINT"

// fields is jira-cli's settings table. Each row names a field's environment
// variables and built-in default. Service fields are the team presets a setup
// load keeps; the user name, default project and secrets are personal.
var fields = []kit.Field{
	{Key: fieldServer, Env: []string{"JIRA_SERVER"}, Service: true},
	{Key: fieldFlavor, Env: []string{"JIRA_FLAVOR"}, Default: FlavorAuto, Service: true},
	// A cached probe result, dropped when the server or flavor changes.
	{Key: fieldDetectedFlavor, ResetBy: []string{fieldServer, fieldFlavor}},
	{Key: fieldCredentialURL, Env: []string{"JIRA_CREDENTIAL_URL"}, Service: true},
	{Key: fieldAuthScheme, Env: []string{"JIRA_AUTH_SCHEME"}, Default: SchemePAT, Service: true},
	{Key: fieldAuthUsername, Env: []string{"JIRA_USERNAME"}},
	{Key: fieldProject, Env: []string{"JIRA_DEFAULT_PROJECT"}},
	{Key: fieldFormat, Env: []string{"JIRA_FORMAT"}, Default: constants.DefaultFormat, Service: true},
	{Key: fieldPageSize, Default: strconv.Itoa(constants.DefaultPageSize)},
	{Key: fieldTimeout, Default: constants.DefaultTimeout.String()},
	{Key: fieldMaxRetries, Default: strconv.Itoa(constants.DefaultMaxRetries)},
	{Key: fieldReadOnly, Env: []string{"JIRA_CLI_READ_ONLY"}, Service: true},
	// A secret in the environment selects its scheme: a PAT wins over a
	// password, which wins over an API token. JIRA_PERSONAL_ACCESS_TOKEN wins
	// over its shorthand JIRA_TOKEN.
	{Key: fieldPAT, Env: []string{envPAT, "JIRA_TOKEN"}, Implies: SchemePAT},
	{Key: fieldPassword, Env: []string{"JIRA_PASSWORD"}, Implies: SchemeBasic},
	{Key: fieldAPIToken, Env: []string{"JIRA_API_TOKEN"}, Implies: SchemeBasic},
}

func init() {
	kit.Register(&kit.Schema{
		Fields: fields,
		Paths: kit.Paths{
			Dir:  filepath.Join(constants.ConfigParentDirName, constants.ConfigDirName),
			File: constants.ConfigFileName,
		},
		Words: kit.Words{
			App:          constants.AppName,
			ContextFlag:  "--use-context",
			ContextEnv:   "JIRA_CONTEXT",
			NoHintEnv:    EnvNoContextHint,
			ListContexts: "config get-contexts",
			UseContext:   "config use-context",
			SetContext:   "config set-context",
			AuthGuide:    "auth guide",
			AuthLogin:    "auth login",
		},
		Schemes:    []string{SchemePAT, SchemeBasic},
		AfterMerge: resolveAuthDefaults,
		Validate:   validateFlavor,
	})
}

// resolveAuthDefaults picks basic auth on Cloud when nothing chose a scheme:
// Cloud accepts an email and API token, never a Bearer PAT.
func resolveAuthDefaults(values kit.Values, sources map[string]string) {
	if sources[fieldAuthScheme] == "default" && setupFlavor(configFromMap(values)) == FlavorCloud {
		values[fieldAuthScheme] = SchemeBasic
	}
}

// validateFlavor adds the flavor checks to ValidateService.
func validateFlavor(values kit.Values) error {
	cfg := configFromMap(values)
	if cfg.Flavor != "" && cfg.Flavor != FlavorAuto && cfg.Flavor != FlavorCloud && cfg.Flavor != FlavorDataCenter {
		return cerrors.New(cerrors.CategoryConfig, "BAD_FLAVOR", "flavor must be auto, cloud or datacenter")
	}
	if setupFlavor(cfg) == FlavorCloud && cfg.Auth.Scheme == SchemePAT {
		return cerrors.New(cerrors.CategoryConfig, "AUTH_CLOUD_NEEDS_BASIC", "Cloud requires email and an API token using basic authentication")
	}
	return nil
}
