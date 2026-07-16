# Writing issues and comments

Every command here is a **write**: each accepts `--dry-run` (preview the HTTP
request without sending it), and all of them are blocked by read-only mode.
See [safety-modes.md](safety-modes.md).

## Create

```bash
jira-cli issue create --project ENG --type Task --summary "Fix login crash"
jira-cli issue create --project ENG --type Bug --summary "..." \
    --description-file report.txt --assignee alice@example.com \
    --label urgent --label auth --priority High --parent ENG-100
```

- `--project` falls back to `defaults.project` / `JIRA_DEFAULT_PROJECT`.
- `--type` is the issue type *name* (`Task`, `Bug`, `Story`, ...); it must
  exist in the project. The server rejects unknown types with a field error.
- `--parent` places the issue under an epic, or makes a subtask when `--type`
  is a subtask type.
- The created issue is re-read and printed in full, so the output carries the
  new key.

## Edit

Only the flags you pass change:

```bash
jira-cli issue edit ENG-123 --summary "New title"
jira-cli issue edit ENG-123 --description-file spec.txt
jira-cli issue edit ENG-123 --add-label triaged --remove-label needs-triage
jira-cli issue edit ENG-123 --priority Low
```

`--add-label`/`--remove-label` adjust the label set incrementally (other
labels are untouched). The updated issue is re-read and printed.

## Assign

```bash
jira-cli issue assign ENG-123 --to alice@example.com
jira-cli issue assign ENG-123 --unassign
```

`--to` accepts a Cloud accountId, a Data Center username, or a display-name/
email query. On Cloud, non-accountId values are resolved via user search and
must match exactly one active user — `USER_AMBIGUOUS` lists the candidates
when they don't; pick the accountId from the list. Resolution needs the
Browse Users permission.

## Transition (workflow)

Transitions depend on the issue's *current* status and your permissions, so
always discover first when unsure:

```bash
jira-cli issue transitions ENG-123
# → items: [{id: "21", name: "Start Progress", to: {name: "In Progress"}}, ...]
jira-cli issue transition ENG-123 --to "Start Progress"
jira-cli issue transition ENG-123 --to 21 --comment "Picked up."
```

`--to` matches a transition ID, a transition name (case-insensitive), or an
unambiguous target-status name. Ambiguity and misses fail with the candidate
list. The issue is re-read after the transition so the output shows the new
status.

## Comments

```bash
jira-cli comment list ENG-123 --all
jira-cli comment add ENG-123 --body "Deployed to staging."
echo "Longer text..." | jira-cli comment add ENG-123 --body-file -
jira-cli comment update 10042 --issue ENG-123 --body "Revised."
jira-cli comment delete 10042 --issue ENG-123 --yes
```

`comment delete` is destructive: it requires `--yes` (exit 2 with
`DELETE_NEEDS_YES` otherwise). It takes several IDs at once, or a single `-`
to read newline-separated IDs from stdin; with more than one, output is a
per-item `ok`/`error` aggregate and the exit code is non-zero on any failure.

## Body text: the Cloud/DC asymmetry

Descriptions and comment bodies are plain text on both flavors, with one
flavor-specific behavior each:

- **Cloud (REST v3)**: the CLI converts your text to an ADF document — a
  blank line starts a new paragraph, a single newline becomes a line break.
  On read, ADF is flattened back to text (lists become `- ` lines, mentions
  become their display text; tables/panels degrade to their text content).
  Markdown and Jira wiki markup are **not** interpreted.
- **Data Center (REST v2)**: your text is sent verbatim. Jira wiki markup
  (`*bold*`, `{code}`) renders server-side, so it works — but it will not
  render on Cloud, so avoid it in content that may be read from both.

Safe subset for cross-flavor content: plain paragraphs and blank lines.
