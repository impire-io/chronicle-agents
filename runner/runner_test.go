package runner

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"log/slog"
	"sort"
	"testing"
	"time"

	"github.com/impire-io/chronicle-agents/declaration"
	"github.com/impire-io/chronicle-agents/harness"
	"github.com/impire-io/chronicle-agents/internal/chrontest"
	"github.com/impire-io/chronicle-agents/types"
	"github.com/impire-io/chronicle/client"
)

// the test's harness: answers with the last line of the context — the
// "Reply with the message text only." instruction — deterministically and
// in no time.
const harnessYAML = `
harness:
  template:
    command: tail
    args: ["-n", "1"]
  deadline: 10s
`

type world struct {
	t     *testing.T
	l     *chrontest.Local
	admin *client.Client
	ctx   context.Context
}

func newWorld(t *testing.T) *world {
	t.Helper()
	l := chrontest.Up(t)
	admin := l.Connect(t, "admin")
	ctx := context.Background()
	if _, err := admin.CreateStore(ctx, "rooms", "test"); err != nil {
		t.Fatal(err)
	}
	if err := types.Declare(ctx, admin, "rooms"); err != nil {
		t.Fatal(err)
	}
	if _, err := admin.AddMember(ctx, "wilson", "writer", client.AsServiceAccount()); err != nil {
		t.Fatal(err)
	}
	if _, err := admin.AddMember(ctx, "ada", "writer", client.AsServiceAccount()); err != nil {
		t.Fatal(err)
	}
	if _, err := admin.Create(ctx, "rooms", "room/r1", "create", []byte(`{"title":"R1","status":"open"}`)); err != nil {
		t.Fatal(err)
	}
	return &world{t: t, l: l, admin: admin, ctx: ctx}
}

// serve starts wilson on the declaration and returns a stop.
func (w *world) serve(decl string) func() {
	w.t.Helper()
	d, err := declaration.Parse([]byte(decl + harnessYAML))
	if err != nil {
		w.t.Fatal(err)
	}
	c := w.l.Connect(w.t, d.Identity)
	r, err := New(c, d, slog.New(slog.NewTextHandler(io.Discard, nil)))
	if err != nil {
		w.t.Fatal(err)
	}
	ctx, cancel := context.WithCancel(w.ctx)
	done := make(chan error, 1)
	go func() { done <- r.Serve(ctx) }()
	// Serving reads the head before anything live lands: give the tails a moment.
	time.Sleep(300 * time.Millisecond)
	return func() {
		cancel()
		if err := <-done; err != nil {
			w.t.Errorf("serve: %v", err)
		}
	}
}

// say posts a message in r1 as the author, mentioning whoever is named.
func (w *world) say(author, id, text string, mentions ...string) client.Ack {
	w.t.Helper()
	c := w.admin
	if author != "admin" {
		c = w.l.Connect(w.t, author)
	}
	// Every message here is kind "message", whoever writes it: agent-ness
	// is judged by the registry, never by the payload, and only the
	// runner's own turns carry kind "turn" — which is what turns() counts.
	payload, _ := json.Marshal(types.MessageData{Text: text, Kind: types.KindMessage, Mentions: mentions})
	ack, err := c.Create(w.ctx, "rooms", "room/r1/messages/"+id, "create", payload)
	if err != nil {
		w.t.Fatal(err)
	}
	return ack
}

type seen struct {
	Path string
	Seq  uint64
	Data types.MessageData
}

// turns lists wilson's turns in r1, by sequence.
func (w *world) turns() []seen {
	w.t.Helper()
	var out []seen
	for info, err := range w.admin.ListInstances(w.ctx, "rooms", client.Under("room/r1")) {
		if err != nil {
			w.t.Fatal(err)
		}
		var d types.MessageData
		_ = json.Unmarshal(info.State, &d)
		if d.Kind == types.KindTurn {
			out = append(out, seen{Path: info.Path, Seq: info.Seq, Data: d})
		}
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Seq < out[j].Seq })
	return out
}

// waitTurns waits until n turns exist, or fails.
func (w *world) waitTurns(n int) []seen {
	w.t.Helper()
	deadline := time.Now().Add(15 * time.Second)
	for {
		got := w.turns()
		if len(got) >= n {
			return got
		}
		if time.Now().After(deadline) {
			w.t.Fatalf("after 15s: %d turns, want %d: %+v", len(got), n, got)
		}
		time.Sleep(100 * time.Millisecond)
	}
}

// settle gives the runner time to answer what it should not, then counts.
func (w *world) settle() []seen {
	time.Sleep(1500 * time.Millisecond)
	return w.turns()
}

const wilson = `
identity: wilson
triggers:
  - kind: mention
instructions: You are wilson.
`

func TestAMentionBecomesOneTurnWithTheTriggerAsParent(t *testing.T) {
	w := newWorld(t)
	w.say("admin", "m-0", "hello room") // history for the runner: never answered
	stop := w.serve(wilson)
	defer stop()

	ack := w.say("admin", "m-1", "@wilson has invoice 345 been paid?", "wilson")
	turns := w.waitTurns(1)
	turn := turns[0]
	if turn.Data.ReplyTo != "room/r1/messages/m-1" || turn.Data.Mentions[0] != "admin" {
		t.Errorf("turn = %+v", turn.Data)
	}
	if turn.Path != TurnPath("room/r1", "wilson", []string{ack.OpID}) {
		t.Errorf("path = %s, want the derived one", turn.Path)
	}
	// The record: author wilson, parent the mention's op id.
	for op, err := range w.admin.History(w.ctx, "rooms", turn.Path) {
		if err != nil {
			t.Fatal(err)
		}
		if op.Author != "wilson" {
			t.Errorf("author = %s", op.Author)
		}
		if len(op.Parents) != 1 || op.Parents[0] != ack.OpID {
			t.Errorf("parents = %v, want [%s]", op.Parents, ack.OpID)
		}
	}
	// The harness saw the transcript: the test harness echoes the last
	// line, which is the instruction to answer the trigger's sequence.
	if got := turn.Data.Text; got == "" {
		t.Error("empty turn")
	}
	// wilson's own turn mentions admin, not wilson, and triggers nothing
	// more; a message that is not a mention triggers nothing either.
	w.say("admin", "m-2", "thanks")
	if got := w.settle(); len(got) != 1 {
		t.Errorf("turns = %d after a non-mention, want 1", len(got))
	}
}

func TestTheSameTriggerIsAnsweredOnce(t *testing.T) {
	w := newWorld(t)
	c := w.l.Connect(t, "wilson")
	d, _ := declaration.Parse([]byte(wilson + harnessYAML))
	r, err := New(c, d, slog.New(slog.NewTextHandler(io.Discard, nil)))
	if err != nil {
		t.Fatal(err)
	}
	ack := w.say("admin", "m-1", "@wilson?", "wilson")
	rm := &room{store: "rooms", path: "room/r1", byOpID: map[string]*message{}}
	trigger := message{OpID: ack.OpID, Path: "room/r1/messages/m-1", Author: "admin"}
	r.post(w.ctx, rm, []message{trigger}, harness.Result{Text: "first answer"})
	r.post(w.ctx, rm, []message{trigger}, harness.Result{Text: "second answer, must not land"})
	turns := w.turns()
	if len(turns) != 1 || turns[0].Data.Text != "first answer" {
		t.Fatalf("turns = %+v, want exactly the first", turns)
	}
	// A different writer under the same path is refused as existing.
	_, err = c.Create(w.ctx, "rooms", TurnPath("room/r1", "wilson", []string{ack.OpID}), "create", []byte(`{"text":"x","kind":"turn"}`))
	if !errors.Is(err, client.ErrInstanceExists) {
		t.Errorf("err = %v, want ErrInstanceExists", err)
	}
}

func TestSelfMentionAndClosedRoomTriggerNothing(t *testing.T) {
	w := newWorld(t)
	stop := w.serve(wilson)
	defer stop()
	w.say("wilson", "m-1", "@wilson talking to myself", "wilson")
	if got := w.settle(); len(got) != 0 {
		t.Errorf("turns = %d after a self-mention, want 0", len(got))
	}
	if _, err := w.admin.Apply(w.ctx, "rooms", "room/r1", "close", []byte(`{"status":"closed"}`)); err != nil {
		t.Fatal(err)
	}
	time.Sleep(200 * time.Millisecond)
	w.say("admin", "m-2", "@wilson anyone there?", "wilson")
	if got := w.settle(); len(got) != 0 {
		t.Errorf("turns = %d in a closed room, want 0", len(got))
	}
}

func TestTheWindowFloorRefusesWithoutATrace(t *testing.T) {
	w := newWorld(t)
	stop := w.serve(wilson + "budget:\n  turns: 2\n  window: 1m\n")
	defer stop()
	w.say("admin", "m-1", "@wilson one", "wilson")
	w.waitTurns(1)
	w.say("admin", "m-2", "@wilson two", "wilson")
	w.waitTurns(2)
	w.say("admin", "m-3", "@wilson three", "wilson")
	got := w.settle()
	if len(got) != 2 {
		t.Fatalf("turns = %d, want 2: the third is refused", len(got))
	}
	// Nothing else was written for the refusal: the room holds exactly
	// the three mentions and the two turns.
	n := 0
	for _, err := range w.admin.ListInstances(w.ctx, "rooms", client.Under("room/r1")) {
		if err != nil {
			t.Fatal(err)
		}
		n++
	}
	if n != 5 {
		t.Errorf("messages = %d, want 5 (no system line, no trace)", n)
	}
}

func TestAnotherAgentsTurnDoesNotTriggerUnlessAllowed(t *testing.T) {
	w := newWorld(t)
	stop := w.serve(wilson)
	w.say("ada", "m-1", "@wilson from another agent", "wilson")
	if got := w.settle(); len(got) != 0 {
		t.Errorf("turns = %d from an agent's mention, want 0", len(got))
	}
	stop()

	stop = w.serve(wilson + "budget:\n  reply_to_agents: true\n")
	defer stop()
	w.say("ada", "m-2", "@wilson now allowed", "wilson")
	w.waitTurns(1)
}

func TestOpTriggerOnTheRoom(t *testing.T) {
	w := newWorld(t)
	stop := w.serve(`
identity: wilson
triggers:
  - kind: op
    under: room/r1
    operation: rename
instructions: You are wilson.
`)
	defer stop()
	if _, err := w.admin.Apply(w.ctx, "rooms", "room/r1", "rename", []byte(`{"title":"R1 renamed"}`)); err != nil {
		t.Fatal(err)
	}
	turns := w.waitTurns(1)
	if turns[0].Data.ReplyTo != "room/r1" {
		t.Errorf("reply_to = %s, want the room", turns[0].Data.ReplyTo)
	}
}

func TestTurnIDIsStable(t *testing.T) {
	a := TurnID("wilson", []string{"x", "y"})
	if a != TurnID("wilson", []string{"x", "y"}) || len(a) != 32 {
		t.Errorf("id = %s", a)
	}
	if a == TurnID("wilson", []string{"y", "x"}) || a == TurnID("ada", []string{"x", "y"}) {
		t.Error("ids collide across order or agent")
	}
}
