package mcpserver

import (
	"context"
	"encoding/json"
	"errors"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/impire-io/chronicle-agents/internal/chrontest"
	"github.com/impire-io/chronicle-agents/types"
	"github.com/impire-io/chronicle/client"
	"github.com/modelcontextprotocol/go-sdk/mcp"
)

type world struct {
	t      *testing.T
	l      *chrontest.Local
	admin  *client.Client
	wilson *client.Client
	ctx    context.Context
	cfg    Config
}

func newWorld(t *testing.T) *world {
	t.Helper()
	l := chrontest.Up(t)
	admin := l.Connect(t, "admin")
	ctx := context.Background()
	if _, err := admin.CreateStore(ctx, "rooms", "test"); err != nil {
		t.Fatal(err)
	}
	if err := types.Declare(ctx, admin, "rooms"); err != nil {
		t.Fatal(err)
	}
	if _, err := admin.AddMember(ctx, "wilson", "writer", client.AsServiceAccount()); err != nil {
		t.Fatal(err)
	}
	if _, err := admin.Create(ctx, "rooms", "room/r1", "create", []byte(`{"title":"R1","status":"open"}`)); err != nil {
		t.Fatal(err)
	}
	if _, err := admin.Create(ctx, "rooms", "room/r2", "create", []byte(`{"title":"R2","purpose":"the other one","status":"closed"}`)); err != nil {
		t.Fatal(err)
	}
	cfg := Config{Agent: "wilson", Store: "rooms", Room: "room/r1", Turn: "t1", Askers: []string{"admin"}, Deadline: time.Now().Add(10 * time.Second)}
	return &world{t: t, l: l, admin: admin, wilson: l.Connect(t, "wilson"), ctx: ctx, cfg: cfg}
}

// question waits for a question wilson wrote in r1 that is not one of
// the paths already seen, and returns its path.
func (w *world) question(seen ...string) (string, types.MessageData) {
	w.t.Helper()
	deadline := time.Now().Add(10 * time.Second)
	for time.Now().Before(deadline) {
		for info, err := range w.admin.ListInstances(w.ctx, "rooms", client.Under("room/r1")) {
			if err != nil {
				w.t.Fatal(err)
			}
			var d types.MessageData
			if json.Unmarshal(info.State, &d) == nil && d.Kind == types.KindQuestion && !slices.Contains(seen, info.Path) {
				return info.Path, d
			}
		}
		time.Sleep(50 * time.Millisecond)
	}
	w.t.Fatal("no question was written")
	return "", types.MessageData{}
}

func (w *world) answer(author, question, text string) {
	w.t.Helper()
	c := w.admin
	if author != "admin" {
		c = w.l.Connect(w.t, author)
	}
	payload, _ := json.Marshal(types.MessageData{Text: text, Kind: types.KindMessage, ReplyTo: question})
	if _, err := c.Create(w.ctx, "rooms", "room/r1/messages/a-"+randomID(), "create", payload); err != nil {
		w.t.Fatal(err)
	}
}

func TestAskWritesAQuestionAndReadsTheAnswer(t *testing.T) {
	w := newWorld(t)
	done := make(chan Answer, 1)
	errs := make(chan error, 1)
	go func() {
		a, err := Ask(w.ctx, w.wilson, w.cfg, "Which invoice?", []string{"345", "346"})
		if err != nil {
			errs <- err
			return
		}
		done <- a
	}()
	path, q := w.question()
	if q.Text != "Which invoice?" || strings.Join(q.Choices, ",") != "345,346" || strings.Join(q.Mentions, ",") != "admin" {
		t.Errorf("question = %+v", q)
	}
	// wilson's own message answering nothing, and a message that answers
	// no question, are not the answer.
	payload, _ := json.Marshal(types.MessageData{Text: "noise", Kind: types.KindMessage})
	if _, err := w.admin.Create(w.ctx, "rooms", "room/r1/messages/noise", "create", payload); err != nil {
		t.Fatal(err)
	}
	w.answer("admin", path, "345")
	select {
	case a := <-done:
		if a.Text != "345" || a.Author != "admin" || !strings.HasPrefix(a.Path, "room/r1/messages/a-") {
			t.Errorf("answer = %+v", a)
		}
	case err := <-errs:
		t.Fatal(err)
	case <-time.After(10 * time.Second):
		t.Fatal("no answer read")
	}
}

func TestAskGivesUpAtTheDeadline(t *testing.T) {
	w := newWorld(t)
	cfg := w.cfg
	cfg.Deadline = time.Now().Add(500 * time.Millisecond)
	_, err := Ask(w.ctx, w.wilson, cfg, "Anyone?", nil)
	if !errors.Is(err, ErrUnanswered) {
		t.Errorf("err = %v, want ErrUnanswered", err)
	}
}

// session connects an MCP client to the server over memory, as the
// harness would over stdio.
func (w *world) session() *mcp.ClientSession {
	w.t.Helper()
	st, ct := mcp.NewInMemoryTransports()
	srv := New(w.cfg, w.wilson)
	ctx, cancel := context.WithCancel(w.ctx)
	w.t.Cleanup(cancel)
	go func() { _ = srv.Run(ctx, st) }()
	cl := mcp.NewClient(&mcp.Implementation{Name: "test", Version: "1"}, nil)
	s, err := cl.Connect(w.ctx, ct, nil)
	if err != nil {
		w.t.Fatal(err)
	}
	w.t.Cleanup(func() { _ = s.Close() })
	return s
}

func textOf(t *testing.T, res *mcp.CallToolResult) string {
	t.Helper()
	if len(res.Content) == 0 {
		t.Fatalf("no content: %+v", res)
	}
	tc, ok := res.Content[0].(*mcp.TextContent)
	if !ok {
		t.Fatalf("content = %T", res.Content[0])
	}
	return tc.Text
}

func TestThePermissionToolAsksTheRoomAndAnswersTheHarness(t *testing.T) {
	w := newWorld(t)
	s := w.session()
	type reply struct {
		Behavior     string          `json:"behavior"`
		UpdatedInput json.RawMessage `json:"updatedInput"`
		Message      string          `json:"message"`
	}
	call := func() chan reply {
		out := make(chan reply, 1)
		go func() {
			res, err := s.CallTool(w.ctx, &mcp.CallToolParams{Name: ToolPermission, Arguments: map[string]any{
				"tool_name": "Bash", "input": map[string]any{"command": "ls -la"}, "tool_use_id": "t1"}})
			if err != nil || res.IsError {
				t.Errorf("call: %v %+v", err, res)
				out <- reply{}
				return
			}
			var r reply
			if err := json.Unmarshal([]byte(textOf(t, res)), &r); err != nil {
				t.Error(err)
			}
			out <- r
		}()
		return out
	}
	// Allowed: the question names the agent, the tool and the command; the answer carries the input back.
	out := call()
	path, q := w.question()
	if q.Text != "wilson wants to use Bash: ls -la. Allow it?" || strings.Join(q.Choices, ",") != "allow,deny" {
		t.Errorf("question = %+v", q)
	}
	w.answer("admin", path, "allow")
	r := <-out
	if r.Behavior != "allow" || !strings.Contains(string(r.UpdatedInput), `"ls -la"`) {
		t.Errorf("reply = %+v", r)
	}
	// Denied, in the person's words.
	out = call()
	path, _ = w.question(path)
	w.answer("admin", path, "not on my watch")
	r = <-out
	if r.Behavior != "deny" || r.Message != "admin said: not on my watch" {
		t.Errorf("reply = %+v", r)
	}
}

func TestReadRoomAndListRooms(t *testing.T) {
	w := newWorld(t)
	for i, text := range []string{"first", "second"} {
		payload, _ := json.Marshal(types.MessageData{Text: text, Kind: types.KindMessage})
		if _, err := w.admin.Create(w.ctx, "rooms", "room/r1/messages/m-"+string(rune('a'+i)), "create", payload); err != nil {
			t.Fatal(err)
		}
	}
	payload, _ := json.Marshal(types.MessageData{Text: "a turn", Kind: types.KindTurn})
	if _, err := w.wilson.Create(w.ctx, "rooms", "room/r1/messages/m-t", "create", payload); err != nil {
		t.Fatal(err)
	}
	s := w.session()
	res, err := s.CallTool(w.ctx, &mcp.CallToolParams{Name: ToolReadRoom, Arguments: map[string]any{}})
	if err != nil {
		t.Fatal(err)
	}
	var out ReadRoomOutput
	if err := json.Unmarshal([]byte(textOf(t, res)), &out); err != nil {
		t.Fatal(err)
	}
	if out.Room != "room/r1" || out.Messages != 3 {
		t.Errorf("read = %+v", out)
	}
	lines := strings.Split(strings.TrimSpace(out.Transcript), "\n")
	if len(lines) != 3 || !strings.HasSuffix(lines[0], "admin (message): first") || !strings.HasSuffix(lines[2], "wilson (turn): a turn") {
		t.Errorf("transcript = %q", out.Transcript)
	}
	res, err = s.CallTool(w.ctx, &mcp.CallToolParams{Name: ToolReadRoom, Arguments: map[string]any{"room": "room/r1", "limit": 1}})
	if err != nil {
		t.Fatal(err)
	}
	_ = json.Unmarshal([]byte(textOf(t, res)), &out)
	if out.Messages != 1 || !strings.Contains(out.Transcript, "a turn") {
		t.Errorf("limited read = %+v", out)
	}
	if res, err := s.CallTool(w.ctx, &mcp.CallToolParams{Name: ToolReadRoom, Arguments: map[string]any{"room": "agent/wilson"}}); err == nil && !res.IsError {
		t.Error("a path that is not a room was read")
	}
	res, err = s.CallTool(w.ctx, &mcp.CallToolParams{Name: ToolListRooms, Arguments: map[string]any{}})
	if err != nil {
		t.Fatal(err)
	}
	var rooms ListRoomsOutput
	if err := json.Unmarshal([]byte(textOf(t, res)), &rooms); err != nil {
		t.Fatal(err)
	}
	if len(rooms.Rooms) != 2 || rooms.Rooms[0].Path != "room/r1" || rooms.Rooms[0].Status != "open" ||
		rooms.Rooms[1].Title != "R2" || rooms.Rooms[1].Status != "closed" || rooms.Rooms[1].Purpose != "the other one" {
		t.Errorf("rooms = %+v", rooms.Rooms)
	}
}

func TestAllowsReadsAPersonsAnswer(t *testing.T) {
	for answer, want := range map[string]bool{
		"allow": true, "Allow": true, "yes": true, "yes, go on": true, "ok": true, "go ahead": true, "y": true,
		"deny": false, "no": false, "yesterday it broke": false, "": false, "allowance is not it": false,
	} {
		if got := Allows(answer); got != want {
			t.Errorf("Allows(%q) = %v", answer, got)
		}
	}
}

func TestConfigTravelsInTheEnvironment(t *testing.T) {
	in := Config{URL: "nats://x:4222", Nkey: "/k.nk", Agent: "wilson", Store: "rooms", Room: "room/r1", Turn: "t1", Askers: []string{"admin", "ada"}, Deadline: time.UnixMilli(1700000000123)}
	for k, v := range in.Env() {
		t.Setenv(k, v)
	}
	out, err := ConfigFromEnv()
	if err != nil {
		t.Fatal(err)
	}
	if out.URL != in.URL || out.Nkey != in.Nkey || out.Agent != in.Agent || out.Store != in.Store || out.Room != in.Room ||
		out.Turn != in.Turn || strings.Join(out.Askers, ",") != "admin,ada" || !out.Deadline.Equal(in.Deadline) {
		t.Errorf("round trip: %+v -> %+v", in, out)
	}
	t.Setenv(EnvURL, "")
	if _, err := ConfigFromEnv(); err == nil || !strings.Contains(err.Error(), EnvURL) {
		t.Errorf("err = %v", err)
	}
}

func TestSummarizeInputKeepsWhatIsAsked(t *testing.T) {
	if got := SummarizeInput(json.RawMessage(`{"command":"rm -rf /","description":"x"}`)); got != "rm -rf /" {
		t.Errorf("got %q", got)
	}
	if got := SummarizeInput(json.RawMessage(`{"z":1,"a":"b"}`)); got != `a="b" z=1` {
		t.Errorf("got %q", got)
	}
	if got := SummarizeInput(nil); got != "" {
		t.Errorf("got %q", got)
	}
}
