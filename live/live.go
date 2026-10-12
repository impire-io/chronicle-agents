// Package live is the runner's voice off the record (chronicle-agents-hq
// decision 0009, design 02 § live signals): what a trigger became — picked
// up, thinking, done, refused, failed — and that an agent is here. It
// travels on the account's ephemeral namespace, CHRON.LIVE.<store>.
// (chronicle-hq decision 0055), which no stream captures and no agent
// treats as a trigger. Nothing here is stored, retried or acknowledged: a
// signal nobody heard cost nothing, and a page that missed one still
// reads the turn from the record. The rooms page mirrors these shapes.
package live

import (
	"encoding/json"
	"strings"
	"time"

	"github.com/nats-io/nats.go"
)

// Root is the ephemeral namespace's root: CHRON.LIVE, never under a
// store's own CHRON.<store>. prefix.
const Root = "CHRON.LIVE"

// RoomSubject is where a room's signals go: CHRON.LIVE.<store>.room.<id>,
// the id being the instance id after "room/".
func RoomSubject(store, roomID string) string {
	return Root + "." + store + ".room." + roomID
}

// AgentSubject is where an agent's presence goes: CHRON.LIVE.<store>.agent.<name>.
func AgentSubject(store, agent string) string {
	return Root + "." + store + ".agent." + agent
}

// RoomID is the id a room's path carries after "room/"; the path itself
// when it has no such prefix.
func RoomID(path string) string {
	return strings.TrimPrefix(path, "room/")
}

// State is a signal's moment.
type State string

// The states a signal may carry.
const (
	// PickedUp: the trigger was admitted and a turn is being composed.
	PickedUp State = "picked-up"
	// Thinking: the harness is running, said every ThinkingEvery.
	Thinking State = "thinking"
	// Done: the turn is written.
	Done State = "done"
	// Refused: a rule refused the trigger; Reason says which, in the
	// person's words.
	Refused State = "refused"
	// Failed: the harness did not answer; Reason says why.
	Failed State = "failed"
	// Online: the agent's presence, said every PresenceEvery.
	Online State = "online"
)

// The cadences, as design 02 states them.
const (
	// ThinkingEvery is how often "thinking" is repeated while the harness runs.
	ThinkingEvery = 3 * time.Second
	// PresenceEvery is how often an agent says it is online.
	PresenceEvery = 10 * time.Second
	// OfflineAfter is how long a reader waits in silence before taking an
	// agent as offline.
	OfflineAfter = 30 * time.Second
)

// Signal is one moment of a turn in a room.
type Signal struct {
	Agent   string    `json:"agent"`
	State   State     `json:"state"`
	Trigger string    `json:"trigger,omitempty"`
	Reason  string    `json:"reason,omitempty"`
	At      time.Time `json:"at"`
}

// Presence is an agent's heartbeat.
type Presence struct {
	Agent   string    `json:"agent"`
	State   State     `json:"state"`
	Since   time.Time `json:"since"`
	Version string    `json:"version,omitempty"`
}

// Publish says one thing on a subject: a plain publish, no JetStream, no
// reply, no retry.
func Publish(nc *nats.Conn, subject string, v any) error {
	body, err := json.Marshal(v)
	if err != nil {
		return err
	}
	return nc.Publish(subject, body)
}
