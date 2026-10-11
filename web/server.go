// Package web serves the rooms interface: the static application built
// from _app/ into dist/ and embedded here, plus one dynamic document,
// /config.json, that tells the browser how to reach chronicle
// (chronicle-agents-hq design 03). The binary adds nothing else — the
// interface is a chronicle client that happens to run in a browser.
package web

import (
	"context"
	"embed"
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"net"
	"net/http"
	"strings"
	"time"
)

//go:embed all:dist
var dist embed.FS

// Config is what /config.json carries: connection facts, never data.
type Config struct {
	// Mode is "local" — a websocket and a credential over loopback, the
	// quick start's or a self-hosted context's — or "hosted" — the install
	// profile and the account; the browser asks for a GitHub token.
	Mode string `json:"mode"`
	// Servers is the websocket URL (local).
	Servers string `json:"servers,omitempty"`
	// NkeySeed is the user seed the context names (local, the quick start).
	NkeySeed string `json:"nkeySeed,omitempty"`
	// Creds is a credential file's contents (local, self-hosted).
	Creds string `json:"creds,omitempty"`
	// Author is the name the credential carries, when it carries one.
	Author string `json:"author,omitempty"`
	// Profile is the install profile's address (hosted).
	Profile string `json:"profile,omitempty"`
	// Account is the account to land in (hosted).
	Account string `json:"account,omitempty"`
	// Store is the rooms store.
	Store string `json:"store"`
	// TickStore is the tick store, shown beside the rooms.
	TickStore string `json:"tickStore,omitempty"`
}

// Handler serves the interface and its configuration.
func Handler(cfg Config) (http.Handler, error) {
	sub, err := fs.Sub(dist, "dist")
	if err != nil {
		return nil, err
	}
	files := http.FS(sub)
	fileServer := http.FileServer(files)
	body, err := json.Marshal(cfg)
	if err != nil {
		return nil, err
	}
	mux := http.NewServeMux()
	mux.HandleFunc("/config.json", func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.Header().Set("Cache-Control", "no-store")
		_, _ = w.Write(body)
	})
	mux.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		// A file the build produced is served as is; anything else is the
		// application, which reads the address itself.
		path := strings.TrimPrefix(r.URL.Path, "/")
		if path != "" {
			if f, err := sub.Open(path); err == nil {
				_ = f.Close()
				fileServer.ServeHTTP(w, r)
				return
			}
		}
		r.URL.Path = "/"
		fileServer.ServeHTTP(w, r)
	})
	return mux, nil
}

// Serve listens and serves until the context ends. The listen address is
// the caller's choice; loopback is the default because the local form
// hands the page a credential in the clear, as the quick start does.
func Serve(ctx context.Context, listen string, cfg Config) error {
	h, err := Handler(cfg)
	if err != nil {
		return err
	}
	ln, err := net.Listen("tcp", listen)
	if err != nil {
		return fmt.Errorf("listen %s: %w", listen, err)
	}
	srv := &http.Server{Handler: h, ReadHeaderTimeout: 10 * time.Second}
	go func() {
		<-ctx.Done()
		shutdown, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		_ = srv.Shutdown(shutdown)
	}()
	if err := srv.Serve(ln); err != nil && !errors.Is(err, http.ErrServerClosed) {
		return err
	}
	return nil
}

// Built reports whether the embedded build is the real application or the
// placeholder a source checkout carries before `make web`.
func Built() bool {
	entries, err := dist.ReadDir("dist")
	if err != nil {
		return false
	}
	for _, e := range entries {
		if e.Name() == "assets" && e.IsDir() {
			return true
		}
	}
	return false
}
