# Tracking time with worklogs

A worklog is one entry of work logged against an issue: how long, when it
started, and an optional note. `worklog add/update/delete` are **writes** —
each accepts `--dry-run`, each is blocked by read-only mode, and `delete`
additionally needs `--yes`. See [safety-modes.md](safety-modes.md).

Log work the user actually did, in the units they gave you. Do not invent
durations, round silently, or reconstruct a week's timesheet from guesses —
gather evidence first, then log what the user confirms.

## Reading what is already logged

```bash
jira-cli worklog list PROJ-123
jira-cli worklog list PROJ-123 --all
```

Oldest first, in the `{items, next, has_more}` envelope. Every item carries
`issue_key`, `id`, `author`, `started`, `time_spent` (Jira's human form) and
`time_spent_seconds` (the machine form). **Worklog IDs come from here** —
`worklog update` and `worklog delete` need one, and there is no other way to
discover it.

## Logging work

```bash
jira-cli worklog add PROJ-123 --time-spent "3h 20m" --comment "Traced the retry loop"
jira-cli worklog add PROJ-123 --time-spent 45m --started 2026-09-19T09:00:00
jira-cli worklog add PROJ-123 --time-spent-seconds 1800
echo "Long note." | jira-cli worklog add PROJ-123 --time-spent 1h --comment-file -
```

### `--time-spent` uses Jira's units, not Go's

Valid: `45m`, `2h`, `3h 20m`, `1d 4h`, `1w 2d`, `1.5h`. The units are `w`, `d`,
`h`, `m`, space-separated. A Go-style duration such as `1h30m0s` is rejected
with `WORKLOG_BAD_TIME_SPENT` (exit 2).

The value is sent to Jira **verbatim**. `w` and `d` expand according to the
instance's time-tracking configuration (often 5-day weeks and 8-hour days), so
only the server can convert them — never pre-compute hours yourself.
`--time-spent-seconds` is the unambiguous alternative; the two flags are
mutually exclusive.

### `--started` is normalized for you

Omit it and Jira uses now. Otherwise pass `now`, an RFC 3339 instant with an
offset (`2026-09-19T09:00:00+08:00`), a local wall clock
(`2026-09-19T09:00:00`, read in your machine's zone), or a date
(`2026-09-19`, read as **UTC** midnight). The CLI converts it to the
colon-free instant Jira's API demands — do not hand-build that format.

### The comment is plain text

`--comment` / `--comment-file` follow the same plain-text contract as issue
comments: ADF paragraphs on Cloud, a verbatim string on Data Center. The flag
is `--comment`, not `--body`, and it matches the output field. See
[writing-issues.md](writing-issues.md).

## Correcting and removing entries

```bash
jira-cli worklog update 10042 --issue PROJ-123 --time-spent 2h
jira-cli worklog update 10042 --issue PROJ-123 --comment "Corrected the note"
jira-cli worklog delete 10042 --issue PROJ-123 --yes
jira-cli worklog delete 10042 10043 --issue PROJ-123 --yes
```

`--issue` is **required** on `update` and `delete`: a Jira worklog ID is only
addressable together with its issue. Unset flags on `update` are left
untouched; passing none fails with `WORKLOG_NO_UPDATE_FIELDS`. `delete` takes
several IDs, or a single `-` to read newline-separated IDs from stdin.

Prefer correcting an existing entry over deleting and re-adding — the original
author, creation time and any visibility restriction are preserved.

## The remaining estimate

By default Jira subtracts the logged time from the issue's remaining estimate.
`--adjust-estimate` changes that:

| Value | Effect | Companion flag |
|---|---|---|
| `auto` (default) | subtract the logged time | — |
| `leave` | leave the estimate untouched | — |
| `new` | set the estimate outright | `--new-estimate "2d"` |
| `manual` | adjust by a specific amount | `--reduce-by` on `add`, `--increase-by` on `delete` |

```bash
jira-cli worklog add PROJ-123 --time-spent 2h --adjust-estimate leave
jira-cli worklog add PROJ-123 --time-spent 2h --adjust-estimate new --new-estimate 1d
jira-cli worklog delete 10042 --issue PROJ-123 --yes --adjust-estimate manual --increase-by 1h
```

`manual` is **not available on `update`** — Jira's worklog update endpoint has
no such mode; use `new` with `--new-estimate`. Illegal combinations fail before
any request is sent (`WORKLOG_BAD_ADJUST_ESTIMATE`,
`WORKLOG_ESTIMATE_FLAG_MISSING`, `WORKLOG_ESTIMATE_FLAG_UNSUPPORTED`).

Changing someone's estimate is a visible planning signal. Use `auto` unless the
user asked for something else, and preview with `--dry-run` when unsure — the
preview shows the exact query string, e.g.
`?adjustEstimate=new&newEstimate=2d`.

## Finding what someone logged over a period

```bash
jira-cli worklog search --since 7d
jira-cli worklog search --project ENG --since 24h
jira-cli worklog search --author alice \
    --from 2026-09-01T00:00:00+08:00 --to 2026-09-08T00:00:00+08:00
```

- `--author` defaults to `me` and resolves to a stable identity (a Cloud
  accountId, a Data Center username). A display name never identifies a person.
- **A window is required** (`WORKLOG_TIME_REQUIRED`), because the query spans
  many issues. `--since` is a relative window ending now; `--from` is
  inclusive and `--to` is exclusive, defaulting to now. `--since` cannot be
  combined with `--from`/`--to`, and `--to` requires `--from`.
- The interval is half-open `[from, to)`, so adjacent windows never
  double-count. Date-only values are **UTC** — use an offset-bearing instant
  for a local calendar day.
- `--limit`, `--cursor` and `--all` page the **candidate issue search**, not
  the worklogs, so the number of items per page varies.

### Report what this can and cannot see

Do not present the result as a complete timesheet. It is bounded by:

- only issues the authenticated account can **browse**;
- worklogs Jira **withholds** because of visibility restrictions;
- projects where the *Log Work* field is hidden;
- entries whose author carries no stable identifier — these are excluded and
  counted in a `WORKLOG_ACTOR_COVERAGE` notice on stderr;
- issues that could not be read — listed in a `WORKLOG_SOURCE_FAILED` notice
  on stderr.

Read those stderr notices and pass their substance on. "7h 30m across 3 issues
in the window I could see" is honest; "you logged 7h 30m last week" is not.

To reconstruct a worklog from evidence before logging it, the sibling CLIs
cover the other half: `confluence-cli page history --actor me --since 7d` for
page edits and `bitbucket-cli pr activity --actor me --since 7d` for review
activity. They use the same window flags and the same half-open semantics.
