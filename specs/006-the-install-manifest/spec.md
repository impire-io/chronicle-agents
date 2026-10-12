# Spec 006 — the install manifest

**Work ID:** `chronicle-agents-4` (tracker chronicle-agents-4)
**Design:** `chronicle-agents-hq` @ `1751701` (PR #12) —
[`03-DECISIONS/0008-the-workspace-is-declared-once-and-published-as-an-install-manifest.md`](../../../chronicle-agents-hq/03-DECISIONS/0008-the-workspace-is-declared-once-and-published-as-an-install-manifest.md),
[`02-DESIGN/03-the-rooms-interface.md`](../../../chronicle-agents-hq/02-DESIGN/03-the-rooms-interface.md)
§ serving it (`/install.json`); the manifest's shape is chronicle-hq
`02-DESIGN/14-solutions.md` § the install manifest @ `88fa386` (decision
0054). The console's side is chronicle-hq's tracker chronicle-67, not this
spec's.
**Status:** in progress ([plan.md](plan.md)).

## What this delivers

- **One declaration** (`types/workspace.go`): `types.Declaration(rooms,
  ticks)` — the rooms store with `message`, `room` and `solution` in that
  order, the tick store allowing schedules, the announcement at
  `solution/chronicle-agents` in the rooms store. `store init` walks it:
  the same stores, types, announcement, `--interface` and output as before.
- **The manifest** (`Workspace.Manifest(version, url)`): design 14's shape
  — `manifest: 1`, the `solution` (name, description, version, home, url —
  the announcement's fields), the `stores` in order with their `types` in
  order, each definition in the wire's names (`schema`, `history`,
  `children`, `operations`; the Go client's `TypeDefinition` carries no
  JSON tags, so `types.Wire` restates it), `schedules: true` on the tick
  store, and `announce`.
- **`/install.json`**, served by `chronicle-agents web` in every form
  beside `/config.json`: the manifest with the binary's version and the
  public address (`--public-url`, default the listen address); no-store;
  `Access-Control-Allow-Origin: *` and an answered preflight, since
  chronicle's console fetches it from its own origin. `/config.json`
  carries neither field.
- **Proofs**: the marshalled shape and order (`types/workspace_test.go`);
  `store init` against the embedded quick start creates exactly what the
  manifest declares — every store with its schedules flag, every type
  equal by value to the manifest's definition (`cli/store_test.go`);
  `/install.json`'s shape, headers and preflight (`web/server_test.go`).

## What it does not do

Nothing reads `/install.json` on the quick start or a self-hosted install;
the console does, hosted (chronicle-67). The tick store's `schedules`
flag is in the manifest; chronicle-js 0.3.0's `createStore` has no such
option yet, which the console's build must add or work around. The
announcement's `version` on an existing workspace is still the one that
announced it (spec 004's note).
