# Spec 001 — rooms and the runner: the walking skeleton

**Work ID:** `001-the-room-model` (chronicle-agents-hq research 001, questions 1, 4 and 5 graduated)
**Design:** `chronicle-agents-hq` @ `736c9aa` (hq PR #2, merged) —
[`03-DECISIONS/0003-a-room-is-a-type-an-agent-is-a-declaration.md`](../../../chronicle-agents-hq/03-DECISIONS/0003-a-room-is-a-type-an-agent-is-a-declaration.md),
[`02-DESIGN/01-rooms-and-messages.md`](../../../chronicle-agents-hq/02-DESIGN/01-rooms-and-messages.md),
[`02-DESIGN/02-agents-and-the-runner.md`](../../../chronicle-agents-hq/02-DESIGN/02-agents-and-the-runner.md);
their `lands` block names chronicle-agents-hq first, this repo after it.
**Status:** in progress ([plan.md](plan.md)).

## What this delivers

The first code of this repository: the two types a workspace declares, the
definition file, and a runner that turns a mention or an op in a chronicle
store into one idempotent turn by an agent's own identity — design 02's
core contract, on chronicle as released (`github.com/impire-io/chronicle`
v0.4.0), tested against a real embedded chronicle, never a mock.

- **The types.** `room` and `message` as design 01 states them, shipped as
  data the CLI declares into a store: `chronicle-agents store init` declares
  both types (again to revise) through the chronicle client, into the
  context's selected store or `--store`.
- **The definition file.** One YAML file per agent with the six facets of
  design 02: `identity` (the service-account name), `triggers` (a list of
  `mention` and `op` entries; `subject` and `schedule` entries are parsed
  and **refused at load** with the reason — subject lands in a following
  spec, schedule waits on chronicle-61), `harness` (a preset name or a
  template), `instructions`, `toolkits` (MCP server commands), `budget`
  (`turns`, `window`, `depth`, `reply_to_agents`). Loading validates;
  an invalid file is refused with the field named.
- **The runner.** `chronicle-agents run <file>` — one process, one
  declaration, one credential (the connection flags chronicle's CLI uses:
  `--url`, `--creds` or `--nkey` with the identity as principal). It tails
  the declaration's stores from its checkpoint, matches each op against
  the triggers, drops its own ops and ops under closed rooms, admits or
  refuses by the window floor, the depth bound and the agent-to-agent rule
  (read from the record: authored turns under the room, the parent chain,
  the registry's service accounts), composes the turn from the room's
  projection (the state bucket listing under the room, plus the runner's
  per-room aggregate) with queued triggers coalesced, invokes the harness,
  and writes the turn as `room/<id>/messages/m-<derived>` with the
  create-if-absent guard, `kind: turn`, `reply_to`, `mentions`, and every
  trigger's op id as a parent. A refused turn posts nothing and logs one
  structured line; a refused write (already answered) is read as such.
- **The harness.** The invocation template of design 02: command and
  arguments with placeholders for the prompt file, the context file, the
  MCP configuration and the run directory; how stdout is read; an optional
  environment block. A fresh run directory per turn, the MCP configuration
  generated from the toolkits, the inherited environment scrubbed, the
  process group killed at the deadline. Two presets: `claude-code`
  (`claude -p`, tools from the MCP configuration only, as the spike ran it)
  and `command` (any executable reading the context on stdin — the
  tests' harness).
- **The gate.** `make check` green; tests boot the chronicle quick start
  in-process (`github.com/impire-io/chronicle/up`) and prove: the types
  declare; a mention produces exactly one turn by the agent's identity with
  the trigger as parent; the same trigger delivered again produces no
  second turn; the agent's own turn triggers nothing; the third turn inside
  the window is refused with no op written; a mention from a service
  account is refused unless `reply_to_agents` is set; a declaration with a
  schedule trigger is refused at load.

## What it does not do

No subject triggers (next spec), no schedule triggers (chronicle-61), no
web interface, no placement, no release pipeline, no persisted checkpoint
beyond the process (the record is the position: a restart replays from the
start and the guard refuses what was answered), no retrieval toolkit over
chronicle's indexes (chronicle's MCP server is research 013's), no
children-inclusive tail (chronicle-59: the runner tails the whole store
and matches on the path). Nothing here adds to chronicle's wire; every
read and write is the Go client's.
