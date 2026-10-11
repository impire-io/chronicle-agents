# Spec 002 — subjects, schedules and the workspace

**Work ID:** `001-the-room-model` (chronicle-agents-hq research 001, graduated by decision 0004)
**Design:** `chronicle-agents-hq` @ `84f5d97` (hq PR #5) —
[`03-DECISIONS/0004-the-definition-file-is-fixed-and-a-trigger-outside-a-room-names-its-room.md`](../../../chronicle-agents-hq/03-DECISIONS/0004-the-definition-file-is-fixed-and-a-trigger-outside-a-room-names-its-room.md),
the amended [`02-DESIGN/01-rooms-and-messages.md`](../../../chronicle-agents-hq/02-DESIGN/01-rooms-and-messages.md) and
[`02-DESIGN/02-agents-and-the-runner.md`](../../../chronicle-agents-hq/02-DESIGN/02-agents-and-the-runner.md).
**Status:** in progress ([plan.md](plan.md)).

## What this delivers

The two trigger kinds spec 001 refused at load, on chronicle v0.5.0, and
the workspace they need:

- **Subject triggers.** A `subject` entry names a NATS subject and the
  `room` its turn is written in. The runner subscribes with the agent's
  credential; one message is one trigger whose identity is a digest of the
  subject and payload, so the same message twice derives the same turn and
  the guard refuses the second. The payload is rendered into the context as
  what arrived.
- **Schedule triggers.** A `schedule` entry names `schedule/<name>` in the
  tick store and the `room` its turn goes to. `chronicle-agents schedule
  create NAME (--every D | --cron EXPR) [--ttl D]` registers the schedule as
  one headered publish; the server writes a tick under
  `schedule/<name>/ticks/t` at every firing (chronicle-hq decision 0050).
  A tick's identity is its sequence. The runner has no clock.
- **The workspace.** `chronicle-agents store init` creates the rooms store
  (declaring the two types) and the tick store (`--schedules`) when
  missing, and is idempotent. `room` and `room_store` on a trigger entry;
  an `op` entry under a path outside the rooms names its room too.
- **Harness presets `pi` and `hermes`**, with the text placeholders
  `{{PROMPT}}` and `{{CONTEXT_TEXT}}` for harnesses that take their prompt
  as an argument. Toolkits stay wired for `claude-code` only; the others
  configure MCP in their own homes.
- **Proofs** against the embedded quick start: a subject message triggers a
  turn in the named room and the same message is answered once; a schedule
  registered at one second produces two distinct turns and its ticks are
  in the record under the schedule; `store init` twice; `schedule create`.

## What it does not do

No children-inclusive tail in the runner yet (it tails whole stores; the
web interface uses the children tail), no mentions index, no web
interface, no placement, no toolkits for pi and hermes.
