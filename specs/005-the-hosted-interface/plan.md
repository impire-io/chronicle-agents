# Plan 005 — the hosted interface

## Layout

```
web/_app/src/signin.ts        the ported sign-in: start, complete, fresh token, sign out; the pure parts — readReturn, chooseAccount, siteOf
web/_app/src/signin.test.ts   vitest on the pure parts
web/_app/src/chronicle.ts     connecting: local as before; hosted — loadProfile, identityOf (memberships), enterAccount (the bridge)
web/_app/src/app.tsx          the phases: signed-out, no-account, choosing, connected; Sign out in the sidebar
web/_app/src/styles.css       the account list, the link button
web/server.go                 Config's hosted comment: the account is a pin
cli/cli.go                    web --hosted without --account (announces nothing); store init --interface and its default; InterfaceFor; a sign-in context refused with the credential to use instead
cli/web_test.go               web --hosted pinned and unpinned; InterfaceFor
cli/store_test.go             store init --interface lands on the announcement
web/server_test.go            the hosted /config.json shapes
.gitignore                    !web/dist/ — the build is committed, as 0005 says
web/dist/                     the committed build
README.md                     the hosted form
```

## Mechanics

1. `App` loads `/config.json`; hosted, it fetches the profile (which must
   name `websocket_url` and `github_token_url`), completes a return from
   GitHub when the address carries one, then `freshToken` → `identityOf` →
   `chooseAccount` → `enterAccount`. The token keep-alive runs every minute
   while connected; `connectBridge` reads the held token at every
   (re)connect.
2. `web --hosted`: `Config{Mode: hosted, Profile, Account?}`; no `locate`.
3. `store init`: `--interface`, else `defaultInterface()` — the context's
   bridge profile decoded with `bridge.DecodeProfile`, `InterfaceFor` on
   its `github_token_url`'s host; then `types.Locate` after `Announce`.
4. The gate: `make web` rebuilds `web/dist` (now tracked), `make check`.

## Landing

Draft PR labelled `work/chronicle-agents-3`, after the chronicle-agents-2
PR (the release flow) merges; rebased onto main, gate green, ready, merged.
Design 03 flips to `implemented` when this is on main.
