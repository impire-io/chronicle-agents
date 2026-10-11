package types

import (
	"context"
	"testing"

	"github.com/impire-io/chronicle-agents/internal/chrontest"
)

func TestDeclareDefinesBothTypesAndRevises(t *testing.T) {
	l := chrontest.Up(t)
	admin := l.Connect(t, "admin")
	ctx := context.Background()
	if _, err := admin.CreateStore(ctx, "rooms", "test"); err != nil {
		t.Fatal(err)
	}
	if err := Declare(ctx, admin, "rooms"); err != nil {
		t.Fatal(err)
	}
	room, err := admin.GetType(ctx, "rooms", Room)
	if err != nil {
		t.Fatal(err)
	}
	if room.Children[Messages] != Message {
		t.Errorf("room children = %v", room.Children)
	}
	if _, err := admin.GetType(ctx, "rooms", Message); err != nil {
		t.Fatal(err)
	}
	// Again: a revision, not an error.
	if err := Declare(ctx, admin, "rooms"); err != nil {
		t.Fatalf("second declare: %v", err)
	}
	// The types hold: a room and a message are created through them.
	if _, err := admin.Create(ctx, "rooms", "room/r1", "create", []byte(`{"title":"R1","status":"open"}`)); err != nil {
		t.Fatal(err)
	}
	if _, err := admin.Create(ctx, "rooms", "room/r1/messages/m-1", "create", []byte(`{"text":"hello","kind":"message"}`)); err != nil {
		t.Fatal(err)
	}
	// And refuse what they declare against: a message without text.
	if _, err := admin.Create(ctx, "rooms", "room/r1/messages/m-2", "create", []byte(`{"kind":"message"}`)); err == nil {
		t.Error("a message without text was accepted")
	}
}
