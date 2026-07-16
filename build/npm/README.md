# @angelmsger/jira-cli

npm distribution of [`jira-cli`](https://github.com/AngelMsger/jira-cli)
— a command-line tool that reads, searches and maintains Jira issues from the
terminal — JQL search, workflow transitions, comments — built for coding agents
(Claude Code and others) and humans alike. Supports Jira Cloud and
Data Center / Server.

```bash
npm install -g @angelmsger/jira-cli
jira-cli config init --pretty        # interactive TUI: server URL + credentials
jira-cli skill install               # deploy the companion agent Skill
jira-cli issue search --assignee me  # my open issues
```

Installing this package downloads the prebuilt binary for your platform from the
matching GitHub Release and verifies its SHA-256 checksum. If your npm setup
disables install scripts, the binary is fetched on first run instead.

The companion `jira` Skill for coding agents is embedded in the binary;
`jira-cli skill install` deploys a copy that always matches the installed
CLI version.

See the [project README](https://github.com/AngelMsger/jira-cli) and the
[installation guide](https://github.com/AngelMsger/jira-cli/blob/main/docs/installation.md)
for full documentation.
