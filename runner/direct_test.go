package runner

import (
	"encoding/json"
	"testing"

	"github.com/impire-io/chronicle-agents/declaration"
	"github.com/impire-io/chronicle-agents/types"
)

// A direct conversation (decision 0011): a message by the other party
// triggers the agent without a mention; a message in a room that is not a
// conversation of the agent's does not.
func TestADirectConversationTriggersWithoutAMention(t *testing.T) {
	w := newWorld(t)
	dm := types.DirectPath("admin", "wilson")
	payload, _ := json.Marshal(types.DirectRoom("admin", "wilson"))
	if _, err := w.admin.Create(w.ctx, "rooms", dm, "create", payload); err != nil {
		t.Fatal(err)
	}
	other, _ := json.Marshal(types.DirectRoom("admin", "ada"))
	if _, err := w.admin.Create(w.ctx, "rooms", types.DirectPath("admin", "ada"), "create", other); err != nil {
		t.Fatal(err)
	}
	stop := w.serve(`
identity: wilson
triggers:
  - kind: direct
instructions: You are wilson.
`)
	defer stop()
	say := func(room, id, text string) {
		t.Helper()
		p, _ := json.Marshal(types.MessageData{Text: text, Kind: types.KindMessage})
		if _, err := w.admin.Create(w.ctx, "rooms", room+"/messages/"+id, "create", p); err != nil {
			t.Fatal(err)
		}
	}
	say(dm, "m-1", "is invoice 345 paid?")
	turns := w.turnsIn(dm)
	if len(turns) == 0 {
		turns = w.waitTurnsIn(dm, 1)
	}
	if turns[0].Data.ReplyTo != dm+"/messages/m-1" || turns[0].Data.Mentions[0] != "admin" {
		t.Errorf("turn = %+v", turns[0].Data)
	}
	// Not wilson's conversation, and a room: nothing.
	say(types.DirectPath("admin", "ada"), "m-1", "ada, are you there?")
	say("room/r1", "m-1", "anyone?")
	w.settle()
	if got := w.turnsIn(types.DirectPath("admin", "ada")); len(got) != 0 {
		t.Errorf("wilson answered in admin and ada's conversation")
	}
	if got := w.turns(); len(got) != 0 {
		t.Errorf("wilson answered a room without a mention")
	}
	if d, err := declaration.Parse([]byte("identity: wilson\ntriggers:\n  - kind: direct\n    under: room/x\nharness:\n  preset: claude-code\n")); err == nil {
		t.Errorf("a direct trigger with under was accepted: %+v", d)
	}
}
