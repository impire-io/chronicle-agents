# Agent guide for chronicle-agents

Durable instructions for any coding agent working in this repository. The
full rules live in `../chronicle-agents-hq/00-META/`; this file is the
orientation and the non-negotiables.

## Orientation (read in this order)

1. [`../chronicle-agents-hq/00-META/mission.md`](../chronicle-agents-hq/00-META/mission.md) —
   what chronicle-agents is and refuses to become.
2. [`../chronicle-agents-hq/00-META/how-we-build.md`](../chronicle-agents-hq/00-META/how-we-build.md) —
   the binding engineering postures. When this repo and that document
   disagree, that document wins.
3. [`../chronicle-agents-hq/00-META/repos.md`](../chronicle-agents-hq/00-META/repos.md) —
   the repo map and what this repo owns.
4. [`../chronicle-agents-hq/02-DESIGN/`](../chronicle-agents-hq/02-DESIGN/) —
   the designs this code implements. Capabilities land here through the
   build handoff
   ([playbook 04](../chronicle-agents-hq/00-META/process/04-build-handoff.md)),
   not by invention in this repo.

## Non-negotiables

- **chronicle is the record, reached as a client.** Rooms, messages and
  agent turns are instances and operations in a chronicle store, written
  and read through chronicle's SDKs and API. Nothing here reaches past
  them into chronicle's internals, and nothing here keeps a second copy of
  what the store already holds.
- **Work in an isolated workspace**, never in the shared clone; open it
  with `../chronicle-agents-hq/05-TOOLS/open-workspace.sh <work-id> chronicle-agents`
  and push every commit to a draft PR
  ([playbook 07](../chronicle-agents-hq/00-META/process/07-parallel-work.md)).
- **The gate is `make check`**, green before a commit: gofmt, tidy, build,
  race-detected tests, lint. Tests run against real NATS and a real
  chronicle, never a mocked client.
- **Sign every commit** (`git commit -s`): contributions are accepted under
  the Developer Certificate of Origin ([CONTRIBUTING.md](CONTRIBUTING.md)).
- **Issues and tasks live in the HITS tracker** under the initiative
  `chronicle-agents`, project `chronicle-agents`, never in files here.
