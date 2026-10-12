// Package mcpserver is the MCP server the runner hands its harness for one
// turn (chronicle-agents-hq design 02 § the harness): the tools through
// which an agent reads the rooms beyond its context, asks the people in
// its room a question, and through which a permission request the harness
// cannot decide reaches those people — as a question in the room, answered
// by a message, within the turn's deadline. It runs as a process the
// harness starts from the generated MCP configuration, speaks MCP over
// standard input and output, and acts on chronicle under the agent's own
// credential, which the runner hands it in the environment.
package mcpserver

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/impire-io/chronicle-agents/types"
	"github.com/impire-io/chronicle/client"
	"github.com/impire-io/chronicle/contract"
	"github.com/modelcontextprotocol/go-sdk/mcp"
)

// ServerName is the name the server has in the MCP configuration; the
// harness names its tools mcp__chronicle-agents__<tool>.
const ServerName = "chronicle-agents"

// The tools.
const (
	ToolAsk        = "ask"
	ToolPermission = "permission"
	ToolReadRoom   = "read_room"
	ToolListRooms  = "list_rooms"
)

// Config is what one turn's server needs, handed over in the environment.
type Config struct {
	// The connection: the URL and one credential, as the runner was given.
	URL   string
	Nkey  string
	Creds string
	// Agent is the identity the server acts as: the author of every
	// question it writes.
	Agent string
	// Store and Room are where the turn is.
	Store string
	Room  string
	// Turn is the turn's id, for the log.
	Turn string
	// Askers are the people a question is addressed to: the trigger's
	// authors, as mentions on the question.
	Askers []string
	// Deadline is when the turn ends; a question not answered by then is
	// not answered.
	Deadline time.Time
}

// The environment the configuration travels in.
const (
	EnvURL      = "CHRONICLE_AGENTS_URL"
	EnvNkey     = "CHRONICLE_AGENTS_NKEY"
	EnvCreds    = "CHRONICLE_AGENTS_CREDS"
	EnvAgent    = "CHRONICLE_AGENTS_AGENT"
	EnvStore    = "CHRONICLE_AGENTS_STORE"
	EnvRoom     = "CHRONICLE_AGENTS_ROOM"
	EnvTurn     = "CHRONICLE_AGENTS_TURN"
	EnvAskers   = "CHRONICLE_AGENTS_ASKERS"
	EnvDeadline = "CHRONICLE_AGENTS_DEADLINE"
)

// Env is the configuration as the environment the harness starts the
// server with.
func (c Config) Env() map[string]string {
	env := map[string]string{
		EnvURL:   c.URL,
		EnvAgent: c.Agent,
		EnvStore: c.Store,
		EnvRoom:  c.Room,
		EnvTurn:  c.Turn,
	}
	if c.Nkey != "" {
		env[EnvNkey] = c.Nkey
	}
	if c.Creds != "" {
		env[EnvCreds] = c.Creds
	}
	if len(c.Askers) > 0 {
		env[EnvAskers] = strings.Join(c.Askers, ",")
	}
	if !c.Deadline.IsZero() {
		env[EnvDeadline] = strconv.FormatInt(c.Deadline.UnixMilli(), 10)
	}
	return env
}

// ConfigFromEnv reads the configuration the runner handed over.
func ConfigFromEnv() (Config, error) {
	c := Config{
		URL:   os.Getenv(EnvURL),
		Nkey:  os.Getenv(EnvNkey),
		Creds: os.Getenv(EnvCreds),
		Agent: os.Getenv(EnvAgent),
		Store: os.Getenv(EnvStore),
		Room:  os.Getenv(EnvRoom),
		Turn:  os.Getenv(EnvTurn),
	}
	if a := os.Getenv(EnvAskers); a != "" {
		c.Askers = strings.Split(a, ",")
	}
	if d := os.Getenv(EnvDeadline); d != "" {
		ms, err := strconv.ParseInt(d, 10, 64)
		if err != nil {
			return c, fmt.Errorf("%s: %w", EnvDeadline, err)
		}
		c.Deadline = time.UnixMilli(ms)
	}
	switch {
	case c.URL == "":
		return c, errors.New(EnvURL + " is not set: this server is started by the runner for one turn")
	case c.Nkey == "" && c.Creds == "":
		return c, errors.New(EnvNkey + " or " + EnvCreds + " is not set")
	case c.Agent == "", c.Store == "", c.Room == "":
		return c, errors.New(EnvAgent + ", " + EnvStore + " and " + EnvRoom + " are required")
	}
	return c, nil
}

// Connect opens the agent's own connection.
func (c Config) Connect() (*client.Client, error) {
	if c.Creds != "" {
		return client.ConnectFile(c.URL, c.Creds)
	}
	return client.ConnectNkeyFile(c.URL, c.Nkey, c.Agent)
}

// Serve runs the server over standard input and output until the harness
// closes them or the context ends.
func Serve(ctx context.Context, cfg Config) error {
	c, err := cfg.Connect()
	if err != nil {
		return fmt.Errorf("connect: %w", err)
	}
	defer c.Close()
	return New(cfg, c).Run(ctx, &mcp.StdioTransport{})
}

// Server is the tools over one connection.
type Server struct {
	cfg Config
	c   *client.Client
}

// New builds the server's tools over a connection, as the MCP server to
// run over a transport.
func New(cfg Config, c *client.Client) *mcp.Server {
	s := &Server{cfg: cfg, c: c}
	srv := mcp.NewServer(&mcp.Implementation{Name: ServerName, Version: "1"}, nil)
	mcp.AddTool(srv, &mcp.Tool{
		Name: ToolAsk,
		Description: "Ask the people you are answering a question and wait for their answer. " +
			"Offer choices when the answer is one of a few; they may always answer in their own words. " +
			"The question and the answer are messages in the room. Returns the answer, or says that nobody answered in time.",
	}, s.ask)
	mcp.AddTool(srv, &mcp.Tool{
		Name: ToolPermission,
		Description: "Asks the people in the room whether a tool call may run. Called by the harness for a permission it cannot decide; " +
			"not for the agent to call.",
	}, s.permission)
	mcp.AddTool(srv, &mcp.Tool{
		Name: ToolReadRoom,
		Description: "Read a room's transcript, oldest first: every message with its sequence, author and kind. " +
			"Your own room when no room is named; use list_rooms for the others.",
	}, s.readRoom)
	mcp.AddTool(srv, &mcp.Tool{
		Name:        ToolListRooms,
		Description: "List the workspace's rooms: path, title, status and purpose.",
	}, s.listRooms)
	return srv
}

// AskInput is what the agent asks.
type AskInput struct {
	Question string   `json:"question" jsonschema:"the question, in the words the people in the room read"`
	Choices  []string `json:"choices,omitempty" jsonschema:"the answers offered, in order, when the answer is one of a few"`
}

// AskOutput is the answer.
type AskOutput struct {
	Answer   string `json:"answer"`
	By       string `json:"by,omitempty"`
	Answered bool   `json:"answered"`
}

func (s *Server) ask(ctx context.Context, _ *mcp.CallToolRequest, in AskInput) (*mcp.CallToolResult, AskOutput, error) {
	if strings.TrimSpace(in.Question) == "" {
		return nil, AskOutput{}, errors.New("question: required")
	}
	ans, err := Ask(ctx, s.c, s.cfg, in.Question, in.Choices)
	if errors.Is(err, ErrUnanswered) {
		return nil, AskOutput{Answer: "nobody answered before the turn's deadline", Answered: false}, nil
	}
	if err != nil {
		return nil, AskOutput{}, err
	}
	return nil, AskOutput{Answer: ans.Text, By: ans.Author, Answered: true}, nil
}

// PermissionInput is what Claude Code sends its permission-prompt tool.
type PermissionInput struct {
	ToolName  string         `json:"tool_name" jsonschema:"the tool that needs permission"`
	Input     map[string]any `json:"input,omitempty" jsonschema:"the tool's input"`
	ToolUseID string         `json:"tool_use_id,omitempty" jsonschema:"the call's id"`
}

// permissionAnswer is what the tool answers, JSON-encoded in its text.
type permissionAnswer struct {
	Behavior     string          `json:"behavior"`
	UpdatedInput json.RawMessage `json:"updatedInput,omitempty"`
	Message      string          `json:"message,omitempty"`
}

func (s *Server) permission(ctx context.Context, _ *mcp.CallToolRequest, in PermissionInput) (*mcp.CallToolResult, any, error) {
	question := fmt.Sprintf("%s wants to use %s", s.cfg.Agent, in.ToolName)
	raw, _ := json.Marshal(in.Input)
	if sum := SummarizeInput(raw); sum != "" {
		question += ": " + sum
	}
	question += ". Allow it?"
	ans, err := Ask(ctx, s.c, s.cfg, question, []string{"allow", "deny"})
	var reply permissionAnswer
	switch {
	case errors.Is(err, ErrUnanswered):
		reply = permissionAnswer{Behavior: "deny", Message: "nobody in the room answered before the turn's deadline"}
	case err != nil:
		reply = permissionAnswer{Behavior: "deny", Message: "the question could not be asked: " + err.Error()}
	case Allows(ans.Text):
		input := json.RawMessage(raw)
		if in.Input == nil {
			input = json.RawMessage(`{}`)
		}
		reply = permissionAnswer{Behavior: "allow", UpdatedInput: input}
	default:
		reply = permissionAnswer{Behavior: "deny", Message: ans.Author + " said: " + ans.Text}
	}
	body, _ := json.Marshal(reply)
	return &mcp.CallToolResult{Content: []mcp.Content{&mcp.TextContent{Text: string(body)}}}, nil, nil
}

// Allows reads a person's answer to "allow it?": allow, yes, ok, go
// ahead, or a choice that starts with any of them.
func Allows(answer string) bool {
	a := strings.ToLower(strings.TrimSpace(answer))
	for _, yes := range []string{"allow", "yes", "ok", "go ahead", "sure", "approved", "approve", "y"} {
		if a == yes || strings.HasPrefix(a, yes+" ") || strings.HasPrefix(a, yes+",") || strings.HasPrefix(a, yes+".") {
			return true
		}
	}
	return false
}

// SummarizeInput is the one line a question shows of a tool's input: the
// field that says what is asked, else the input as compact JSON.
func SummarizeInput(input json.RawMessage) string {
	var m map[string]any
	if json.Unmarshal(input, &m) != nil || len(m) == 0 {
		return types.Bound(strings.TrimSpace(string(input)))
	}
	for _, k := range []string{"command", "file_path", "path", "pattern", "query", "url", "prompt", "description"} {
		if v, ok := m[k].(string); ok && v != "" {
			return types.Bound(v)
		}
	}
	keys := make([]string, 0, len(m))
	for k := range m {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	parts := make([]string, 0, len(keys))
	for _, k := range keys {
		v, _ := json.Marshal(m[k])
		parts = append(parts, k+"="+string(v))
	}
	return types.Bound(strings.Join(parts, " "))
}

// ReadRoomInput names the room to read.
type ReadRoomInput struct {
	Room  string `json:"room,omitempty" jsonschema:"the room's path, room/<id>; your own room when empty"`
	Limit int    `json:"limit,omitempty" jsonschema:"at most this many of the latest messages; all when 0"`
}

// ReadRoomOutput is the transcript.
type ReadRoomOutput struct {
	Room       string `json:"room"`
	Transcript string `json:"transcript"`
	Messages   int    `json:"messages"`
}

func (s *Server) readRoom(ctx context.Context, _ *mcp.CallToolRequest, in ReadRoomInput) (*mcp.CallToolResult, ReadRoomOutput, error) {
	room := in.Room
	if room == "" {
		room = s.cfg.Room
	}
	if contract.PathType(room) != types.Room || strings.Count(room, "/") != 1 {
		return nil, ReadRoomOutput{}, fmt.Errorf("room: %q is not room/<id>", room)
	}
	msgs, err := Transcript(ctx, s.c, s.cfg.Store, room)
	if err != nil {
		return nil, ReadRoomOutput{}, err
	}
	if in.Limit > 0 && len(msgs) > in.Limit {
		msgs = msgs[len(msgs)-in.Limit:]
	}
	var b strings.Builder
	for _, m := range msgs {
		fmt.Fprintf(&b, "[%d] %s (%s): %s\n", m.Seq, m.Author, m.Kind, m.Text)
	}
	return nil, ReadRoomOutput{Room: room, Transcript: b.String(), Messages: len(msgs)}, nil
}

// RoomInfo is one room as list_rooms yields it.
type RoomInfo struct {
	Path    string `json:"path"`
	Title   string `json:"title"`
	Status  string `json:"status"`
	Purpose string `json:"purpose,omitempty"`
}

// ListRoomsOutput is the rooms.
type ListRoomsOutput struct {
	Rooms []RoomInfo `json:"rooms"`
}

func (s *Server) listRooms(ctx context.Context, _ *mcp.CallToolRequest, _ struct{}) (*mcp.CallToolResult, ListRoomsOutput, error) {
	var out ListRoomsOutput
	for info, err := range s.c.ListInstances(ctx, s.cfg.Store, client.ByType(types.Room)) {
		if err != nil {
			return nil, out, fmt.Errorf("list rooms: %w", err)
		}
		var d types.RoomData
		_ = json.Unmarshal(info.State, &d)
		status := d.Status
		if status == "" {
			status = "open"
		}
		out.Rooms = append(out.Rooms, RoomInfo{Path: info.Path, Title: d.Title, Status: status, Purpose: d.Purpose})
	}
	sort.Slice(out.Rooms, func(i, j int) bool { return out.Rooms[i].Path < out.Rooms[j].Path })
	return nil, out, nil
}

// TranscriptLine is one message as the transcript reads it.
type TranscriptLine struct {
	Seq    uint64
	Author string
	Kind   string
	Text   string
}

// Transcript reads a room's messages from the store's state, in sequence
// order — the record, never a replay.
func Transcript(ctx context.Context, c *client.Client, store, room string) ([]TranscriptLine, error) {
	var out []TranscriptLine
	for info, err := range c.ListInstances(ctx, store, client.Under(room)) {
		if err != nil {
			return nil, fmt.Errorf("read %s: %w", room, err)
		}
		if !strings.HasPrefix(info.Path, room+"/"+types.Messages+"/") {
			continue
		}
		var d types.MessageData
		if json.Unmarshal(info.State, &d) != nil {
			continue
		}
		kind := d.Kind
		if kind == "" {
			kind = types.KindMessage
		}
		author := ""
		for op, err := range c.History(ctx, store, info.Path) {
			if err == nil && author == "" {
				author = op.Author
			}
		}
		out = append(out, TranscriptLine{Seq: info.Seq, Author: author, Kind: kind, Text: d.Text})
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Seq < out[j].Seq })
	return out, nil
}

// ErrUnanswered is a question nobody answered before the deadline.
var ErrUnanswered = errors.New("nobody answered before the deadline")

// Answer is what a person said to a question.
type Answer struct {
	Text   string
	Author string
	Path   string
}

// Ask writes a question in the room — kind question, the choices, the
// askers as mentions — and waits for the first message by anyone but the
// agent whose reply_to names it, until the deadline.
func Ask(ctx context.Context, c *client.Client, cfg Config, question string, choices []string) (Answer, error) {
	if !cfg.Deadline.IsZero() {
		var cancel context.CancelFunc
		ctx, cancel = context.WithDeadline(ctx, cfg.Deadline)
		defer cancel()
	}
	path := cfg.Room + "/" + types.Messages + "/q-" + randomID()
	payload, _ := json.Marshal(types.MessageData{Text: question, Kind: types.KindQuestion, Choices: choices, Mentions: cfg.Askers})
	ack, err := c.Create(ctx, cfg.Store, path, "create", payload)
	if err != nil {
		return Answer{}, fmt.Errorf("ask: %w", err)
	}
	for op, err := range c.Tail(ctx, cfg.Store, "", client.After(ack.Seq)) {
		if err != nil {
			if ctx.Err() != nil {
				return Answer{}, ErrUnanswered
			}
			return Answer{}, fmt.Errorf("waiting for an answer: %w", err)
		}
		if op.Type != "create" || op.Author == cfg.Agent {
			continue
		}
		opPath := contract.TailPath(contract.InstanceFromSubject(cfg.Store, op.Subject))
		if !strings.HasPrefix(opPath, cfg.Room+"/"+types.Messages+"/") {
			continue
		}
		var d types.MessageData
		if json.Unmarshal(op.Payload, &d) != nil || d.ReplyTo != path {
			continue
		}
		return Answer{Text: d.Text, Author: op.Author, Path: opPath}, nil
	}
	return Answer{}, ErrUnanswered
}

func randomID() string {
	var b [6]byte
	_, _ = rand.Read(b[:])
	return hex.EncodeToString(b[:])
}
