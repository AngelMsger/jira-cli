# Changelog

All notable changes to `jira-cli` are documented in this file.

The format is based on [Keep a Changelog](https://keepachangelog.com/en/1.1.0/),
and this project adheres to [Semantic Versioning](https://semver.org/spec/v2.0.0.html).

## [Unreleased]

## [0.2.0] - 2026-08-11

### Added

- **Metadata discovery: enumerate the valid values of issue fields.** The CLI
  could validate a known value through JQL but not tell an agent what values
  exist — the normalized issue even dropped an explicitly requested
  `components` field. New commands close the gap:
  - `project components` / `project versions` / `project issuetypes` /
    `project statuses` list a project's components, fix/affects versions,
    creatable issue types (from the create metadata, so permissions apply) and
    workflow statuses per issue type.
  - `priority list` and `label list` cover the instance-wide value sets
    (`label list` is Jira Cloud only — Data Center has no label-listing
    endpoint; the DC error says how to discover labels from search results).
  - `field list --project --type` and `field options <field> --project
    [--type]` are the generic form: create-screen field metadata and any
    constrained field's allowed values — including `customfield_*` select
    fields — honoring Jira field contexts. Without `--type`, every creatable
    issue type is scanned and each option is annotated with the issue types it
    applies to.
- Normalized issues now carry `components`, `fix_versions` and
  `affects_versions` (name arrays); search results include components and
  fixVersions in the default field set, and an explicitly requested
  `--field components` is no longer dropped by the mapper.
- Two new capability-table rows record the flavor divergences: label listing
  (unsupported on DC) and metadata-list pagination (DC returns components,
  versions and priorities as one full-list response).
- **Grok Build skill install target.** `skill install` now deploys the companion
  Skill for Grok Build (`--agent grok`) to `~/.grok/skills/<name>` globally and
  `./.grok/skills/<name>` with `--project`. Auto-detection probes `~/.grok` /
  `./.grok` alongside Claude Code and Codex. Installation guides, generated CLI
  docs, and agent help text list the new target.


## [0.1.1] - 2026-07-16

### Added

- Family-style hero image (`docs/image.png`), shown in the README and in the
  docs-site hero like the sibling CLIs.

### Changed

- README now follows the family-canonical section order ("Errors and exit
  codes" section, "Related" moved to the end) and lists siblings in the
  family-canonical order, as does the docs-site footer.

### Fixed

- The npm installation banner now suggests Jira `issue search` and `issue get`
  commands instead of copied Confluence commands.
- The npm package README now describes Jira issue workflows (the previous
  description was copied from confluence-cli's "external knowledge base"
  text).

## [0.1.0] - 2026-07-16

### Added

- Initial release: an agent-facing CLI for Jira Cloud (REST v3) and Data
  Center / Server (REST v2) behind one flavor-agnostic client.
- `issue get` / `issue search` (raw JQL or composed from
  `--project/--assignee/--status/--type/--label/--text/--order-by`, with
  `me`/`unassigned` conveniences), `issue create`, `issue edit`,
  `issue assign` (with Cloud user resolution via `user resolve`),
  `issue transitions` and `issue transition` (name → ID resolution with
  candidate listing on ambiguity).
- `project list` / `project get`, `comment list/add/update/delete`
  (batch + stdin `-` on delete), `whoami`, `user resolve`.
- Family safety contract: `--dry-run` on every write via
  `Client.DescribeWrite`, session read-only posture
  (`defaults.read_only` / `JIRA_CLI_READ_ONLY=1`, overridable with
  `--allow-writes`), `--yes` confirmation on `comment delete`.
- Plain-text body contract on both flavors: text → ADF paragraphs on Cloud
  writes, ADF → text on Cloud reads; verbatim strings on Data Center. The
  divergence is recorded in the capability table
  (`pkg/apiclient/capability.go`).
- Meta commands shared with the CLI family: `config` (multi-context wizard),
  `auth`, `doctor`, `skill` (embedded companion Skill for Claude Code /
  Codex), `completion`, `version`; structured JSON errors with stable exit
  codes; `{items, next, has_more}` list envelope; `--fields` projection;
  forgiving flag normalization; unknown-subcommand rejection (exit 2);
  update notifier.

### Known gaps

- Issue types and priorities are not yet discoverable via a dedicated
  command (`issue create --type` relies on server-side validation errors).
- Worklogs, watchers, issue links, attachments and Agile boards/sprints are
  out of scope for v0.1.
- The e2e mockserver fakes the Data Center dialect only; the Cloud
  `/search/jql` path is covered by unit tests.

[Unreleased]: https://github.com/AngelMsger/jira-cli/compare/v0.2.0...HEAD
[0.2.0]: https://github.com/AngelMsger/jira-cli/compare/v0.1.1...v0.2.0
[0.1.1]: https://github.com/AngelMsger/jira-cli/compare/v0.1.0...v0.1.1
[0.1.0]: https://github.com/AngelMsger/jira-cli/releases/tag/v0.1.0
