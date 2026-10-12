// Package types holds the chronicle types a workspace declares — room,
// message and agent, and the solution it announces itself with — exactly
// as chronicle-agents-hq design 01 states them, and declares them into a
// store through the chronicle client.
package types

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/impire-io/chronicle/client"
	"github.com/impire-io/chronicle/contract"
)

// The type names, and the child segment a room's messages live under.
const (
	Room     = "room"
	Message  = "message"
	Messages = "messages"
)

// Message kinds.
const (
	KindMessage = "message"
	KindTurn    = "turn"
	KindSystem  = "system"
)

// RoomDefinition is the room type: a title, a purpose, a status; create,
// rename and close, all merge; compactable; messages as children.
func RoomDefinition() client.TypeDefinition {
	return client.TypeDefinition{
		Schema: json.RawMessage(`{
  "type": "object",
  "properties": {
    "title":   {"type": "string"},
    "purpose": {"type": "string"},
    "status":  {"type": "string", "enum": ["open", "closed"]}
  },
  "required": ["title"]
}`),
		History:  contract.HistoryCompactable,
		Children: map[string]string{Messages: Message},
		Operations: map[string]contract.OpDef{
			"create": {Schema: json.RawMessage(`{"type":"object","required":["title"]}`), Effect: "merge"},
			"rename": {Schema: json.RawMessage(`{"type":"object","required":["title"]}`), Effect: "merge"},
			"close":  {Schema: json.RawMessage(`{"type":"object"}`), Effect: "merge"},
		},
	}
}

// MessageDefinition is the message type: text, kind, mentions, reply_to;
// one operation, create; full history — a message is never compacted away.
func MessageDefinition() client.TypeDefinition {
	return client.TypeDefinition{
		Schema: json.RawMessage(`{
  "type": "object",
  "properties": {
    "text":     {"type": "string"},
    "kind":     {"type": "string", "enum": ["message", "turn", "system"]},
    "mentions": {"type": "array", "items": {"type": "string"}},
    "reply_to": {"type": "string"}
  },
  "required": ["text"]
}`),
		History:  contract.HistoryFull,
		Children: map[string]string{},
		Operations: map[string]contract.OpDef{
			"create": {Schema: json.RawMessage(`{"type":"object","required":["text"]}`), Effect: "merge"},
		},
	}
}

// Declare defines the workspace's types in the store — message first, so
// the room's children map names a type that exists, then room, then
// agent — and again to revise them.
func Declare(ctx context.Context, c *client.Client, store string) error {
	for _, t := range []struct {
		name string
		def  client.TypeDefinition
	}{{Message, MessageDefinition()}, {Room, RoomDefinition()}, {Agent, AgentDefinition()}} {
		if _, err := c.DefineType(ctx, store, t.name, t.def); err != nil {
			return fmt.Errorf("define %s: %w", t.name, err)
		}
	}
	return nil
}

// MessageData is a message's state, as written and as read.
type MessageData struct {
	Text     string   `json:"text"`
	Kind     string   `json:"kind,omitempty"`
	Mentions []string `json:"mentions,omitempty"`
	ReplyTo  string   `json:"reply_to,omitempty"`
}

// RoomData is a room's state.
type RoomData struct {
	Title   string `json:"title"`
	Purpose string `json:"purpose,omitempty"`
	Status  string `json:"status,omitempty"`
}
