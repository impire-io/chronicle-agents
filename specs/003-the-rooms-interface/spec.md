# Spec 003 — the rooms interface

**Work ID:** `1` (tracker chronicle-agents-1)
**Design:** `chronicle-agents-hq` @ the commit the hq PR lands —
[`03-DECISIONS/0005-the-rooms-interface-is-served-by-the-binary.md`](../../../chronicle-agents-hq/03-DECISIONS/0005-the-rooms-interface-is-served-by-the-binary.md),
[`02-DESIGN/03-the-rooms-interface.md`](../../../chronicle-agents-hq/02-DESIGN/03-the-rooms-interface.md);
chronicle-hq decision 0051 and design 14 for the `solution` type.
**Status:** in progress ([plan.md](plan.md)).

## What this delivers

- **`chronicle-agents web`** — serves the static application embedded in
  the binary (`web/dist`, built from `web/_app`) on `--listen`
  (`127.0.0.1:8080` by default) and `/config.json`: for the quick start
  or a self-hosted context, the websocket URL (`--ws`, or the quick
  start's recorded one) and the context's credential; with `--hosted`,
  the install profile's address and `--account`. On start it applies
  `locate` to `solution/chronicle-agents` with `--public-url` (default the
  listen address), so chronicle's console links here.
- **The announcement** — `store init` declares the `solution` type
  (chronicle-hq design 14) in the rooms store and creates
  `solution/chronicle-agents` with name, description, version and home;
  again is no error.
- **The application** (`web/_app`: Vite, React, TypeScript, plain CSS on
  chronicle-web's tokens, `@impire-io/chronicle`): the rooms list live;
  a room live — the messages under it, then the store's tail filtered to
  the room; a turn marked as an agent's by the registry; writing with the
  `mentions` filled from the `@name`s and names offered on `@`; a new
  room; closing a room behind the confirmation; who may be addressed. No
  presence.
- **The build, committed and checked fresh**: `make web` rebuilds
  `web/dist`; `make check` runs it and fails on a difference; CI the same.
- **Proofs**: the server serves the configuration and the application at
  every address; the announcement is written and idempotent; the
  application's pure parts — mentions from text, the room order, the
  transcript fold — under vitest.

## What it does not do

No presence, typing or read receipts; no edits; no search; no GitHub
web-flow sign-in (a token, hosted); no browser walk in the gate yet. The
children tail replaces the browser-side filter when `chronicle-js`
implements contract 2.1.0.
