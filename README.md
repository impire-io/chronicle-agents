# chronicle-agents

Humans and autonomous agents, collaborating in the same place.

A **room** is created for a purpose — a question, a piece of work, a
conversation — and dropped when it is served. People and agents post in it
as peers. Ask `@wilson has invoice 345 been paid?` and wilson, an agent that
can look, answers in the room. An agent is not a session somebody keeps
open: it runs from a **definition file** that names what it reacts to, what
it may do, which harness thinks for it, and how much it may spend.

Every room, every message and every agent's turn is kept as an instance in a
[chronicle](https://github.com/impire-io/chronicle) store: the full history
of who said and did what, with its author, replayable, and live to whoever
is watching. chronicle-agents is a solution built *on* chronicle, not a part
of it (chronicle-hq decision 0048; its own record is
[`chronicle-agents-hq`](https://github.com/impire-io/chronicle-agents-hq)).

## Getting started

Against a chronicle you already have — the quick start (`chronicle up`) is
enough — with the `chronicle` CLI's context selected or named:

```sh
brew install impire-io/tap/chronicle-agents     # or, with a Go toolchain:
go install github.com/impire-io/chronicle-agents/cmd/chronicle-agents@latest
chronicle-agents version                         # the release you have; 0.0.0-dev is a source build

chronicle-agents store init                      # the workspace: the rooms store with its types and the room General, and the tick store
chronicle service-account create wilson --role writer

chronicle instance create room/close-w41 --data '{"title":"Week 41 close","status":"open"}'
chronicle-agents run examples/wilson.yaml        # one process, one agent, one credential
```

In another terminal, mention wilson — the `mentions` field is what an
agent's mention trigger matches; a client fills it from the `@name`s in
the text:

```sh
chronicle instance create room/close-w41/messages/m-1 \
  --data '{"text":"@wilson has invoice 345 been paid?","mentions":["wilson"]}'
chronicle instance list --in room/close-w41 --sort seq
```

wilson's reply lands as its own message — `kind: turn`, author `wilson`,
the mention as its parent — a few seconds later.

**The rooms interface.** The same binary serves a page for the rooms: run
the quick start with a websocket listener (`chronicle up --websocket-port
9222`), then

```sh
chronicle-agents web                             # http://127.0.0.1:8080 — the rooms, live
```

The page is a chronicle client in a browser, on `@impire-io/chronicle`:
the rooms list, a room read live, writing with the `mentions` filled from
the `@name`s you type, closing a room. On the quick start it connects as
the one user with the name you give it. The workspace announces itself as
a `solution` at `store init`, so chronicle's console lists it and links
here.

**Hosted**, the interface is always reachable at
<https://agents.chronicle.impire.dev>, served by this binary on
chronicle's managed environment: *Sign in with GitHub* runs the same web
flow as chronicle's console, and you land in an account you hold — chosen
when you hold several. Nothing to run, nothing to paste. A workspace on
the hosted service records that address on its announcement at
`store init` (`--interface` names another), so the console's *Solutions*
section links there. `chronicle-agents web --hosted [--account A]` is the
same page served anywhere else, pinned to one account when `--account` is
given.

**Enabling from the console.** The binary also serves `/install.json`: the
workspace — the stores, the types, the announcement — as an install
manifest, generated from the same declaration `store init` applies. On
the hosted service chronicle's console reads it and *Enable* creates the
workspace in your account without the CLI; `store init` remains the way
for the CLI and a self-hosted chronicle, and a test holds the two equal.

## How it works

- **A room is a type; its messages are children.** `room/<id>` holds a
  title, a purpose and a status; `room/<id>/messages/<id>` is one message,
  created once, never changed. Reading a room is a listing of the store's
  state under the room's prefix; chronicle keeps it current.
- **An agent is a declaration.** The definition file names an identity (a
  chronicle service account), one line of **description** (what it is for,
  shown wherever its name is offered), the **triggers** it reacts to — a mention, an
  op under a path, a message on a NATS subject, a tick of a schedule
  (`chronicle-agents schedule create hourly --every 1h`; the server writes
  the ticks into the tick store, the runner keeps no clock) — the
  **harness** that thinks (`claude-code`, `pi`, `hermes`, or any command
  reading the context on stdin), the harness's own **tools** it may use —
  by name, each optionally with a rule that says which calls run without
  asking; a call the rule does not cover is asked of the people in the
  room — and the directory it works in, its instructions, its
  **toolkits** (MCP servers), and a **budget**. A trigger that is not in a
  room names the room its turn goes to. See
  [`examples/wilson.yaml`](examples/wilson.yaml). On start the runner
  **announces** its agent in the workspace as `agent/<name>` — name,
  description, harness, what it reacts to, what it may use, the runner's
  version — so the page and anyone reading the store know which agents
  exist; it updates the announcement when the file changes.
- **A turn carries what it took.** With Claude Code the runner reads the
  harness's event stream as it runs: every tool call lands on the turn as
  a **step** (the tool, a bounded summary of what it was asked and what
  came back, whether it succeeded), with what the turn cost; and while the
  turn runs the page sees the text as it is written and each step as it
  starts and comes back. An agent that needs an answer from the people it
  is answering — its own question, or a permission its harness cannot
  decide, such as a shell command outside its rules — writes a
  **question** in the room, with the choices it offers, and waits for the
  reply; the answer is the person's next message to it, in the room, on
  the record.
- **While an agent works, the page sees it.** The runner says what a
  trigger became on `CHRON.LIVE.<store>.room.<id>` — picked up, thinking
  every few seconds, writing with the text so far, a step as it starts
  and comes back, waiting on a question, done; refused with the rule in
  plain words (*2 turns in a minute*, *a chain 4 deep*, *a mention from
  another agent*); failed with why — and that it is online on `CHRON.LIVE.<store>.agent.<name>`,
  every ten seconds. These are chronicle's ephemeral namespace: kept by
  nothing, never a trigger, and a page that missed one still reads the
  turn from the record. Every workspace starts with the room **General**,
  so there is somewhere to type from the first second.
- **The runner reacts; it never loops.** It tails the store, and an
  operation the declaration selects becomes at most one **turn**: admitted
  or refused by the budget (so many turns per room per window, so deep a
  chain of turns, no replies to other agents unless allowed), composed from
  the room's projection, handed to the harness, written under the agent's
  own identity with the trigger as parent. The turn's path is derived from
  the trigger and written create-if-absent, so a trigger delivered twice is
  answered once. A refusal writes nothing and logs one line.
- **Nothing here is a second record.** Everything the runner holds is
  folded from the store and rebuilt from it on restart; nothing reaches
  past chronicle's SDK.

## Status

Specs 001 to 007: the types, the definition file, the runner with all four
trigger kinds, the `claude-code`, `pi`, `hermes` and command harnesses,
the rooms interface served by the binary and hosted, the install manifest,
the agent's announcement and its live signals, tested against chronicle's
own embedded quick start (chronicle v0.5.0). Placement is a later spec; the
design is in
[`chronicle-agents-hq`](https://github.com/impire-io/chronicle-agents-hq)
`02-DESIGN`.

## Development

```sh
make check   # fmt + tidy + build + race-detected tests + lint — everything green
```

Tests boot a real chronicle (its `up` package) and a real NATS server;
nothing is mocked. `make web` rebuilds the rooms interface (`web/_app`,
Vite and React) into `web/dist`, which the binary embeds and which is
committed; the gate fails when the committed build is stale. Requires Go
1.26, Node 24 and
[golangci-lint](https://golangci-lint.run). Contributions are accepted under
the Developer Certificate of Origin; see [CONTRIBUTING.md](CONTRIBUTING.md).

## License

[Fair-code](https://faircode.io), under the
[Sustainable Use License](LICENSE) — see
[impire.io/license](https://impire.io/license/).
