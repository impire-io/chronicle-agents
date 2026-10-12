package runner

import (
	"context"
	"encoding/json"
	"io"
	"log/slog"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/impire-io/chronicle-agents/declaration"
	"github.com/impire-io/chronicle-agents/live"
	"github.com/impire-io/chronicle-agents/types"
	"github.com/impire-io/chronicle/devdir"
)

// A fake Claude Code for the runner: the event stream a real one writes,
// from a file — a text streamed in deltas, a tool call and its result,
// the final text, the result with its cost.
const fakeStream = `{"type":"system","subtype":"init","session_id":"s-1"}
{"type":"stream_event","event":{"type":"content_block_start","index":0,"content_block":{"type":"text"}},"parent_tool_use_id":null}
{"type":"stream_event","event":{"type":"content_block_delta","index":0,"delta":{"type":"text_delta","text":"Looking"}},"parent_tool_use_id":null}
{"type":"assistant","message":{"model":"claude-x","content":[{"type":"text","text":"Looking."}]},"parent_tool_use_id":null}
{"type":"assistant","message":{"model":"claude-x","content":[{"type":"tool_use","id":"t1","name":"Bash","input":{"command":"ls"}}]},"parent_tool_use_id":null}
{"type":"user","message":{"role":"user","content":[{"type":"tool_result","tool_use_id":"t1","content":"a.txt","is_error":false}]},"parent_tool_use_id":null}
{"type":"assistant","message":{"model":"claude-x","content":[{"type":"text","text":"One file: a.txt."}]},"parent_tool_use_id":null}
{"type":"result","subtype":"success","is_error":false,"result":"One file: a.txt.","session_id":"s-1","total_cost_usd":0.01,"num_turns":2,"usage":{"input_tokens":10,"output_tokens":5}}
`

func fakeClaudeYAML(t *testing.T) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), "stream.jsonl")
	if err := os.WriteFile(path, []byte(fakeStream), 0o600); err != nil {
		t.Fatal(err)
	}
	return "harness:\n  template:\n    command: cat\n    args: [\"" + path + "\"]\n    output: claude-stream\n  deadline: 10s\n"
}

// serveWith starts wilson on a declaration with its own harness block,
// with the connection facts the runner hands its server.
func (w *world) serveWith(decl string) func() {
	w.t.Helper()
	d, err := declaration.Parse([]byte(decl))
	if err != nil {
		w.t.Fatal(err)
	}
	c := w.l.Connect(w.t, d.Identity)
	r, err := New(c, d, slog.New(slog.NewTextHandler(io.Discard, nil)),
		WithConnection(Connection{URL: w.l.URL, Nkey: devdir.UserNkeyPath(w.l.Dir)}))
	if err != nil {
		w.t.Fatal(err)
	}
	ctx, cancel := context.WithCancel(w.ctx)
	done := make(chan error, 1)
	go func() { done <- r.Serve(ctx) }()
	time.Sleep(300 * time.Millisecond)
	return func() {
		cancel()
		if err := <-done; err != nil {
			w.t.Errorf("serve: %v", err)
		}
	}
}

func TestATurnCarriesItsStepsAndSaysThemLive(t *testing.T) {
	w := newWorld(t)
	e := listen(t, w.admin.Conn())
	stop := w.serveWith(`
identity: wilson
triggers:
  - kind: mention
instructions: You are wilson.
` + fakeClaudeYAML(t))
	defer stop()

	w.say("admin", "m-1", "@wilson what is in your directory?", "wilson")
	turns := w.waitTurns(1)
	turn := turns[0].Data
	if turn.Text != "One file: a.txt." {
		t.Errorf("text = %q", turn.Text)
	}
	// The record: the step the harness took, bounded, and what it cost.
	if len(turn.Steps) != 1 || turn.Steps[0].Tool != "Bash" || turn.Steps[0].Input != "ls" || turn.Steps[0].Output != "a.txt" || !turn.Steps[0].OK {
		t.Errorf("steps = %+v", turn.Steps)
	}
	if turn.Usage == nil || turn.Usage.CostUSD != 0.01 || turn.Usage.Turns != 2 || turn.Usage.Model != "claude-x" {
		t.Errorf("usage = %+v", turn.Usage)
	}
	// Live: the text as it was written, the step as it started and came back.
	writing := e.wait(t, live.Writing)
	if writing.Text == "" || writing.Trigger != "room/r1/messages/m-1" {
		t.Errorf("writing = %+v", writing)
	}
	var started, finished *live.Signal
	deadline := time.Now().Add(5 * time.Second)
	for (started == nil || finished == nil) && time.Now().Before(deadline) {
		e.mu.Lock()
		for i := range e.signals {
			s := e.signals[i]
			if s.State == live.Step && s.Step != nil {
				if s.Step.Phase == live.Started && started == nil {
					started = &s
				}
				if s.Step.Phase == live.Finished && finished == nil {
					finished = &s
				}
			}
		}
		e.mu.Unlock()
		time.Sleep(20 * time.Millisecond)
	}
	if started == nil || started.Step.Tool != "Bash" || started.Step.Input != "ls" {
		t.Errorf("started = %+v", started)
	}
	if finished == nil || finished.Step.Output != "a.txt" || !finished.Step.OK {
		t.Errorf("finished = %+v", finished)
	}
}

func TestAQuestionOfTheAgentsOwnIsSaidLiveAndItsAnswerIsNoTrigger(t *testing.T) {
	w := newWorld(t)
	e := listen(t, w.admin.Conn())
	// A harness slow enough to be asked something while it runs.
	stop := w.serveWith(`
identity: wilson
triggers:
  - kind: mention
instructions: You are wilson.
harness:
  template:
    command: sh
    args: ["-c", "sleep 1.5; echo the answer"]
  deadline: 10s
`)
	defer stop()
	w.say("admin", "m-1", "@wilson check this", "wilson")
	e.wait(t, live.PickedUp)
	// While the turn runs, the server (here: the test, as wilson) writes a
	// question in the room; the runner says the turn waits on it.
	wilson := w.l.Connect(t, "wilson")
	payload, _ := json.Marshal(types.MessageData{Text: "wilson wants to use Bash: ls. Allow it?", Kind: types.KindQuestion, Choices: []string{"allow", "deny"}, Mentions: []string{"admin"}})
	if _, err := wilson.Create(w.ctx, "rooms", "room/r1/messages/q-1", "create", payload); err != nil {
		t.Fatal(err)
	}
	waiting := e.wait(t, live.Waiting)
	if waiting.Trigger != "room/r1/messages/q-1" || !strings.Contains(waiting.Text, "Allow it?") {
		t.Errorf("waiting = %+v", waiting)
	}
	// The person answers, naming wilson: it is the answer, never a trigger.
	payload, _ = json.Marshal(types.MessageData{Text: "@wilson allow", Kind: types.KindMessage, Mentions: []string{"wilson"}, ReplyTo: "room/r1/messages/q-1"})
	if _, err := w.admin.Create(w.ctx, "rooms", "room/r1/messages/a-1", "create", payload); err != nil {
		t.Fatal(err)
	}
	w.waitTurns(1)
	if got := w.settle(); len(got) != 1 {
		t.Errorf("turns = %d: the answer to a question triggered a turn", len(got))
	}
}

func TestTheAnnouncementNamesTheTools(t *testing.T) {
	w := newWorld(t)
	stop := w.serveWith(`
identity: wilson
triggers:
  - kind: mention
instructions: You are wilson.
harness:
  template:
    command: tail
    args: ["-n", "1"]
  tools: ["Bash(git *)", "Read"]
toolkits:
  - name: invoices
    command: inv
`)
	defer stop()
	deadline := time.Now().Add(5 * time.Second)
	for time.Now().Before(deadline) {
		a, ok, err := types.ReadAgent(w.ctx, w.admin, "rooms", "wilson")
		if err != nil {
			t.Fatal(err)
		}
		if ok && strings.Join(a.Tools, ",") == "Bash(git *),Read,invoices" {
			return
		}
		time.Sleep(50 * time.Millisecond)
	}
	t.Error("the announcement does not name the tools")
}
