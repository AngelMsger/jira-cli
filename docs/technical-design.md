# jira-cli technical design

## 1. Goals and scope

`jira-cli` is a Go command-line tool that lets a coding agent
(Claude Code, Codex, etc.) drive Jira issue-tracking workflows —
searching, reading, and maintaining issues.

- **Cross-flavor**: works with Jira Cloud (REST v3) and Jira Data
  Center / Server (REST v2) behind one flavor-agnostic client.
- **Agent-first**: JSON output by default, structured errors, a plain-text
  body contract that hides the ADF asymmetry, error messages that carry
  actionable next steps.
- **Layered configuration**: CLI flags / environment variables / `.env`
  / config file, with an interactive `init` wizard.
- **Operation surface**: reads and writes. Reads cover issue fetching,
  JQL search, project listing, transitions discovery, comments, and user
  resolution. Writes cover issues (create / edit / assign / transition)
  and comments (post / edit / delete). `whoami` reports the user attached
  to the current credentials. Every write command accepts `--dry-run` to
  preview the request that would be sent.

Non-goals for v0.1: worklogs, watchers, issue links, attachments,
Agile boards / sprints (the Agile REST API), issue deletion, project
administration, and third-party OAuth 2.0 authorization (extension point
reserved, not shipped in this cycle).

## 2. API flavor difference matrix

The CLI uses a `Flavor` value to distinguish two backends:

| Flavor | Description | REST base |
|--------|-------------|-----------|
| `cloud` | Jira Cloud (`*.atlassian.net`) | `/rest/api/3` |
| `datacenter` | Data Center / Server (self-hosted) | `/rest/api/2` |

Most relative paths are identical across flavors; the real divergences
(`{base}` is the site root URL) are:

| Concern | cloud | datacenter |
|---------|-------|------------|
| JQL search | `POST {base}/rest/api/3/search/jql` (token pagination: `nextPageToken` / `isLast`; the legacy startAt `/search` was removed from Cloud in 2025); a `fields` list is mandatory or only IDs return | `GET {base}/rest/api/2/search?jql&startAt&maxResults` (offset + `total`) |
| Rich-text bodies (description, comments) | ADF JSON documents; the CLI converts plain text → ADF paragraphs on write and flattens ADF → text on read (`pkg/apiclient/adf.go`) | plain strings, sent verbatim (wiki markup renders server-side) |
| User identifier | `accountId`; non-accountId selectors resolve via `GET /rest/api/3/user/search?query=` and must match exactly one active user | `name` (username), passed through as given |
| Project listing | `GET {base}/rest/api/3/project/search` (paginated, server-side `query`) | `GET {base}/rest/api/2/project` (unpaginated array; `--query` filters client-side) |
| Auth | Basic only: email + API token (Bearer PATs rejected; guarded by `AUTH_CLOUD_NEEDS_BASIC`) | PAT Bearer (8.14+) or Basic username + password |

These divergences are recorded in the capability table
(`pkg/apiclient/capability.go`), which drives runtime guards and docs so
the two branches cannot drift silently.

**Pagination**: everything collapses into the opaque `ListResult.Next`
cursor — a `nextPageToken` on Cloud search, a stringified `startAt`
offset elsewhere. `--limit/--all/--cursor` and the `{items, next,
has_more}` envelope behave identically on both flavors.

**Flavor detection**: explicit `--flavor` / config wins; otherwise the
hostname shortcut (`*.atlassian.net` → cloud); otherwise
`GET /rest/api/2/serverInfo` — present and anonymous on both flavors —
whose `deploymentType` field says `Cloud` or `Server`. The detection
result can be cached as `detected_flavor` in the config file.

## 3. Normalized data model

Every API method returns flavor-agnostic models
(`pkg/apiclient/models.go`):

```
ServerInfo { Flavor, BaseURL, Reachable, Version, DeploymentType }
Project    { ID, Key, Name, Type, Lead, URL }
Issue      { ID, Key, ProjectKey, Type, Summary, Description, Status,
             Priority, Assignee *User, Reporter *User, Labels,
             ParentKey, Created, Updated, URL }
Status     { ID, Name, Category }
Comment    { ID, IssueKey, Author *User, Body, Created, Updated }
Transition { ID, Name, To *Status }
User       { AccountID, Username, DisplayName, Email, Active }
```

`Issue.Description` and `Comment.Body` are plain text on both flavors
(the ADF bridge normalizes Cloud). JSON output fields use snake_case;
`Issue.URL` is the human `/browse/<key>` link.

## 4. Configuration and authentication

### 4.1 Config structure

```
Config {
  BaseURL  string                 // site root URL
  Flavor   string                 // cloud | datacenter | auto
  Auth     AuthConfig
  Defaults Defaults
  DetectedFlavor string            // cached auto-detect result
}
AuthConfig { Scheme string         // pat | basic
             Username string }     // used by basic; secrets do not live here
Defaults   { Format string         // json (default)
             PageSize int          // 25
             Timeout  duration     // 30s
             MaxRetries int        // 3
             Project string        // default project key (optional)
             ReadOnly bool }       // session-level write block
```

### 4.2 Sources and precedence

Highest → lowest: CLI flags > environment variables (`JIRA_*`) >
`.env` file > `~/.angelmsger/jira/config.yaml` > built-in defaults.
Each layer is a sparse map, and non-empty fields override lower layers.
Provenance is recorded per-field so `config show --explain` can report it.

Environment variable mapping:

| Variable | Field |
|----------|-------|
| `JIRA_SERVER` | `BaseURL` |
| `JIRA_FLAVOR` | `Flavor` |
| `JIRA_PERSONAL_ACCESS_TOKEN` (alias `JIRA_TOKEN`) | PAT secret (scheme=pat) |
| `JIRA_USERNAME` | `Auth.Username` |
| `JIRA_PASSWORD` | basic secret |
| `JIRA_API_TOKEN` | basic secret (cloud: paired with email) |
| `JIRA_DEFAULT_PROJECT` | `Defaults.Project` |
| `JIRA_FORMAT` | `Defaults.Format` |
| `JIRA_CLI_READ_ONLY` | `Defaults.ReadOnly` |

`.env` is read via `godotenv` into a temporary map without mutating the
process environment, so the rule "environment variables outrank `.env`"
holds.

### 4.3 Authentication

- **pat**: `Authorization: Bearer <token>`. Data Center 8.14+ only.
- **basic**: `Authorization: Basic base64(user:secret)`. Data Center uses
  username + password; Cloud uses email + API token.

Cloud rejects Bearer PATs, so `flavor=cloud` + `scheme=pat` fails fast at
client build with `AUTH_CLOUD_NEEDS_BASIC` instead of surfacing a
confusing 403.

Secrets are never persisted to `config.yaml`. `config init` stores them
in the OS keychain (`go-keyring`, service `jira-cli`, account
`<host>:<scheme>`); on failure it falls back to a `credentials` file
inside the resolved config directory (per-user DPAPI on Windows; file 0600
and dir 0700 on macOS/Linux) — `~/.angelmsger/jira/credentials`. Runtime
secrets supplied via env / `.env` / flag are used transiently and not
persisted.

Credential reads distinguish "not found" from "store inaccessible". When a
sandbox cannot inspect the host keychain/file, resolution returns
`CREDENTIAL_STORE_INACCESSIBLE`; an ambiguous absence returns
`CREDENTIAL_NOT_VISIBLE_OR_MISSING`. Both carry an optional structured
`recovery` action requesting one retry in host scope, without marking the
error normally retryable or allowing the CLI to elevate itself.

### 4.4 The `init` wizard

Enter base URL → detect and confirm the flavor (via `serverInfo`) → pick
auth scheme and credentials (Cloud defaults to basic email + API token;
DC to PAT) → live validation via `/myself` → choose where to store the
secret → write non-secret fields to `config.yaml`, secret to keychain /
file → print suggested next commands.

## 5. Command surface

Global persistent flags: `--base-url`, `--flavor`,
`--format` (json|table|ndjson), `--fields`, `--timeout`, `--config`,
`--use-context`, `--verbose`, `--pretty`, `--allow-writes`.

Commands group by resource: `issue`, `project`, `comment`, `user`,
`config`, `auth`, `doctor`, `whoami`, `skill`, `version`. Cross-command
conventions:

- **Identifier parsing**: issue inputs accept a bare key (`PROJ-123`), a
  `/browse/` URL, or a board URL carrying `?selectedIssue=`, parsed by
  `pkg/urlref`; project inputs accept a key or a project URL.
- **Writes**: `issue create/edit/assign/transition` and
  `comment add/update/delete` are the write commands. Every write accepts
  `--dry-run` to preview the HTTP request it would send; destructive
  commands (`comment delete`) additionally require `--yes`.
- **Pagination**: list commands (`issue search`, `comment list`,
  `project list`) accept `--limit/--all/--cursor` and emit a
  `{items, next, has_more}` envelope.
- **Name resolution**: `issue transition --to` resolves transition names
  against the live transitions list; assignee flags resolve
  display-name/email queries to Cloud accountIds. Both fail with the
  candidate list on ambiguity, never guess.

The full command / flag / example reference is auto-generated from the
command tree — see [docs/cli/](cli/) (`make docs` produces it, CI
checks for drift). This section deliberately does not maintain a
parallel command list, to keep documentation from diverging from the
implementation.

## 6. Output and error model

### 6.1 Output

Three `Formatter` implementations: `json` (default, agent-oriented,
stdout), `table` (human-readable), and `ndjson` (streaming for large
result sets). `--fields a,b.c` projects by dot-path. List commands
emit the pagination envelope `{items, next, has_more}`; `--cursor`
continues from a prior page's `next`.

Successful output is unified as JSON on stdout, with two deliberate
raw-output exceptions:

- `version` prints a plain text version line (matches the `--version`
  flag).
- `skill show` prints the embedded `SKILL.md` verbatim.

Prompts from interactive wizards (`config init`, `auth login`) and all
errors go to stderr.

### 6.2 Errors

Errors are JSON on **stderr**:

```json
{"error":{"category":"config","code":"CREDENTIAL_STORE_INACCESSIBLE",
  "message":"stored Jira credentials cannot be read in this execution environment",
  "hint":"The configured credential store is inaccessible from the current process.",
  "next_steps":["Retry the same command with access to the host user environment."],
  "retryable":false,
  "recovery":{"action":"retry_current_command","scope":"host",
    "requires":["user_home","os_keychain"]}}}
```

`recovery` is optional and describes an environment change; `retryable` still
means the same invocation may succeed in the current environment. `doctor`
mirrors this distinction with per-check `status` and optional
`recovery_scope` fields.

Jira's error envelope (`{"errorMessages": [...], "errors": {"field":
"msg"}}`) is flattened into the message so field-level validation errors
("summary: You must specify a summary") stay actionable.

Categories: `usage config auth not_found permission conflict rate_limit
network server parse internal`.

### 6.3 Exit codes

| Code | Category | Code | Category |
|------|----------|------|----------|
| 0 | success | 6 | not_found |
| 1 | internal | 7 | rate_limit |
| 2 | usage | 8 | network |
| 3 | config | 9 | server |
| 4 | auth | 10 | parse |
| 5 | permission | 11 | conflict |

`hints.go` maps each category to `next_steps`, guiding agents to
self-correct.

## 7. The ADF body bridge

Jira Cloud's REST v3 represents rich-text fields (issue description,
comment bodies) as Atlassian Document Format (ADF) JSON; Data Center's
REST v2 uses plain strings. `pkg/apiclient/adf.go` gives the CLI a single
plain-text contract:

- **Write** (`textToBody`): on Cloud, `TextToADF` builds a minimal ADF
  document — blank lines split paragraphs, single newlines become
  `hardBreak` nodes. On DC the string passes through verbatim.
- **Read** (`bodyToText`): a JSON string (DC) is unwrapped; an ADF object
  (Cloud) is flattened by `ADFToText` — block nodes become paragraphs,
  list items become `- ` lines, mentions/emoji render their display text,
  unknown nodes are recursed into so no text is silently dropped.

The conversion is intentionally lossy for rich nodes (tables, panels,
marks); the loss is one-directional and documented in the capability
table and the companion Skill. Text written through the bridge round-trips
exactly (pinned by `TestTextToADFRoundTrip`).

## 8. JQL construction

When `issue search` has no positional argument the JQL is assembled from
flags (`pkg/apiclient/jql.go`):

| Flag | JQL fragment |
|------|--------------|
| `--project` | `project = "<v>"` |
| `--assignee` | `assignee = "<v>"`; `me` → `assignee = currentUser()`; `unassigned` → `assignee is EMPTY` |
| `--reporter` | `reporter = "<v>"` (same conveniences) |
| `--status` | `status = "<v>"` |
| `--type` | `issuetype = "<v>"` |
| `--label` | `labels = "<v>"` |
| `--text` | `text ~ "<v>"` |
| `--order-by` | `ORDER BY <v>` (appended) |

Fragments join with `AND`; string values have inner quotes escaped. If a
positional `<jql>` argument is supplied it is passed through verbatim
(mixing it with filter flags is rejected). With no filters at all,
`defaults.project` scopes the search when configured.

## 9. Safety modes

Two orthogonal write-protections, layered on top of `--yes`:

1. **`--dry-run`** is wired on every mutating command. It resolves the
   operation via `Client.DescribeWrite(ctx, op)` and emits the resulting
   `WriteRequestPlan{Method, URL, Payload}` instead of sending the
   request. The build helper is shared with the live write, so the
   preview cannot drift from the actual HTTP call.
2. **Read-only mode** is session-level. `defaults.read_only: true` in
   `config.yaml` or `JIRA_CLI_READ_ONLY=1` in the environment
   makes `appState.newClient()` wrap the client in
   `apiclient.NewReadOnly(...)`, which returns a structured
   `READONLY_BLOCKED` (`category=permission`, exit code 5) from every
   mutating method before any HTTP request is sent. The root persistent
   flag `--allow-writes` overrides the posture for a single invocation.
   `DescribeWrite` (used by `--dry-run`) is intentionally not
   overridden by the wrapper, so previews still work under a locked
   session.

Out of scope: `config init`, `auth login|logout`, and `skill install` are
CLI self-configuration / local IO, not remote mutations — they remain
available under read-only.

## 10. Skill outline

`skills/jira/SKILL.md` (YAML frontmatter: `name: jira`,
trigger-word description, `metadata.requires.bins`,
`metadata.cliHelp`) + `references/`:

- `getting-started.md` — configuration / auth checks, `doctor`, the
  flavor concept, sandbox credential recovery.
- `searching-jql.md` — the flag → JQL table, pagination, user-value
  resolution.
- `writing-issues.md` — create / edit / assign / transition / comments,
  and the ADF plain-text contract.
- `safety-modes.md` — `--dry-run` and read-only mode for agents.
- `errors-and-exit-codes.md` — exit-code table + per-category recovery
  steps, including the Jira-specific codes.

Core golden rule: resolve URLs / topics into issue keys before acting.

The same `SKILL.md` ships to **Claude Code**, **Codex**, and **Grok Build**
(all three only require frontmatter `name` + `description`). `skill install`
uses an agent path table (`agentSpecs` in `internal/app/skill.go`)
mapping each agent to its global / project skills directory and probe
markers: Claude Code uses `~/.claude/skills` and `./.claude/skills`; Codex uses
`~/.codex/skills` and `./.agents/skills`; Grok Build uses `~/.grok/skills` and
`./.grok/skills`. With no flag it
probes which directories exist and installs / removes for each hit;
`--agent` selects explicitly; `--dir` is the agent-agnostic explicit
path.

## 11. Testing strategy

- **Unit tests**: stdlib `testing`, table-driven, `t.Parallel()`.
  Coverage includes config precedence, auth resolution and file
  permissions, JQL construction, the ADF bridge (including the
  write→read round-trip), the per-flavor `DescribeWrite` plan table
  (method / path / payload dialect for every write op), Cloud user
  resolution, read-only blocking of every mutator, every output format
  and `--fields`, errors mapping, and urlref.
- **HTTP-layer tests**: `httptest.Server` drives the client; assertions
  cover the search endpoint split (Cloud `POST /search/jql` with token
  cursor vs DC `GET /search` with startAt) and header/auth wiring.
- **End-to-end**: `scripts/e2e.sh` builds the binary against an
  in-process mock Jira (Data Center v2 dialect) and exercises every
  command, asserting stdout contract and exit codes. The read-only /
  dry-run safety modes are covered here as well — every blocked-write
  path is paired with its `--allow-writes` and `--dry-run` counter-test.
  The Cloud dialect is covered by the unit layer; run a live-Cloud smoke
  before a release.
- **Read-only live verification**: `make e2e-live` runs only
  `doctor` / `project list` against a real instance.
