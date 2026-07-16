# Getting started

Before any Jira command works, `jira-cli` needs a server URL and a credential.

## Check the current state

```bash
jira-cli doctor
```

`doctor` runs three checks — configuration, credentials, connectivity — and
prints a JSON report. If `healthy` is `true`, you are ready. Otherwise each
failing check's `detail` explains what to fix.

```bash
jira-cli auth status   # is a usable credential resolvable?
jira-cli config show   # the resolved, non-secret configuration
jira-cli config show --explain   # ...annotated with each value's source
```

## Configuration sources

Settings are resolved in this precedence order (highest first):

1. CLI flags (`--base-url`, `--flavor`, `--format`, `--timeout`)
2. Environment variables (`JIRA_*`)
3. A `.env` file in the working directory
4. `~/.angelmsger/jira/config.yaml`
5. Built-in defaults

Key environment variables:

| Variable | Meaning |
|----------|---------|
| `JIRA_SERVER` | Site URL |
| `JIRA_FLAVOR` | `cloud`, `datacenter` or `auto` |
| `JIRA_PERSONAL_ACCESS_TOKEN` (alias `JIRA_TOKEN`) | Data Center PAT (Bearer auth) |
| `JIRA_USERNAME` + `JIRA_PASSWORD` | Basic auth (Data Center) |
| `JIRA_USERNAME` + `JIRA_API_TOKEN` | Basic auth (Cloud: email + API token) |
| `JIRA_DEFAULT_PROJECT` | Default project key for `issue create` / `issue search` |
| `JIRA_CONTEXT` | Select a named context (multi-server setups) |
| `JIRA_CLI_READ_ONLY` | Block all writes for the session |

## Interactive setup

For a human running the CLI in a terminal, point them at the TUI:

```bash
jira-cli config init --pretty
```

The plain `config init` form (no flag) is line-by-line — use it from scripts,
dotfiles bootstrap, and non-TTY environments where the TUI cannot render.
`--pretty` is **human-only** (interactive TUI + colorized JSON) and errors
without a TTY — agents should never pass it. Both forms ask for the server
URL, detect the flavor, collect a credential, validate it live (via
`/myself`), and store the secret in the OS keychain (falling back to per-user
DPAPI on Windows or a `0600` file on macOS/Linux). Non-secret settings go to
`~/.angelmsger/jira/config.yaml`; secrets are never written there.

> **Cloud auth note.** On Atlassian Cloud (`*.atlassian.net`) the auth scheme
> must be `basic` with the user's Atlassian email + an API token from
> id.atlassian.com — the wizard defaults to this when it sees a Cloud tenant.
> `pat` (Bearer) is Data Center 8.14+ only; Cloud REST endpoints reject Bearer
> even when the token itself is valid, and the CLI fails fast with
> `AUTH_CLOUD_NEEDS_BASIC` before sending anything.

## For agents and sandboxes

If you are an AI agent driving `jira-cli`, the user has normally already
configured it. **Reuse their existing config and credentials** from
`~/.angelmsger/jira/config.yaml` + the OS keychain — do not run `config init`
to create a fresh setup, and do not pass `--pretty`.

When you run inside a **sandbox** that cannot read the user's home directory or
keychain, credential resolution returns `CREDENTIAL_STORE_INACCESSIBLE` or
`CREDENTIAL_NOT_VISIBLE_OR_MISSING` with `recovery.scope=host`. Do **not** give
up, and do **not** re-initialize config inside the sandbox. Instead:

- **Request elevated permissions** (or otherwise re-run with access to the
  user's real environment), then retry the same command once.
- If the host retry also reports credentials missing, ask the user to run
  `jira-cli config init` in their own terminal, or to export `JIRA_*` env vars
  for the session.

## Multiple servers (contexts)

The config file holds named contexts (kubeconfig-style). Inspect and switch:

```bash
jira-cli config get-contexts
jira-cli config use-context prod
jira-cli --use-context staging issue search --project ENG   # one-off
```
