// Package types holds the chronicle types a workspace declares — room,
// message and agent, and the solution it announces itself with — exactly
// as chronicle-agents-hq design 01 states them, and declares them into a
// store through the chronicle client.
package types

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

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
	// KindQuestion is a question an agent asks the people it answers —
	// its own, or a permission its harness needs — carrying the choices it
	// offers; the answer is a person's message whose reply_to names it.
	KindQuestion = "question"
)

// Room kinds.
const (
	// RoomKindRoom is a room opened for a purpose: the default.
	RoomKindRoom = "room"
	// RoomKindDirect is a direct conversation between two parties — a
	// person and an agent, or two people — the place a person starts.
	RoomKindDirect = "direct"
)

// RoomDefinition is the room type: a title, a purpose, a status, a kind
// (a room, or a direct conversation with its two participants), where it
// was broken out from; create, rename and close, all merge; compactable;
// messages as children.
func RoomDefinition() client.TypeDefinition {
	return client.TypeDefinition{
		Schema: json.RawMessage(`{
  "type": "object",
  "properties": {
    "title":        {"type": "string"},
    "purpose":      {"type": "string"},
    "status":       {"type": "string", "enum": ["open", "closed"]},
    "kind":         {"type": "string", "enum": ["room", "direct"]},
    "participants": {"type": "array", "items": {"type": "string"}},
    "origin":       {"type": "string"}
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

// MessageDefinition is the message type: text, kind, mentions, reply_to,
// and for a turn the steps the harness took and what the turn cost, for
// a question the choices it offers; one operation, create; full history —
// a message is never compacted away.
func MessageDefinition() client.TypeDefinition {
	return client.TypeDefinition{
		Schema: json.RawMessage(`{
  "type": "object",
  "properties": {
    "text":     {"type": "string"},
    "kind":     {"type": "string", "enum": ["message", "turn", "system", "question"]},
    "mentions": {"type": "array", "items": {"type": "string"}},
    "reply_to": {"type": "string"},
    "choices":  {"type": "array", "items": {"type": "string"}},
    "steps":    {"type": "array", "items": {"type": "object"}},
    "usage":    {"type": "object"}
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
	// Choices are what a question offers, in order; a free answer is
	// always allowed as well.
	Choices []string `json:"choices,omitempty"`
	// Steps are what the harness did on the way to a turn: each tool it
	// called, in order, with a bounded summary of the input and the result
	// — the record of "which agent checked what".
	Steps []Step `json:"steps,omitempty"`
	// Usage is what the turn cost, as the harness reported it.
	Usage *Usage `json:"usage,omitempty"`
}

// Step is one tool call within a turn.
type Step struct {
	// Tool is the tool's name as the harness names it: Bash, Read,
	// mcp__invoices__lookup.
	Tool string `json:"tool"`
	// Input is a bounded summary of what the tool was asked: the command,
	// the path, the query — never the whole input.
	Input string `json:"input,omitempty"`
	// Output is a bounded summary of what came back.
	Output string `json:"output,omitempty"`
	// OK is false when the tool reported an error or was denied.
	OK bool `json:"ok"`
	// Took is how long the call ran, in milliseconds.
	Took int64 `json:"took_ms,omitempty"`
}

// Usage is what a turn cost.
type Usage struct {
	Model        string  `json:"model,omitempty"`
	InputTokens  int64   `json:"input_tokens,omitempty"`
	OutputTokens int64   `json:"output_tokens,omitempty"`
	CostUSD      float64 `json:"cost_usd,omitempty"`
	// Turns is how many rounds the harness took: one with no tool, one
	// more per tool call it waited on.
	Turns int `json:"turns,omitempty"`
	// Session is the harness's own session id, for its logs.
	Session string `json:"session,omitempty"`
}

// StepSummaryLimit bounds what a step keeps of an input or an output.
const StepSummaryLimit = 400

// Bound cuts a text to StepSummaryLimit runes, marking the cut.
func Bound(s string) string {
	s = strings.TrimSpace(s)
	r := []rune(s)
	if len(r) <= StepSummaryLimit {
		return s
	}
	return string(r[:StepSummaryLimit]) + "…"
}

// RoomData is a room's state.
type RoomData struct {
	Title   string `json:"title"`
	Purpose string `json:"purpose,omitempty"`
	Status  string `json:"status,omitempty"`
	// Kind is room (the default) or direct.
	Kind string `json:"kind,omitempty"`
	// Participants are a direct conversation's two parties, sorted.
	Participants []string `json:"participants,omitempty"`
	// Origin is the path of the message a room was broken out from, when
	// it was.
	Origin string `json:"origin,omitempty"`
}

// IsDirect says whether the room is a direct conversation.
func (r RoomData) IsDirect() bool { return r.Kind == RoomKindDirect }

// DirectID is the id of the direct conversation between two parties: the
// same whoever starts it, so one pair has one conversation.
func DirectID(a, b string) string {
	if b < a {
		a, b = b, a
	}
	return "direct--" + a + "--" + b
}

// DirectPath is the path of the direct conversation between two parties.
func DirectPath(a, b string) string { return Room + "/" + DirectID(a, b) }

// DirectRoom is the state a direct conversation is created with.
func DirectRoom(a, b string) RoomData {
	if b < a {
		a, b = b, a
	}
	return RoomData{Title: a + " & " + b, Purpose: "a direct conversation", Status: "open", Kind: RoomKindDirect, Participants: []string{a, b}}
}
