package main

import (
	"context"
	"encoding/json"
	"io"
	"log/slog"
	"net/http"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestParseConfig_FlagsEnvDefaultsAndRandomNodeName(t *testing.T) {
	t.Parallel()

	envMap := map[string]string{
		"QKMJ_STORE_DRIVER":       "rqlite",
		"QKMJ_STORE_DSN":          "http://rqlite-1:4001, http://rqlite-2:4001",
		"QKMJ_RQLITE_CONSISTENCY": "strong",
		"QKMJ_NODE_NAME":          "env-node",
	}
	envFn := func(k string) string { return envMap[k] }

	cfg, err := parseConfig(nil, envFn)
	if err != nil {
		t.Fatalf("parseConfig(env): %v", err)
	}
	if cfg.storeDriver != "rqlite" || cfg.nodeName != "env-node" || cfg.rqliteConsistency != "strong" {
		t.Fatalf("cfg = %+v", cfg)
	}

	// Flag overrides env.
	cfg2, err := parseConfig([]string{"--node-name=flag-node", "--store-driver=sqlite", "--store-dsn=:memory:"}, envFn)
	if err != nil {
		t.Fatalf("parseConfig(flags): %v", err)
	}
	if cfg2.nodeName != "flag-node" || cfg2.storeDriver != "sqlite" || cfg2.storeDSN != ":memory:" {
		t.Fatalf("cfg2 = %+v", cfg2)
	}

	// Empty env & flags generates a readable Docker-style random Node Name (adjective-noun).
	cfg3, err := parseConfig(nil, func(string) string { return "" })
	if err != nil {
		t.Fatalf("parseConfig(defaults): %v", err)
	}
	if !strings.Contains(cfg3.nodeName, "-") || len(cfg3.nodeName) < 5 {
		t.Fatalf("generated nodeName = %q, want Docker-style adjective-noun", cfg3.nodeName)
	}
}

func TestRun_SQLiteBootstrapAndGracefulShutdown(t *testing.T) {
	t.Parallel()

	dbPath := filepath.Join(t.TempDir(), "qkmj.db")
	ctx, cancel := context.WithCancel(t.Context())
	defer cancel()

	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	readyCh := make(chan string, 1)
	errCh := make(chan error, 1)
	go func() {
		errCh <- runWithReady(ctx, []string{
			"--store-driver=sqlite",
			"--store-dsn=" + dbPath,
			"--node-name=test-node",
			"--health-addr=127.0.0.1:0",
		}, func(string) string { return "" }, logger, readyCh)
	}()

	var boundAddr string
	select {
	case boundAddr = <-readyCh:
	case err := <-errCh:
		t.Fatalf("run exited early before ready: %v", err)
	case <-time.After(5 * time.Second):
		t.Fatal("timed out waiting for qkmj startup")
	}

	req, err := http.NewRequestWithContext(t.Context(), http.MethodGet, "http://"+boundAddr+"/healthz", http.NoBody)
	if err != nil {
		t.Fatalf("build healthz request: %v", err)
	}
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("GET /healthz: %v", err)
	}
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("GET /healthz status = %d, want 200", resp.StatusCode)
	}
	var payload map[string]string
	if err := json.NewDecoder(resp.Body).Decode(&payload); err != nil {
		t.Fatalf("decode /healthz: %v", err)
	}
	if payload["status"] != "ok" || payload["node"] != "test-node" || payload["driver"] != "sqlite" {
		t.Fatalf("/healthz payload = %+v", payload)
	}

	cancel()
	select {
	case err := <-errCh:
		if err != nil {
			t.Fatalf("run returned unexpected error on graceful shutdown: %v", err)
		}
	case <-time.After(3 * time.Second):
		t.Fatal("run did not shut down within 3s after context cancel")
	}
}

func TestRun_RejectsUnsupportedDriver(t *testing.T) {
	t.Parallel()

	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	err := run(t.Context(), []string{"--store-driver=postgres"}, func(string) string { return "" }, logger)
	if err == nil {
		t.Fatal("expected error for unsupported store driver, got nil")
	}
}
