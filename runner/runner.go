// Package runner serves one declaration under one credential: it tails the
// declaration's stores, matches every operation against the triggers,
// admits or refuses, composes the turn from the room's projection, hands
// it to the harness and writes the turn as the agent's own operation with
// the triggers as parents (chronicle-agents-hq design 02).
//
// There is no clock here and no second record: everything the runner
// holds is folded from the stores it tails and is rebuilt from them when
// the process restarts. A trigger delivered twice is answered once,
// because the turn's path is derived from the trigger and written with
// chronicle's create-if-absent guard.
package runner

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"strings"
	"time"

	"github.com/impire-io/chronicle-agents/declaration"
	"github.com/impire-io/chronicle-agents/harness"
	"github.com/impire-io/chronicle-agents/types"
	"github.com/impire-io/chronicle/client"
	"github.com/impire-io/chronicle/contract"
	"github.com/nats-io/nats.go/jetstream"
)

// Runner serves one declaration.
type Runner struct {
	c    *client.Client
	decl declaration.Declaration
	tpl  harness.Template
	log  *slog.Logger

	rooms   map[string]*room  // store + "/" + room path
	members map[string]string // principal -> kind, from the registry
}

// room is the runner's projection of one room: what the record says
// about it, folded from the tail, rebuilt from the tail.
type room struct {
	store    string
	path     string
	closed   bool
	msgs     []message
	byOpID   map[string]*message
	thinking bool
	pending  []message // triggers queued while a turn is composed
}

// message is one message as the fold sees it: the op and its data.
type message struct {
	Seq     uint64
	OpID    string
	Parents []string
	Path    string
	Author  string
	Ts      time.Time
	Data    types.MessageData
}

// New builds a runner for a declaration over a connected client whose
// author is the declaration's identity.
func New(c *client.Client, decl declaration.Declaration, log *slog.Logger) (*Runner, error) {
	if c.Author() != decl.Identity {
		return nil, fmt.Errorf("the credential acts as %q but the declaration is %q: an agent writes under its own identity", c.Author(), decl.Identity)
	}
	var tpl harness.Template
	if decl.Harness.Preset != "" {
		t, ok := harness.Preset(decl.Harness.Preset)
		if !ok {
			return nil, fmt.Errorf("harness.preset %q: not one of %s", decl.Harness.Preset, strings.Join(harness.PresetNames(), ", "))
		}
		tpl = t
		if decl.Harness.Preset == "command" {
			return nil, errors.New("harness.preset command: needs a template with the command to run; name it under harness.template")
		}
	} else {
		tpl = harness.Template{Command: decl.Harness.Template.Command, Args: decl.Harness.Template.Args, Env: decl.Harness.Template.Env}
	}
	if log == nil {
		log = slog.Default()
	}
	return &Runner{
		c:       c,
		decl:    decl,
		tpl:     tpl,
		log:     log.With("agent", decl.Identity),
		rooms:   map[string]*room{},
		members: map[string]string{},
	}, nil
}

// event is what the serving loop consumes: an operation from a store's
// tail, or a finished turn.
type event struct {
	store string
	op    contract.Op
	live  bool
	err   error

	done *turnResult
}

type turnResult struct {
	room     *room
	triggers []message
	text     string
	err      error
}

// Serve runs until the context ends. It tails every store the
// declaration names from the start — history feeds the projection and
// is never answered; what lands after the stream head seen at start is
// live and may trigger.
func (r *Runner) Serve(ctx context.Context) error {
	stores := r.decl.Stores()
	if len(stores) == 0 {
		stores = []string{declaration.DefaultStore}
	}
	js, err := jetstream.New(r.c.Conn())
	if err != nil {
		return fmt.Errorf("jetstream: %w", err)
	}
	events := make(chan event, 64)
	for _, store := range stores {
		st, err := js.Stream(ctx, contract.StreamName(store))
		if err != nil {
			return fmt.Errorf("store %s: %w", store, err)
		}
		info, err := st.Info(ctx)
		if err != nil {
			return fmt.Errorf("store %s: %w", store, err)
		}
		head := info.State.LastSeq
		r.log.Info("serving", "store", store, "head", head, "triggers", len(r.decl.Triggers))
		go func(store string, head uint64) {
			for op, err := range r.c.Tail(ctx, store, "") {
				if err != nil {
					events <- event{store: store, err: err}
					return
				}
				events <- event{store: store, op: op, live: op.Seq > head}
			}
		}(store, head)
	}
	for {
		select {
		case <-ctx.Done():
			return nil
		case ev := <-events:
			switch {
			case ev.err != nil:
				if ctx.Err() != nil {
					return nil
				}
				return fmt.Errorf("tail %s: %w", ev.store, ev.err)
			case ev.done != nil:
				r.finish(ctx, ev.done, events)
			default:
				r.observe(ctx, ev.store, ev.op, ev.live, events)
			}
		}
	}
}

// observe folds one operation into the projection and, if it is live,
// runs it through the triggers.
func (r *Runner) observe(ctx context.Context, store string, op contract.Op, live bool, events chan<- event) {
	path := contract.TailPath(contract.InstanceFromSubject(store, op.Subject))
	toks := strings.Split(path, "/")
	if len(toks) < 2 || toks[0] != types.Room {
		return // not a room, not a message: outside design 01's model
	}
	rm := r.room(store, toks[0]+"/"+toks[1])

	var msg *message
	switch {
	case len(toks) == 2:
		r.foldRoomOp(rm, op)
	case len(toks) == 4 && toks[2] == types.Messages && op.Type == "create":
		var data types.MessageData
		if err := json.Unmarshal(op.Payload, &data); err != nil {
			r.log.Warn("message payload unreadable", "path", path, "seq", op.Seq, "err", err)
			return
		}
		m := message{Seq: op.Seq, OpID: op.ID, Parents: op.Parents, Path: path, Author: op.Author, Ts: op.Ts, Data: data}
		rm.msgs = append(rm.msgs, m)
		msg = &rm.msgs[len(rm.msgs)-1]
		rm.byOpID[op.ID] = msg
	default:
		return
	}
	if !live {
		return // history: remembered, never answered
	}
	if op.Author == r.decl.Identity {
		return // self-exclusion is mechanical
	}
	if !r.matches(store, path, op, msg) {
		return
	}
	if rm.closed {
		r.log.Info("dropped: closed room", "room", rm.path, "seq", op.Seq)
		return
	}
	if msg == nil {
		// An op trigger on the room itself (a rename, a close). The turn
		// answers in the room; the trigger is the op.
		m := message{Seq: op.Seq, OpID: op.ID, Parents: op.Parents, Path: path, Author: op.Author, Ts: op.Ts,
			Data: types.MessageData{Text: fmt.Sprintf("(%s on the room)", op.Type), Kind: types.KindSystem}}
		msg = &m
	}
	if rule := r.admit(ctx, rm, *msg); rule != "" {
		r.log.Warn("refused", "room", rm.path, "seq", op.Seq, "trigger", op.ID, "author", op.Author, "rule", rule)
		return
	}
	if rm.thinking {
		rm.pending = append(rm.pending, *msg)
		r.log.Info("queued: a turn is being composed", "room", rm.path, "seq", op.Seq)
		return
	}
	r.start(ctx, rm, []message{*msg}, events)
}

func (r *Runner) room(store, path string) *room {
	key := store + "/" + path
	rm, ok := r.rooms[key]
	if !ok {
		rm = &room{store: store, path: path, byOpID: map[string]*message{}}
		r.rooms[key] = rm
	}
	return rm
}

func (r *Runner) foldRoomOp(rm *room, op contract.Op) {
	switch op.Type {
	case "close":
		rm.closed = true
	case "create", "rename", contract.OpTypeSnapshot:
		var data types.RoomData
		if json.Unmarshal(op.Payload, &data) == nil && data.Status != "" {
			rm.closed = data.Status == "closed"
		}
	}
}

// matches reports whether any trigger entry selects this operation.
func (r *Runner) matches(store, path string, op contract.Op, msg *message) bool {
	for _, t := range r.decl.Triggers {
		if t.Store != store {
			continue
		}
		if t.Under != "" && path != t.Under && !strings.HasPrefix(path, t.Under+"/") {
			continue
		}
		switch t.Kind {
		case declaration.KindMention:
			if msg == nil {
				continue
			}
			for _, name := range msg.Data.Mentions {
				if name == r.decl.Identity {
					return true
				}
			}
		case declaration.KindOp:
			if t.Operation == "" || t.Operation == op.Type {
				return true
			}
		}
	}
	return false
}

// admit applies the budget from the record alone and names the rule
// that refuses, or "" to admit.
func (r *Runner) admit(ctx context.Context, rm *room, trigger message) string {
	// Agent to agent: the author is a service account in the registry.
	if !r.decl.Budget.ReplyToAgents && r.kindOf(ctx, trigger.Author) == "service" {
		return "agent-to-agent: the author is a service account and reply_to_agents is off"
	}
	// The window floor: the agent's own turns in this room within the window.
	since := time.Now().Add(-r.decl.Budget.Window.Std())
	own := 0
	for _, m := range rm.msgs {
		if m.Author == r.decl.Identity && m.Data.Kind == types.KindTurn && m.Ts.After(since) {
			own++
		}
	}
	if own >= r.decl.Budget.Turns {
		return fmt.Sprintf("window floor: %d turns in the last %s", own, r.decl.Budget.Window.Std())
	}
	// The depth bound: the chain of turns this trigger extends.
	depth := 0
	cur := &trigger
	for cur != nil && cur.Data.Kind == types.KindTurn && len(cur.Parents) > 0 {
		depth++
		cur = rm.byOpID[cur.Parents[0]]
	}
	if depth >= r.decl.Budget.Depth {
		return fmt.Sprintf("depth bound: the trigger chain is %d turns deep", depth)
	}
	return ""
}

func (r *Runner) kindOf(ctx context.Context, principal string) string {
	if kind, ok := r.members[principal]; ok {
		return kind
	}
	for m, err := range r.c.ListMembers(ctx) {
		if err != nil {
			r.log.Warn("registry unreadable", "err", err)
			break
		}
		r.members[m.Name] = m.Kind
	}
	if kind, ok := r.members[principal]; ok {
		return kind
	}
	r.members[principal] = "unknown"
	return "unknown"
}

// start composes a turn for the triggers and hands it to the harness,
// off the serving loop; the result comes back as an event.
func (r *Runner) start(ctx context.Context, rm *room, triggers []message, events chan<- event) {
	rm.thinking = true
	input := harness.Input{
		Agent:        r.decl.Identity,
		Instructions: r.decl.Instructions,
		Context:      r.compose(rm, triggers),
		Model:        r.decl.Harness.Model,
		Deadline:     r.decl.Harness.Deadline.Std(),
	}
	for _, tk := range r.decl.Toolkits {
		input.Toolkits = append(input.Toolkits, harness.Toolkit{Name: tk.Name, Command: tk.Command, Args: tk.Args, Env: tk.Env})
	}
	r.log.Info("turn", "room", rm.path, "triggers", len(triggers), "seq", triggers[len(triggers)-1].Seq)
	go func() {
		started := time.Now()
		text, err := harness.Invoke(ctx, r.tpl, input)
		if err == nil {
			r.log.Info("harness answered", "room", rm.path, "took", time.Since(started).Round(time.Millisecond))
		}
		events <- event{done: &turnResult{room: rm, triggers: triggers, text: text, err: err}}
	}()
}

// compose renders the room's projection as the harness's context: the
// transcript, oldest first, and which lines are the triggers.
func (r *Runner) compose(rm *room, triggers []message) string {
	var b strings.Builder
	fmt.Fprintf(&b, "You are %s, an agent in the room %s. The room's transcript, oldest first:\n\n", r.decl.Identity, rm.path)
	for _, m := range rm.msgs {
		kind := m.Data.Kind
		if kind == "" {
			kind = types.KindMessage
		}
		fmt.Fprintf(&b, "[%d] %s (%s): %s\n", m.Seq, m.Author, kind, m.Data.Text)
	}
	b.WriteString("\nAnswer the message")
	if len(triggers) > 1 {
		b.WriteString("s")
	}
	b.WriteString(" at sequence")
	for i, t := range triggers {
		if i > 0 {
			b.WriteString(",")
		}
		fmt.Fprintf(&b, " %d", t.Seq)
	}
	b.WriteString(". Reply with the message text only.\n")
	return b.String()
}

// finish writes the turn, or logs why it could not, then composes the
// next turn from whatever queued meanwhile.
func (r *Runner) finish(ctx context.Context, res *turnResult, events chan<- event) {
	rm := res.room
	rm.thinking = false
	if res.err != nil {
		r.log.Error("harness failed: no turn", "room", rm.path, "err", res.err)
	} else {
		r.post(ctx, rm, res.triggers, res.text)
	}
	if len(rm.pending) > 0 && !rm.closed {
		next := rm.pending
		rm.pending = nil
		r.start(ctx, rm, next, events)
	}
}

// post writes the turn: its path derived from the triggers, the
// create-if-absent guard the client's default, the triggers as parents.
func (r *Runner) post(ctx context.Context, rm *room, triggers []message, text string) {
	ids := make([]string, len(triggers))
	mentions := make([]string, 0, len(triggers))
	seen := map[string]bool{}
	for i, t := range triggers {
		ids[i] = t.OpID
		if !seen[t.Author] {
			seen[t.Author] = true
			mentions = append(mentions, t.Author)
		}
	}
	id := TurnID(r.decl.Identity, ids)
	path := rm.path + "/messages/m-" + id
	payload, _ := json.Marshal(types.MessageData{
		Text:     text,
		Kind:     types.KindTurn,
		Mentions: mentions,
		ReplyTo:  triggers[len(triggers)-1].Path,
	})
	ack, err := r.c.Create(ctx, rm.store, path, "create", payload, client.WithOpID(id), client.WithParents(ids...))
	switch {
	case errors.Is(err, client.ErrInstanceExists):
		r.log.Info("already answered: the turn exists", "room", rm.path, "turn", path)
	case err != nil:
		r.log.Error("turn not written", "room", rm.path, "turn", path, "err", err)
	default:
		r.log.Info("turn written", "room", rm.path, "turn", path, "seq", ack.Seq)
	}
}
