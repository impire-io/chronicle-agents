package types

import (
	"encoding/json"
	"strings"
	"testing"
)

// The declaration marshals as design 14's manifest: the shape's version,
// the announcement's fields, the stores in order with their types in
// order and the wire's field names, the tick store allowing schedules.
func TestManifestIsTheDeclarationInTheWiresShape(t *testing.T) {
	m := Declaration("rooms", "ticks").Manifest("1.2.3", "https://agents.example.test")
	body, err := json.Marshal(m)
	if err != nil {
		t.Fatal(err)
	}
	s := string(body)
	for _, want := range []string{
		`"manifest":1`,
		`"solution":{"name":"chronicle-agents","description":"` + SolutionDescription + `","url":"https://agents.example.test","version":"1.2.3","home":"` + SolutionHome + `"}`,
		`"name":"rooms","description":"` + RoomsDescription + `","types":[{"name":"message","definition":{"schema":{`,
		`"history":"full","operations":{"create":{"schema":{"type":"object","required":["text"]},"effect":"merge"}}}},{"name":"room","definition":{"schema":{`,
		`"children":{"messages":"message"}`,
		`{"name":"solution","definition":{"schema":{`,
		`"name":"ticks","description":"` + TicksDescription + `","schedules":true,"types":[]`,
		`"announce":{"store":"rooms","path":"solution/chronicle-agents"}`,
	} {
		if !strings.Contains(s, want) {
			t.Errorf("manifest lacks %s:\n%s", want, s)
		}
	}
	if strings.Contains(s, `"Schema"`) || strings.Contains(s, `"Operations"`) {
		t.Errorf("manifest carries the Go names, not the wire's:\n%s", s)
	}
	// The types come in definition order: message, room, solution.
	if i, j, k := strings.Index(s, `"name":"message"`), strings.Index(s, `"name":"room"`), strings.Index(s, `"name":"solution"`); i >= j || j >= k {
		t.Errorf("types out of order: message %d, room %d, solution %d", i, j, k)
	}
	// No tick store: one store, nothing else changes.
	if n := len(Declaration("rooms", "").Stores); n != 1 {
		t.Errorf("without ticks: %d stores", n)
	}
	if got := Declaration("rooms", "ticks").WorkspaceTypes("rooms"); strings.Join(got, ",") != "message,room" {
		t.Errorf("workspace types = %v", got)
	}
}
