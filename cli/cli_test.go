package cli

import (
	"bytes"
	"context"
	"strings"
	"testing"
)

func TestHelpAndUnknown(t *testing.T) {
	var out bytes.Buffer
	if err := Run(context.Background(), nil, &out); err != nil || !strings.Contains(out.String(), "store init") {
		t.Errorf("help: err=%v out=%q", err, out.String())
	}
	if err := Run(context.Background(), []string{"dance"}, &out); err == nil {
		t.Error("unknown command accepted")
	}
}

func TestRunWantsOneFileAndTakesFlagsAfterIt(t *testing.T) {
	var out bytes.Buffer
	if err := Run(context.Background(), []string{"run"}, &out); err == nil || !strings.Contains(err.Error(), "one definition file") {
		t.Errorf("err = %v", err)
	}
	if err := Run(context.Background(), []string{"run", "a.yaml", "b.yaml"}, &out); err == nil || !strings.Contains(err.Error(), "one definition file") {
		t.Errorf("err = %v", err)
	}
	// Flags after the file are parsed: the file is read before any connection.
	err := Run(context.Background(), []string{"run", "nonexistent.yaml", "--context", "nope"}, &out)
	if err == nil || !strings.Contains(err.Error(), "nonexistent.yaml") {
		t.Errorf("err = %v, want the file named", err)
	}
}
