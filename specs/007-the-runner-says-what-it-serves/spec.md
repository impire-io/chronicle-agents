# Spec 007 — the runner says what it serves

**Work ID:** `chronicle-agents-6` (tracker chronicle-agents-6)
**Design:** `chronicle-agents-hq` @ `af1bb2b` (PR #14) —
[`03-DECISIONS/0009-the-rooms-page-is-a-chat.md`](../../../chronicle-agents-hq/03-DECISIONS/0009-the-rooms-page-is-a-chat.md),
[`02-DESIGN/01-rooms-and-messages.md`](../../../chronicle-agents-hq/02-DESIGN/01-rooms-and-messages.md)
§ the room (*General*), § the workspace (the `agent` type), § transient
traffic;
[`02-DESIGN/02-agents-and-the-runner.md`](../../../chronicle-agents-hq/02-DESIGN/02-agents-and-the-runner.md)
§ the declaration (`description`), § live signals. The namespace is
chronicle-hq decision 0055 (`CHRON.LIVE.>`, @ `b23de71`). The page's side
is chronicle-agents-7, not this spec's.
**Status:** in progress ([plan.md](plan.md)).

## What this delivers

- **`description`** in the definition file (`declaration`): one line of
  what the agent is for; optional; `examples/wilson.yaml` has one. The
  file stays strict about unknown fields.
- **The `agent` type** (`types/agent.go`): `agent/<name>` — `name`
  (required), `description`, `harness`, `triggers` (one line each),
  `version`; `create` and `update`, both merge; compactable; no children.
  `Declare` defines it after `room`; the workspace declaration lists it
  after `solution`, so `store init` defines it and `/install.json`
  carries it.
- **The announcement**: on `run`, after connecting and before the tails
  start, the runner writes `agent/<identity>` in every rooms store it
  serves (`Declaration.RoomStores`: a mention's or op's store, the room
  store a subject, schedule or out-of-rooms op names; never the tick
  store) — create-if-absent, then `update` only when a field differs
  (`types.AnnounceAgent`). A store that is not there is a warning. The
  version is the binary's (`runner.Version`, from `cli.Version`).
- **General** (`types.EnsureGeneral`): `store init` creates `room/general`
  — *General*, "where a workspace starts", open — create-if-absent in the
  announcing store, after the types; the manifest is unchanged (the page
  creates it on an empty workspace, decision 0009 point 1).
- **Live signals** (`live`): `CHRON.LIVE.<store>.room.<id>` carries
  `{agent, state, trigger, at}` with `reason` on `refused` and `failed` —
  `picked-up` on admission, `thinking` every 3 s while the harness runs,
  `done` after the turn is written, `refused` in the person's words
  (*2 turns in a minute*, *a chain 4 deep*, *a mention from another
  agent*), `failed` with why; `CHRON.LIVE.<store>.agent.<name>` carries
  `{agent, state: online, since, version}` at start and every 10 s. Plain
  publishes on the client's connection; nothing stored, nothing retried.
- **Proofs** (`runner/live_test.go`, `types/solution_test.go`,
  `declaration/declaration_test.go`, `cli/store_test.go`): the signal
  sequence for an admitted mention, a window refusal and an agent's
  mention, each in words; `thinking` from a slow harness; `failed` from a
  harness that exits non-zero; the announcement read back by wilson's
  own authorship, unchanged on re-announcing, one `update` on a change;
  presence with the version; the stream grows by the mention alone — no
  signal reaches it; General once and again; `Describe` for every
  trigger kind; `store init` naming three types and General.

## What it does not do

Nothing reads the signals yet — the page does (chronicle-agents-7).
Presence is per rooms store the agent serves, not per account. A
workspace enabled from the console before this spec has no General until
the page or `store init` makes one. The announcement is written by the
agent's own credential, so a reader's role check sees a service account's
write, as every turn is.
