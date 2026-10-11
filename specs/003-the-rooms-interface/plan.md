# Plan 003 — the rooms interface

## Layout

```
types/solution.go       the solution type, Announce, Locate
web/server.go           the embedded build, /config.json, the SPA fallback; Serve
web/dist/               the committed build (placeholder index.html before the first make web)
web/_app/               the Vite application: package.json, vite.config.ts, index.html, src/
  src/main.tsx            boot: fetch /config.json, connect, render
  src/chronicle.ts        connecting per config: local (seed or creds) and hosted (profile, token, bridge)
  src/model.ts            the pure parts: mentions from text, the room order, folding ops into a transcript
  src/app.tsx             the pages: rooms, a room, writing, who may be addressed
  src/styles.css          chronicle-web's tokens, copied
  src/model.test.ts       vitest
cli/cli.go              the web command; store init announces
Makefile                web, and check depends on it; the freshness diff
.github/workflows/ci.yml  setup-node, make web, git diff --exit-code web/dist
specs/003-the-rooms-interface/
```

The application lives under `web/_app` — a directory the go tool ignores by
its leading underscore, so `node_modules` is never walked as Go source —
and builds into `web/dist`, which `web/server.go` embeds.

## Mechanics

1. `types.Announce` in `store init`; `types.Locate` from `web`.
2. `web.Handler`/`web.Serve`; the `web` command resolves the context, reads the credential and the websocket URL, serves, locates.
3. The application: connect per config; list rooms (`listInstances` by type, then the tail for liveness); a room's messages (`listInstances` under, then `tail` from the head filtered to the room); `create` for a message and a room; `apply` close; `listMembers` for the names and the agent badge.
4. `make web`: `npm ci && npm run build` in `web/_app`; `make check` = `web fmt tidy build test lint` then `git diff --exit-code web/dist`.
5. Tests: `web/server_test.go`, `types/solution_test.go`, `web/_app/src/model.test.ts`.

## Landing

Two PRs, draft from the first push, labelled `work/1`: chronicle-agents-hq first, this repository after it. When this lands on main, design 03 flips to `implemented`.
