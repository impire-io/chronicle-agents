package web

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
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
