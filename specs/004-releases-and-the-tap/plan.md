# Plan 004 — releases from tags, and the Homebrew formula

## Layout

```
.goreleaser.yaml                 the build, the archive, the checksums, the formula, the release
.github/workflows/release.yml    on v* tags: test, goreleaser release --clean
README.md                        brew install first, go install second
specs/004-releases-and-the-tap/
```

## Mechanics

1. `goreleaser` builds `./cmd/chronicle-agents` with
   `-X github.com/impire-io/chronicle-agents/cli.Version={{.Version}}` —
   the variable `cli.Version` already exists and `chronicle-agents version`
   prints it.
2. The `brews` block writes the formula into `impire-io/homebrew-tap`
   (`Formula/`, branch `main`) on every release, with the tap token the
   repository already holds as a secret.
3. The workflow pins `GORELEASER_CURRENT_TAG` to the pushed tag, as
   chronicle's does, so an rc and its final on one commit never confuse it.
4. Nothing rebuilds `web/dist`: the committed build, proven fresh by the
   gate on every PR, is what the tag embeds.
