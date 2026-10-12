package web

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/impire-io/chronicle-agents/types"
)

func TestHandlerServesTheConfigurationAndTheApplication(t *testing.T) {
	h, err := Handler(Config{Mode: "local", Servers: "ws://127.0.0.1:9222", Author: "admin", Store: "rooms"})
	if err != nil {
		t.Fatal(err)
	}
	srv := httptest.NewServer(h)
	defer srv.Close()

	res, err := http.Get(srv.URL + "/config.json")
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = res.Body.Close() }()
	var cfg Config
	if err := json.NewDecoder(res.Body).Decode(&cfg); err != nil {
		t.Fatal(err)
	}
	if cfg.Mode != "local" || cfg.Servers != "ws://127.0.0.1:9222" || cfg.Store != "rooms" {
		t.Errorf("config = %+v", cfg)
	}
	if res.Header.Get("Cache-Control") != "no-store" {
		t.Error("the configuration is cached")
	}

	// Hosted, the configuration carries the profile and, pinned, the account; unpinned, no account at all.
	for _, tc := range []struct {
		cfg  Config
		want string
	}{
		{Config{Mode: "hosted", Profile: "https://chronicle.example.test/p.json", Store: "rooms"}, `{"mode":"hosted","profile":"https://chronicle.example.test/p.json","store":"rooms"}`},
		{Config{Mode: "hosted", Profile: "https://chronicle.example.test/p.json", Account: "acme", Store: "rooms"}, `{"mode":"hosted","profile":"https://chronicle.example.test/p.json","account":"acme","store":"rooms"}`},
	} {
		hh, err := Handler(tc.cfg)
		if err != nil {
			t.Fatal(err)
		}
		hs := httptest.NewServer(hh)
		res, err := http.Get(hs.URL + "/config.json")
		if err != nil {
			t.Fatal(err)
		}
		body := make([]byte, 1024)
		n, _ := res.Body.Read(body)
		_ = res.Body.Close()
		hs.Close()
		if got := strings.TrimSpace(string(body[:n])); got != tc.want {
			t.Errorf("hosted config = %s, want %s", got, tc.want)
		}
	}

	// The application is served at / and at any address it owns.
	for _, p := range []string{"/", "/rooms/close-w41"} {
		res, err := http.Get(srv.URL + p)
		if err != nil {
			t.Fatal(err)
		}
		body := make([]byte, 4096)
		n, _ := res.Body.Read(body)
		_ = res.Body.Close()
		if res.StatusCode != 200 || !strings.Contains(strings.ToLower(string(body[:n])), "<!doctype html>") {
			t.Errorf("%s: status %d, body %q", p, res.StatusCode, string(body[:n]))
		}
	}
}

// /install.json is the workspace as design 14's install manifest, with the
// binary's version and the public address, answering any origin.
func TestHandlerServesTheInstallManifest(t *testing.T) {
	h, err := Handler(Config{Mode: "hosted", Profile: "https://chronicle.example.test/p.json", Store: "rooms", TickStore: "ticks", Version: "1.2.3", PublicURL: "https://agents.example.test"})
	if err != nil {
		t.Fatal(err)
	}
	srv := httptest.NewServer(h)
	defer srv.Close()
	res, err := http.Get(srv.URL + "/install.json")
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = res.Body.Close() }()
	var m types.Manifest
	if err := json.NewDecoder(res.Body).Decode(&m); err != nil {
		t.Fatal(err)
	}
	if m.Manifest != 1 || m.Solution.Name != "chronicle-agents" || m.Solution.Version != "1.2.3" || m.Solution.URL != "https://agents.example.test" {
		t.Errorf("manifest = %+v", m)
	}
	if len(m.Stores) != 2 || m.Stores[0].Name != "rooms" || len(m.Stores[0].Types) != 3 || m.Stores[0].Types[0].Name != "message" || m.Stores[1].Name != "ticks" || !m.Stores[1].Schedules {
		t.Errorf("stores = %+v", m.Stores)
	}
	if m.Announce.Store != "rooms" || m.Announce.Path != "solution/chronicle-agents" {
		t.Errorf("announce = %+v", m.Announce)
	}
	if res.Header.Get("Access-Control-Allow-Origin") != "*" || res.Header.Get("Cache-Control") != "no-store" || res.Header.Get("Content-Type") != "application/json" {
		t.Errorf("headers = %v", res.Header)
	}
	// The browser's preflight is answered too.
	req, _ := http.NewRequest(http.MethodOptions, srv.URL+"/install.json", nil)
	pre, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	_ = pre.Body.Close()
	if pre.StatusCode != http.StatusNoContent || pre.Header.Get("Access-Control-Allow-Origin") != "*" {
		t.Errorf("preflight: %d %v", pre.StatusCode, pre.Header)
	}
	// /config.json carries neither the version nor the address.
	cres, err := http.Get(srv.URL + "/config.json")
	if err != nil {
		t.Fatal(err)
	}
	body := make([]byte, 1024)
	n, _ := cres.Body.Read(body)
	_ = cres.Body.Close()
	if s := string(body[:n]); strings.Contains(s, "1.2.3") || strings.Contains(s, "agents.example.test") {
		t.Errorf("config.json leaks the manifest's fields: %s", s)
	}
}
