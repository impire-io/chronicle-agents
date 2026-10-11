package runner

import (
	"testing"
	"time"
)

// A trigger reaches the runner the moment it lands: the tail is a
// JetStream consumer, delivered on arrival, never polled. With a harness
// that answers in no time, the turn is in the record well under a second
// after the mention. This guards the reaction against a tail that waits.
func TestATriggerIsReactedToOnArrival(t *testing.T) {
	w := newWorld(t)
	stop := w.serve(wilson)
	defer stop()

	const rounds = 3
	var worst time.Duration
	for i := 0; i < rounds; i++ {
		start := time.Now()
		w.say("admin", "m-"+string(rune('a'+i)), "@wilson now", "wilson")
		w.waitTurns(i + 1)
		took := time.Since(start)
		t.Logf("mention to turn: %s", took.Round(time.Millisecond))
		if took > worst {
			worst = took
		}
	}
	if worst > 2*time.Second {
		t.Errorf("mention to turn took %s at worst; the tail should deliver on arrival", worst.Round(time.Millisecond))
	}
}
