# Spec 005 — the hosted interface

**Work ID:** `chronicle-agents-3` (tracker chronicle-agents-3)
**Design:** `chronicle-agents-hq` @ `a85ed21` (PR #9) —
[`03-DECISIONS/0007-the-rooms-interface-has-a-hosted-form-on-chronicles-managed-environment.md`](../../../chronicle-agents-hq/03-DECISIONS/0007-the-rooms-interface-has-a-hosted-form-on-chronicles-managed-environment.md),
[`02-DESIGN/03-the-rooms-interface.md`](../../../chronicle-agents-hq/02-DESIGN/03-the-rooms-interface.md)
§ serving it, § signing in hosted; the environment's side is chronicle-hq
decision 0053 (tracker chronicle-65), not this spec's.
**Status:** in progress ([plan.md](plan.md)).

## What this delivers

- **Signing in, hosted** (`web/_app/src/signin.ts`): the console's flow,
  ported onto `@impire-io/chronicle` 0.3.0's primitives — *Sign in with
  GitHub* sends the browser to GitHub with a state and a PKCE challenge,
  the callback is the page's own origin, the code is traded at the
  profile's `github_token_url`, the tokens live in `sessionStorage` and
  are refreshed before they lapse (a minute's keep-alive), and the bridge
  reads the token at every re-placement. *Sign out* forgets them. No token
  is ever typed; the pasted-token form is gone from the hosted path.
- **The account, chosen after sign-in**: the identity plane's memberships;
  one is entered without asking, several are offered with the role beside
  each (the last one opened is remembered per login), none says the
  person has no account on this chronicle and links to the site; a pinned
  account is entered when the identity holds it and refused when not.
- **`chronicle-agents web --hosted [--account NAME]`**: `--account` is a
  pin, not a requirement; `/config.json` carries the profile and the
  account only when pinned; the hosted deployment serves no account's
  context, so `web` announces nothing and says that `store init
  --interface` does.
- **`chronicle-agents store init --interface URL`**: records where the
  interface is on `solution/chronicle-agents` (`locate`). Without the
  flag, a sign-in context defaults to `https://agents.<host>` of the site
  its install profile names (`InterfaceFor`); a credential context defaults
  to nothing, and `web` records the address when it serves, as before.
- **The build, committed for real**: `web/dist` was ignored by
  `.gitignore`'s `dist/` despite decision 0005 point 6, so `go install`
  built a binary without the interface and the freshness check compared
  nothing; `!web/dist/` re-includes it, and the build is committed.
- **Proofs**: the return from GitHub, the account choice and the site under
  vitest; `web --hosted` with and without a pin against a real listener;
  `store init --interface` landing on the announcement against the
  embedded quick start; the hosted `/config.json` shapes; `InterfaceFor`.

## What it does not do

A sign-in context cannot yet be *dialed* by chronicle-agents' own CLI:
chronicle's CLI keeps the login beside the profile and dials the bridge
with it, and exports neither. `store init` on the hosted service runs with
a service account's credential (an admin's, issued from the console) and
`--interface`; the default from a sign-in context is wired and tested but
reachable only once chronicle exports its bridge dial — an ask for
chronicle-hq, filed by the parent work. No browser walk in the gate.
