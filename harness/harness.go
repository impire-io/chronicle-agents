// Package harness is what thinks, as a process: the invocation template
// that keeps the runner harness-agnostic, the presets for the common
// harnesses, and one invocation — a fresh run directory or the declared
// one, the MCP configuration generated from the toolkits and the runner's
// own server, the environment scrubbed, the process group killed at the
// deadline (chronicle-agents-hq design 02, "the harness"). What the harness
// writes on standard output is read the way its template says: as the
// turn's text, whole, or as Claude Code's event stream, decoded as it
// arrives — the text as it is written, every tool call as it starts and
// comes back, and the result with what it cost.
package harness

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"syscall"
	"time"

	"github.com/impire-io/chronicle-agents/types"
)

// Output says how a harness's standard output is read.
type Output string

// The output kinds a template may name.
const (
	// OutputText: the whole of standard output, trimmed, is the turn.
	OutputText Output = "text"
	// OutputClaudeStream: one JSON event per line, as Claude Code's
	// --output-format stream-json writes them; the turn is the result
	// event's text, the steps are its tool calls.
	OutputClaudeStream Output = "claude-stream"
)

// Template states how a harness is invoked. Args may carry the
// placeholders: {{INSTRUCTIONS}} (a file holding the instructions),
// {{CONTEXT}} (a file holding the composed context, also on stdin),
// {{MCP_CONFIG}} (the generated MCP configuration file), {{RUN_DIR}} (the
// run directory), {{MODEL}} (the model's name, when given), {{TOOLS}} (the
// harness's own tools granted, by name, comma-separated), {{ALLOWED}} (the
// rules that run without asking: the granted tools' rules and every MCP
// server's tools), {{PERMISSION_TOOL}} (the MCP tool a permission request
// goes to), and the texts themselves — {{CONTEXT_TEXT}}, and {{PROMPT}} for
// the instructions and the context as one.
type Template struct {
	Command string
	Args    []string
	// Env is the harness's own environment: a provider credential, a home.
	// It is added to the scrubbed environment, never to the record.
	Env map[string]string
	// ModelArgs are appended when a model is named: a preset's way to pass
	// it ({{MODEL}} inside).
	ModelArgs []string
	// Output is how standard output is read; text when empty.
	Output Output
}

// Toolkit is one MCP server written into the configuration.
type Toolkit struct {
	Name    string
	Command string
	Args    []string
	Env     map[string]string
}

// Input is one turn's input.
type Input struct {
	Agent        string
	Instructions string
	Context      string
	Model        string
	Toolkits     []Toolkit
	// Server is the runner's own MCP server, handed to the harness beside
	// the toolkits: how the agent reads a room beyond its context, asks
	// the people in the room a question, and how a permission request
	// reaches them. Nil for a harness that takes no MCP configuration.
	Server *Toolkit
	// Tools are the harness's own tools the agent may use, as the
	// declaration states them: names, or names with a rule.
	Tools []string
	// Workdir is where the harness runs; a fresh temporary directory
	// when empty.
	Workdir  string
	Deadline time.Duration
}

// Event is one thing the harness did, reported as it happens.
type Event struct {
	// Text is the turn's text so far, whole, when the harness wrote some.
	Text string
	// Step is a tool call: started (no output yet) or, with Finished,
	// come back.
	Step     *types.Step
	Finished bool
}

// Observer hears the events of a turn in progress. It is called from the
// goroutine reading the harness and must return promptly.
type Observer func(Event)

// Result is what a turn produced.
type Result struct {
	Text  string
	Steps []types.Step
	Usage *types.Usage
}

// PermissionToolName is the MCP tool name a permission request goes to,
// for the runner's own server: mcp__<server>__permission.
func PermissionToolName(server string) string {
	return "mcp__" + server + "__permission"
}

// Presets by name. claude-code is headless Claude Code: the instructions
// appended to its system prompt, the context on stdin, none of the
// operator's own settings (no allow rules, hooks or plugins of theirs: the
// declaration is the whole configuration), the MCP servers from the
// generated configuration only, the granted tools and no other, the
// granted rules and every MCP server pre-approved, every other permission
// asked through the runner's server, and the event stream on standard
// output.
var presets = map[string]Template{
	"claude-code": {
		Command: "claude",
		Args: []string{"-p", "--output-format", "stream-json", "--verbose", "--include-partial-messages",
			"--setting-sources", "", "--permission-mode", "default",
			"--append-system-prompt-file", "{{INSTRUCTIONS}}",
			"--mcp-config", "{{MCP_CONFIG}}", "--strict-mcp-config",
			"--tools", "{{TOOLS}}", "--allowedTools", "{{ALLOWED}}",
			"--permission-prompt-tool", "{{PERMISSION_TOOL}}"},
		ModelArgs: []string{"--model", "{{MODEL}}"},
		Output:    OutputClaudeStream,
	},
	// pi: non-interactive, no tools, no session kept; the instructions
	// appended to its system prompt from the file, the context attached
	// as a file. Toolkits are not wired: pi takes extensions, not an MCP
	// configuration per run.
	"pi": {
		Command:   "pi",
		Args:      []string{"-p", "--no-tools", "--no-session", "--append-system-prompt", "{{INSTRUCTIONS}}", "@{{CONTEXT}}", "The attached file is the room's transcript and what triggered you. Reply as instructed, with the message text only."},
		ModelArgs: []string{"--model", "{{MODEL}}"},
	},
	// hermes: one-shot mode prints only the final response; the
	// instructions and the context travel as the one prompt. Toolkits are
	// not wired: hermes configures MCP servers in its own home.
	"hermes": {
		Command:   "hermes",
		Args:      []string{"-z", "{{PROMPT}}"},
		ModelArgs: []string{"-m", "{{MODEL}}"},
	},
	// command runs any executable that reads the context on stdin and
	// writes the turn on stdout: the tests' harness, and the escape hatch.
	"command": {},
}

// Preset returns a named preset.
func Preset(name string) (Template, bool) {
	t, ok := presets[name]
	return t, ok
}

// PresetNames lists the presets.
func PresetNames() []string {
	return []string{"claude-code", "pi", "hermes", "command"}
}

// ErrNoOutput is a harness that exited well and said nothing.
var ErrNoOutput = errors.New("the harness returned no text")

// Invoke runs one turn: the run directory, the files, the generated MCP
// configuration, the scrubbed environment, the deadline with a
// process-group kill; the output read as the template says, with every
// event reported to the observer as it happens.
func Invoke(ctx context.Context, tpl Template, in Input, observe Observer) (Result, error) {
	if tpl.Command == "" {
		return Result{}, errors.New("template: no command")
	}
	if observe == nil {
		observe = func(Event) {}
	}
	dir, err := os.MkdirTemp("", "chronicle-agents-"+in.Agent+"-")
	if err != nil {
		return Result{}, fmt.Errorf("run dir: %w", err)
	}
	defer func() { _ = os.RemoveAll(dir) }()

	instructions := filepath.Join(dir, "instructions.md")
	if err := os.WriteFile(instructions, []byte(in.Instructions), 0o600); err != nil {
		return Result{}, fmt.Errorf("write instructions: %w", err)
	}
	contextFile := filepath.Join(dir, "context.md")
	if err := os.WriteFile(contextFile, []byte(in.Context), 0o600); err != nil {
		return Result{}, fmt.Errorf("write context: %w", err)
	}
	mcpConfig := filepath.Join(dir, "mcp.json")
	if err := os.WriteFile(mcpConfig, mcpConfigFor(in.Toolkits, in.Server), 0o600); err != nil {
		return Result{}, fmt.Errorf("write mcp config: %w", err)
	}

	args := append([]string{}, tpl.Args...)
	if in.Model != "" {
		args = append(args, tpl.ModelArgs...)
	}
	permissionTool := ""
	if in.Server != nil {
		permissionTool = PermissionToolName(in.Server.Name)
	}
	subs := map[string]string{
		"{{INSTRUCTIONS}}":    instructions,
		"{{CONTEXT}}":         contextFile,
		"{{MCP_CONFIG}}":      mcpConfig,
		"{{RUN_DIR}}":         dir,
		"{{MODEL}}":           in.Model,
		"{{TOOLS}}":           strings.Join(toolNames(in.Tools), ","),
		"{{ALLOWED}}":         strings.Join(allowedRules(in), ","),
		"{{PERMISSION_TOOL}}": permissionTool,
		"{{MCP_TOOLS}}":       mcpToolPatterns(in.Toolkits, in.Server),
		"{{CONTEXT_TEXT}}":    in.Context,
		"{{PROMPT}}":          in.Instructions + "\n\n" + in.Context,
	}
	for i, a := range args {
		for k, v := range subs {
			a = strings.ReplaceAll(a, k, v)
		}
		args[i] = a
	}

	deadline := in.Deadline
	if deadline <= 0 {
		deadline = 3 * time.Minute
	}
	cctx, cancel := context.WithTimeout(ctx, deadline)
	defer cancel()
	cmd := exec.CommandContext(cctx, tpl.Command, args...)
	cmd.Dir = dir
	if in.Workdir != "" {
		cmd.Dir = in.Workdir
	}
	cmd.Stdin = strings.NewReader(in.Context)
	env := map[string]string{}
	for k, v := range tpl.Env {
		env[k] = v
	}
	// A tool call that waits on a person — a permission request answered
	// in the room — may take as long as the turn itself.
	env["MCP_TOOL_TIMEOUT"] = strconv.FormatInt(deadline.Milliseconds(), 10)
	cmd.Env = scrubbedEnv(env)
	cmd.SysProcAttr = &syscall.SysProcAttr{Setpgid: true}
	cmd.Cancel = func() error {
		// The harness may have children of its own (an MCP server, a
		// shell): the whole process group goes.
		return syscall.Kill(-cmd.Process.Pid, syscall.SIGKILL)
	}
	var errb bytes.Buffer
	cmd.Stderr = &errb
	stdout, err := cmd.StdoutPipe()
	if err != nil {
		return Result{}, fmt.Errorf("harness %s: %w", tpl.Command, err)
	}
	if err := cmd.Start(); err != nil {
		return Result{}, fmt.Errorf("harness %s: %w", tpl.Command, err)
	}

	var res Result
	var readErr error
	switch tpl.Output {
	case OutputClaudeStream:
		res, readErr = decodeClaudeStream(stdout, observe)
	default:
		var out bytes.Buffer
		_, readErr = out.ReadFrom(stdout)
		res.Text = strings.TrimSpace(out.String())
	}
	waitErr := cmd.Wait()
	if cctx.Err() != nil {
		return Result{}, fmt.Errorf("harness %s: deadline of %s exceeded: %w", tpl.Command, deadline, context.DeadlineExceeded)
	}
	if waitErr != nil {
		return Result{}, fmt.Errorf("harness %s: %w: %s", tpl.Command, waitErr, strings.TrimSpace(errb.String()))
	}
	if readErr != nil {
		return Result{}, fmt.Errorf("harness %s: %w", tpl.Command, readErr)
	}
	if res.Text == "" {
		return Result{}, ErrNoOutput
	}
	return res, nil
}

// line is one event of Claude Code's stream, in the fields read here.
type line struct {
	Type            string          `json:"type"`
	Subtype         string          `json:"subtype"`
	ParentToolUseID json.RawMessage `json:"parent_tool_use_id"`
	Message         struct {
		Model   string  `json:"model"`
		Content []block `json:"content"`
	} `json:"message"`
	Event struct {
		Type         string `json:"type"`
		ContentBlock struct {
			Type string `json:"type"`
		} `json:"content_block"`
		Delta struct {
			Type string `json:"type"`
			Text string `json:"text"`
		} `json:"delta"`
	} `json:"event"`
	Result       json.RawMessage `json:"result"`
	IsError      bool            `json:"is_error"`
	SessionID    string          `json:"session_id"`
	TotalCostUSD float64         `json:"total_cost_usd"`
	NumTurns     int             `json:"num_turns"`
	Usage        struct {
		InputTokens  int64 `json:"input_tokens"`
		OutputTokens int64 `json:"output_tokens"`
	} `json:"usage"`
	Errors []string `json:"errors"`
}

// block is one content block of an assistant or user message.
type block struct {
	Type      string          `json:"type"`
	Text      string          `json:"text"`
	ID        string          `json:"id"`
	Name      string          `json:"name"`
	Input     json.RawMessage `json:"input"`
	ToolUseID string          `json:"tool_use_id"`
	Content   json.RawMessage `json:"content"`
	IsError   bool            `json:"is_error"`
}

// pending is a tool call that has not come back.
type pending struct {
	step    types.Step
	started time.Time
}

// decodeClaudeStream reads the event stream line by line: text deltas as
// the model writes, tool calls as they start (assistant tool_use) and
// come back (user tool_result), the result's text, cost and session. A
// line that is not JSON, or an event of a kind not read here, is passed
// over; a subagent's lines (parent_tool_use_id set) are not the turn's.
func decodeClaudeStream(r io.Reader, observe Observer) (Result, error) {
	sc := bufio.NewScanner(r)
	sc.Buffer(make([]byte, 0, 64*1024), 32*1024*1024)
	var res Result
	var cur strings.Builder
	calls := map[string]*pending{}
	model := ""
	sawResult := false
	for sc.Scan() {
		raw := sc.Bytes()
		if len(raw) == 0 || raw[0] != '{' {
			continue
		}
		var l line
		if err := json.Unmarshal(raw, &l); err != nil {
			continue
		}
		if len(l.ParentToolUseID) > 0 && string(l.ParentToolUseID) != "null" {
			continue
		}
		switch l.Type {
		case "stream_event":
			switch l.Event.Type {
			case "content_block_start":
				if l.Event.ContentBlock.Type == "text" {
					cur.Reset()
				}
			case "content_block_delta":
				if l.Event.Delta.Type == "text_delta" && l.Event.Delta.Text != "" {
					cur.WriteString(l.Event.Delta.Text)
					observe(Event{Text: cur.String()})
				}
			}
		case "assistant":
			if l.Message.Model != "" {
				model = l.Message.Model
			}
			for _, b := range l.Message.Content {
				switch b.Type {
				case "text":
					if b.Text != "" {
						cur.Reset()
						cur.WriteString(b.Text)
						observe(Event{Text: b.Text})
					}
				case "tool_use":
					step := types.Step{Tool: b.Name, Input: summarize(b.Name, b.Input)}
					calls[b.ID] = &pending{step: step, started: time.Now()}
					copied := step
					observe(Event{Step: &copied})
				}
			}
		case "user":
			for _, b := range l.Message.Content {
				if b.Type != "tool_result" {
					continue
				}
				p, ok := calls[b.ToolUseID]
				if !ok {
					continue
				}
				delete(calls, b.ToolUseID)
				p.step.Output = types.Bound(contentText(b.Content))
				p.step.OK = !b.IsError
				p.step.Took = time.Since(p.started).Milliseconds()
				res.Steps = append(res.Steps, p.step)
				copied := p.step
				observe(Event{Step: &copied, Finished: true})
			}
		case "result":
			sawResult = true
			var text string
			if len(l.Result) > 0 && string(l.Result) != "null" {
				_ = json.Unmarshal(l.Result, &text)
			}
			res.Text = strings.TrimSpace(text)
			res.Usage = &types.Usage{
				Model:        model,
				InputTokens:  l.Usage.InputTokens,
				OutputTokens: l.Usage.OutputTokens,
				CostUSD:      l.TotalCostUSD,
				Turns:        l.NumTurns,
				Session:      l.SessionID,
			}
			if l.Subtype != "" && l.Subtype != "success" || l.IsError {
				why := l.Subtype
				if len(l.Errors) > 0 {
					why += ": " + strings.Join(l.Errors, "; ")
				}
				return res, fmt.Errorf("the run ended with %s", why)
			}
		}
	}
	if err := sc.Err(); err != nil {
		return res, fmt.Errorf("reading the event stream: %w", err)
	}
	// A call that never came back is a step without an answer.
	for _, p := range calls {
		p.step.Output = "no result"
		res.Steps = append(res.Steps, p.step)
	}
	if !sawResult && res.Text == "" {
		res.Text = strings.TrimSpace(cur.String())
	}
	return res, nil
}

// summarize is the one line a step keeps of a tool's input: the field
// that says what was asked — the command, the path, the query, the
// prompt — else the input as compact JSON; bounded either way.
func summarize(tool string, input json.RawMessage) string {
	var m map[string]any
	if json.Unmarshal(input, &m) != nil || len(m) == 0 {
		return types.Bound(string(input))
	}
	for _, k := range []string{"command", "file_path", "path", "pattern", "query", "url", "prompt", "description", "text", "question"} {
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
	_ = tool
	return types.Bound(strings.Join(parts, " "))
}

// contentText is a tool result's content as text: a string, or the text
// blocks of a list.
func contentText(raw json.RawMessage) string {
	var s string
	if json.Unmarshal(raw, &s) == nil {
		return s
	}
	var blocks []struct {
		Type string `json:"type"`
		Text string `json:"text"`
	}
	if json.Unmarshal(raw, &blocks) == nil {
		var parts []string
		for _, b := range blocks {
			if b.Type == "text" && b.Text != "" {
				parts = append(parts, b.Text)
			}
		}
		return strings.Join(parts, "\n")
	}
	return string(raw)
}

// toolNames lists the tools the entries grant, each once: the name
// before a rule's parenthesis.
func toolNames(entries []string) []string {
	seen := map[string]bool{}
	var out []string
	for _, e := range entries {
		name, _, _ := strings.Cut(strings.TrimSpace(e), "(")
		name = strings.TrimSpace(name)
		if name != "" && !seen[name] {
			seen[name] = true
			out = append(out, name)
		}
	}
	return out
}

// allowedRules lists what runs without asking: the granted entries as
// written (a bare name pre-approves every call, a rule only the calls it
// covers), and every MCP server's tools.
func allowedRules(in Input) []string {
	var out []string
	for _, e := range in.Tools {
		if e = strings.TrimSpace(e); e != "" {
			out = append(out, e)
		}
	}
	for _, t := range in.Toolkits {
		out = append(out, "mcp__"+t.Name)
	}
	if in.Server != nil {
		out = append(out, "mcp__"+in.Server.Name)
	}
	return out
}

// mcpConfigFor writes the toolkits and the runner's server in the MCP
// configuration shape the coding agents read: {"mcpServers": {name:
// {command, args, env}}}.
func mcpConfigFor(toolkits []Toolkit, server *Toolkit) []byte {
	servers := map[string]any{}
	add := func(t Toolkit) {
		s := map[string]any{"command": t.Command}
		if len(t.Args) > 0 {
			s["args"] = t.Args
		}
		if len(t.Env) > 0 {
			s["env"] = t.Env
		}
		servers[t.Name] = s
	}
	for _, t := range toolkits {
		add(t)
	}
	if server != nil {
		add(*server)
	}
	b, _ := json.MarshalIndent(map[string]any{"mcpServers": servers}, "", "  ")
	return b
}

// mcpToolPatterns names every MCP server's tools for a harness that takes
// an allow-list: mcp__<name> per server, comma-separated.
func mcpToolPatterns(toolkits []Toolkit, server *Toolkit) string {
	names := make([]string, 0, len(toolkits)+1)
	for _, t := range toolkits {
		names = append(names, "mcp__"+t.Name)
	}
	if server != nil {
		names = append(names, "mcp__"+server.Name)
	}
	return strings.Join(names, ",")
}

// keptEnv is what a harness inherits: enough to find its binaries, its
// home and its own login state; nothing of the runner's and nothing that
// names a session the harness is not in.
var keptEnv = map[string]bool{
	"PATH": true, "HOME": true, "USER": true, "LOGNAME": true, "SHELL": true,
	"LANG": true, "LC_ALL": true, "TERM": true, "TMPDIR": true, "TZ": true,
	"XDG_CONFIG_HOME": true, "XDG_CACHE_HOME": true, "XDG_DATA_HOME": true,
}

func scrubbedEnv(extra map[string]string) []string {
	var env []string
	for _, kv := range os.Environ() {
		k, _, _ := strings.Cut(kv, "=")
		if keptEnv[k] {
			env = append(env, kv)
		}
	}
	keys := make([]string, 0, len(extra))
	for k := range extra {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	for _, k := range keys {
		env = append(env, k+"="+extra[k])
	}
	return env
}
