package cli

import (
	"bytes"
	"context"
	"encoding/json"
	"reflect"
	"strings"
	"testing"
	"time"

	"github.com/impire-io/chronicle-agents/internal/chrontest"
	"github.com/impire-io/chronicle-agents/types"
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
	if s := out.String(); !strings.Contains(s, "store rooms created") || !strings.Contains(s, "types message, room and agent declared") || !strings.Contains(s, "room general created in store rooms") || !strings.Contains(s, "store ticks created (schedules allowed)") {
		t.Errorf("first init said:\n%s", s)
	}
	out.Reset()
	if err := Run(context.Background(), append([]string{"store", "init", "--interface", "https://agents.example.test"}, conn...), &out); err != nil {
		t.Fatalf("second store init: %v\n%s", err, out.String())
	}
	if s := out.String(); strings.Contains(s, "created") || strings.Contains(s, "room general") || !strings.Contains(s, "store ticks present") || !strings.Contains(s, "the rooms interface is at https://agents.example.test") {
		t.Errorf("second init said:\n%s", s)
	}
	// What store init made is what the manifest declares (decision 0008):
	// every store, with schedules where it says so; every type, defined as
	// the manifest's definition.
	admin := l.Connect(t, "admin")
	m := types.Declaration("rooms", "ticks").Manifest("0.0.0-test", "https://agents.example.test")
	for _, s := range m.Stores {
		cfg, err := admin.GetStore(context.Background(), s.Name)
		if err != nil {
			t.Fatalf("store %s: %v", s.Name, err)
		}
		if cfg.Schedules != s.Schedules {
			t.Errorf("store %s: schedules = %v, manifest says %v", s.Name, cfg.Schedules, s.Schedules)
		}
		for _, ty := range s.Types {
			rec, err := admin.GetType(context.Background(), s.Name, ty.Name)
			if err != nil {
				t.Fatalf("type %s in %s: %v", ty.Name, s.Name, err)
			}
			got, _ := json.Marshal(types.WireType{Schema: rec.Schema, History: rec.History, Children: rec.Children, Operations: rec.Operations})
			want, _ := json.Marshal(ty.Definition)
			if !jsonEqual(t, got, want) {
				t.Errorf("type %s in %s:\n defined %s\n manifest %s", ty.Name, s.Name, got, want)
			}
		}
	}
	if m.Announce.Store != "rooms" || m.Announce.Path != types.SolutionPath {
		t.Errorf("announce = %+v", m.Announce)
	}
	// --interface lands on the announcement, where the console reads the link.
	deadline := time.Now().Add(10 * time.Second)
	for {
		var d types.SolutionData
		if sv, err := admin.State(context.Background(), "rooms", types.SolutionPath); err == nil {
			_ = json.Unmarshal(sv.State, &d)
		}
		if d.URL == "https://agents.example.test" {
			break
		}
		if time.Now().After(deadline) {
			t.Fatalf("the announcement's url = %q", d.URL)
		}
		time.Sleep(100 * time.Millisecond)
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

// jsonEqual compares two JSON documents by value: key order and the
// schemas' whitespace are not part of a definition.
func jsonEqual(t *testing.T, a, b []byte) bool {
	t.Helper()
	var x, y any
	if err := json.Unmarshal(a, &x); err != nil {
		t.Fatalf("not JSON: %s", a)
	}
	if err := json.Unmarshal(b, &y); err != nil {
		t.Fatalf("not JSON: %s", b)
	}
	return reflect.DeepEqual(x, y)
}
