# Spec 004 — releases from tags, and the Homebrew formula

**Work ID:** `chronicle-agents-2` (tracker chronicle-agents-2)
**Design:** `chronicle-agents-hq` @ `3d48e40` —
[`03-DECISIONS/0006-chronicle-agents-cuts-releases-and-ships-in-the-homebrew-tap.md`](../../../chronicle-agents-hq/03-DECISIONS/0006-chronicle-agents-cuts-releases-and-ships-in-the-homebrew-tap.md);
chronicle's own flow (`impire-io/chronicle`'s `.goreleaser.yaml` and
`release.yml`, chronicle-hq decision 0017) is the model.
**Status:** done ([plan.md](plan.md)).

## What this delivers

- **A release is a tag.** `.github/workflows/release.yml` runs on `v*`:
  the race-detected tests, then `goreleaser release --clean` with the
  pushed tag pinned as the release (`GORELEASER_CURRENT_TAG`).
- **`.goreleaser.yaml`**: one binary, `chronicle-agents`, for linux and
  darwin on amd64 and arm64, CGO off, `cli.Version` stamped with the tag's
  bare semver; a `tar.gz` per platform carrying the README and the
  LICENSE; `checksums.txt`; the GitHub release.
- **The formula**: `brews` writes `Formula/chronicle-agents.rb` into
  `impire-io/homebrew-tap` with the repository's `HOMEBREW_TAP_TOKEN`,
  installing the one binary and testing that `chronicle-agents version`
  prints the version.
- **The embedded interface ships as committed**: the flow does not run
  `make web`; `web/dist` on `main` is what every release embeds (decision
  0006 point 4).
- **The version on the announcement**: already the case — `store init`
  passes `cli.Version` to `types.Announce`, so a released binary announces
  its stamped version (decision 0006 point 3). The announcement is
  create-if-absent, so a workspace announced by an earlier binary keeps
  that version until re-announced; refreshing it is a follow-up, not this
  spec.
- **The README** leads with `brew install impire-io/tap/chronicle-agents`
  and keeps `go install` for people with a toolchain.

## What it does not do

No windows build (nothing runs the runner there yet). No tag is cut here:
the operator signs and pushes `v0.1.0` once the hosted interface
(chronicle-agents-3) is on `main`, so the first release carries it. No
rebuild of the interface at release time.

## Proofs

`goreleaser check` validates the configuration; `make check` stays green;
the release workflow is proven by the first tag.
