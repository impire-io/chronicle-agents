# Spec 009 — the agent's turn: the event stream, the steps on the record, the question in the room

**Work ID:** `the-agents-turn` (chronicle-agents-hq research 003)
**Design:** `chronicle-agents-hq` @ the commit the `lands` block of
[`02-DESIGN/02-agents-and-the-runner.md`](../../../chronicle-agents-hq/02-DESIGN/02-agents-and-the-runner.md)
names (PR #17) —
[`03-DECISIONS/0010-a-turn-carries-its-steps-and-a-question-reaches-the-room.md`](../../../chronicle-agents-hq/03-DECISIONS/0010-a-turn-carries-its-steps-and-a-question-reaches-the-room.md),
[`02-DESIGN/02-agents-and-the-runner.md`](../../../chronicle-agents-hq/02-DESIGN/02-agents-and-the-runner.md)
§ the declaration (tools, workdir), § from trigger to turn (the turn's steps and
usage), § live signals (writing, step, waiting), § the harness (the event
stream, the runner's MCP server, the question and the permission);
[`02-DESIGN/01-rooms-and-messages.md`](../../../chronicle-agents-hq/02-DESIGN/01-rooms-and-messages.md)
§ the message (the question kind, choices, steps, usage).
**Status:** in progress ([plan.md](plan.md)).

## What this delivers

- **The harness's output is an event stream, decoded as it arrives**
  (`harness/`): a template names how its standard output is read — `text`,
  the whole of it as the turn, or `claude-stream`, Claude Code's
  `--output-format stream-json`, one JSON object per line. The decoder
  reads the text as it is written (the partial-message deltas, then each
  text block), every tool call as it starts (`assistant` / `tool_use`) and
  comes back (`user` / `tool_result`, with `is_error`), and the `result`
  with its text, session, cost, tokens and turns; a subagent's lines are
  not the turn's; a result that is not a success fails the turn with its
  subtype and errors; a stream without a result keeps the last text. The
  deadline error wraps `context.DeadlineExceeded`, so the runner names it.
- **The `claude-code` preset, corrected**: `--output-format stream-json
  --verbose --include-partial-messages`; `--setting-sources ""` and
  `--permission-mode default`, so none of the operator's own allow rules,
  hooks or plugins reach the agent; `--mcp-config` with `--strict-mcp-config`
  always, carrying the declaration's toolkits and the runner's own server;
  `--tools` with the granted tools' names and nothing else (`""` when none);
  `--allowedTools` with the granted entries as written, plus every MCP
  server; `--permission-prompt-tool mcp__chronicle-agents__permission`.
  `MCP_TOOL_TIMEOUT` is the turn's deadline, so a call that waits on a
  person may last the turn. The run directory is the declaration's
  `workdir` when it names one.
- **The declaration gains `harness.tools` and `harness.workdir`**
  (`declaration/`): `tools` are the harness's own tools by the harness's
  names, each optionally with a rule in the harness's syntax (`Bash(git *)`)
  — the name grants the tool, the entry pre-approves the calls it covers, a
  bare name pre-approves every call; `workdir` is an absolute directory
  that exists. A template may name `output`. The announcement
  (`agent/<name>`) gains `tools`: the entries and the toolkits by name.
- **The runner's MCP server, one per turn** (`mcpserver/`, `chronicle-agents
  mcp`): started by the harness from the generated MCP configuration, over
  standard input and output, configured by `CHRONICLE_AGENTS_*` in its
  environment (the connection as the runner was given it, the agent, the
  store, the room, the turn, the askers, the deadline), acting under the
  agent's credential. Its tools: `ask` (a question with optional choices,
  answered in the room), `permission` (Claude Code's permission prompt:
  the tool's name and a summary of its input as a question with the
  choices *allow* and *deny*; the answer back as `{"behavior":"allow",
  "updatedInput":…}` or a deny with the person's words), `read_room` (a
  room's transcript from the state bucket, the latest N when asked) and
  `list_rooms`.
- **A question is a message** (`types/`): `kind: question`, `text`, the
  `choices`, the askers as `mentions`, written by the agent at
  `room/<id>/messages/q-<id>`; the answer is the first message by anyone
  but the agent whose `reply_to` names it, read from the store's tail,
  until the turn's deadline (`ErrUnanswered`). The runner sees its own
  question land while its turn runs and says `waiting` live; an answer to
  its question is never a trigger, mention or not.
- **The turn carries its steps and its cost**: `steps` — per tool call
  the tool, a bounded summary of the input (the command, the path, the
  query, else compact JSON; 400 runes) and of the output, `ok`, and how
  long it took — and `usage` — model, tokens, cost, the harness's rounds,
  its session id. The message type's schema carries `choices`, `steps`,
  `usage`, and `question` among the kinds; `store init` and the manifest
  carry the revised type.
- **Live, while the turn runs** (`live/`): `writing` with the text so far,
  whole, at most every 300 ms; `step` with the tool, the input summary and
  `started`, then `finished` with the output summary and `ok`; `waiting`
  with the question's path and text. `picked-up`, `thinking` (the
  heartbeat), `done`, `refused` and `failed` stay as they were.
- **The context tells the agent what it has**: its tools and toolkits by
  name, or that it has none and must answer from the transcript rather
  than write out a call it cannot make; and that it may read rooms and
  ask the people it answers through the runner's server.
- **Proofs**: the decoder on a recorded stream (`harness/harness_test.go`);
  the preset's arguments; the run directory; the deadline wrapped; the
  server's `ask`, `permission` (allow and deny, in the person's words),
  `read_room`, `list_rooms`, the deadline, the configuration's round trip
  through the environment, over an in-memory MCP transport against a real
  quick start (`mcpserver/mcpserver_test.go`); the runner writing steps
  and usage on the turn and saying `writing` and `step` live, saying
  `waiting` on its own question and not answering the answer, announcing
  its tools (`runner/turn_test.go`); the `mcp` sentence refusing to run
  without the runner's environment (`cli/mcp_test.go`). Walked by hand
  against a quick start with the real Claude Code: wilson with
  `Bash(ls *)` listing its directory in one step; asked for a write, the
  question *wilson wants to use Bash: date > stamp2.txt… Allow it?* in the
  room, answered *allow* from the CLI, the file written and the turn
  posted 8.9 s after the mention.

## What this does not do

No session resume (research 003 question 7 closes as not yet); no step
written as a message of its own; the full tool input and output are not
on the record (the summaries are, the rest is the harness's own log);
the page does not render the new signals or kinds yet — that is research
002's build, which follows this one.
