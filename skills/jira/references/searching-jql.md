# Searching with JQL

`jira-cli issue search` is the discovery path: run it whenever you have a
topic, a person, or a status — anything but a concrete issue key.

## Two ways to express a query

**Raw JQL** (full power, quoting is on you):

```bash
jira-cli issue search 'project = ENG AND status = "In Progress" ORDER BY updated DESC'
```

**Filter flags** (AND-joined; the CLI quotes for you):

```bash
jira-cli issue search --project ENG --assignee me --status "In Progress"
jira-cli issue search --text "login crash" --type Bug --label urgent
jira-cli issue search --project ENG --order-by "updated DESC"
```

Do not mix the two — a raw JQL argument plus filter flags is rejected
(`JQL_CONFLICT`).

Flag → JQL mapping:

| Flag | JQL clause |
|------|------------|
| `--project ENG` | `project = "ENG"` |
| `--assignee me` | `assignee = currentUser()` |
| `--assignee unassigned` | `assignee is EMPTY` |
| `--assignee <user>` | `assignee = "<user>"` |
| `--reporter me` | `reporter = currentUser()` |
| `--status "In Progress"` | `status = "In Progress"` |
| `--type Bug` | `issuetype = "Bug"` |
| `--label urgent` | `labels = "urgent"` |
| `--text "crash"` | `text ~ "crash"` |
| `--order-by "updated DESC"` | `ORDER BY updated DESC` |

With no flags at all, `defaults.project` / `JIRA_DEFAULT_PROJECT` (when
configured) scopes the search to that project.

## Valid values

JQL silently matches nothing when a value does not exist (`component =
"Paas"` vs `"PaaS"`). When composing clauses over components, versions,
statuses, types, priorities or labels from a user's loose phrasing, discover
the exact values first — see
[discovering-metadata.md](discovering-metadata.md):

```bash
jira-cli project components ENG     # then: component = "PaaS"
jira-cli project statuses ENG       # then: status = "In Progress"
```

## Fields

Results carry a curated field set (summary, status, assignee, reporter, type,
priority, labels, components, fix versions, project, parent, created, updated).
`--field` replaces the server-side field selection (repeatable):

```bash
jira-cli issue search --project ENG --field summary --field description
```

Only fields represented by the CLI's normalized issue model appear in output.
Requesting `duedate` or `customfield_*` does not expose their values: they are
discarded during normalization. Do not interpret their absence as empty Jira
data. Use another authorized interface when the task needs unsupported fields.
The output-side `--fields a,b.c` projection selects normalized output fields;
it cannot recover discarded data.

## Pagination

One page per call by default. The envelope is `{items, next, has_more}`:

```bash
jira-cli issue search --project ENG --limit 50          # first page
jira-cli issue search --project ENG --cursor "<next>"   # continue
jira-cli issue search --project ENG --all               # only for a complete inventory
```

The cursor is opaque — pass it back verbatim. (Internally it is a
`nextPageToken` on Cloud and a `startAt` offset on Data Center; never
construct one yourself.)

Start with a project and other useful filters. `--limit` controls page size,
not total results; `--all` collects every page before output. Follow `next`
only until the task's evidence needs are met, and disclose incomplete coverage.

## Assignee / reporter values

On Cloud, user-field JQL wants an accountId or an exact display name. Resolve
a fuzzy Cloud name/email query first:

```bash
jira-cli user resolve "alice@example.com"
```

On Data Center, supply a known username from existing issue data or the user.
`user resolve` echoes the input without looking it up; success does not verify
that the account exists or resolve a display name/email to its username.

`me` and `unassigned` always work and need no resolution.
