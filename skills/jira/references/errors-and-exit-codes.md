# Errors and exit codes

On failure `jira-cli` writes a JSON object to **stderr** and exits with a
category-specific code. stdout stays empty for a single failed operation;
multi-item deletes emit per-item results there even when some items fail.

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
`retryable` indicates whether retrying the same invocation can help safely.
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

## Writes that succeeded or may have succeeded

`issue create`, `issue edit` and `issue transition` read the issue after the
write. If that read fails, `WRITE_SUCCEEDED_READ_FAILED` means the write was
acknowledged: the error retains the issue key and browse URL, uses the read
failure's category/exit code, and sets `retryable: false`. Follow the supplied
`issue get` command; do not repeat the write.

`WRITE_SUCCEEDED_RESPONSE_INVALID` means Jira acknowledged the write but its
response could not be decoded or omitted its identifier. For a creation with
no usable key, the recovery command searches the project's newest 25 issues;
compare the description/summary and follow cursors if needed. An empty first
page is not evidence that nothing was created.

Network, rate-limit and server failures during a write have an **unknown
outcome** and `retryable: false`. Verify the issue or comment state before
considering another write. This also applies to comments added by transitions.
The CLI does not replay mutations automatically. A failed batch may contain
successful deletions: inspect per-item results, never replay the whole batch.

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
- **auth (4)** → `jira-cli auth status`; a server 401 means the supplied
  credential was rejected, so ask the user to renew it through their normal
  setup. Host access retries apply to the credential-resolution codes above,
  not to an ordinary 401. Do not initialize a replacement sandbox config.
- **not_found (6)** → the key/URL is wrong or the issue moved projects;
  `jira-cli issue search --text "<keywords>"` to relocate it.
- **permission (5)** → either a 403 from Jira (the credential works but lacks
  rights — not fixable by retrying, tell the user the account needs access),
  **or** `READONLY_BLOCKED` from local read-only mode (`defaults.read_only` /
  `JIRA_CLI_READ_ONLY=1`). Use `--allow-writes` only when the current task
  authorizes the concrete write despite that default; never override an
  explicit read-only instruction. To preview, add `--dry-run`. See
  `safety-modes.md`.
- **rate_limit (7) / server (9) / network (8)** → for reads marked
  `retryable: true`, retry with bounded backoff and prefer a narrower query
  over `--all`. For writes, use the outcome rules above.
