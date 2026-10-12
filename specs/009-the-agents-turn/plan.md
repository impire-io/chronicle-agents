# Plan 009 — the agent's turn

## Layout

```
harness/harness.go            Template.Output (text | claude-stream); Input.Server, Tools, Workdir; Observer and Event; Result; the claude-stream decoder; the preset
harness/harness_test.go       the decoder on a recorded stream, the preset's arguments, the run directory, the deadline wrapped
mcpserver/mcpserver.go        Config and its environment; the server (ask, permission, read_room, list_rooms); Ask; Transcript; Allows
mcpserver/mcpserver_test.go   over an in-memory MCP transport against a real quick start
declaration/declaration.go    Harness.Tools, Harness.Workdir, Template.Output; ToolName, ToolNames; validation
types/types.go                KindQuestion; Step, Usage; MessageData.Choices, Steps, Usage; the revised message schema; Bound
types/agent.go                AgentData.Tools
live/live.go                  Writing, Step, Waiting; StepSignal; Signal.Text, Signal.Step; WritingEvery
runner/runner.go              Connection; the server per turn; the observer; the question seen live, the answer not a trigger; steps and usage on the turn; the means sentence
runner/turn_test.go           the runner on a fake Claude Code stream; the question and its answer; the announcement's tools
cli/cli.go                    run hands the connection to the runner; the mcp sentence
examples/wilson.yaml          tools and workdir, commented
```

## Mechanics

1. `Invoke` starts the process with a pipe on stdout and reads it as the
   template says; the observer is called from the reading goroutine; the
   process group is killed at the deadline and the error wraps
   `context.DeadlineExceeded`.
2. The runner builds the server toolkit per turn from the connection it
   was given (`WithConnection`), this binary (`os.Executable`) and the
   `mcp` sentence, with the trigger's authors that are people as the
   askers and the turn's deadline as the question's.
3. `Ask` writes the question and tails the store after its sequence for
   the answer; the runner, tailing the same store, sees the question (its
   own author, kind question, while thinking) and says `waiting`, and
   drops the answer (reply_to names a question of its own) before matching
   triggers.
4. The observer throttles `writing` to one signal per 300 ms with the
   latest text, flushing the last one by timer; `step` signals go out as
   they happen.

## Landing

Draft PR labelled `work/the-agents-turn`, after chronicle-agents-hq PR #17
(research 003, decision 0010, the amendments) merges; rebased onto main,
gate green, ready, merged. Design 02 flips to `implemented` with it.
