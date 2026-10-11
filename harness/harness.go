// Package harness is what thinks, as a process: the invocation template
// that keeps the runner harness-agnostic, the presets for the common
// harnesses, and one invocation — a fresh run directory, the MCP
// configuration generated from the toolkits, the environment scrubbed, the
// process group killed at the deadline (chronicle-agents-hq design 02,
// "the harness"; soulstream's wrap § 5, re-decided).
package harness

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"syscall"
	"time"
)

// Template states how a harness is invoked. Args may carry the
// placeholders: {{INSTRUCTIONS}} (a file holding the instructions),
// {{CONTEXT}} (a file holding the composed context, also on stdin),
// {{MCP_CONFIG}} (the generated MCP configuration file), {{RUN_DIR}} (the
// run directory), {{MODEL}} (the model's name, when given), and the texts
// themselves — {{CONTEXT_TEXT}}, and {{PROMPT}} for the instructions and
// the context as one prompt.
type Template struct {
	Command string
	Args    []string
	// Env is the harness's own environment: a provider credential, a home.
	// It is added to the scrubbed environment, never to the record.
	Env map[string]string
	// ModelArgs are appended when a model is named: a preset's way to pass
	// it ({{MODEL}} inside).
	ModelArgs []string
	// ToolkitArgs are appended when toolkits exist — a preset's way to
	// restrict the harness to them; NoToolkitArgs when none do.
	ToolkitArgs   []string
	NoToolkitArgs []string
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
	Deadline     time.Duration
}

// Presets by name. claude-code is the spike's harness: a headless run,
// the instructions appended to the system prompt, tools limited to the
// toolkits (none when there are none), the context on stdin.
var presets = map[string]Template{
	"claude-code": {
		Command:       "claude",
		Args:          []string{"-p", "--output-format", "text", "--append-system-prompt-file", "{{INSTRUCTIONS}}"},
		ModelArgs:     []string{"--model", "{{MODEL}}"},
		ToolkitArgs:   []string{"--mcp-config", "{{MCP_CONFIG}}", "--strict-mcp-config", "--allowedTools", "{{MCP_TOOLS}}"},
		NoToolkitArgs: []string{"--disallowedTools", "*"},
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

// Invoke runs one turn: a fresh run directory, the files, the generated
// MCP configuration, the scrubbed environment, the deadline with a
// process-group kill. The turn's text is stdout, trimmed.
func Invoke(ctx context.Context, tpl Template, in Input) (string, error) {
	if tpl.Command == "" {
		return "", errors.New("template: no command")
	}
	dir, err := os.MkdirTemp("", "chronicle-agents-"+in.Agent+"-")
	if err != nil {
		return "", fmt.Errorf("run dir: %w", err)
	}
	defer func() { _ = os.RemoveAll(dir) }()

	instructions := filepath.Join(dir, "instructions.md")
	if err := os.WriteFile(instructions, []byte(in.Instructions), 0o600); err != nil {
		return "", fmt.Errorf("write instructions: %w", err)
	}
	contextFile := filepath.Join(dir, "context.md")
	if err := os.WriteFile(contextFile, []byte(in.Context), 0o600); err != nil {
		return "", fmt.Errorf("write context: %w", err)
	}
	mcpConfig := filepath.Join(dir, "mcp.json")
	if err := os.WriteFile(mcpConfig, mcpConfigFor(in.Toolkits), 0o600); err != nil {
		return "", fmt.Errorf("write mcp config: %w", err)
	}

	args := append([]string{}, tpl.Args...)
	if in.Model != "" {
		args = append(args, tpl.ModelArgs...)
	}
	if len(in.Toolkits) > 0 {
		args = append(args, tpl.ToolkitArgs...)
	} else {
		args = append(args, tpl.NoToolkitArgs...)
	}
	subs := map[string]string{
		"{{INSTRUCTIONS}}": instructions,
		"{{CONTEXT}}":      contextFile,
		"{{MCP_CONFIG}}":   mcpConfig,
		"{{RUN_DIR}}":      dir,
		"{{MODEL}}":        in.Model,
		"{{MCP_TOOLS}}":    mcpToolPatterns(in.Toolkits),
		// The texts themselves, for a harness that takes its prompt as an
		// argument rather than a file or stdin.
		"{{CONTEXT_TEXT}}": in.Context,
		"{{PROMPT}}":       in.Instructions + "\n\n" + in.Context,
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
	cmd.Stdin = strings.NewReader(in.Context)
	cmd.Env = scrubbedEnv(tpl.Env)
	cmd.SysProcAttr = &syscall.SysProcAttr{Setpgid: true}
	cmd.Cancel = func() error {
		// The harness may have children of its own (an MCP server, a
		// shell): the whole process group goes.
		return syscall.Kill(-cmd.Process.Pid, syscall.SIGKILL)
	}
	var out, errb bytes.Buffer
	cmd.Stdout, cmd.Stderr = &out, &errb
	if err := cmd.Run(); err != nil {
		if cctx.Err() != nil {
			return "", fmt.Errorf("harness %s: deadline of %s exceeded", tpl.Command, deadline)
		}
		return "", fmt.Errorf("harness %s: %w: %s", tpl.Command, err, strings.TrimSpace(errb.String()))
	}
	text := strings.TrimSpace(out.String())
	if text == "" {
		return "", ErrNoOutput
	}
	return text, nil
}

// mcpConfigFor writes the toolkits in the MCP configuration shape the
// coding agents read: {"mcpServers": {name: {command, args, env}}}.
func mcpConfigFor(toolkits []Toolkit) []byte {
	servers := map[string]any{}
	for _, t := range toolkits {
		s := map[string]any{"command": t.Command}
		if len(t.Args) > 0 {
			s["args"] = t.Args
		}
		if len(t.Env) > 0 {
			s["env"] = t.Env
		}
		servers[t.Name] = s
	}
	b, _ := json.MarshalIndent(map[string]any{"mcpServers": servers}, "", "  ")
	return b
}

// mcpToolPatterns names the toolkits' tools for a harness that takes an
// allow-list: mcp__<name> per toolkit, comma-separated.
func mcpToolPatterns(toolkits []Toolkit) string {
	names := make([]string, 0, len(toolkits))
	for _, t := range toolkits {
		names = append(names, "mcp__"+t.Name)
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
	for k, v := range extra {
		env = append(env, k+"="+v)
	}
	return env
}
