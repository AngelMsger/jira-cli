---
name: jira
version: 0.2.0
description: "Drive Jira issue-tracking workflows from the command line. Read issues, search with JQL or filter flags, create and edit issues, assign them, move them through workflow transitions, read/post/edit/delete comments, and discover valid field values (components, versions, issue types, statuses, priorities, labels, custom select options). Every mutating command accepts --dry-run, and a session read-only posture (defaults.read_only / JIRA_CLI_READ_ONLY=1, overridable via --allow-writes) blocks writes before they leave the CLI. Use this skill when the user gives a Jira issue key (like PROJ-123) or a Jira URL, or mentions a Jira ticket/issue; asks to find, read or summarise issues; run a JQL query; create/edit/assign an issue; transition an issue (start progress, close, reopen); read or post/edit/delete a comment; browse projects; asks which values a project or an issue field allows; check which Jira user they are; or wants a dry-run / read-only / safe-mode session. Works with Jira Cloud and Data Center / Server."
metadata:
  requires:
    bins: ["jira-cli"]
  cliHelp: "jira-cli --help; jira-cli issue --help; jira-cli issue search --help"
---

# jira

`jira-cli` reads and writes a Jira instance for you. Output is JSON by
default; errors are JSON on stderr with a `category`, a `hint` and
`next_steps`.

## Golden rule — resolve to an issue key first

Jira operations act on an **issue key** (`PROJ-123`). If the user gives a URL,
pass it directly: every command accepts a `/browse/...` URL, a board URL with
`?selectedIssue=`, or a bare key, and resolves the key itself. If the user
gives only a *topic* or *description*, do **not** guess a key — run
`jira-cli issue search` first, then act on the key from the hit.

## Decision tree

- User gave an **issue key or URL** and wants its content → `issue get`.
- User describes a topic / keywords but no key → `jira-cli issue search`
  (see [searching-jql.md](references/searching-jql.md)), then `issue get`.
- User wants to **create** an issue → `issue create` (see
  [writing-issues.md](references/writing-issues.md)).
- User wants to **edit** summary/description/priority/labels → `issue edit`.
- User wants to **assign** an issue → `issue assign --to <user>` (or
  `--unassign`); resolve ambiguous names first with `user resolve`.
- User wants to **move an issue through its workflow** (start progress, mark
  done, close, reopen) → `issue transitions <key>` to see what is available,
  then `issue transition <key> --to <name-or-id>`.
- User wants the **comments** on an issue → `comment list`; to post one →
  `comment add`; to edit or delete one → `comment update` / `comment delete`
  (see [writing-issues.md](references/writing-issues.md)).
- User wants to browse **projects** → `project list` / `project get`.
- User asks **which values a field allows** (components, versions, issue
  types, statuses, priorities, labels, a custom select field), or a value you
  passed was rejected / matched nothing → the metadata discovery commands
  (see [discovering-metadata.md](references/discovering-metadata.md)).
- User asks **who they are** / which Jira account is in use → `whoami`.
- A command fails → read the JSON error on stderr and follow `next_steps`
  (see [errors-and-exit-codes.md](references/errors-and-exit-codes.md)).
- Nothing is configured yet → [getting-started.md](references/getting-started.md).

## Commands

```
jira-cli issue get <key|url>           # one issue, normalized fields
jira-cli issue search [jql]            # JQL search (or compose via flags)
jira-cli issue create                  # create (--project --type --summary ...)
jira-cli issue edit <key>              # update summary/description/priority/labels
jira-cli issue assign <key> --to <u>   # change assignee (--unassign clears)
jira-cli issue transitions <key>       # transitions available right now
jira-cli issue transition <key> --to X # move through the workflow
jira-cli comment list <key>            # comments, oldest first
jira-cli comment add <key> --body ...  # post a comment
jira-cli comment update <id> --issue <key> --body ...
jira-cli comment delete <id>... --issue <key> --yes
jira-cli project list [--query q]      # projects visible to the user
jira-cli project get <key>
jira-cli project components <key>      # valid components in the project
jira-cli project versions <key>        # valid fix/affects versions
jira-cli project issuetypes <key>      # what `issue create --type` accepts
jira-cli project statuses <key>        # workflow statuses per issue type
jira-cli priority list                 # what --priority accepts
jira-cli label list                    # labels in use (Cloud only)
jira-cli field list --project K --type T     # create-screen fields
jira-cli field options <field> --project K   # a field's allowed values
jira-cli user resolve <selector>       # what assignee flags accept
jira-cli whoami                        # the authenticated user
jira-cli doctor                        # config / credentials / connectivity
jira-cli skill status|install          # this Skill's install state
```

Every write (`issue create/edit/assign/transition`, `comment add/update/delete`)
takes `--dry-run`; `comment delete` additionally needs `--yes`. See
[safety-modes.md](references/safety-modes.md).

## Bodies are plain text (Cloud/DC asymmetry)

Descriptions and comment bodies are **plain text in, plain text out** on both
flavors:

- **Cloud (REST v3)** stores rich text as ADF. The CLI converts your text to
  ADF paragraphs on write (blank line = new paragraph) and flattens ADF to
  text on read. Rich nodes (tables, panels) degrade to their text content.
- **Data Center (REST v2)** takes strings verbatim; Jira wiki markup renders
  server-side.

Do not paste raw ADF JSON or expect markdown to render on Cloud. Details in
[writing-issues.md](references/writing-issues.md).

## Large result sets

`issue search`, `comment list` and `project list` return a `{items, next,
has_more}` envelope. By default they return one page; when `has_more` is true,
pass `--cursor` with the `next` value to read the following page. Use `--all`
to fetch every page in one call, or `--limit N` to size each request. For very
large outputs use `--format ndjson` (one JSON object per line, items only).
Use `--fields a,b.c` to project output down to the fields you need.

## Agent-facing conventions

- **Skill handshake — set `JIRA_CLI_SKILL=1`.** Once you have loaded this
  Skill, export `JIRA_CLI_SKILL=1` in the environment you run the CLI from.
  When the variable is absent the CLI assumes you may be inferring commands
  blindly and prints a one-line `{"_notice":{"skill":{…}}}` discovery hint on
  **stderr** (non-interactive sessions only). Setting it silences the hint;
  `jira-cli skill status` reports whether it is set. (To suppress the hint
  without loading the Skill, use `JIRA_CLI_NO_SKILL_HINT=1`.)
- **Update notices on stderr.** When a newer release exists, commands print a
  one-line `{"_notice":{"update":{…}}}` to **stderr** (never stdout, so parsing
  the data is unaffected). `doctor` reports it too. Silence with
  `JIRA_CLI_NO_UPDATE_NOTIFIER=1`.
- **Forgiving flags.** camelCase/snake_case flag names (`--orderBy`) and a flag
  stuck to its value (`--limit100`) are auto-corrected to the canonical form when
  it is a real flag; each fix is echoed as a `{"_notice":{"corrections":[…]}}`
  line on stderr. Prefer the canonical `--kebab-case value` form regardless.

## AI attribution (agent writes)

When you, as an AI agent, write to Jira on the user's behalf, mark the content
as AI-authored with a link back to the tool. This applies **only** to
agent-driven writes — `issue create`/`issue edit` descriptions and
`comment add` — never to anything a human authored directly. Include the
marker exactly once per description/comment: prefix the body with a plain
`[AI](https://angelmsger.github.io/jira-cli/)` line (bodies are plain text, so
the bare URL form is fine on both flavors). Write the attribution sentence in
the **same language as the content** (the user's language); keep the
plain-ASCII `[AI]` marker.

## Configuration & credentials (agents)

The user has normally already configured `jira-cli`. **Reuse their existing
config and credentials** from `~/.angelmsger/jira/config.yaml` + the OS keychain
— do not run `config init` to create a fresh setup, and never pass `--pretty`
(a human-only flag for the interactive TUI / colorized JSON; it errors without
a TTY and agents never need it).

If a failure has code `CREDENTIAL_STORE_INACCESSIBLE` or
`CREDENTIAL_NOT_VISIBLE_OR_MISSING`, or its `recovery.scope` is `host`,
**request elevated permissions / re-run the same command with access to the
user's real environment, then retry once — do not re-initialize config inside
the sandbox.** Never launch interactive `config init` / `auth login` yourself
(no TTY → they fail fast); if credentials are truly missing, ask the user to
run `config init` in their own terminal or to export `JIRA_*` env vars. See
[getting-started.md](references/getting-started.md).

## Global flags

`--format json|table|ndjson` · `--fields a,b.c` (project fields) ·
`--base-url` · `--flavor cloud|datacenter` · `--config <dir>` ·
`--use-context <name>` (pick a named server) · `--allow-writes` · `--verbose`
