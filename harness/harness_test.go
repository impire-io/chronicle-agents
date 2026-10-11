package harness

import (
	"context"
	"encoding/json"
	"os"
	"strings"
	"testing"
	"time"
)

func TestInvokeRunsACommandOnTheContext(t *testing.T) {
	tpl := Template{Command: "tail", Args: []string{"-n", "1"}}
	text, err := Invoke(context.Background(), tpl, Input{Agent: "wilson", Context: "first line\nlast line\n"})
	if err != nil {
		t.Fatal(err)
	}
	if text != "last line" {
		t.Errorf("text = %q", text)
	}
}

func TestInvokeWritesTheFilesAndSubstitutes(t *testing.T) {
	// cat the instructions file and the mcp config: the placeholders resolve to files in the run dir.
	tpl := Template{Command: "sh", Args: []string{"-c", `cat "$1"; echo; cat "$2"; echo; echo "model=$3"`, "sh", "{{INSTRUCTIONS}}", "{{MCP_CONFIG}}", "{{MODEL}}"}}
	in := Input{Agent: "wilson", Instructions: "be brief", Model: "m1", Context: "x",
		Toolkits: []Toolkit{{Name: "invoices", Command: "inv", Args: []string{"--prod"}}}}
	text, err := Invoke(context.Background(), tpl, in)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.HasPrefix(text, "be brief") || !strings.HasSuffix(text, "model=m1") {
		t.Errorf("text = %q", text)
	}
	var cfg struct {
		Servers map[string]struct {
			Command string   `json:"command"`
			Args    []string `json:"args"`
		} `json:"mcpServers"`
	}
	body := text[len("be brief") : len(text)-len("model=m1")]
	if err := json.Unmarshal([]byte(body), &cfg); err != nil {
		t.Fatalf("mcp config: %v in %q", err, body)
	}
	if s, ok := cfg.Servers["invoices"]; !ok || s.Command != "inv" || s.Args[0] != "--prod" {
		t.Errorf("mcp config = %+v", cfg)
	}
}

func TestInvokeScrubsTheEnvironment(t *testing.T) {
	t.Setenv("CLAUDE_CODE_SESSION_ID", "leak")
	tpl := Template{Command: "sh", Args: []string{"-c", `echo "sid=${CLAUDE_CODE_SESSION_ID:-none} key=${PROVIDER_KEY:-none}"`}, Env: map[string]string{"PROVIDER_KEY": "k"}}
	text, err := Invoke(context.Background(), tpl, Input{Agent: "wilson", Context: "x"})
	if err != nil {
		t.Fatal(err)
	}
	if text != "sid=none key=k" {
		t.Errorf("text = %q", text)
	}
}

func TestInvokeKillsAtTheDeadline(t *testing.T) {
	tpl := Template{Command: "sh", Args: []string{"-c", "sleep 30; echo late"}}
	start := time.Now()
	_, err := Invoke(context.Background(), tpl, Input{Agent: "wilson", Context: "x", Deadline: 300 * time.Millisecond})
	if err == nil || !strings.Contains(err.Error(), "deadline") {
		t.Fatalf("err = %v, want a deadline error", err)
	}
	if time.Since(start) > 5*time.Second {
		t.Errorf("took %s: the process group was not killed", time.Since(start))
	}
}

func TestInvokeRefusesSilence(t *testing.T) {
	tpl := Template{Command: "true"}
	if _, err := Invoke(context.Background(), tpl, Input{Agent: "wilson", Context: "x"}); err != ErrNoOutput {
		t.Errorf("err = %v, want ErrNoOutput", err)
	}
}

func TestPresetsAndToolkitArgs(t *testing.T) {
	tpl, ok := Preset("claude-code")
	if !ok || tpl.Command != "claude" {
		t.Fatalf("preset = %+v", tpl)
	}
	if _, ok := Preset("nope"); ok {
		t.Error("unknown preset found")
	}
	if got := mcpToolPatterns([]Toolkit{{Name: "a"}, {Name: "b"}}); got != "mcp__a,mcp__b" {
		t.Errorf("patterns = %q", got)
	}
	if _, err := os.Stat("/nonexistent"); err == nil {
		t.Skip()
	}
}
