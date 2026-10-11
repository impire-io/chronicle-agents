package cli

import (
	"bytes"
	"context"
	"strings"
	"testing"

	"github.com/impire-io/chronicle-agents/internal/chrontest"
	"github.com/impire-io/chronicle/devdir"
)

// store init makes the workspace from nothing, and again without
// complaint; schedule create registers a schedule in the tick store.
func TestStoreInitAndScheduleCreate(t *testing.T) {
	l := chrontest.Up(t)
	conn := []string{"--url", l.URL, "--nkey", devdir.UserNkeyPath(l.Dir), "--principal", "admin"}
	var out bytes.Buffer
	if err := Run(context.Background(), append([]string{"store", "init"}, conn...), &out); err != nil {
		t.Fatalf("store init: %v\n%s", err, out.String())
	}
	if s := out.String(); !strings.Contains(s, "store rooms created") || !strings.Contains(s, "types room and message declared") || !strings.Contains(s, "store ticks created (schedules allowed)") {
		t.Errorf("first init said:\n%s", s)
	}
	out.Reset()
	if err := Run(context.Background(), append([]string{"store", "init"}, conn...), &out); err != nil {
		t.Fatalf("second store init: %v\n%s", err, out.String())
	}
	if s := out.String(); strings.Contains(s, "created") || !strings.Contains(s, "store ticks present") {
		t.Errorf("second init said:\n%s", s)
	}
	out.Reset()
	if err := Run(context.Background(), append([]string{"schedule", "create", "hourly", "--every", "1h", "--ttl", "10m"}, conn...), &out); err != nil {
		t.Fatalf("schedule create: %v\n%s", err, out.String())
	}
	if s := out.String(); !strings.Contains(s, "schedule hourly registered in ticks (@every 1h0m0s)") {
		t.Errorf("schedule create said:\n%s", s)
	}
	if err := Run(context.Background(), append([]string{"schedule", "create", "x"}, conn...), &out); err == nil || !strings.Contains(err.Error(), "--every D or --cron EXPR") {
		t.Errorf("schedule without a pattern: err = %v", err)
	}
}
