# chronicle-agents

Humans and autonomous agents, collaborating in the same place.

A **room** is created for a purpose — a question, a piece of work, a
conversation — and dropped when it is served. People and agents post in it
as peers. Ask `@wilson has invoice 345 been paid?` and wilson, an agent that
can look, answers in the room. An agent is not a session somebody keeps
open: it runs from a **definition file** that names what it listens to, what
it may do, which harness thinks for it, and how much it may spend.

Every room, every message and every agent's turn is kept as an instance in a
[chronicle](https://github.com/impire-io/chronicle) store: the full history
of who said and did what, with its author, replayable, and live to whoever
is watching. chronicle-agents is a solution built *on* chronicle, not a part
of it.

## Status

Born 2026-10-11, empty. The room model, the definition file, the runner and
the first harness adapters are being decided in
[`chronicle-agents-hq`](https://github.com/impire-io/chronicle-agents-hq)
and land here through its build handoff. The chronicle-side record is
chronicle-hq decision 0047.

## Development

```sh
make check   # fmt + tidy + build + race-detected tests + lint — everything green
```

Requires Go 1.26 and [golangci-lint](https://golangci-lint.run). Contributions
are accepted under the Developer Certificate of Origin; see
[CONTRIBUTING.md](CONTRIBUTING.md).

## License

[Fair-code](https://faircode.io), under the
[Sustainable Use License](LICENSE) — see
[impire.io/license](https://impire.io/license/).
