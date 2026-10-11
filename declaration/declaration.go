// Package declaration is the definition file: what an agent is, in the
// six facets chronicle-agents-hq design 02 states — an identity, its
// triggers, a harness, its instructions, its toolkits, a budget. Loading a
// file validates it and refuses, with the field named, what the runner
// cannot serve.
package declaration

import (
	"errors"
	"fmt"
	"os"
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
	// Store is the chronicle store the trigger reads (mention, op); the
	// runner's default store when empty.
	Store string `json:"store,omitempty"`
	// Under narrows a mention or op trigger to the instances under a path
	// (a room, a type); every room in the store when empty.
	Under string `json:"under,omitempty"`
	// Operation narrows an op trigger to one operation name.
	Operation string `json:"operation,omitempty"`
	// Subject is the NATS subject of a subject trigger.
	Subject string `json:"subject,omitempty"`
	// Schedule is the schedule's path in a tick store.
	Schedule string `json:"schedule,omitempty"`
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
}

// Template is an invocation template as the definition file writes it.
// The placeholders are the harness package's.
type Template struct {
	Command string            `json:"command"`
	Args    []string          `json:"args"`
	Env     map[string]string `json:"env,omitempty"`
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

// The defaults a file may leave unsaid.
const (
	DefaultStore    = "rooms"
	DefaultTurns    = 3
	DefaultWindow   = Duration(time.Minute)
	DefaultDepth    = 4
	DefaultDeadline = Duration(3 * time.Minute)
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

// ErrRefusedAtLoad marks a trigger kind the runner cannot serve yet.
var ErrRefusedAtLoad = errors.New("refused at load")

func (d *Declaration) validate() error {
	if d.Identity == "" {
		return errors.New("identity: required — the service account the agent acts as")
	}
	if err := contract.ValidatePrincipalName(d.Identity); err != nil {
		return fmt.Errorf("identity: %w", err)
	}
	for i := range d.Triggers {
		t := &d.Triggers[i]
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
		case KindSubject:
			return fmt.Errorf("triggers[%d]: subject triggers %w — they land in a following spec", i, ErrRefusedAtLoad)
		case KindSchedule:
			return fmt.Errorf("triggers[%d]: schedule triggers %w — a tick store needs a store that allows schedules, which chronicle does not offer yet (chronicle-61)", i, ErrRefusedAtLoad)
		case "":
			return fmt.Errorf("triggers[%d].kind: required — mention, op, subject or schedule", i)
		default:
			return fmt.Errorf("triggers[%d].kind: %q is not mention, op, subject or schedule", i, t.Kind)
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
	if d.Harness.Deadline == 0 {
		d.Harness.Deadline = DefaultDeadline
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

// Stores lists the stores the declaration's triggers read, each once.
func (d Declaration) Stores() []string {
	seen := map[string]bool{}
	var out []string
	for _, t := range d.Triggers {
		if t.Store != "" && !seen[t.Store] {
			seen[t.Store] = true
			out = append(out, t.Store)
		}
	}
	return out
}
