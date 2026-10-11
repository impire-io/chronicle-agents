# Plan 002 — subjects, schedules and the workspace

## Layout

```
declaration/       room, room_store on a trigger; subject and schedule validated, not refused
schedule/          Create: the headered registration publish; the paths and headers
runner/            subject subscriptions beside the tails; non-room triggers routed to their room;
                   identity = op id | digest | sequence; parents only where an op id exists
harness/           presets pi and hermes; {{PROMPT}}, {{CONTEXT_TEXT}}
cli/               store init makes both stores (idempotent); schedule create
examples/          wilson.yaml shows every kind
specs/002-…/
```

## Mechanics

1. `go get github.com/impire-io/chronicle@v0.5.0` — `WithSchedules`, `HistoryUnder`, `Children`.
2. declaration: the `room`/`room_store` fields and their validation; `Stores()` covers the tick store and the rooms the turns go to.
3. schedule: `Create(ctx, nc, author, store, name, pattern, ttl)` — no message id, so the server's copied headers never dedup a tick.
4. runner: `match` returns the entry; `react` is the common path (closed room, admission, coalescing); `observeSubject` digests the message; `post` derives the id from the triggers' identities.
5. harness: the two presets and the text placeholders.
6. cli: `ensureStore` on `store-exists`; `schedule create`; flags before or after the positional.
7. Tests: `triggers_test.go` (subject, schedule at `@every 1s`), `cli/store_test.go`, declaration refusals; `make check` green.

## Landing

Two PRs, draft from the first push, labelled `work/001-the-room-model`: chronicle-agents-hq first (decision 0004, the amended designs, research 001 graduated), then this repository. When this lands on main, the designs flip to `implemented`.
