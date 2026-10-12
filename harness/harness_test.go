package harness

import (
	"context"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/impire-io/chronicle-agents/types"
)

func TestInvokeRunsACommandOnTheContext(t *testing.T) {
	tpl := Template{Command: "tail", Args: []string{"-n", "1"}}
	res, err := Invoke(context.Background(), tpl, Input{Agent: "wilson", Context: "first line\nlast line\n"}, nil)
	if err != nil {
		t.Fatal(err)
	}
	if res.Text != "last line" {
		t.Errorf("text = %q", res.Text)
	}
}

func TestInvokeWritesTheFilesAndSubstitutes(t *testing.T) {
	// cat the instructions file and the mcp config: the placeholders resolve to files in the run dir.
	tpl := Template{Command: "sh", Args: []string{"-c", `cat "$1"; echo; cat "$2"; echo; echo "model=$3"`, "sh", "{{INSTRUCTIONS}}", "{{MCP_CONFIG}}", "{{MODEL}}"}}
	in := Input{Agent: "wilson", Instructions: "be brief", Model: "m1", Context: "x",
		Toolkits: []Toolkit{{Name: "invoices", Command: "inv", Args: []string{"--prod"}}},
		Server:   &Toolkit{Name: "chronicle-agents", Command: "/bin/chronicle-agents", Args: []string{"mcp"}, Env: map[string]string{"CHRONICLE_AGENTS_ROOM": "room/r1"}}}
	res, err := Invoke(context.Background(), tpl, in, nil)
	if err != nil {
		t.Fatal(err)
	}
	text := res.Text
	if !strings.HasPrefix(text, "be brief") || !strings.HasSuffix(text, "model=m1") {
		t.Errorf("text = %q", text)
	}
	var cfg struct {
		Servers map[string]struct {
			Command string            `json:"command"`
			Args    []string          `json:"args"`
			Env     map[string]string `json:"env"`
		} `json:"mcpServers"`
	}
	body := text[len("be brief") : len(text)-len("model=m1")]
	if err := json.Unmarshal([]byte(body), &cfg); err != nil {
		t.Fatalf("mcp config: %v in %q", err, body)
	}
	if s, ok := cfg.Servers["invoices"]; !ok || s.Command != "inv" || s.Args[0] != "--prod" {
		t.Errorf("mcp config = %+v", cfg)
	}
	// The runner's own server is in the configuration beside the toolkits, with its environment.
	if s, ok := cfg.Servers["chronicle-agents"]; !ok || s.Command != "/bin/chronicle-agents" || s.Args[0] != "mcp" || s.Env["CHRONICLE_AGENTS_ROOM"] != "room/r1" {
		t.Errorf("server = %+v", cfg.Servers["chronicle-agents"])
	}
}

func TestInvokeScrubsTheEnvironmentAndBoundsToolCalls(t *testing.T) {
	t.Setenv("CLAUDE_CODE_SESSION_ID", "leak")
	tpl := Template{Command: "sh", Args: []string{"-c", `echo "sid=${CLAUDE_CODE_SESSION_ID:-none} key=${PROVIDER_KEY:-none} mcp=${MCP_TOOL_TIMEOUT:-none}"`}, Env: map[string]string{"PROVIDER_KEY": "k"}}
	res, err := Invoke(context.Background(), tpl, Input{Agent: "wilson", Context: "x", Deadline: 2 * time.Second}, nil)
	if err != nil {
		t.Fatal(err)
	}
	// A tool call may wait as long as the turn: the MCP tool timeout is the deadline, in milliseconds.
	if res.Text != "sid=none key=k mcp=2000" {
		t.Errorf("text = %q", res.Text)
	}
}

func TestInvokeRunsInTheDeclaredDirectory(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "marker.txt"), []byte("here"), 0o600); err != nil {
		t.Fatal(err)
	}
	tpl := Template{Command: "sh", Args: []string{"-c", `cat marker.txt`}}
	res, err := Invoke(context.Background(), tpl, Input{Agent: "wilson", Context: "x", Workdir: dir}, nil)
	if err != nil {
		t.Fatal(err)
	}
	if res.Text != "here" {
		t.Errorf("text = %q: the harness did not run in the declared directory", res.Text)
	}
}

func TestInvokeKillsAtTheDeadline(t *testing.T) {
	tpl := Template{Command: "sh", Args: []string{"-c", "sleep 30; echo late"}}
	start := time.Now()
	_, err := Invoke(context.Background(), tpl, Input{Agent: "wilson", Context: "x", Deadline: 300 * time.Millisecond}, nil)
	if err == nil || !strings.Contains(err.Error(), "deadline") {
		t.Fatalf("err = %v, want a deadline error", err)
	}
	if !errors.Is(err, context.DeadlineExceeded) {
		t.Errorf("err = %v does not wrap context.DeadlineExceeded: the runner cannot name it", err)
	}
	if time.Since(start) > 5*time.Second {
		t.Errorf("took %s: the process group was not killed", time.Since(start))
	}
}

func TestInvokeRefusesSilence(t *testing.T) {
	tpl := Template{Command: "true"}
	if _, err := Invoke(context.Background(), tpl, Input{Agent: "wilson", Context: "x"}, nil); err != ErrNoOutput {
		t.Errorf("err = %v, want ErrNoOutput", err)
	}
}

// The claude-code preset: the event stream on stdout, the granted tools
// and no other, the granted rules and every MCP server pre-approved, the
// permission requests routed to the runner's server.
func TestClaudePresetArgs(t *testing.T) {
	tpl, ok := Preset("claude-code")
	if !ok || tpl.Command != "claude" || tpl.Output != OutputClaudeStream {
		t.Fatalf("preset = %+v", tpl)
	}
	if _, ok := Preset("nope"); ok {
		t.Error("unknown preset found")
	}
	for _, name := range PresetNames() {
		if name == "command" {
			continue
		}
		if p, ok := Preset(name); !ok || p.Command == "" {
			t.Errorf("preset %s: %+v", name, p)
		}
	}
	in := Input{
		Tools:    []string{"Bash(git *)", "Read", "Bash"},
		Toolkits: []Toolkit{{Name: "invoices"}},
		Server:   &Toolkit{Name: "chronicle-agents"},
	}
	if got := strings.Join(toolNames(in.Tools), ","); got != "Bash,Read" {
		t.Errorf("tools = %q", got)
	}
	if got := strings.Join(allowedRules(in), ","); got != "Bash(git *),Read,Bash,mcp__invoices,mcp__chronicle-agents" {
		t.Errorf("allowed = %q", got)
	}
	if got := mcpToolPatterns(in.Toolkits, in.Server); got != "mcp__invoices,mcp__chronicle-agents" {
		t.Errorf("patterns = %q", got)
	}
	if got := PermissionToolName("chronicle-agents"); got != "mcp__chronicle-agents__permission" {
		t.Errorf("permission tool = %q", got)
	}
	// No tools at all: the harness is told so, in its own grammar.
	if got := strings.Join(toolNames(nil), ","); got != "" {
		t.Errorf("tools = %q, want none", got)
	}
}

// The text placeholders carry the prompt as an argument, the way hermes
// and pi take it: {{PROMPT}} is the instructions and the context as one.
func TestTextPlaceholders(t *testing.T) {
	tpl := Template{Command: "sh", Args: []string{"-c", `printf '%s' "$1"`, "sh", "{{PROMPT}}"}}
	res, err := Invoke(context.Background(), tpl, Input{Agent: "wilson", Instructions: "be brief", Context: "the room"}, nil)
	if err != nil {
		t.Fatal(err)
	}
	if res.Text != "be brief\n\nthe room" {
		t.Errorf("prompt = %q", res.Text)
	}
	tpl = Template{Command: "sh", Args: []string{"-c", `printf '%s' "$1"`, "sh", "{{CONTEXT_TEXT}}"}}
	if res, _ = Invoke(context.Background(), tpl, Input{Agent: "wilson", Context: "the room"}, nil); res.Text != "the room" {
		t.Errorf("context text = %q", res.Text)
	}
}

// A fake Claude Code: the event stream as the real one writes it — the
// init, a text block streamed in deltas, a tool call and its result, the
// final text, the result with its cost — on stdout, one JSON object per
// line, with a line of noise between.
const fakeStream = `{"type":"system","subtype":"init","session_id":"s-1","tools":["Bash"]}
{"type":"stream_event","event":{"type":"content_block_start","index":0,"content_block":{"type":"text"}},"parent_tool_use_id":null}
{"type":"stream_event","event":{"type":"content_block_delta","index":0,"delta":{"type":"text_delta","text":"Let me "}},"parent_tool_use_id":null}
{"type":"stream_event","event":{"type":"content_block_delta","index":0,"delta":{"type":"text_delta","text":"look."}},"parent_tool_use_id":null}
{"type":"assistant","message":{"model":"claude-x","content":[{"type":"text","text":"Let me look."}]},"parent_tool_use_id":null,"session_id":"s-1"}
{"type":"assistant","message":{"model":"claude-x","content":[{"type":"tool_use","id":"t1","name":"Bash","input":{"command":"ls -la","description":"List"}}]},"parent_tool_use_id":null,"session_id":"s-1"}
not json at all
{"type":"user","message":{"role":"user","content":[{"type":"tool_result","tool_use_id":"t1","content":"a.txt\nb.txt","is_error":false}]},"parent_tool_use_id":null,"session_id":"s-1"}
{"type":"assistant","message":{"model":"claude-x","content":[{"type":"tool_use","id":"t2","name":"Read","input":{"file_path":"a.txt"}}]},"parent_tool_use_id":"sub-1","session_id":"s-1"}
{"type":"assistant","message":{"model":"claude-x","content":[{"type":"tool_use","id":"t3","name":"Read","input":{"file_path":"b.txt"}}]},"parent_tool_use_id":null,"session_id":"s-1"}
{"type":"user","message":{"role":"user","content":[{"type":"tool_result","tool_use_id":"t3","content":[{"type":"text","text":"nope"}],"is_error":true}]},"parent_tool_use_id":null,"session_id":"s-1"}
{"type":"assistant","message":{"model":"claude-x","content":[{"type":"text","text":"Two files: a.txt and b.txt."}]},"parent_tool_use_id":null,"session_id":"s-1"}
{"type":"result","subtype":"success","is_error":false,"result":"Two files: a.txt and b.txt.","session_id":"s-1","total_cost_usd":0.0123,"num_turns":3,"usage":{"input_tokens":120,"output_tokens":45}}
`

func fakeClaude(t *testing.T, stream string) Template {
	t.Helper()
	path := filepath.Join(t.TempDir(), "stream.jsonl")
	if err := os.WriteFile(path, []byte(stream), 0o600); err != nil {
		t.Fatal(err)
	}
	return Template{Command: "cat", Args: []string{path}, Output: OutputClaudeStream}
}

func TestClaudeStreamIsDecodedIntoTextStepsAndUsage(t *testing.T) {
	var mu sync.Mutex
	var texts []string
	var steps []string
	observe := func(ev Event) {
		mu.Lock()
		defer mu.Unlock()
		switch {
		case ev.Step != nil && ev.Finished:
			steps = append(steps, "finished "+ev.Step.Tool+" ok="+boolS(ev.Step.OK)+" out="+ev.Step.Output)
		case ev.Step != nil:
			steps = append(steps, "started "+ev.Step.Tool+" in="+ev.Step.Input)
		case ev.Text != "":
			texts = append(texts, ev.Text)
		}
	}
	res, err := Invoke(context.Background(), fakeClaude(t, fakeStream), Input{Agent: "wilson", Context: "x"}, observe)
	if err != nil {
		t.Fatal(err)
	}
	if res.Text != "Two files: a.txt and b.txt." {
		t.Errorf("text = %q", res.Text)
	}
	// The steps: the shell call and the failed read; the subagent's read is not the turn's.
	want := []types.Step{
		{Tool: "Bash", Input: "ls -la", Output: "a.txt\nb.txt", OK: true},
		{Tool: "Read", Input: "b.txt", Output: "nope", OK: false},
	}
	if len(res.Steps) != len(want) {
		t.Fatalf("steps = %+v", res.Steps)
	}
	for i, s := range res.Steps {
		if s.Tool != want[i].Tool || s.Input != want[i].Input || s.Output != want[i].Output || s.OK != want[i].OK {
			t.Errorf("step %d = %+v, want %+v", i, s, want[i])
		}
	}
	if res.Usage == nil || res.Usage.Model != "claude-x" || res.Usage.CostUSD != 0.0123 || res.Usage.Turns != 3 ||
		res.Usage.InputTokens != 120 || res.Usage.OutputTokens != 45 || res.Usage.Session != "s-1" {
		t.Errorf("usage = %+v", res.Usage)
	}
	mu.Lock()
	defer mu.Unlock()
	// The text as it was written: the deltas accumulate, the block's text confirms, the final text replaces.
	if strings.Join(texts, "|") != "Let me |Let me look.|Let me look.|Two files: a.txt and b.txt." {
		t.Errorf("texts = %q", texts)
	}
	if strings.Join(steps, "|") != "started Bash in=ls -la|finished Bash ok=true out=a.txt\nb.txt|started Read in=b.txt|finished Read ok=false out=nope" {
		t.Errorf("steps = %q", steps)
	}
}

func TestClaudeStreamResultThatIsNotASuccessFailsTheTurn(t *testing.T) {
	stream := `{"type":"system","subtype":"init","session_id":"s-2"}
{"type":"result","subtype":"error_max_turns","is_error":true,"result":null,"session_id":"s-2","errors":["too many turns"],"num_turns":8}
`
	_, err := Invoke(context.Background(), fakeClaude(t, stream), Input{Agent: "wilson", Context: "x"}, nil)
	if err == nil || !strings.Contains(err.Error(), "error_max_turns") || !strings.Contains(err.Error(), "too many turns") {
		t.Errorf("err = %v", err)
	}
}

func TestClaudeStreamWithoutAResultKeepsTheLastText(t *testing.T) {
	stream := `{"type":"assistant","message":{"content":[{"type":"text","text":"half an answer"}]},"parent_tool_use_id":null}
`
	res, err := Invoke(context.Background(), fakeClaude(t, stream), Input{Agent: "wilson", Context: "x"}, nil)
	if err != nil {
		t.Fatal(err)
	}
	if res.Text != "half an answer" {
		t.Errorf("text = %q", res.Text)
	}
}

func TestSummarizeKeepsWhatWasAsked(t *testing.T) {
	cases := map[string]string{
		`{"command":"ls -la","description":"List"}`: "ls -la",
		`{"file_path":"/tmp/a.txt"}`:                "/tmp/a.txt",
		`{"query":"invoice 345"}`:                   "invoice 345",
		`{"b":2,"a":"x"}`:                           `a="x" b=2`,
		`"just text"`:                               `"just text"`,
	}
	for input, want := range cases {
		if got := summarize("T", json.RawMessage(input)); got != want {
			t.Errorf("summarize(%s) = %q, want %q", input, got, want)
		}
	}
	long := strings.Repeat("x", types.StepSummaryLimit+10)
	if got := summarize("T", json.RawMessage(`{"command":"`+long+`"}`)); len([]rune(got)) != types.StepSummaryLimit+1 || !strings.HasSuffix(got, "…") {
		t.Errorf("summary not bounded: %d runes", len([]rune(got)))
	}
}

func boolS(b bool) string {
	if b {
		return "true"
	}
	return "false"
}
