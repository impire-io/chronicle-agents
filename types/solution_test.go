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

func TestAgentAnnouncementAndGeneral(t *testing.T) {
	l := chrontest.Up(t)
	admin := l.Connect(t, "admin")
	ctx := context.Background()
	if _, err := admin.CreateStore(ctx, "rooms", "test"); err != nil {
		t.Fatal(err)
	}
	if err := Declare(ctx, admin, "rooms"); err != nil {
		t.Fatal(err)
	}
	// General: once, and again is nothing.
	created, err := EnsureGeneral(ctx, admin, "rooms")
	if err != nil || !created {
		t.Fatalf("general: created=%v err=%v", created, err)
	}
	created, err = EnsureGeneral(ctx, admin, "rooms")
	if err != nil || created {
		t.Fatalf("general again: created=%v err=%v", created, err)
	}
	// The agent type holds the announcement and refuses one without a name.
	data := AgentData{Name: "wilson", Description: "answers briefly", Harness: "claude-code", Triggers: []string{"mention in rooms"}, Version: "1.0.0"}
	if err := AnnounceAgent(ctx, admin, "rooms", data); err != nil {
		t.Fatal(err)
	}
	got, ok, err := ReadAgent(ctx, admin, "rooms", "wilson")
	if err != nil || !ok || !got.Equal(data) {
		t.Fatalf("read agent: ok=%v err=%v got=%+v", ok, err, got)
	}
	if _, _, err := ReadAgent(ctx, admin, "rooms", "nobody"); err != nil {
		t.Fatal(err)
	}
	if _, err := admin.Create(ctx, "rooms", AgentPath("anon"), "create", []byte(`{"description":"no name"}`)); err == nil {
		t.Error("an announcement without a name was accepted")
	}
}
