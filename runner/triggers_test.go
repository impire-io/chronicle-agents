package runner

import (
	"context"
	"testing"
	"time"

	"github.com/impire-io/chronicle-agents/schedule"
	"github.com/impire-io/chronicle/client"
)

// A message on a subject triggers a turn in the room the entry names; the
// same message again derives the same turn and is answered once.
func TestASubjectMessageTriggersATurnInTheNamedRoom(t *testing.T) {
	w := newWorld(t)
	stop := w.serve(`
identity: wilson
triggers:
  - kind: subject
    subject: alerts.test
    room: room/r1
instructions: You are wilson.
`)
	defer stop()
	if err := w.admin.Conn().Publish("alerts.test", []byte(`{"level":"high","text":"disk at 91%"}`)); err != nil {
		t.Fatal(err)
	}
	turns := w.waitTurns(1)
	if turns[0].Data.ReplyTo != "" || len(turns[0].Data.Mentions) != 0 {
		t.Errorf("a subject turn answers nobody in particular: %+v", turns[0].Data)
	}
	// The same message again: the same digest, the same turn, refused by the guard.
	if err := w.admin.Conn().Publish("alerts.test", []byte(`{"level":"high","text":"disk at 91%"}`)); err != nil {
		t.Fatal(err)
	}
	if got := w.settle(); len(got) != 1 {
		t.Errorf("turns = %d after the same message twice, want 1", len(got))
	}
	// A different message: a second turn.
	if err := w.admin.Conn().Publish("alerts.test", []byte(`{"level":"low","text":"disk at 70%"}`)); err != nil {
		t.Fatal(err)
	}
	w.waitTurns(2)
}

// A schedule's ticks — written by the server into a store that allows
// schedules — trigger turns in the room the entry names; the runner has
// no clock of its own.
func TestATickTriggersATurn(t *testing.T) {
	w := newWorld(t)
	if _, err := w.admin.CreateStore(w.ctx, "ticks", "the tick store", client.WithSchedules()); err != nil {
		t.Fatal(err)
	}
	if err := schedule.Create(w.ctx, w.admin.Conn(), "admin", "ticks", "fast", schedule.Every(time.Second), 10*time.Second); err != nil {
		t.Fatal(err)
	}
	stop := w.serve(`
identity: wilson
triggers:
  - kind: schedule
    schedule: schedule/fast
    room: room/r1
instructions: You are wilson.
budget:
  turns: 20
`)
	defer stop()
	turns := w.waitTurns(2)
	if turns[0].Path == turns[1].Path {
		t.Error("two ticks derived the same turn")
	}
	if turns[0].Data.ReplyTo != "" {
		t.Errorf("a tick's turn answers nobody: %+v", turns[0].Data)
	}
	// The ticks are in the record, written by the server: ops under the
	// schedule's ticks path, with the scheduler's provenance.
	n := 0
	for op, err := range w.admin.HistoryUnder(context.Background(), "ticks", "schedule/fast") {
		if err != nil {
			t.Fatal(err)
		}
		n++
		_ = op
	}
	if n < 3 { // the registration and at least two ticks
		t.Errorf("ticks store holds %d ops under the schedule, want the registration and two ticks", n)
	}
}
