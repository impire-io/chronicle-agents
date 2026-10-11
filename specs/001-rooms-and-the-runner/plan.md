# Plan 001 — rooms and the runner

## Layout

```
cmd/chronicle-agents/     the CLI: store init, run (thin main; logic in cli)
cli/                      the two sentences and the connection flags
types/                    the room and message type definitions as data, declared through the client
declaration/              the definition file: schema, load, validation, the refusals at load
runner/                   the feed, the match, admission, the projection, compose, the turn
harness/                  the invocation template, the presets, the run directory, the invocation
internal/natstest/        nothing — tests boot chronicle's own quick start (chronicle/up)
specs/001-rooms-and-the-runner/
```

Dependencies: `github.com/impire-io/chronicle` v0.4.0 (client, contract,
up for tests), `github.com/nats-io/nats.go`, `sigs.k8s.io/yaml` (the
library chronicle's CLI reads type files with). No UUID library: the
derived turn id is a name-based UUID computed in-tree.

## Mechanics

1. **types**: the two definitions as Go values mirroring design 01's tables,
   and `Declare(ctx, client, store)` calling `DefineType` for both. A test
   declares them into a fresh quick start and reads them back.
2. **declaration**: the YAML shape; `Load(path)` → `Declaration` or an error
   naming the field; `subject` and `schedule` entries fail load with the
   design's reasons. Presets are resolved here by name.
3. **harness**: `Template` (command, args with `{{PROMPT}}`, `{{CONTEXT}}`,
   `{{MCP_CONFIG}}`, `{{RUN_DIR}}`; output mode; env block); the two
   presets; `Invoke(ctx, tpl, turn input) (text, error)` — run dir under
   the OS temp dir, MCP config written from the toolkits, environment
   scrubbed to an allow-list plus the template's block, deadline with
   process-group kill. Tested with the `command` preset.
4. **runner**: `Serve(ctx, client, decl)`. The feed is `client.Tail(store,
   "")` per store from the checkpoint (the stream head read once at start
   separates history from live; history only feeds the aggregate). Match:
   path parsed, closed rooms known from the room instances' state, own
   author dropped. Admission: count the agent's turns under the room within
   the window from the listing; walk `Op-Parents` through turns for the
   depth bound; look the author up in `ListMembers` for the agent-to-agent
   rule. Compose: `ListInstances(Under(room))` sorted by sequence plus the
   aggregate; coalesce triggers queued during a harness run. The turn: path
   `room/<id>/messages/m-<uuid5(agent, trigger id)>`, `Create` with the
   create-only guard (the client's default), `WithParents(all triggers)`,
   `WithOpID(the same uuid)`; a guard refusal is logged as already answered.
   Refusals: one `slog` line, nothing written.
5. **cli / cmd**: `chronicle-agents store init [--store S]` and
   `chronicle-agents run FILE`, with `--url`, `--creds`, `--nkey`; the
   identity's principal is the declaration's. `--help` on both.
6. **Tests** (`go test -race ./...`): every package's tests boot
   `up.Up` on a free port with a temp dir and connect through the quick
   start's user key stating the author — the admin for a person's messages,
   the declaration's identity for the runner — and run the spec's seven
   proofs end to end with the `command` preset.
7. `make check` green; CI green on the PR; the DCO trailer on every commit.

## Landing

Two PRs, draft from the first push, labelled `work/001-the-room-model`:
chronicle-agents-hq first (the designs flipped to `in-progress` with their
`lands` block, this spec cited), then this repository after it merges. When
this lands on main, the designs flip to `implemented` in the hq.
