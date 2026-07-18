# Discovering valid field values (metadata)

Jira rejects writes and silently empty-matches JQL when a field value does not
exist — and most field values are **scoped**: components, versions, issue
types and statuses belong to a project, and custom select options can vary by
project *and* issue type (Jira "field contexts"). Never guess a value; when
the user's phrasing does not match a value you have already seen, discover
first, then act.

## Project-scoped discovery

```bash
jira-cli project components ENG    # values for the components field
jira-cli project versions ENG      # values for fixVersions / affects versions
jira-cli project issuetypes ENG    # what `issue create --type` accepts here
jira-cli project statuses ENG      # workflow statuses, grouped by issue type
```

All four accept a project key or any Jira URL that carries one.
`components` / `versions` / `issuetypes` return the standard `{items, next,
has_more}` envelope; `statuses` is always a single page, and its items group
statuses per issue type (different issue types can run different workflows).

## Instance-wide discovery

```bash
jira-cli priority list             # what --priority accepts
jira-cli label list                # every label in use (Jira Cloud only)
```

`label list` is **Cloud-only**: Data Center has no label-listing endpoint and
the command fails there with `LABEL_LIST_DC`. On DC, discover labels from the
issues that carry them (labels are part of search results).

## Generic discovery: any field, incl. custom fields

`field list` shows every field on an issue type's create screen; `field
options` reads one constrained field's allowed values. This is the path for
**custom select fields** (`customfield_*`), and it honors Jira field contexts:

```bash
jira-cli field list --project ENG --type Bug
# → items: [{id, name, required, type, options_count, ...}]

jira-cli field options components --project ENG
jira-cli field options "Severity" --project ENG --type Bug
# → {project, field: {id, name, type}, options: [{id, value, issue_types}]}
```

- `<field>` matches the field id (`components`, `priority`,
  `customfield_10010`) or its display name, case-insensitively.
- With `--type`, options are read for that one issue type. Without it, every
  creatable issue type is scanned and each option's `issue_types` says where
  it applies — expect one request per issue type.
- `FIELD_NOT_FOUND` lists the fields that do have options in the project.
- `--project` falls back to `defaults.project` / `JIRA_DEFAULT_PROJECT`.

## Typical flows

- **Compose valid JQL**: `project components ENG` → pick the exact name →
  `issue search 'project = ENG AND component = "PaaS"'`.
- **Create without a rejection round-trip**: `project issuetypes ENG` for
  `--type`, `priority list` for `--priority`, then `issue create`.
- **A write failed with a field error**: run `field options <field>
  --project <key>` and retry with a listed value.

## Reading option fields on issues

Normalized issues carry `components`, `fix_versions` and `affects_versions`
(name arrays). Search results include components and fixVersions in the
default field set; request `--field versions` to add affects-versions.
