// Package runner serves one declaration under one credential: it tails the
// declaration's stores and subscribes to its subjects, matches every
// operation and message against the triggers, admits or refuses, composes
// the turn from the room's projection, hands it to the harness and writes
// the turn as the agent's own operation with the triggers as parents
// (chronicle-agents-hq design 02, decisions 0003 and 0004).
//
// There is no clock here and no second record: everything the runner
// holds is folded from the stores it tails and is rebuilt from them when
// the process restarts. A trigger delivered twice is answered once,
// because the turn's path is derived from the trigger and written with
// chronicle's create-if-absent guard.
package runner

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"strings"
	"time"

	"github.com/impire-io/chronicle-agents/declaration"
	"github.com/impire-io/chronicle-agents/harness"
	"github.com/impire-io/chronicle-agents/live"
	"github.com/impire-io/chronicle-agents/types"
	"github.com/impire-io/chronicle/client"
	"github.com/impire-io/chronicle/contract"
	"github.com/nats-io/nats.go"
	"github.com/nats-io/nats.go/jetstream"
)

// Runner serves one declaration.
type Runner struct {
	c       *client.Client
	decl    declaration.Declaration
	tpl     harness.Template
	log     *slog.Logger
	version string
	since   time.Time

	rooms   map[string]*room  // store + "/" + room path
	members map[string]string // principal -> kind, from the registry
}

// Option configures a runner.
type Option func(*Runner)

// Version names the runner's version, announced beside the agent and
// said in its presence (decision 0009); a source build says 0.0.0-dev.
func Version(v string) Option {
	return func(r *Runner) { r.version = v }
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

// message is one message as the fold sees it — or one trigger that is not
// a message: a tick, a message on a subject, an op on the room.
type message struct {
	Seq     uint64
	OpID    string // empty for a tick or a subject message
	Digest  string // a subject message's identity; empty otherwise
	Parents []string
	Path    string
	Author  string
	Ts      time.Time
	Data    types.MessageData
}

// identity is what the turn is derived from: the op id, else the digest,
// else the sequence (decision 0004: a tick's identity is its sequence).
func (m message) identity() string {
	switch {
	case m.OpID != "":
		return m.OpID
	case m.Digest != "":
		return "digest:" + m.Digest
	default:
		return fmt.Sprintf("seq:%d", m.Seq)
	}
}

// New builds a runner for a declaration over a connected client whose
// author is the declaration's identity.
func New(c *client.Client, decl declaration.Declaration, log *slog.Logger, opts ...Option) (*Runner, error) {
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
	r := &Runner{
		c:       c,
		decl:    decl,
		tpl:     tpl,
		log:     log.With("agent", decl.Identity),
		version: "0.0.0-dev",
		rooms:   map[string]*room{},
		members: map[string]string{},
	}
	for _, o := range opts {
		o(r)
	}
	return r, nil
}

// announcement is what the runner says it serves (design 01 § the
// workspace): the declaration's facets as the record keeps them.
func (r *Runner) announcement() types.AgentData {
	triggers := make([]string, 0, len(r.decl.Triggers))
	for _, t := range r.decl.Triggers {
		triggers = append(triggers, t.Describe())
	}
	return types.AgentData{
		Name:        r.decl.Identity,
		Description: r.decl.Description,
		Harness:     r.decl.HarnessName(),
		Triggers:    triggers,
		Version:     r.version,
	}
}

// announce writes the announcement in every rooms store the agent
// serves; a store that is not there yet is a warning, not a stop — the
// record is the workspace's to make.
func (r *Runner) announce(ctx context.Context) {
	data := r.announcement()
	for _, store := range r.decl.RoomStores() {
		if err := types.AnnounceAgent(ctx, r.c, store, data); err != nil {
			r.log.Warn("not announced", "store", store, "err", err)
			continue
		}
		r.log.Info("announced", "store", store, "path", types.AgentPath(data.Name))
	}
}

// presence says the agent is online, now and every PresenceEvery, in
// every rooms store it serves, until the context ends.
func (r *Runner) presence(ctx context.Context) {
	say := func() {
		p := live.Presence{Agent: r.decl.Identity, State: live.Online, Since: r.since, Version: r.version}
		for _, store := range r.decl.RoomStores() {
			if err := live.Publish(r.c.Conn(), live.AgentSubject(store, r.decl.Identity), p); err != nil {
				r.log.Debug("presence not said", "store", store, "err", err)
			}
		}
	}
	say()
	tick := time.NewTicker(live.PresenceEvery)
	defer tick.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-tick.C:
			say()
		}
	}
}

// signal says one moment of a turn in the room's live subject.
func (r *Runner) signal(rm *room, state live.State, trigger, reason string) {
	sig := live.Signal{Agent: r.decl.Identity, State: state, Trigger: trigger, Reason: reason, At: time.Now()}
	if err := live.Publish(r.c.Conn(), live.RoomSubject(rm.store, live.RoomID(rm.path)), sig); err != nil {
		r.log.Debug("signal not said", "room", rm.path, "state", state, "err", err)
	}
}

// event is what the serving loop consumes: an operation from a store's
// tail, a message on a subject, or a finished turn.
type event struct {
	store string
	op    contract.Op
	live  bool
	err   error

	subject *declaration.Trigger // a subject trigger's entry
	msg     *nats.Msg

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
// live and may trigger — and subscribes to every subject it names.
func (r *Runner) Serve(ctx context.Context) error {
	stores := r.decl.Stores()
	if len(stores) == 0 {
		stores = []string{declaration.DefaultStore}
	}
	js, err := jetstream.New(r.c.Conn())
	if err != nil {
		return fmt.Errorf("jetstream: %w", err)
	}
	r.since = time.Now()
	r.announce(ctx)
	go r.presence(ctx)
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
	for i := range r.decl.Triggers {
		t := &r.decl.Triggers[i]
		if t.Kind != declaration.KindSubject {
			continue
		}
		sub, err := r.c.Conn().Subscribe(t.Subject, func(m *nats.Msg) {
			events <- event{subject: t, msg: m}
		})
		if err != nil {
			return fmt.Errorf("subject %s: %w", t.Subject, err)
		}
		defer func() { _ = sub.Unsubscribe() }()
		r.log.Info("listening", "subject", t.Subject, "room", t.Room)
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
			case ev.msg != nil:
				r.observeSubject(ctx, ev.subject, ev.msg, events)
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
	if len(toks) < 2 {
		return
	}
	inRooms := toks[0] == types.Room
	var rm *room
	var msg *message
	if inRooms {
		rm = r.room(store, toks[0]+"/"+toks[1])
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
	}
	if !live {
		return // history: remembered, never answered
	}
	if op.Author == r.decl.Identity {
		return // self-exclusion is mechanical
	}
	entry := r.match(store, path, op, msg)
	if entry == nil {
		return
	}
	// The trigger as the turn sees it: the message itself, or the op.
	trigger := message{Seq: op.Seq, OpID: op.ID, Parents: op.Parents, Path: path, Author: op.Author, Ts: op.Ts,
		Data: types.MessageData{Text: fmt.Sprintf("(%s on %s)", op.Type, path), Kind: types.KindSystem}}
	if msg != nil {
		trigger = *msg
	}
	if entry.Kind == declaration.KindSchedule {
		trigger.Data.Text = fmt.Sprintf("(tick on %s)", path)
	}
	// Where the turn goes: the trigger's room, or the room the entry names.
	if entry.Room != "" {
		rm = r.room(entry.RoomStore, entry.Room)
	}
	if rm == nil {
		return
	}
	r.react(ctx, rm, trigger, events)
}

// observeSubject turns a message on a subject into a trigger in the room
// the entry names; its identity is a digest of subject and payload.
func (r *Runner) observeSubject(ctx context.Context, entry *declaration.Trigger, m *nats.Msg, events chan<- event) {
	sum := sha256.Sum256(append([]byte(m.Subject+"\n"), m.Data...))
	payload := string(m.Data)
	if len(payload) > 2000 {
		payload = payload[:2000] + "…"
	}
	trigger := message{
		Digest: hex.EncodeToString(sum[:]),
		Path:   m.Subject,
		Ts:     time.Now(),
		Data:   types.MessageData{Text: fmt.Sprintf("(a message on %s) %s", m.Subject, payload), Kind: types.KindSystem},
	}
	r.log.Info("subject message", "subject", m.Subject, "room", entry.Room, "bytes", len(m.Data))
	r.react(ctx, r.room(entry.RoomStore, entry.Room), trigger, events)
}

// react runs a matched trigger through the room: drop in a closed room,
// admission, coalescing, the turn.
func (r *Runner) react(ctx context.Context, rm *room, trigger message, events chan<- event) {
	if rm.closed {
		r.log.Info("dropped: closed room", "room", rm.path, "trigger", trigger.identity())
		return
	}
	if rule, reason := r.admit(ctx, rm, trigger); rule != "" {
		r.log.Warn("refused", "room", rm.path, "trigger", trigger.identity(), "author", trigger.Author, "rule", rule)
		r.signal(rm, live.Refused, trigger.Path, reason)
		return
	}
	if rm.thinking {
		rm.pending = append(rm.pending, trigger)
		r.log.Info("queued: a turn is being composed", "room", rm.path, "trigger", trigger.identity())
		return
	}
	r.start(ctx, rm, []message{trigger}, events)
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

// match returns the first trigger entry that selects this operation.
func (r *Runner) match(store, path string, op contract.Op, msg *message) *declaration.Trigger {
	for i := range r.decl.Triggers {
		t := &r.decl.Triggers[i]
		switch t.Kind {
		case declaration.KindMention:
			if t.Store != store || msg == nil {
				continue
			}
			if t.Under != "" && path != t.Under && !strings.HasPrefix(path, t.Under+"/") {
				continue
			}
			for _, name := range msg.Data.Mentions {
				if name == r.decl.Identity {
					return t
				}
			}
		case declaration.KindOp:
			if t.Store != store {
				continue
			}
			if t.Under != "" && path != t.Under && !strings.HasPrefix(path, t.Under+"/") {
				continue
			}
			if t.Operation == "" || t.Operation == op.Type {
				return t
			}
		case declaration.KindSchedule:
			// The server's ticks land under schedule/<name>/ticks/…; the
			// registration itself (an op on schedule/<name>) is not a tick.
			if t.Store == store && strings.HasPrefix(path, t.Schedule+"/ticks/") {
				return t
			}
		}
	}
	return nil
}

// admit applies the budget from the record alone and names the rule
// that refuses — for the log, and in the person's words for the live
// signal (decision 0009) — or "" to admit.
func (r *Runner) admit(ctx context.Context, rm *room, trigger message) (rule, reason string) {
	// Agent to agent: the author is a service account in the registry.
	if trigger.Author != "" && !r.decl.Budget.ReplyToAgents && r.kindOf(ctx, trigger.Author) == "service" {
		return "agent-to-agent: the author is a service account and reply_to_agents is off", "a mention from another agent"
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
		return fmt.Sprintf("window floor: %d turns in the last %s", own, r.decl.Budget.Window.Std()),
			fmt.Sprintf("%d turns in %s", own, spoken(r.decl.Budget.Window.Std()))
	}
	// The depth bound: the chain of turns this trigger extends.
	depth := 0
	cur := &trigger
	for cur != nil && cur.Data.Kind == types.KindTurn && len(cur.Parents) > 0 {
		depth++
		cur = rm.byOpID[cur.Parents[0]]
	}
	if depth >= r.decl.Budget.Depth {
		return fmt.Sprintf("depth bound: the trigger chain is %d turns deep", depth), fmt.Sprintf("a chain %d deep", depth)
	}
	return "", ""
}

// spoken is a duration as a person says it: a minute, 30 seconds, 2
// minutes, an hour.
func spoken(d time.Duration) string {
	switch {
	case d == time.Minute:
		return "a minute"
	case d == time.Hour:
		return "an hour"
	case d%time.Hour == 0:
		return fmt.Sprintf("%d hours", int(d/time.Hour))
	case d%time.Minute == 0:
		return fmt.Sprintf("%d minutes", int(d/time.Minute))
	case d == time.Second:
		return "a second"
	default:
		return fmt.Sprintf("%d seconds", int(d/time.Second))
	}
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
	last := triggers[len(triggers)-1]
	r.log.Info("turn", "room", rm.path, "triggers", len(triggers), "trigger", last.identity())
	r.signal(rm, live.PickedUp, last.Path, "")
	go func() {
		started := time.Now()
		thinking, stop := context.WithCancel(ctx)
		go func() {
			tick := time.NewTicker(live.ThinkingEvery)
			defer tick.Stop()
			for {
				select {
				case <-thinking.Done():
					return
				case <-tick.C:
					r.signal(rm, live.Thinking, last.Path, "")
				}
			}
		}()
		text, err := harness.Invoke(ctx, r.tpl, input)
		stop()
		if err == nil {
			r.log.Info("harness answered", "room", rm.path, "took", time.Since(started).Round(time.Millisecond))
		}
		events <- event{done: &turnResult{room: rm, triggers: triggers, text: text, err: err}}
	}()
}

// compose renders the room's projection as the harness's context: the
// transcript, oldest first, and what triggered the turn.
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
	b.WriteString("\n")
	for _, t := range triggers {
		if t.OpID != "" && rm.byOpID[t.OpID] != nil {
			fmt.Fprintf(&b, "Answer the message at sequence %d.\n", t.Seq)
		} else {
			fmt.Fprintf(&b, "You were triggered by: %s\n", t.Data.Text)
		}
	}
	b.WriteString("Reply with the message text only.\n")
	return b.String()
}

// finish writes the turn, or logs why it could not, then composes the
// next turn from whatever queued meanwhile.
func (r *Runner) finish(ctx context.Context, res *turnResult, events chan<- event) {
	rm := res.room
	rm.thinking = false
	last := res.triggers[len(res.triggers)-1]
	if res.err != nil {
		r.log.Error("harness failed: no turn", "room", rm.path, "err", res.err)
		r.signal(rm, live.Failed, last.Path, failure(res.err))
	} else {
		r.post(ctx, rm, res.triggers, res.text)
		r.signal(rm, live.Done, last.Path, "")
	}
	if len(rm.pending) > 0 && !rm.closed {
		next := rm.pending
		rm.pending = nil
		r.start(ctx, rm, next, events)
	}
}

// failure is why a harness did not answer, in the person's words.
func failure(err error) string {
	switch {
	case errors.Is(err, context.DeadlineExceeded):
		return "the harness did not answer before its deadline"
	default:
		return "the harness did not answer: " + err.Error()
	}
}

// post writes the turn: its path derived from the triggers, the
// create-if-absent guard the client's default, the triggers' op ids as
// parents where they have one.
func (r *Runner) post(ctx context.Context, rm *room, triggers []message, text string) {
	ids := make([]string, len(triggers))
	var parents, mentions []string
	seen := map[string]bool{}
	replyTo := ""
	for i, t := range triggers {
		ids[i] = t.identity()
		if t.OpID != "" {
			parents = append(parents, t.OpID)
		}
		if t.Author != "" && !seen[t.Author] {
			seen[t.Author] = true
			mentions = append(mentions, t.Author)
		}
		if t.OpID != "" {
			replyTo = t.Path // the last op in the record — a message, the room itself — is what the turn answers; a tick or a subject message has no path in the record
		}
	}
	id := TurnID(r.decl.Identity, ids)
	path := rm.path + "/messages/m-" + id
	payload, _ := json.Marshal(types.MessageData{Text: text, Kind: types.KindTurn, Mentions: mentions, ReplyTo: replyTo})
	opts := []client.ApplyOpt{client.WithOpID(id)}
	if len(parents) > 0 {
		opts = append(opts, client.WithParents(parents...))
	}
	ack, err := r.c.Create(ctx, rm.store, path, "create", payload, opts...)
	switch {
	case errors.Is(err, client.ErrInstanceExists):
		r.log.Info("already answered: the turn exists", "room", rm.path, "turn", path)
	case err != nil:
		r.log.Error("turn not written", "room", rm.path, "turn", path, "err", err)
	default:
		r.log.Info("turn written", "room", rm.path, "turn", path, "seq", ack.Seq)
	}
}
