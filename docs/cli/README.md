# jira-cli command reference

This index is generated from the CLI command tree — do not edit it by
hand; run `make docs`. The full reference, with every flag and example,
is published at <https://angelmsger.github.io/jira-cli/cli/>.

## auth

| Command | Description |
| --- | --- |
| [`jira-cli auth`](https://angelmsger.github.io/jira-cli/cli/#jira-cli-auth) | Inspect and manage stored credentials |
| [`jira-cli auth login`](https://angelmsger.github.io/jira-cli/cli/#jira-cli-auth-login) | Store a credential for the configured server |
| [`jira-cli auth logout`](https://angelmsger.github.io/jira-cli/cli/#jira-cli-auth-logout) | Remove the stored credential for the configured server |
| [`jira-cli auth status`](https://angelmsger.github.io/jira-cli/cli/#jira-cli-auth-status) | Show whether a usable credential is configured |

## comment

| Command | Description |
| --- | --- |
| [`jira-cli comment`](https://angelmsger.github.io/jira-cli/cli/#jira-cli-comment) | Read and post issue comments |
| [`jira-cli comment add`](https://angelmsger.github.io/jira-cli/cli/#jira-cli-comment-add) | Post a comment on an issue |
| [`jira-cli comment delete`](https://angelmsger.github.io/jira-cli/cli/#jira-cli-comment-delete) | Delete one or more comments |
| [`jira-cli comment list`](https://angelmsger.github.io/jira-cli/cli/#jira-cli-comment-list) | List an issue's comments (oldest first) |
| [`jira-cli comment update`](https://angelmsger.github.io/jira-cli/cli/#jira-cli-comment-update) | Replace a comment's body |

## config

| Command | Description |
| --- | --- |
| [`jira-cli config`](https://angelmsger.github.io/jira-cli/cli/#jira-cli-config) | Manage jira-cli configuration |
| [`jira-cli config delete-context`](https://angelmsger.github.io/jira-cli/cli/#jira-cli-config-delete-context) | Delete a context and its stored credential |
| [`jira-cli config get-contexts`](https://angelmsger.github.io/jira-cli/cli/#jira-cli-config-get-contexts) | List the configured contexts |
| [`jira-cli config init`](https://angelmsger.github.io/jira-cli/cli/#jira-cli-config-init) | Interactively set up server URL and credentials |
| [`jira-cli config path`](https://angelmsger.github.io/jira-cli/cli/#jira-cli-config-path) | Print the config file path |
| [`jira-cli config show`](https://angelmsger.github.io/jira-cli/cli/#jira-cli-config-show) | Show the resolved configuration |
| [`jira-cli config use-context`](https://angelmsger.github.io/jira-cli/cli/#jira-cli-config-use-context) | Switch the current context |

## doctor

| Command | Description |
| --- | --- |
| [`jira-cli doctor`](https://angelmsger.github.io/jira-cli/cli/#jira-cli-doctor) | Diagnose configuration, credentials and connectivity |

## field

| Command | Description |
| --- | --- |
| [`jira-cli field`](https://angelmsger.github.io/jira-cli/cli/#jira-cli-field) | Discover issue fields and their allowed values |
| [`jira-cli field list`](https://angelmsger.github.io/jira-cli/cli/#jira-cli-field-list) | List the fields on an issue type's create screen |
| [`jira-cli field options`](https://angelmsger.github.io/jira-cli/cli/#jira-cli-field-options) | List a field's allowed values in a project |

## issue

| Command | Description |
| --- | --- |
| [`jira-cli issue`](https://angelmsger.github.io/jira-cli/cli/#jira-cli-issue) | Read, search and write Jira issues |
| [`jira-cli issue assign`](https://angelmsger.github.io/jira-cli/cli/#jira-cli-issue-assign) | Change or clear an issue's assignee |
| [`jira-cli issue create`](https://angelmsger.github.io/jira-cli/cli/#jira-cli-issue-create) | Create an issue |
| [`jira-cli issue edit`](https://angelmsger.github.io/jira-cli/cli/#jira-cli-issue-edit) | Update issue fields |
| [`jira-cli issue get`](https://angelmsger.github.io/jira-cli/cli/#jira-cli-issue-get) | Show one issue |
| [`jira-cli issue search`](https://angelmsger.github.io/jira-cli/cli/#jira-cli-issue-search) | Search issues with JQL or filter flags |
| [`jira-cli issue transition`](https://angelmsger.github.io/jira-cli/cli/#jira-cli-issue-transition) | Move an issue through a workflow transition |
| [`jira-cli issue transitions`](https://angelmsger.github.io/jira-cli/cli/#jira-cli-issue-transitions) | List the workflow transitions currently available on an issue |

## label

| Command | Description |
| --- | --- |
| [`jira-cli label`](https://angelmsger.github.io/jira-cli/cli/#jira-cli-label) | Browse issue labels |
| [`jira-cli label list`](https://angelmsger.github.io/jira-cli/cli/#jira-cli-label-list) | List issue labels (Jira Cloud only) |

## priority

| Command | Description |
| --- | --- |
| [`jira-cli priority`](https://angelmsger.github.io/jira-cli/cli/#jira-cli-priority) | Browse issue priorities |
| [`jira-cli priority list`](https://angelmsger.github.io/jira-cli/cli/#jira-cli-priority-list) | List issue priorities |

## project

| Command | Description |
| --- | --- |
| [`jira-cli project`](https://angelmsger.github.io/jira-cli/cli/#jira-cli-project) | Browse Jira projects |
| [`jira-cli project components`](https://angelmsger.github.io/jira-cli/cli/#jira-cli-project-components) | List a project's components |
| [`jira-cli project get`](https://angelmsger.github.io/jira-cli/cli/#jira-cli-project-get) | Show one project |
| [`jira-cli project issuetypes`](https://angelmsger.github.io/jira-cli/cli/#jira-cli-project-issuetypes) | List the issue types creatable in a project |
| [`jira-cli project list`](https://angelmsger.github.io/jira-cli/cli/#jira-cli-project-list) | List projects visible to the authenticated user |
| [`jira-cli project statuses`](https://angelmsger.github.io/jira-cli/cli/#jira-cli-project-statuses) | List a project's workflow statuses per issue type |
| [`jira-cli project versions`](https://angelmsger.github.io/jira-cli/cli/#jira-cli-project-versions) | List a project's versions |

## skill

| Command | Description |
| --- | --- |
| [`jira-cli skill`](https://angelmsger.github.io/jira-cli/cli/#jira-cli-skill) | Install the companion Skill for coding agents (Claude Code, Codex, Grok Build) |
| [`jira-cli skill install`](https://angelmsger.github.io/jira-cli/cli/#jira-cli-skill-install) | Deploy the embedded Skill into a coding agent's skills directory |
| [`jira-cli skill path`](https://angelmsger.github.io/jira-cli/cli/#jira-cli-skill-path) | Print where the Skill would be installed, and whether it is |
| [`jira-cli skill show`](https://angelmsger.github.io/jira-cli/cli/#jira-cli-skill-show) | Print the embedded SKILL.md to stdout |
| [`jira-cli skill status`](https://angelmsger.github.io/jira-cli/cli/#jira-cli-skill-status) | Report whether the companion Skill is loaded and installed |
| [`jira-cli skill uninstall`](https://angelmsger.github.io/jira-cli/cli/#jira-cli-skill-uninstall) | Remove the companion Skill from a coding agent's skills directory |

## user

| Command | Description |
| --- | --- |
| [`jira-cli user`](https://angelmsger.github.io/jira-cli/cli/#jira-cli-user) | Discover Jira users — the values assignee flags accept |
| [`jira-cli user me`](https://angelmsger.github.io/jira-cli/cli/#jira-cli-user-me) | Print the user the configured credentials authenticate as (alias for whoami) |
| [`jira-cli user resolve`](https://angelmsger.github.io/jira-cli/cli/#jira-cli-user-resolve) | Resolve a user selector to a unique user |

## version

| Command | Description |
| --- | --- |
| [`jira-cli version`](https://angelmsger.github.io/jira-cli/cli/#jira-cli-version) | Print version information |

## whoami

| Command | Description |
| --- | --- |
| [`jira-cli whoami`](https://angelmsger.github.io/jira-cli/cli/#jira-cli-whoami) | Print the user the configured credentials authenticate as |

