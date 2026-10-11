package runner

import (
	"crypto/sha1"
	"encoding/hex"
	"strings"
)

// namespace is the name-based UUID namespace of chronicle-agents turns —
// fixed, so a turn's id is the same function of the agent and the trigger
// on every host and every run.
var namespace = [16]byte{0x5c, 0x7a, 0x1e, 0x44, 0x9b, 0x2d, 0x4f, 0x1a, 0x8e, 0x3c, 0x6d, 0x0b, 0x7f, 0x21, 0xa9, 0x53}

// TurnID derives a turn's id from the agent and the triggers it answers:
// a version-5 UUID over the agent's name and the trigger op ids, in order,
// written as 32 hex digits. The same triggers to the same agent always
// name the same turn — which is what makes a second delivery a refused
// write rather than a second answer.
func TurnID(agent string, triggerIDs []string) string {
	h := sha1.New()
	h.Write(namespace[:])
	h.Write([]byte(agent))
	h.Write([]byte{0})
	h.Write([]byte(strings.Join(triggerIDs, "\n")))
	sum := h.Sum(nil)
	var u [16]byte
	copy(u[:], sum[:16])
	u[6] = (u[6] & 0x0f) | 0x50 // version 5
	u[8] = (u[8] & 0x3f) | 0x80 // RFC 4122 variant
	return hex.EncodeToString(u[:])
}

// TurnPath is where the turn lives: the room's messages, m-<id>.
func TurnPath(room, agent string, triggerIDs []string) string {
	return room + "/messages/m-" + TurnID(agent, triggerIDs)
}
