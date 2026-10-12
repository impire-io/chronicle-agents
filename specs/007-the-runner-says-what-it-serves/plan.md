# Plan 007 — the runner says what it serves

## Layout

```
live/live.go                   Root, RoomSubject, AgentSubject, RoomID; State; Signal, Presence; the cadences; Publish
types/agent.go                 Agent, AgentPath, AgentDefinition, AgentData (+Equal), AnnounceAgent, ReadAgent, ListAgents; General*, EnsureGeneral
types/types.go                 Declare defines message, room, agent
types/workspace.go             the declaration lists agent after solution
declaration/declaration.go     Description; Trigger.Describe; HarnessName; RoomStores
runner/runner.go               Option, Version; announcement, announce, presence, signal; admit returns (rule, reason); spoken; failure
cli/cli.go                     run passes runner.Version(Version); store init makes General, names the types as a list
examples/wilson.yaml           description
runner/live_test.go            the signals, the announcement, presence, nothing in the stream
```

## Mechanics

1. `Serve` announces first, then starts presence, then the tails: a page
   that opens while the runner boots already sees the agent.
2. `react` turns a refusal into a signal with the rule's spoken form;
   `start` says `picked-up` and repeats `thinking` from a goroutine the
   harness's return cancels; `finish` says `done` after `post`, or
   `failed` with the harness's reason.
3. Signals and presence are `nats.Conn.Publish` on the client's own
   connection — the member baseline grants `CHRON.>`, and `CHRON.LIVE.` is
   under no store's stream (chronicle-hq decision 0055).
4. `AnnounceAgent` reads the current state through `ListInstances` by
   type and compares by value, so a restart with the same file writes
   nothing to the record.
