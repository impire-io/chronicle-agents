package cli

import (
	"bytes"
	"context"
	"encoding/json"
	"net"
	"net/http"
	"strings"
	"testing"
	"time"

	"github.com/impire-io/chronicle/bridge"
)

// The hosted form needs no account: the person chooses among the accounts
// they hold, and /config.json carries the profile and nothing of an
// account; with --account the interface is pinned to it.
func TestWebHostedServesWithoutAnAccount(t *testing.T) {
	for _, tc := range []struct {
		name    string
		args    []string
		account string
		says    string
	}{
		{"unpinned", nil, "", "chooses among the accounts they hold"},
		{"pinned", []string{"--account", "acme"}, "acme", "pinned to account acme"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			ln, err := net.Listen("tcp", "127.0.0.1:0")
			if err != nil {
				t.Fatal(err)
			}
			addr := ln.Addr().String()
			_ = ln.Close()
			ctx, cancel := context.WithCancel(context.Background())
			defer cancel()
			var out bytes.Buffer
			done := make(chan error, 1)
			go func() {
				done <- Run(ctx, append([]string{"web", "--hosted", "--listen", addr, "--profile", "https://example.test/.well-known/chronicle/profile.json"}, tc.args...), &out)
			}()
			var cfg map[string]any
			deadline := time.Now().Add(10 * time.Second)
			for {
				res, err := http.Get("http://" + addr + "/config.json")
				if err == nil {
					err = json.NewDecoder(res.Body).Decode(&cfg)
					_ = res.Body.Close()
					if err == nil {
						break
					}
				}
				if time.Now().After(deadline) {
					t.Fatalf("the interface never answered: %v\n%s", err, out.String())
				}
				time.Sleep(50 * time.Millisecond)
			}
			cancel()
			if err := <-done; err != nil {
				t.Fatalf("web: %v\n%s", err, out.String())
			}
			if cfg["mode"] != "hosted" || cfg["profile"] != "https://example.test/.well-known/chronicle/profile.json" {
				t.Errorf("config = %v", cfg)
			}
			if got, _ := cfg["account"].(string); got != tc.account {
				t.Errorf("account = %q, want %q", got, tc.account)
			}
			if s := out.String(); !strings.Contains(s, tc.says) || !strings.Contains(s, "store init --interface") {
				t.Errorf("web said:\n%s", s)
			}
		})
	}
}

// The hosted interface's address follows the site the profile names;
// a profile without a web-flow sign-in names no site.
func TestInterfaceForFollowsTheSite(t *testing.T) {
	if got := InterfaceFor(bridge.Profile{GithubTokenURL: "https://chronicle.impire.dev/auth/github/token"}); got != "https://agents.chronicle.impire.dev" {
		t.Errorf("hosted: %q", got)
	}
	if got := InterfaceFor(bridge.Profile{GithubTokenURL: "https://chronicle.example.test:8443/auth/github/token"}); got != "https://agents.chronicle.example.test:8443" {
		t.Errorf("another install: %q", got)
	}
	if got := InterfaceFor(bridge.Profile{URL: "nats://127.0.0.1:4222"}); got != "" {
		t.Errorf("no sign-in: %q", got)
	}
}
