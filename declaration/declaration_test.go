package declaration

import (
	"strings"
	"testing"
	"time"
)

const minimal = `
identity: wilson
triggers:
  - kind: mention
harness:
  preset: claude-code
instructions: Answer briefly.
`

func TestParseFillsTheDefaults(t *testing.T) {
	d, err := Parse([]byte(minimal))
	if err != nil {
		t.Fatal(err)
	}
	if d.Triggers[0].Store != DefaultStore {
		t.Errorf("store = %q, want %q", d.Triggers[0].Store, DefaultStore)
	}
	if d.Budget.Turns != DefaultTurns || d.Budget.Window.Std() != time.Minute || d.Budget.Depth != DefaultDepth {
		t.Errorf("budget = %+v, want the defaults", d.Budget)
	}
	if d.Harness.Deadline.Std() != 3*time.Minute {
		t.Errorf("deadline = %s, want 3m", d.Harness.Deadline.Std())
	}
	if got := d.Stores(); len(got) != 1 || got[0] != "rooms" {
		t.Errorf("stores = %v", got)
	}
}

func TestParseReadsEveryFacetAndEveryKind(t *testing.T) {
	d, err := Parse([]byte(`
identity: wilson
triggers:
  - kind: mention
    store: finance
    under: room/close-w41
  - kind: op
    under: room/close-w41
    operation: close
  - kind: subject
    subject: alerts.>
    room: room/ops
  - kind: schedule
    schedule: schedule/hourly
    room: room/ops
  - kind: op
    under: invoice/inv-1
    room: room/close-w41
    room_store: finance
harness:
  template:
    command: tail
    args: ["-n", "1"]
    env: {A: b}
  deadline: 90s
instructions: You are wilson.
toolkits:
  - name: invoices
    command: invoices-mcp
    args: ["--prod"]
budget:
  turns: 2
  window: 30s
  depth: 2
  reply_to_agents: true
`))
	if err != nil {
		t.Fatal(err)
	}
	tr := d.Triggers
	if tr[0].Store != "finance" || tr[1].Store != DefaultStore || tr[1].Operation != "close" {
		t.Errorf("triggers = %+v", tr[:2])
	}
	if tr[2].Room != "room/ops" || tr[2].RoomStore != DefaultStore {
		t.Errorf("subject trigger = %+v", tr[2])
	}
	if tr[3].Store != DefaultTickStore || tr[3].RoomStore != DefaultStore {
		t.Errorf("schedule trigger = %+v", tr[3])
	}
	if tr[4].InRooms() || tr[4].RoomStore != "finance" {
		t.Errorf("op trigger outside the rooms = %+v", tr[4])
	}
	if d.Harness.Template == nil || d.Harness.Template.Command != "tail" || d.Harness.Deadline.Std() != 90*time.Second {
		t.Errorf("harness = %+v", d.Harness)
	}
	if len(d.Toolkits) != 1 || d.Toolkits[0].Args[0] != "--prod" {
		t.Errorf("toolkits = %+v", d.Toolkits)
	}
	if d.Budget.Turns != 2 || d.Budget.Window.Std() != 30*time.Second || !d.Budget.ReplyToAgents {
		t.Errorf("budget = %+v", d.Budget)
	}
	// finance, rooms, ticks — each once, in first-seen order.
	if got := d.Stores(); len(got) != 3 || got[0] != "finance" || got[1] != "rooms" || got[2] != "ticks" {
		t.Errorf("stores = %v, want finance, rooms, ticks", got)
	}
}

func TestRefusals(t *testing.T) {
	h := "harness: {preset: claude-code}\n"
	cases := map[string]struct {
		file string
		want string
	}{
		"no identity":          {file: "triggers: []\n" + h, want: "identity: required"},
		"bad identity":         {file: "identity: 'wil son'\n" + h, want: "identity:"},
		"no harness":           {file: "identity: wilson\n", want: "harness: a preset or a template"},
		"both harness":         {file: "identity: wilson\nharness: {preset: claude-code, template: {command: x}}\n", want: "not both"},
		"unknown kind":         {file: "identity: wilson\ntriggers: [{kind: poke}]\n" + h, want: "not mention, op, subject or schedule"},
		"mention + op":         {file: "identity: wilson\ntriggers: [{kind: mention, operation: close}]\n" + h, want: "selects messages"},
		"subject, no room":     {file: "identity: wilson\ntriggers: [{kind: subject, subject: alerts.>}]\n" + h, want: "names the room"},
		"subject, no subject":  {file: "identity: wilson\ntriggers: [{kind: subject, room: room/ops}]\n" + h, want: "subject: required"},
		"schedule, no room":    {file: "identity: wilson\ntriggers: [{kind: schedule, schedule: schedule/hourly}]\n" + h, want: "names the room"},
		"schedule, bad path":   {file: "identity: wilson\ntriggers: [{kind: schedule, schedule: tick/hourly, room: room/ops}]\n" + h, want: "not schedule/<name>"},
		"op outside, no room":  {file: "identity: wilson\ntriggers: [{kind: op, under: invoice/inv-1}]\n" + h, want: "outside the rooms names the room"},
		"room is not a room":   {file: "identity: wilson\ntriggers: [{kind: subject, subject: a, room: invoice/inv-1}]\n" + h, want: "not room/<id>"},
		"unknown field":        {file: "identity: wilson\n" + h + "wake: true\n", want: "wake"},
		"toolkit without name": {file: "identity: wilson\n" + h + "toolkits: [{command: x}]\n", want: "toolkits[0].name"},
	}
	for name, tc := range cases {
		t.Run(name, func(t *testing.T) {
			_, err := Parse([]byte(tc.file))
			if err == nil {
				t.Fatal("parsed; want a refusal")
			}
			if !strings.Contains(err.Error(), tc.want) {
				t.Errorf("error %q does not name %q", err, tc.want)
			}
		})
	}
}
