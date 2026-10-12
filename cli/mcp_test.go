package cli

import (
	"bytes"
	"context"
	"strings"
	"testing"
)

// The mcp sentence is the runner's, started for one turn with its
// configuration in the environment; by hand it says so.
func TestMCPWantsTheRunnersEnvironment(t *testing.T) {
	t.Setenv("CHRONICLE_AGENTS_URL", "")
	var out bytes.Buffer
	err := Run(context.Background(), []string{"mcp"}, &out)
	if err == nil || !strings.Contains(err.Error(), "CHRONICLE_AGENTS_URL") {
		t.Errorf("err = %v", err)
	}
	if err := Run(context.Background(), nil, &out); err != nil || !strings.Contains(out.String(), "chronicle-agents mcp") {
		t.Errorf("usage does not name mcp: %q", out.String())
	}
}
