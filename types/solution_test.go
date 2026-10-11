package types

import (
	"context"
	"encoding/json"
	"testing"
	"time"

	"github.com/impire-io/chronicle-agents/internal/chrontest"
)

func TestAnnounceIsIdempotentAndLocateSetsTheURL(t *testing.T) {
	l := chrontest.Up(t)
	admin := l.Connect(t, "admin")
	ctx := context.Background()
	if _, err := admin.CreateStore(ctx, "rooms", "test"); err != nil {
		t.Fatal(err)
	}
	if err := Announce(ctx, admin, "rooms", "0.0.0-test"); err != nil {
		t.Fatal(err)
	}
	if err := Announce(ctx, admin, "rooms", "0.0.0-test"); err != nil {
		t.Fatalf("second announce: %v", err)
	}
	if err := Locate(ctx, admin, "rooms", "http://127.0.0.1:8080"); err != nil {
		t.Fatal(err)
	}
	// The state is derived and may trail the history by a moment.
	var d SolutionData
	deadline := time.Now().Add(10 * time.Second)
	for {
		sv, err := admin.State(ctx, "rooms", SolutionPath)
		if err == nil {
			if err := json.Unmarshal(sv.State, &d); err != nil {
				t.Fatal(err)
			}
		}
		if d.URL != "" || time.Now().After(deadline) {
			break
		}
		time.Sleep(100 * time.Millisecond)
	}
	if d.Name != SolutionName || d.URL != "http://127.0.0.1:8080" || d.Version != "0.0.0-test" {
		t.Errorf("announcement = %+v", d)
	}
}
