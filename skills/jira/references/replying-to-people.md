# Replying to people

Help the user understand a colleague's comment and prepare an answer they can
stand behind. Do the analysis and draft before asking for approval.

## Classify the comment

Read the relevant comment and surrounding issue context. `comment list`
includes `author` and `body`, but no bot-account flag. Use a recognizable
automation identity or an explicit `[AI]` attribution marker plus the available
context to identify bot/agent content. Treat uncertain authorship as human.

## Human comments

Explain the reason once per session: the reply is posted under the user's name,
and their colleague expects their answer. For each human-authored comment,
present the original point in the author's words, relevant issue evidence,
your reasoning, and the concrete draft. Ask for approval of that reply and
invite corrections. Reuse approval already given for that exact draft and
target; do not ask again before posting it.

Approval of one reply does not approve unseen replies. A broad request to
handle comments is enough to investigate and prepare drafts, but not to post
new answers to people. Honor an explicit informed override if the user gives
one after the reason is explained.

If the user supplies or rewrites the reply, post that text verbatim without
an AI marker. An approved agent-written draft keeps its attribution. Jira's
`comment add` posts a top-level issue comment; there is no `--reply-to` flag.

## Bot comments and ordinary writes

Bot/agent replies and ordinary issue edits follow the user's existing task
authorization without an extra first-write confirmation. Keep agent attribution
and report consequential decisions. The human-comment gate does not stop
independent authorized work.

Preview the request as described in [safety-modes.md](safety-modes.md), and
respect an explicit read-only scope. If writing is outside scope, return the
draft and explain what remains rather than enabling writes yourself.
