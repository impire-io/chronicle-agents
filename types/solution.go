package types

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"

	"github.com/impire-io/chronicle/client"
	"github.com/impire-io/chronicle/contract"
)

// Solution is the type a solution announces itself with, in a store it
// owns — the convention chronicle-hq decision 0051 fixes so chronicle's
// console can list every solution the same way.
const Solution = "solution"

// SolutionName is this solution's name, and its announcement's id.
const SolutionName = "chronicle-agents"

// SolutionPath is where the announcement lives.
const SolutionPath = Solution + "/" + SolutionName

// SolutionDefinition is the solution type: name, description, url,
// version, home; create and locate, both merge; compactable.
func SolutionDefinition() client.TypeDefinition {
	return client.TypeDefinition{
		Schema: json.RawMessage(`{
  "type": "object",
  "properties": {
    "name":        {"type": "string"},
    "description": {"type": "string"},
    "url":         {"type": "string"},
    "version":     {"type": "string"},
    "home":        {"type": "string"}
  },
  "required": ["name"]
}`),
		History:  contract.HistoryCompactable,
		Children: map[string]string{},
		Operations: map[string]contract.OpDef{
			"create": {Schema: json.RawMessage(`{"type":"object","required":["name"]}`), Effect: "merge"},
			"locate": {Schema: json.RawMessage(`{"type":"object","required":["url"]}`), Effect: "merge"},
		},
	}
}

// SolutionData is the announcement's state.
type SolutionData struct {
	Name        string `json:"name"`
	Description string `json:"description,omitempty"`
	URL         string `json:"url,omitempty"`
	Version     string `json:"version,omitempty"`
	Home        string `json:"home,omitempty"`
}

// Announce declares the solution type in the store and writes this
// solution's announcement; announcing again is no error — the record
// already says so.
func Announce(ctx context.Context, c *client.Client, store, version string) error {
	if _, err := c.DefineType(ctx, store, Solution, SolutionDefinition()); err != nil {
		return fmt.Errorf("define %s: %w", Solution, err)
	}
	payload, _ := json.Marshal(SolutionData{
		Name:        SolutionName,
		Description: "humans and agents collaborating in rooms kept on chronicle",
		Version:     version,
		Home:        "https://github.com/impire-io/chronicle-agents",
	})
	_, err := c.Create(ctx, store, SolutionPath, "create", payload)
	if err != nil && !errors.Is(err, client.ErrInstanceExists) {
		return fmt.Errorf("announce %s: %w", SolutionName, err)
	}
	return nil
}

// Locate records where the interface runs, so the console links to it.
func Locate(ctx context.Context, c *client.Client, store, url string) error {
	payload, _ := json.Marshal(map[string]string{"url": url})
	if _, err := c.Apply(ctx, store, SolutionPath, "locate", payload); err != nil {
		return fmt.Errorf("locate %s: %w", SolutionName, err)
	}
	return nil
}
