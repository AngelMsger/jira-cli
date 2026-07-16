# Errors and exit codes

On failure `jira-cli` writes a JSON object to **stderr** and exits with a
category-specific code. stdout stays empty, so a successful pipeline never has
to parse errors.

## Error shape

```json
{
  "error": {
    "category": "auth",
    "code": "HTTP_Unauthorized",
    "message": "Jira returned HTTP 401: ...",
    "hint": "The server rejected the credentials. The token may be expired.",
    "next_steps": ["jira-cli auth status", "jira-cli config init"],
    "retryable": false,
    "http_status": 401
  }
}
```

Always read `hint` and `next_steps` — they tell you how to recover.
`retryable` indicates whether retrying in the same environment can succeed.
Environment changes such as a host retry use the optional `recovery` object
instead.

## Exit codes

| Code | Category | Meaning & recovery |
|------|----------|--------------------|
| 0 | — | success |
| 1 | internal | unexpected bug; re-run with `--verbose` |
| 2 | usage | bad flags/arguments; check `--help` |
| 3 | config | config/credential resolution failed; inspect `code` and `recovery` before reconfiguring |
| 4 | auth | credentials rejected (401); run `auth status`, re-`config init` |
| 5 | permission | valid login, no access (403), or local `READONLY_BLOCKED` |
| 6 | not_found | issue/project/comment does not exist (404); verify the key, or `issue search` |
| 7 | rate_limit | server throttling (429); wait, then retry; avoid `--all` on huge queries |
| 8 | network | DNS/TLS/timeout; check `--base-url`, run `doctor` |
| 9 | server | Jira 5xx; retry later |
| 10 | parse | a response could not be decoded; likely a client bug — the write may have succeeded, verify with a read |
| 11 | conflict | a write hit a conflict (409); re-read the issue, then retry |

## Common Jira-specific codes

- **`AUTH_CLOUD_NEEDS_BASIC`** (config, 3) → the flavor resolved to Cloud but
  the auth scheme is `pat`. Cloud only accepts basic auth (email + API
  token); set `JIRA_USERNAME` + `JIRA_API_TOKEN` or re-run `config init`.
- **`USER_AMBIGUOUS` / `USER_NOT_FOUND`** (usage/not_found) → an assignee
  selector did not resolve to exactly one active Cloud user. `next_steps`
  lists the candidates with their accountIds — pass the accountId.
- **`TRANSITION_NOT_FOUND` / `TRANSITION_AMBIGUOUS`** (not_found/usage) →
  `--to` did not match exactly one available transition. `next_steps` lists
  what the issue can currently do; transitions depend on the current status.
- **`DELETE_NEEDS_YES`** (usage, 2) → a destructive command ran without
  `--yes` in a non-interactive session. Re-run with `--yes` once the target
  is confirmed (a `--dry-run` first is good practice).
- **`UNKNOWN_COMMAND`** (usage, 2) → a typo'd subcommand; the message carries
  a "Did you mean" suggestion.

## Recovery patterns

- **`CREDENTIAL_STORE_INACCESSIBLE` / `CREDENTIAL_NOT_VISIBLE_OR_MISSING`** →
  when `recovery.scope` is `host`, request host access and retry the same
  invocation once. Repeating it in the same sandbox will not help. Only
  configure credentials when the host retry also reports them missing.
- **auth (4)** → `jira-cli auth status`; if not configured, `config init`.
  Agents in a sandbox: the credential is usually the user's, just unreadable
  from the sandbox — request elevation and retry rather than re-initializing.
  See `getting-started.md` › "For agents and sandboxes".
- **not_found (6)** → the key/URL is wrong or the issue moved projects;
  `jira-cli issue search --text "<keywords>"` to relocate it.
- **permission (5)** → either a 403 from Jira (the credential works but lacks
  rights — not fixable by retrying, tell the user the account needs access),
  **or** `READONLY_BLOCKED` from local read-only mode (`defaults.read_only` /
  `JIRA_CLI_READ_ONLY=1`). To send the blocked write anyway, add
  `--allow-writes`; to preview without sending, add `--dry-run`. See
  `safety-modes.md`.
- **rate_limit (7) / server (9) / network (8)** → `retryable: true`; wait and
  retry, and prefer a narrower query over `--all`.
