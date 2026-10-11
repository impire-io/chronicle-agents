// Package schedule registers a schedule in a tick store: one headered
// publish under schedule/<name>, after which the server writes a tick
// under schedule/<name>/ticks/t at every firing, whether anyone reads
// (chronicle-agents-hq decision 0004 point 4; chronicle-hq decision 0050).
package schedule

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/impire-io/chronicle/contract"
	"github.com/nats-io/nats.go"
	"github.com/nats-io/nats.go/jetstream"
)

// The server's scheduling headers (NATS 2.12 message scheduling).
const (
	HdrSchedule       = "Nats-Schedule"
	HdrScheduleTarget = "Nats-Schedule-Target"
	HdrScheduleTTL    = "Nats-Schedule-TTL"
	HdrScheduler      = "Nats-Scheduler" // provenance, on every tick the server writes
)

// OpType is the op type a registration carries, so the history reads sanely.
const OpType = "schedule"

// Path is the schedule's instance path in the tick store.
func Path(name string) string { return "schedule/" + name }

// TickPath is where the server writes the schedule's ticks: one instance,
// one op per tick.
func TickPath(name string) string { return Path(name) + "/ticks/t" }

// Every is the pattern for an interval.
func Every(d time.Duration) string { return "@every " + d.String() }

// Create registers the schedule in the store: pattern is the server's
// grammar (Every, or a cron expression), ttl > 0 makes stale ticks expire.
// The registration is one publish; a second registration under the same
// name replaces the first (the server keeps one schedule per subject). No
// message id is set — the server copies a registration's headers onto its
// ticks, and a copied id would make the stream dedup the ticks away.
func Create(ctx context.Context, nc *nats.Conn, author, store, name, pattern string, ttl time.Duration) error {
	if err := contract.ValidateStoreName(store); err != nil {
		return err
	}
	if name == "" || strings.ContainsAny(name, "./ \t") {
		return fmt.Errorf("schedule name %q: letters, digits, - and _ only", name)
	}
	if pattern == "" {
		return errors.New("a pattern is required: an interval or a cron expression")
	}
	js, err := jetstream.New(nc)
	if err != nil {
		return err
	}
	msg := nats.NewMsg(contract.OpsSubject(store, strings.ReplaceAll(Path(name), "/", ".")))
	msg.Header.Set(contract.HdrOpType, OpType)
	msg.Header.Set(contract.HdrOpAuthor, author)
	msg.Header.Set(contract.HdrOpTs, time.Now().UTC().Format(time.RFC3339Nano))
	msg.Header.Set(contract.HdrOpVersion, contract.EnvelopeVersion)
	msg.Header.Set(HdrSchedule, pattern)
	msg.Header.Set(HdrScheduleTarget, contract.OpsSubject(store, strings.ReplaceAll(TickPath(name), "/", ".")))
	if ttl > 0 {
		msg.Header.Set(HdrScheduleTTL, ttl.String())
	}
	msg.Data = []byte(fmt.Sprintf(`{"pattern":%q}`, pattern))
	if _, err := js.PublishMsg(ctx, msg); err != nil {
		return fmt.Errorf("register schedule %s in %s: %w", name, store, err)
	}
	return nil
}
