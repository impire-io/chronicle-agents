package runner

import (
	"context"
	"encoding/json"
	"io"
	"log/slog"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/impire-io/chronicle-agents/declaration"
	"github.com/impire-io/chronicle-agents/live"
	"github.com/impire-io/chronicle-agents/types"
	"github.com/impire-io/chronicle/contract"
	"github.com/nats-io/nats.go"
	"github.com/nats-io/nats.go/jetstream"
)

// The runner's voice off the record (decision 0009, design 02 § live
// signals): what it says on CHRON.LIVE.<store>., heard by a plain
// subscriber on the quick start's NATS, and never a message in the store.

// ear hears every live signal in the rooms store.
type ear struct {
	mu      sync.Mutex
	signals []live.Signal
	present []live.Presence
}

func listen(t *testing.T, nc *nats.Conn) *ear {
	t.Helper()
	e := &ear{}
	sub, err := nc.Subscribe(live.Root+".rooms.>", func(m *nats.Msg) {
		e.mu.Lock()
		defer e.mu.Unlock()
		switch {
		case strings.Contains(m.Subject, ".room."):
			var s live.Signal
			if err := json.Unmarshal(m.Data, &s); err == nil {
				e.signals = append(e.signals, s)
			}
		case strings.Contains(m.Subject, ".agent."):
			var p live.Presence
			if err := json.Unmarshal(m.Data, &p); err == nil {
				e.present = append(e.present, p)
			}
		}
	})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = sub.Unsubscribe() })
	return e
}

func (e *ear) states() []live.State {
	e.mu.Lock()
	defer e.mu.Unlock()
	out := make([]live.State, 0, len(e.signals))
	for _, s := range e.signals {
		out = append(out, s.State)
	}
	return out
}

func (e *ear) wait(t *testing.T, want live.State) live.Signal {
	t.Helper()
	deadline := time.Now().Add(15 * time.Second)
	for {
		e.mu.Lock()
		for _, s := range e.signals {
			if s.State == want {
				e.mu.Unlock()
				return s
			}
		}
		e.mu.Unlock()
		if time.Now().After(deadline) {
			t.Fatalf("after 15s no %q signal; heard %v", want, e.states())
		}
		time.Sleep(50 * time.Millisecond)
	}
}

func streamMsgs(t *testing.T, nc *nats.Conn, store string) uint64 {
	t.Helper()
	js, err := jetstream.New(nc)
	if err != nil {
		t.Fatal(err)
	}
	st, err := js.Stream(context.Background(), contract.StreamName(store))
	if err != nil {
		t.Fatal(err)
	}
	info, err := st.Info(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	return info.State.Msgs
}

func TestAnAdmittedMentionIsPickedUpAndDone(t *testing.T) {
	w := newWorld(t)
	e := listen(t, w.admin.Conn())
	stop := w.serve(wilson)
	defer stop()

	ack := w.say("admin", "m-1", "@wilson has invoice 345 been paid?", "wilson")
	_ = ack
	w.waitTurns(1)
	picked := e.wait(t, live.PickedUp)
	done := e.wait(t, live.Done)
	if picked.Agent != "wilson" || picked.Trigger != "room/r1/messages/m-1" || picked.At.IsZero() {
		t.Errorf("picked-up = %+v", picked)
	}
	if done.Trigger != picked.Trigger || !done.At.After(picked.At) && !done.At.Equal(picked.At) {
		t.Errorf("done = %+v after %+v", done, picked)
	}
	for _, s := range e.states() {
		if s == live.Refused || s == live.Failed {
			t.Errorf("heard %q for an admitted mention", s)
		}
	}
}

func TestAWindowRefusalIsSaidInThePersonsWords(t *testing.T) {
	w := newWorld(t)
	e := listen(t, w.admin.Conn())
	stop := w.serve(wilson + "budget:\n  turns: 2\n  window: 1m\n  reply_to_agents: true\n")
	defer stop()
	w.say("ada", "m-1", "@wilson one", "wilson")
	w.waitTurns(1)
	w.say("ada", "m-2", "@wilson two", "wilson")
	w.waitTurns(2)
	before := streamMsgs(t, w.admin.Conn(), "rooms")
	w.say("ada", "m-3", "@wilson three", "wilson")
	refused := e.wait(t, live.Refused)
	if refused.Trigger != "room/r1/messages/m-3" || refused.Reason != "2 turns in a minute" {
		t.Errorf("refused = %+v", refused)
	}
	// Signals are off the record: the only thing that reached the stream
	// since is the third mention itself.
	if got := w.settle(); len(got) != 2 {
		t.Errorf("turns = %d, want 2", len(got))
	}
	if after := streamMsgs(t, w.admin.Conn(), "rooms"); after != before+1 {
		t.Errorf("stream grew by %d, want 1 (the mention) — a signal was stored", after-before)
	}
}

func TestAnotherAgentsMentionIsRefusedInWords(t *testing.T) {
	w := newWorld(t)
	e := listen(t, w.admin.Conn())
	stop := w.serve(wilson)
	defer stop()
	w.say("ada", "m-1", "@wilson from another agent", "wilson")
	refused := e.wait(t, live.Refused)
	if refused.Reason != "a mention from another agent" {
		t.Errorf("reason = %q", refused.Reason)
	}
}

func TestThinkingIsRepeatedWhileTheHarnessRuns(t *testing.T) {
	w := newWorld(t)
	e := listen(t, w.admin.Conn())
	d, err := declaration.Parse([]byte(wilson + `
harness:
  template:
    command: sh
    args: ["-c", "sleep 4; echo slow answer"]
  deadline: 20s
`))
	if err != nil {
		t.Fatal(err)
	}
	c := w.l.Connect(t, d.Identity)
	r, err := New(c, d, slog.New(slog.NewTextHandler(io.Discard, nil)))
	if err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithCancel(w.ctx)
	defer cancel()
	go func() { _ = r.Serve(ctx) }()
	time.Sleep(300 * time.Millisecond)
	w.say("admin", "m-1", "@wilson slowly", "wilson")
	e.wait(t, live.Thinking)
	e.wait(t, live.Done)
	w.waitTurns(1)
}

func TestAFailedHarnessIsSaid(t *testing.T) {
	w := newWorld(t)
	e := listen(t, w.admin.Conn())
	d, err := declaration.Parse([]byte(wilson + `
harness:
  template:
    command: sh
    args: ["-c", "exit 3"]
  deadline: 20s
`))
	if err != nil {
		t.Fatal(err)
	}
	c := w.l.Connect(t, d.Identity)
	r, err := New(c, d, slog.New(slog.NewTextHandler(io.Discard, nil)))
	if err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithCancel(w.ctx)
	defer cancel()
	go func() { _ = r.Serve(ctx) }()
	time.Sleep(300 * time.Millisecond)
	w.say("admin", "m-1", "@wilson fail", "wilson")
	failed := e.wait(t, live.Failed)
	if !strings.HasPrefix(failed.Reason, "the harness did not answer") {
		t.Errorf("reason = %q", failed.Reason)
	}
	if got := w.settle(); len(got) != 0 {
		t.Errorf("turns = %d from a failed harness", len(got))
	}
}

func TestTheRunnerAnnouncesItselfAndSaysItIsOnline(t *testing.T) {
	w := newWorld(t)
	e := listen(t, w.admin.Conn())
	d, err := declaration.Parse([]byte(`
identity: wilson
description: answers what is asked of it, briefly
triggers:
  - kind: mention
  - kind: schedule
    schedule: schedule/hourly
    room: room/ops
instructions: You are wilson.
` + harnessYAML))
	if err != nil {
		t.Fatal(err)
	}
	c := w.l.Connect(t, d.Identity)
	r, err := New(c, d, slog.New(slog.NewTextHandler(io.Discard, nil)), Version("9.9.9"))
	if err != nil {
		t.Fatal(err)
	}
	// The tick store the schedule names must exist for serving to start.
	if _, err := w.admin.CreateStore(w.ctx, "ticks", "test"); err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithCancel(w.ctx)
	defer cancel()
	go func() { _ = r.Serve(ctx) }()

	// The announcement: what wilson is, in the rooms store, by wilson.
	var got types.AgentData
	deadline := time.Now().Add(10 * time.Second)
	for {
		data, ok, err := types.ReadAgent(w.ctx, w.admin, "rooms", "wilson")
		if err != nil {
			t.Fatal(err)
		}
		if ok {
			got = data
			break
		}
		if time.Now().After(deadline) {
			t.Fatal("agent/wilson was not announced")
		}
		time.Sleep(50 * time.Millisecond)
	}
	want := types.AgentData{Name: "wilson", Description: "answers what is asked of it, briefly", Harness: "tail",
		Triggers: []string{"mention in rooms", "schedule schedule/hourly → room/ops"}, Version: "9.9.9"}
	if !got.Equal(want) {
		t.Errorf("announced %+v, want %+v", got, want)
	}
	for op, err := range w.admin.History(w.ctx, "rooms", types.AgentPath("wilson")) {
		if err != nil {
			t.Fatal(err)
		}
		if op.Author != "wilson" {
			t.Errorf("announced by %s", op.Author)
		}
	}
	// Presence: online, said at start, with the version.
	deadline = time.Now().Add(10 * time.Second)
	for {
		e.mu.Lock()
		n := len(e.present)
		var p live.Presence
		if n > 0 {
			p = e.present[0]
		}
		e.mu.Unlock()
		if n > 0 {
			if p.Agent != "wilson" || p.State != live.Online || p.Version != "9.9.9" || p.Since.IsZero() {
				t.Errorf("presence = %+v", p)
			}
			break
		}
		if time.Now().After(deadline) {
			t.Fatal("no presence heard")
		}
		time.Sleep(50 * time.Millisecond)
	}
	// Serving again with the same file writes nothing new; a changed line
	// is an update, by wilson, and the record still has one agent.
	cancel()
	time.Sleep(200 * time.Millisecond)
	seqBefore := historyLen(t, w, types.AgentPath("wilson"))
	if err := types.AnnounceAgent(w.ctx, c, "rooms", want); err != nil {
		t.Fatal(err)
	}
	if n := historyLen(t, w, types.AgentPath("wilson")); n != seqBefore {
		t.Errorf("announcing the same thing wrote %d operations", n-seqBefore)
	}
	changed := want
	changed.Description = "answers, at length"
	if err := types.AnnounceAgent(w.ctx, c, "rooms", changed); err != nil {
		t.Fatal(err)
	}
	if n := historyLen(t, w, types.AgentPath("wilson")); n != seqBefore+1 {
		t.Errorf("a changed announcement wrote %d operations, want 1", n-seqBefore)
	}
	agents, err := types.ListAgents(w.ctx, w.admin, "rooms")
	if err != nil {
		t.Fatal(err)
	}
	if len(agents) != 1 || agents[0].Description != "answers, at length" {
		t.Errorf("agents = %+v", agents)
	}
}

func historyLen(t *testing.T, w *world, path string) int {
	t.Helper()
	n := 0
	for _, err := range w.admin.History(w.ctx, "rooms", path) {
		if err != nil {
			t.Fatal(err)
		}
		n++
	}
	return n
}
