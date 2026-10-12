package types

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"slices"

	"github.com/impire-io/chronicle/client"
	"github.com/impire-io/chronicle/contract"
)

// Agent is the type the runner announces its agent with (chronicle-agents-hq
// decision 0009, design 01 § the workspace): what an agent is — not
// whether it runs now, which is a live signal, never the record's.
const Agent = "agent"

// AgentPath is where an agent's announcement lives.
func AgentPath(name string) string { return Agent + "/" + name }

// AgentDefinition is the agent type: name, description, harness, the
// triggers as one line each, the runner's version; create and update,
// both merge; compactable; no children.
func AgentDefinition() client.TypeDefinition {
	return client.TypeDefinition{
		Schema: json.RawMessage(`{
  "type": "object",
  "properties": {
    "name":        {"type": "string"},
    "description": {"type": "string"},
    "harness":     {"type": "string"},
    "triggers":    {"type": "array", "items": {"type": "string"}},
    "tools":       {"type": "array", "items": {"type": "string"}},
    "version":     {"type": "string"}
  },
  "required": ["name"]
}`),
		History:  contract.HistoryCompactable,
		Children: map[string]string{},
		Operations: map[string]contract.OpDef{
			"create": {Schema: json.RawMessage(`{"type":"object","required":["name"]}`), Effect: "merge"},
			"update": {Schema: json.RawMessage(`{"type":"object"}`), Effect: "merge"},
		},
	}
}

// AgentData is an announcement's state.
type AgentData struct {
	Name        string   `json:"name"`
	Description string   `json:"description,omitempty"`
	Harness     string   `json:"harness,omitempty"`
	Triggers    []string `json:"triggers,omitempty"`
	// Tools are what the agent can do, for the people who ask it: the
	// harness's tools the declaration grants, as the file states them,
	// and the toolkits by name.
	Tools   []string `json:"tools,omitempty"`
	Version string   `json:"version,omitempty"`
}

// Equal says whether two announcements state the same thing.
func (a AgentData) Equal(b AgentData) bool {
	return a.Name == b.Name && a.Description == b.Description && a.Harness == b.Harness &&
		a.Version == b.Version && slices.Equal(a.Triggers, b.Triggers) && slices.Equal(a.Tools, b.Tools)
}

// AnnounceAgent writes what the runner serves: create-if-absent, then
// update when anything differs from what the record says. Announcing the
// same thing again writes nothing.
func AnnounceAgent(ctx context.Context, c *client.Client, store string, data AgentData) error {
	path := AgentPath(data.Name)
	payload, err := json.Marshal(data)
	if err != nil {
		return err
	}
	_, err = c.Create(ctx, store, path, "create", payload)
	if err == nil {
		return nil
	}
	if !errors.Is(err, client.ErrInstanceExists) {
		return fmt.Errorf("announce %s: %w", path, err)
	}
	current, ok, err := ReadAgent(ctx, c, store, data.Name)
	if err != nil {
		return err
	}
	if ok && current.Equal(data) {
		return nil
	}
	if _, err := c.Apply(ctx, store, path, "update", payload); err != nil {
		return fmt.Errorf("update %s: %w", path, err)
	}
	return nil
}

// ReadAgent reads an agent's announcement from the store's state; ok is
// false when none is there.
func ReadAgent(ctx context.Context, c *client.Client, store, name string) (AgentData, bool, error) {
	var zero AgentData
	path := AgentPath(name)
	for info, err := range c.ListInstances(ctx, store, client.ByType(Agent)) {
		if err != nil {
			return zero, false, fmt.Errorf("read %s: %w", path, err)
		}
		if info.Path != path {
			continue
		}
		var data AgentData
		if err := json.Unmarshal(info.State, &data); err != nil {
			return zero, false, fmt.Errorf("read %s: %w", path, err)
		}
		return data, true, nil
	}
	return zero, false, nil
}

// ListAgents reads every agent announced in the store.
func ListAgents(ctx context.Context, c *client.Client, store string) ([]AgentData, error) {
	var out []AgentData
	for info, err := range c.ListInstances(ctx, store, client.ByType(Agent)) {
		if err != nil {
			return nil, fmt.Errorf("list agents: %w", err)
		}
		var data AgentData
		if err := json.Unmarshal(info.State, &data); err != nil {
			return nil, fmt.Errorf("list agents: %s: %w", info.Path, err)
		}
		out = append(out, data)
	}
	return out, nil
}

// The room every workspace starts with (decision 0009 point 1).
const (
	GeneralID      = "general"
	GeneralPath    = Room + "/" + GeneralID
	GeneralTitle   = "General"
	GeneralPurpose = "where a workspace starts"
)

// EnsureGeneral creates the General room, create-if-absent; a workspace
// that has it already is left as it is.
func EnsureGeneral(ctx context.Context, c *client.Client, store string) (created bool, err error) {
	payload, _ := json.Marshal(RoomData{Title: GeneralTitle, Purpose: GeneralPurpose, Status: "open"})
	_, err = c.Create(ctx, store, GeneralPath, "create", payload)
	switch {
	case err == nil:
		return true, nil
	case errors.Is(err, client.ErrInstanceExists):
		return false, nil
	default:
		return false, fmt.Errorf("create %s: %w", GeneralPath, err)
	}
}
