// Package chrontest boots chronicle's own quick start for tests: a real
// embedded NATS server, one account, the node — never a mock.
package chrontest

import (
	"context"
	"io"
	"log/slog"
	"testing"

	"github.com/impire-io/chronicle/client"
	"github.com/impire-io/chronicle/devdir"
	"github.com/impire-io/chronicle/up"
)

// Local is a running quick start.
type Local struct {
	URL  string
	Dir  string
	stop func()
}

// Up boots a quick start on a free port in a temp dir, stopped with the
// test.
func Up(t *testing.T) *Local {
	t.Helper()
	dir := t.TempDir()
	l, err := up.Up(context.Background(), up.Config{
		Dir:    dir,
		Port:   -1,
		Logger: slog.New(slog.NewTextHandler(io.Discard, nil)),
	})
	if err != nil {
		t.Fatalf("chronicle up: %v", err)
	}
	t.Cleanup(l.Stop)
	return &Local{URL: l.URL, Dir: dir, stop: l.Stop}
}

// Connect opens a client on the quick start's user key, stating the author.
func (l *Local) Connect(t *testing.T, author string) *client.Client {
	t.Helper()
	c, err := client.ConnectNkeyFile(l.URL, devdir.UserNkeyPath(l.Dir), author)
	if err != nil {
		t.Fatalf("connect as %s: %v", author, err)
	}
	t.Cleanup(c.Close)
	return c
}
