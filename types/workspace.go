package types

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/impire-io/chronicle/client"
	"github.com/impire-io/chronicle/contract"
)

// The workspace, declared once (chronicle-agents-hq decision 0008): the
// stores in order, each store's types in order — message before room,
// whose children name it; solution; agent (decision 0009) — and the
// announcement. `store init` applies
// this value; the hosted interface publishes it as chronicle-hq design
// 14's install manifest at /install.json, so chronicle's console enables
// a workspace from the same declaration and the two cannot drift.

// Workspace is the declaration.
type Workspace struct {
	Stores   []StoreDecl
	Announce AnnounceDecl
}

// StoreDecl is one store: its name, its description, whether it allows
// schedules, and its types in the order they are defined.
type StoreDecl struct {
	Name        string
	Description string
	Schedules   bool
	Types       []TypeDecl
}

// TypeDecl is one type by name, with its definition.
type TypeDecl struct {
	Name       string
	Definition client.TypeDefinition
}

// AnnounceDecl says where the announcement is created.
type AnnounceDecl struct {
	Store string `json:"store"`
	Path  string `json:"path"`
}

// The descriptions the stores carry.
const (
	RoomsDescription = "rooms where humans and agents collaborate (chronicle-agents)"
	TicksDescription = "the tick store: schedules and their ticks (chronicle-agents)"
	// SolutionDescription is the announcement's one sentence.
	SolutionDescription = "humans and agents collaborating in rooms kept on chronicle"
	// SolutionHome is the solution's source of truth.
	SolutionHome = "https://github.com/impire-io/chronicle-agents"
)

// Declaration is the workspace for a rooms store and a tick store; an
// empty tick store name declares no tick store.
func Declaration(rooms, ticks string) Workspace {
	w := Workspace{
		Stores: []StoreDecl{{
			Name:        rooms,
			Description: RoomsDescription,
			Types: []TypeDecl{
				{Name: Message, Definition: MessageDefinition()},
				{Name: Room, Definition: RoomDefinition()},
				{Name: Solution, Definition: SolutionDefinition()},
				{Name: Agent, Definition: AgentDefinition()},
			},
		}},
		Announce: AnnounceDecl{Store: rooms, Path: SolutionPath},
	}
	if ticks != "" {
		w.Stores = append(w.Stores, StoreDecl{Name: ticks, Description: TicksDescription, Schedules: true})
	}
	return w
}

// WorkspaceTypes names the types a workspace is made of, in the order
// they are defined — the announcement's type excluded, which the
// announcement itself accounts for.
func (w Workspace) WorkspaceTypes(store string) []string {
	var names []string
	for _, s := range w.Stores {
		if s.Name != store {
			continue
		}
		for _, t := range s.Types {
			if t.Name != Solution {
				names = append(names, t.Name)
			}
		}
	}
	return names
}

// DefineTypes defines a store's types in the declared order; again is a
// revision, never an error.
func (s StoreDecl) DefineTypes(ctx context.Context, c *client.Client) error {
	for _, t := range s.Types {
		if _, err := c.DefineType(ctx, s.Name, t.Name, t.Definition); err != nil {
			return fmt.Errorf("define %s: %w", t.Name, err)
		}
	}
	return nil
}

// Manifest is the declaration as chronicle-hq design 14's install
// manifest: what enabling the solution means, as data.
type Manifest struct {
	Manifest int             `json:"manifest"`
	Solution SolutionData    `json:"solution"`
	Stores   []ManifestStore `json:"stores"`
	Announce AnnounceDecl    `json:"announce"`
}

// ManifestStore is one store to create.
type ManifestStore struct {
	Name        string         `json:"name"`
	Description string         `json:"description,omitempty"`
	Schedules   bool           `json:"schedules,omitempty"`
	Types       []ManifestType `json:"types"`
}

// ManifestType is one type to define.
type ManifestType struct {
	Name       string   `json:"name"`
	Definition WireType `json:"definition"`
}

// WireType is a type definition in the SDK contract's shape (chronicle-hq
// design 12): schema, history, children, operations. The Go client's
// TypeDefinition carries the same facets without JSON tags, so the
// manifest states them in the wire's names.
type WireType struct {
	Schema     json.RawMessage           `json:"schema"`
	History    string                    `json:"history,omitempty"`
	Children   map[string]string         `json:"children,omitempty"`
	Operations map[string]contract.OpDef `json:"operations,omitempty"`
}

// Wire is a definition in the wire's shape.
func Wire(d client.TypeDefinition) WireType {
	return WireType{Schema: d.Schema, History: d.History, Children: d.Children, Operations: d.Operations}
}

// ManifestVersion is the shape's version.
const ManifestVersion = 1

// Manifest is the declaration as a manifest, for this binary's version and
// the address the interface is reachable at.
func (w Workspace) Manifest(version, url string) Manifest {
	m := Manifest{
		Manifest: ManifestVersion,
		Solution: SolutionData{
			Name:        SolutionName,
			Description: SolutionDescription,
			URL:         url,
			Version:     version,
			Home:        SolutionHome,
		},
		Stores:   make([]ManifestStore, 0, len(w.Stores)),
		Announce: w.Announce,
	}
	for _, s := range w.Stores {
		ms := ManifestStore{Name: s.Name, Description: s.Description, Schedules: s.Schedules, Types: make([]ManifestType, 0, len(s.Types))}
		for _, t := range s.Types {
			ms.Types = append(ms.Types, ManifestType{Name: t.Name, Definition: Wire(t.Definition)})
		}
		m.Stores = append(m.Stores, ms)
	}
	return m
}
