// Package declaration is the definition file: what an agent is, in the
// facets chronicle-agents-hq design 02 states — an identity, one line of
// what it is for, its triggers, a harness, its instructions, its
// toolkits, a budget. Loading a
// file validates it and refuses, with the field named, what the runner
// cannot serve (decisions 0003 and 0004).
package declaration

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/impire-io/chronicle/contract"
	"sigs.k8s.io/yaml"
)

// Trigger kinds (design 02, "the four triggers").
const (
	KindMention  = "mention"
	KindOp       = "op"
	KindSubject  = "subject"
	KindSchedule = "schedule"
)

// Declaration is one agent, as its definition file states it.
type Declaration struct {
	// Identity is the agent's name: the chronicle service account it acts
	// as, the author on every turn, and the @name a mention addresses.
	Identity string `json:"identity"`
	// Description is one line of what the agent is for, shown wherever
	// its name is offered (decision 0009). Optional: an agent without one
	// is offered by name alone.
	Description string `json:"description,omitempty"`
	// Triggers is what the agent reacts to. Empty is legal: the agent is
	// served and does nothing.
	Triggers []Trigger `json:"triggers"`
	// Harness is what thinks.
	Harness Harness `json:"harness"`
	// Instructions are the agent's standing instructions, handed to the
	// harness as its system prompt.
	Instructions string `json:"instructions"`
	// Toolkits are the MCP servers the harness may call.
	Toolkits []Toolkit `json:"toolkits"`
	// Budget is the admission rule set.
	Budget Budget `json:"budget"`
}

// Trigger is one entry of the trigger set.
type Trigger struct {
	// Kind is mention, op, subject or schedule.
	Kind string `json:"kind"`
	// Store is the chronicle store the trigger reads: the rooms store for
	// mention and op (default rooms), the tick store for schedule (default
	// ticks). Unused by subject.
	Store string `json:"store,omitempty"`
	// Under narrows a mention or op trigger to the instances under a path
	// (a room, a type); every room in the store when empty.
	Under string `json:"under,omitempty"`
	// Operation narrows an op trigger to one operation name.
	Operation string `json:"operation,omitempty"`
	// Subject is the NATS subject of a subject trigger.
	Subject string `json:"subject,omitempty"`
	// Schedule is the schedule's path in the tick store: schedule/<name>.
	Schedule string `json:"schedule,omitempty"`
	// Room is where the turn is written when the trigger is not in a room
	// — a subject, a schedule, an op under a path outside the rooms
	// (decision 0004 point 2). A mention's turn answers in its own room.
	Room string `json:"room,omitempty"`
	// RoomStore is the store Room lives in; the default rooms store when
	// empty.
	RoomStore string `json:"room_store,omitempty"`
}

// Harness names what thinks: a preset, or a template of the harness
// package's shape.
type Harness struct {
	Preset string `json:"preset,omitempty"`
	// Model is the model's name where the harness takes one.
	Model string `json:"model,omitempty"`
	// Template is an inline invocation template, used when Preset is empty.
	Template *Template `json:"template,omitempty"`
	// Deadline bounds one turn; the runner's default when zero.
	Deadline Duration `json:"deadline,omitempty"`
	// Tools are the harness's own tools the agent may use, by the
	// harness's names (Bash, Read, Edit, WebFetch…), each optionally with
	// a rule in the harness's syntax that says which calls run without
	// asking: "Bash(git *)" grants the shell and pre-approves git; a bare
	// "Bash" grants and pre-approves every command. A call the rule does
	// not cover is asked of the people in the room. None by default: an
	// agent with no tools answers from the transcript and says so.
	Tools []string `json:"tools,omitempty"`
	// Workdir is the directory the harness runs in, for an agent whose
	// tools work on files: an absolute path that exists. A fresh temporary
	// directory per turn when empty.
	Workdir string `json:"workdir,omitempty"`
}

// Template is an invocation template as the definition file writes it.
// The placeholders are the harness package's.
type Template struct {
	Command string            `json:"command"`
	Args    []string          `json:"args"`
	Env     map[string]string `json:"env,omitempty"`
	// Output is how the harness's standard output is read: "text" (the
	// whole of it is the turn; the default) or "claude-stream" (Claude
	// Code's event stream, one JSON object per line).
	Output string `json:"output,omitempty"`
}

// Toolkit is one MCP server the harness may call.
type Toolkit struct {
	Name    string            `json:"name"`
	Command string            `json:"command"`
	Args    []string          `json:"args,omitempty"`
	Env     map[string]string `json:"env,omitempty"`
}

// Budget is the admission rule set (design 02, "admission").
type Budget struct {
	// Turns is the window floor's K: at most this many turns by the agent
	// in one room within Window.
	Turns int `json:"turns"`
	// Window is the window floor's W.
	Window Duration `json:"window"`
	// Depth is the depth bound's D: the longest trigger chain, counted in
	// turns, a turn may extend.
	Depth int `json:"depth"`
	// ReplyToAgents admits a trigger authored by another agent.
	ReplyToAgents bool `json:"reply_to_agents"`
}

// Duration is a Go duration in the file ("1m", "90s").
type Duration time.Duration

// UnmarshalJSON parses the duration's text form.
func (d *Duration) UnmarshalJSON(b []byte) error {
	s := strings.Trim(string(b), `"`)
	if s == "" || s == "null" {
		*d = 0
		return nil
	}
	v, err := time.ParseDuration(s)
	if err != nil {
		return fmt.Errorf("duration %q: %w", s, err)
	}
	*d = Duration(v)
	return nil
}

// Std returns the duration as time.Duration.
func (d Duration) Std() time.Duration { return time.Duration(d) }

// Describe is the trigger as one line, as the file states it — what the
// runner announces beside the agent's name (decision 0009): "mention in
// rooms", "close on room/close-w41 in rooms", "alerts.> → room/ops",
// "schedule schedule/hourly → room/ops".
func (t Trigger) Describe() string {
	switch t.Kind {
	case KindMention:
		if t.Under != "" {
			return fmt.Sprintf("mention under %s in %s", t.Under, t.Store)
		}
		return "mention in " + t.Store
	case KindOp:
		op := t.Operation
		if op == "" {
			op = "any operation"
		}
		where := t.Store
		if t.Under != "" {
			where = t.Under + " in " + t.Store
		}
		if t.Room != "" {
			return fmt.Sprintf("%s on %s → %s", op, where, t.Room)
		}
		return fmt.Sprintf("%s on %s", op, where)
	case KindSubject:
		return fmt.Sprintf("%s → %s", t.Subject, t.Room)
	case KindSchedule:
		return fmt.Sprintf("schedule %s → %s", t.Schedule, t.Room)
	}
	return t.Kind
}

// ToolName is the tool a tools entry grants: the name before the rule's
// parenthesis — "Bash" for "Bash(git *)" — or "" when the entry is not a
// name at all.
func ToolName(entry string) string {
	name, _, _ := strings.Cut(strings.TrimSpace(entry), "(")
	name = strings.TrimSpace(name)
	for _, r := range name {
		if !(r >= 'A' && r <= 'Z' || r >= 'a' && r <= 'z' || r >= '0' && r <= '9' || r == '_') {
			return ""
		}
	}
	return name
}

// ToolNames lists the tools the harness has, each once, in order.
func (d Declaration) ToolNames() []string {
	seen := map[string]bool{}
	var out []string
	for _, t := range d.Harness.Tools {
		n := ToolName(t)
		if n != "" && !seen[n] {
			seen[n] = true
			out = append(out, n)
		}
	}
	return out
}

// HarnessName is the harness as the announcement names it: the preset,
// or the template's command.
func (d Declaration) HarnessName() string {
	if d.Harness.Preset != "" {
		return d.Harness.Preset
	}
	if d.Harness.Template != nil {
		return d.Harness.Template.Command
	}
	return ""
}

// RoomStores names the stores the agent writes turns in — the rooms
// stores: a mention's or an op's store, and the room store a subject, a
// schedule or an op outside the rooms names — never the tick store. The
// default rooms store when the file names none.
func (d Declaration) RoomStores() []string {
	seen := map[string]bool{}
	var out []string
	add := func(s string) {
		if s != "" && !seen[s] {
			seen[s] = true
			out = append(out, s)
		}
	}
	for _, t := range d.Triggers {
		switch t.Kind {
		case KindMention, KindOp:
			if t.Room != "" {
				add(t.RoomStore)
			} else {
				add(t.Store)
			}
		case KindSubject, KindSchedule:
			add(t.RoomStore)
		}
	}
	if len(out) == 0 {
		out = append(out, DefaultStore)
	}
	return out
}

// The defaults a file may leave unsaid.
const (
	DefaultStore     = "rooms"
	DefaultTickStore = "ticks"
	DefaultTurns     = 3
	DefaultWindow    = Duration(time.Minute)
	DefaultDepth     = 4
	DefaultDeadline  = Duration(3 * time.Minute)
)

// Load reads and validates a definition file.
func Load(path string) (Declaration, error) {
	raw, err := os.ReadFile(path)
	if err != nil {
		return Declaration{}, fmt.Errorf("read %s: %w", path, err)
	}
	return Parse(raw)
}

// Parse validates a definition file's bytes.
func Parse(raw []byte) (Declaration, error) {
	var d Declaration
	if err := yaml.UnmarshalStrict(raw, &d); err != nil {
		return Declaration{}, fmt.Errorf("definition file: %w", err)
	}
	if err := d.validate(); err != nil {
		return Declaration{}, err
	}
	return d, nil
}

func (d *Declaration) validate() error {
	if d.Identity == "" {
		return errors.New("identity: required — the service account the agent acts as")
	}
	if err := contract.ValidatePrincipalName(d.Identity); err != nil {
		return fmt.Errorf("identity: %w", err)
	}
	for i := range d.Triggers {
		if err := d.Triggers[i].validate(i); err != nil {
			return err
		}
	}
	if d.Harness.Preset == "" && d.Harness.Template == nil {
		return errors.New("harness: a preset or a template is required")
	}
	if d.Harness.Preset != "" && d.Harness.Template != nil {
		return errors.New("harness: a preset or a template, not both")
	}
	if d.Harness.Template != nil && d.Harness.Template.Command == "" {
		return errors.New("harness.template.command: required")
	}
	if d.Harness.Template != nil {
		switch d.Harness.Template.Output {
		case "", "text", "claude-stream":
		default:
			return fmt.Errorf("harness.template.output: %q is not text or claude-stream", d.Harness.Template.Output)
		}
	}
	if d.Harness.Deadline == 0 {
		d.Harness.Deadline = DefaultDeadline
	}
	for i, tool := range d.Harness.Tools {
		if ToolName(tool) == "" {
			return fmt.Errorf("harness.tools[%d]: %q is not a tool name, or a name with a rule like Bash(git *)", i, tool)
		}
	}
	if d.Harness.Workdir != "" {
		if !filepath.IsAbs(d.Harness.Workdir) {
			return fmt.Errorf("harness.workdir: %q is not an absolute path", d.Harness.Workdir)
		}
		if st, err := os.Stat(d.Harness.Workdir); err != nil || !st.IsDir() {
			return fmt.Errorf("harness.workdir: %q is not a directory that exists", d.Harness.Workdir)
		}
	}
	for i, tk := range d.Toolkits {
		if tk.Name == "" {
			return fmt.Errorf("toolkits[%d].name: required", i)
		}
		if tk.Command == "" {
			return fmt.Errorf("toolkits[%d].command: required", i)
		}
	}
	if d.Budget.Turns == 0 {
		d.Budget.Turns = DefaultTurns
	}
	if d.Budget.Turns < 0 {
		return errors.New("budget.turns: must be positive")
	}
	if d.Budget.Window == 0 {
		d.Budget.Window = DefaultWindow
	}
	if d.Budget.Depth == 0 {
		d.Budget.Depth = DefaultDepth
	}
	if d.Budget.Depth < 0 {
		return errors.New("budget.depth: must be positive")
	}
	return nil
}

func (t *Trigger) validate(i int) error {
	switch t.Kind {
	case KindMention, KindOp:
		if t.Store == "" {
			t.Store = DefaultStore
		}
		if err := contract.ValidateStoreName(t.Store); err != nil {
			return fmt.Errorf("triggers[%d].store: %w", i, err)
		}
		if t.Under != "" {
			if err := contract.ValidatePath(t.Under); err != nil {
				return fmt.Errorf("triggers[%d].under: %w", i, err)
			}
		}
		if t.Kind == KindMention && t.Operation != "" {
			return fmt.Errorf("triggers[%d].operation: a mention trigger selects messages, not an operation", i)
		}
		if t.Kind == KindOp && !t.InRooms() && t.Room == "" {
			return fmt.Errorf("triggers[%d].room: an op trigger outside the rooms names the room its turn is written in", i)
		}
	case KindSubject:
		if t.Subject == "" {
			return fmt.Errorf("triggers[%d].subject: required — the NATS subject to listen on", i)
		}
		if strings.ContainsAny(t.Subject, " \t\n") {
			return fmt.Errorf("triggers[%d].subject: %q is not a subject", i, t.Subject)
		}
		if t.Room == "" {
			return fmt.Errorf("triggers[%d].room: a subject trigger names the room its turn is written in", i)
		}
	case KindSchedule:
		if t.Store == "" {
			t.Store = DefaultTickStore
		}
		if err := contract.ValidateStoreName(t.Store); err != nil {
			return fmt.Errorf("triggers[%d].store: %w", i, err)
		}
		if t.Schedule == "" {
			return fmt.Errorf("triggers[%d].schedule: required — the schedule's path, schedule/<name>", i)
		}
		if err := contract.ValidatePath(t.Schedule); err != nil {
			return fmt.Errorf("triggers[%d].schedule: %w", i, err)
		}
		if contract.PathType(t.Schedule) != "schedule" || strings.Count(t.Schedule, "/") != 1 {
			return fmt.Errorf("triggers[%d].schedule: %q is not schedule/<name>", i, t.Schedule)
		}
		if t.Room == "" {
			return fmt.Errorf("triggers[%d].room: a schedule trigger names the room its turn is written in", i)
		}
	case "":
		return fmt.Errorf("triggers[%d].kind: required — mention, op, subject or schedule", i)
	default:
		return fmt.Errorf("triggers[%d].kind: %q is not mention, op, subject or schedule", i, t.Kind)
	}
	if t.Room != "" {
		if err := contract.ValidatePath(t.Room); err != nil {
			return fmt.Errorf("triggers[%d].room: %w", i, err)
		}
		if contract.PathType(t.Room) != "room" || strings.Count(t.Room, "/") != 1 {
			return fmt.Errorf("triggers[%d].room: %q is not room/<id>", i, t.Room)
		}
		if t.RoomStore == "" {
			t.RoomStore = DefaultStore
		}
		if err := contract.ValidateStoreName(t.RoomStore); err != nil {
			return fmt.Errorf("triggers[%d].room_store: %w", i, err)
		}
	}
	return nil
}

// InRooms reports whether a mention or op trigger selects within the
// rooms — every room, or the instances under one — so its turn answers
// in the room the trigger is in.
func (t Trigger) InRooms() bool {
	return t.Under == "" || contract.PathType(t.Under) == "room"
}

// Stores lists the chronicle stores the declaration tails, each once: the
// stores its mention, op and schedule triggers read, and the stores the
// rooms its turns go to live in.
func (d Declaration) Stores() []string {
	seen := map[string]bool{}
	var out []string
	add := func(s string) {
		if s != "" && !seen[s] {
			seen[s] = true
			out = append(out, s)
		}
	}
	for _, t := range d.Triggers {
		if t.Kind != KindSubject {
			add(t.Store)
		}
		if t.Room != "" {
			add(t.RoomStore)
		}
	}
	return out
}
