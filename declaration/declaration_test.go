package declaration

import (
	"errors"
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

func TestParseReadsEveryFacet(t *testing.T) {
	d, err := Parse([]byte(`
identity: wilson
triggers:
  - kind: mention
    store: finance
    under: room/close-w41
  - kind: op
    under: room/close-w41
    operation: close
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
	if d.Triggers[0].Store != "finance" || d.Triggers[1].Store != DefaultStore || d.Triggers[1].Operation != "close" {
		t.Errorf("triggers = %+v", d.Triggers)
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
	if got := d.Stores(); len(got) != 2 {
		t.Errorf("stores = %v, want finance and rooms", got)
	}
}

func TestRefusals(t *testing.T) {
	cases := map[string]struct {
		file string
		want string
		load bool
	}{
		"no identity":   {file: "triggers: []\nharness: {preset: claude-code}\n", want: "identity: required"},
		"bad identity":  {file: "identity: 'wil son'\nharness: {preset: claude-code}\n", want: "identity:"},
		"no harness":    {file: "identity: wilson\n", want: "harness: a preset or a template"},
		"both harness":  {file: "identity: wilson\nharness: {preset: claude-code, template: {command: x}}\n", want: "not both"},
		"unknown kind":  {file: "identity: wilson\ntriggers: [{kind: poke}]\nharness: {preset: claude-code}\n", want: "not mention, op, subject or schedule"},
		"mention + op":  {file: "identity: wilson\ntriggers: [{kind: mention, operation: close}]\nharness: {preset: claude-code}\n", want: "selects messages"},
		"subject":       {file: "identity: wilson\ntriggers: [{kind: subject, subject: alerts.>}]\nharness: {preset: claude-code}\n", want: "following spec", load: true},
		"schedule":      {file: "identity: wilson\ntriggers: [{kind: schedule, schedule: schedule/hourly}]\nharness: {preset: claude-code}\n", want: "chronicle-61", load: true},
		"unknown field": {file: "identity: wilson\nharness: {preset: claude-code}\nwake: true\n", want: "wake"},
		"toolkit name":  {file: "identity: wilson\nharness: {preset: claude-code}\ntoolkits: [{command: x}]\n", want: "toolkits[0].name"},
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
			if tc.load && !errors.Is(err, ErrRefusedAtLoad) {
				t.Errorf("error %q is not ErrRefusedAtLoad", err)
			}
		})
	}
}
