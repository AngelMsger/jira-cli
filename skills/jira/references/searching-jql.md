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

## Fields

Results carry a curated field set (summary, status, assignee, reporter, type,
priority, labels, project, parent, created, updated). Widen or narrow it with
`--field` (repeatable):

```bash
jira-cli issue search --project ENG --field summary --field duedate
```

This is the *server-side* field list; the output-side `--fields a,b.c`
projection composes with it.

## Pagination

One page per call by default. The envelope is `{items, next, has_more}`:

```bash
jira-cli issue search --project ENG --limit 50          # first page
jira-cli issue search --project ENG --cursor "<next>"   # continue
jira-cli issue search --project ENG --all               # walk every page
```

The cursor is opaque — pass it back verbatim. (Internally it is a
`nextPageToken` on Cloud and a `startAt` offset on Data Center; never
construct one yourself.)

## Assignee / reporter values

On Cloud, user-field JQL wants an accountId or an exact display name; on Data
Center a username. When the user gives you a fuzzy name, resolve it first:

```bash
jira-cli user resolve "alice@example.com"
```

`me` and `unassigned` always work and need no resolution.
