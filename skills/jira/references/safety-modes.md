# Safety modes — `--dry-run` and read-only

`jira-cli` ships two orthogonal safety mechanisms for agents driving Jira on a
user's behalf. Both protect against the most common failure mode — an
unintended remote mutation — but they answer different questions.

| | Question it answers | Scope |
|---|---|---|
| `--dry-run` | "What HTTP request *would* this command send?" | Per command |
| Read-only mode | "Block all writes for this session." | Per invocation / session |

## `--dry-run` — preview, never send

Every mutating command accepts `--dry-run`. It resolves the operation through
`Client.DescribeWrite(...)` and prints the equivalent HTTP request as JSON,
without sending it:

```bash
jira-cli comment delete 10042 --issue ENG-123 --yes --dry-run
# {
#   "dry_run": true,
#   "method": "DELETE",
#   "url": "https://jira.corp.example/rest/api/2/issue/ENG-123/comment/10042"
# }
```

Use `--dry-run` before any destructive command (`comment delete`) and before
any write whose target (issue key, transition, assignee) was inferred rather
than pasted in literally. Confirm the URL ends with the expected resource.

Read-only lookups needed to compute the payload — assignee resolution on
Cloud, transition-name resolution — still run under `--dry-run`; they are
reads.

## Read-only mode — lock the session

A session-level switch that blocks every mutating client method before any
HTTP request is sent. Enable it by either:

- `defaults.read_only: true` in `~/.angelmsger/jira/config.yaml`, or
- `JIRA_CLI_READ_ONLY=1` in the environment.

Blocked operations return a structured error:

```json
{
  "error": {
    "category": "permission",
    "code": "READONLY_BLOCKED",
    "message": "operation \"CreateIssue\" blocked: read-only mode is enabled",
    "next_steps": [
      "Add --allow-writes to the command line",
      "unset JIRA_CLI_READ_ONLY",
      "Set defaults.read_only=false in ~/.angelmsger/jira/config.yaml"
    ]
  }
}
```

Exit code: 5 (`permission`).

### Per-call override: `--allow-writes`

When you genuinely need to write under a read-only posture, add the root-level
`--allow-writes` flag:

```bash
JIRA_CLI_READ_ONLY=1 jira-cli --allow-writes comment add ENG-123 --body "..."
```

This is the only way to flip the posture for one invocation without changing
config or env.

### What read-only does NOT block

CLI self-configuration is intentionally out of scope, otherwise an agent that
flipped on read-only would lose the ability to recover:

- `config init`, `auth login`, `auth logout`, `config use-context`
- `skill install`, `skill uninstall`

Read-only protects **the remote Jira service**, not `jira-cli`'s own state.

## Recommended pattern for agents

When you receive a task that involves any mutation:

1. **Always run the operation with `--dry-run` first**, especially if the
   target resource (issue key, comment ID, transition) was inferred and not
   pasted in literally. Confirm the URL ends with the expected resource.
2. If the user mentioned "read-only", "don't change anything", or "just
   summarize" — set `JIRA_CLI_READ_ONLY=1` for the rest of the session. Then
   every read-and-summarize command works as normal, and any write you try by
   mistake hits `READONLY_BLOCKED` before reaching the server.
3. The two compose: `JIRA_CLI_READ_ONLY=1 jira-cli comment delete 10042
   --issue ENG-123 --yes --dry-run` is fine and useful — it shows what the
   delete call would send without ever sending one.
