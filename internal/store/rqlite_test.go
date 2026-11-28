package store

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"sync"
	"testing"
	"time"
)

// fakeRqliteServer implements the rqlite v8 HTTP JSON API backed by an in-memory SQLite executor
// so both wire-level unit tests and full Store contract tests execute real SQL atomically.
type fakeRqliteServer struct {
	server *httptest.Server
	exec   *sqliteExecutor

	mu           sync.Mutex
	lastQueryURL string
	lastExecURL  string
	lastExecBody [][]any
}

func newFakeRqliteServer(ctx context.Context, t *testing.T) *fakeRqliteServer {
	t.Helper()
	exec, err := newSQLiteExecutor(ctx, ":memory:")
	if err != nil {
		t.Fatalf("newSQLiteExecutor: %v", err)
	}
	f := &fakeRqliteServer{exec: exec}

	mux := http.NewServeMux()
	mux.HandleFunc("POST /db/execute", f.handleExecute)
	mux.HandleFunc("POST /db/query", f.handleQuery)
	f.server = httptest.NewServer(mux)
	t.Cleanup(func() {
		f.server.Close()
		_ = exec.Close()
	})
	return f
}

func writeJSONResponse(w http.ResponseWriter, payload any) {
	w.Header().Set("Content-Type", "application/json")
	if err := json.NewEncoder(w).Encode(payload); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
	}
}

func (f *fakeRqliteServer) handleExecute(w http.ResponseWriter, r *http.Request) {
	stmts, raw, err := decodeRqliteStatements(r)
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	f.mu.Lock()
	f.lastExecURL = r.URL.RequestURI()
	f.lastExecBody = raw
	f.mu.Unlock()

	results, err := f.exec.ExecBatch(r.Context(), stmts)
	if err != nil {
		writeJSONResponse(w, map[string]any{
			"results": []map[string]any{{"error": err.Error()}},
		})
		return
	}
	out := make([]map[string]any, len(results))
	for i, res := range results {
		out[i] = map[string]any{
			"last_insert_id": res.lastInsertID,
			"rows_affected":  res.rowsAffected,
		}
	}
	writeJSONResponse(w, map[string]any{"results": out})
}

func (f *fakeRqliteServer) handleQuery(w http.ResponseWriter, r *http.Request) {
	stmts, _, err := decodeRqliteStatements(r)
	if err != nil || len(stmts) != 1 {
		http.Error(w, "invalid query batch", http.StatusBadRequest)
		return
	}
	f.mu.Lock()
	f.lastQueryURL = r.URL.RequestURI()
	f.mu.Unlock()

	rows, err := f.exec.Query(r.Context(), stmts[0])
	if err != nil {
		writeJSONResponse(w, map[string]any{
			"results": []map[string]any{{"error": err.Error()}},
		})
		return
	}
	writeJSONResponse(w, map[string]any{
		"results": []map[string]any{{"values": rows}},
	})
}

func decodeRqliteStatements(r *http.Request) ([]stmt, [][]any, error) {
	dec := json.NewDecoder(r.Body)
	dec.UseNumber()
	var raw [][]any
	if err := dec.Decode(&raw); err != nil {
		return nil, nil, err
	}
	stmts := make([]stmt, 0, len(raw))
	for _, entry := range raw {
		if len(entry) == 0 {
			return nil, nil, errors.New("empty statement array")
		}
		q, ok := entry[0].(string)
		if !ok {
			return nil, nil, errors.New("statement query must be string")
		}
		args := make([]any, len(entry)-1)
		for i, v := range entry[1:] {
			if num, ok := v.(json.Number); ok {
				if n, err := num.Int64(); err == nil {
					args[i] = n
					continue
				}
				if fl, err := num.Float64(); err == nil {
					args[i] = fl
					continue
				}
			}
			args[i] = v
		}
		stmts = append(stmts, stmt{query: q, args: args})
	}
	return stmts, raw, nil
}

func TestRqliteExecutor_WireProtocolRedirectFailoverAndTimeout(t *testing.T) {
	t.Parallel()

	t.Run("invalid endpoints or consistency rejected", func(t *testing.T) {
		t.Parallel()
		if _, err := newRqliteExecutor(nil, ReadConsistencyWeak, time.Second, nil); !errors.Is(err, ErrInvalidConfig) {
			t.Fatalf("nil endpoints err = %v, want ErrInvalidConfig", err)
		}
		if _, err := newRqliteExecutor([]string{"http://127.0.0.1:4001"}, ReadConsistencyUnknown, time.Second, nil); !errors.Is(err, ErrInvalidConfig) {
			t.Fatalf("unknown consistency err = %v, want ErrInvalidConfig", err)
		}
	})

	t.Run("sends atomic transaction=true batch, respects read consistency, and surfaces statement errors", func(t *testing.T) {
		t.Parallel()
		ctx := t.Context()
		fake := newFakeRqliteServer(ctx, t)
		exec, err := newRqliteExecutor([]string{fake.server.URL}, ReadConsistencyStrong, 2*time.Second, fake.server.Client())
		if err != nil {
			t.Fatalf("newRqliteExecutor: %v", err)
		}
		defer func() { _ = exec.Close() }()

		fixedNow := func() time.Time { return time.Date(2026, 11, 28, 12, 0, 0, 0, time.UTC) }
		if err := migrateAndSeed(ctx, exec, fixedNow); err != nil {
			t.Fatalf("migrateAndSeed over rqlite: %v", err)
		}

		fake.mu.Lock()
		execURL := fake.lastExecURL
		fake.mu.Unlock()
		if execURL != "/db/execute?transaction=true" {
			t.Fatalf("lastExecURL = %q, want /db/execute?transaction=true", execURL)
		}

		rows, err := exec.Query(ctx, stmt{query: "SELECT username, balance FROM accounts WHERE username_lower = ?", args: []any{"qkmj0"}})
		if err != nil {
			t.Fatalf("Query over rqlite: %v", err)
		}
		fake.mu.Lock()
		queryURL := fake.lastQueryURL
		fake.mu.Unlock()
		if queryURL != "/db/query?level=strong" {
			t.Fatalf("lastQueryURL = %q, want /db/query?level=strong", queryURL)
		}
		if len(rows) != 1 || toString(rows[0][0]) != "QKMJ0" || toInt64(rows[0][1]) != InitialBalance {
			t.Fatalf("query rows = %+v, want QKMJ0/%d", rows, InitialBalance)
		}

		// Statement error inside batch MUST fail ExecBatch (not be swallowed).
		_, err = exec.ExecBatch(ctx, []stmt{
			{query: "INSERT INTO schema_migrations (version, name, applied_at) VALUES (?, ?, ?)", args: []any{1, "dup", "2026-11-28T00:00:00Z"}},
		})
		if err == nil || !isUniqueConstraintErr(err) {
			t.Fatalf("duplicate PK err = %v, want unique constraint error", err)
		}
	})

	t.Run("follows HTTP 301 leader redirect preserving POST JSON body", func(t *testing.T) {
		t.Parallel()
		ctx := t.Context()
		leader := newFakeRqliteServer(ctx, t)
		follower := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			http.Redirect(w, r, leader.server.URL+r.URL.RequestURI(), http.StatusMovedPermanently)
		}))
		defer follower.Close()

		exec, err := newRqliteExecutor([]string{follower.URL}, ReadConsistencyWeak, 2*time.Second, follower.Client())
		if err != nil {
			t.Fatalf("newRqliteExecutor: %v", err)
		}
		defer func() { _ = exec.Close() }()

		fixedNow := func() time.Time { return time.Date(2026, 11, 28, 12, 0, 0, 0, time.UTC) }
		if err := migrateAndSeed(ctx, exec, fixedNow); err != nil {
			t.Fatalf("migrateAndSeed via 301 follower redirect: %v", err)
		}
		rows, err := exec.Query(ctx, stmt{query: "SELECT COUNT(*) FROM accounts"})
		if err != nil || toInt64(rows[0][0]) != BotAccountCount {
			t.Fatalf("accounts after redirect = %v, err=%v; want %d", rows, err, BotAccountCount)
		}
	})

	t.Run("fails over to healthy endpoint when first endpoint is unreachable", func(t *testing.T) {
		t.Parallel()
		ctx := t.Context()
		healthy := newFakeRqliteServer(ctx, t)
		dead := httptest.NewServer(http.HandlerFunc(func(http.ResponseWriter, *http.Request) {}))
		deadURL := dead.URL
		dead.Close()

		exec, err := newRqliteExecutor([]string{deadURL, healthy.server.URL}, ReadConsistencyWeak, 2*time.Second, healthy.server.Client())
		if err != nil {
			t.Fatalf("newRqliteExecutor: %v", err)
		}
		defer func() { _ = exec.Close() }()

		fixedNow := func() time.Time { return time.Date(2026, 11, 28, 12, 0, 0, 0, time.UTC) }
		if err := migrateAndSeed(ctx, exec, fixedNow); err != nil {
			t.Fatalf("migrateAndSeed with failover: %v", err)
		}
	})

	t.Run("respects context timeout without hanging", func(t *testing.T) {
		t.Parallel()
		done := make(chan struct{})
		slow := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			select {
			case <-done:
			case <-r.Context().Done():
			}
			w.WriteHeader(http.StatusOK)
		}))
		defer func() {
			close(done)
			slow.Close()
		}()

		exec, err := newRqliteExecutor([]string{slow.URL}, ReadConsistencyWeak, 50*time.Millisecond, slow.Client())
		if err != nil {
			t.Fatalf("newRqliteExecutor: %v", err)
		}
		defer func() { _ = exec.Close() }()

		shortCtx, cancel := context.WithTimeout(t.Context(), 80*time.Millisecond)
		defer cancel()
		if _, err := exec.Query(shortCtx, stmt{query: "SELECT 1"}); err == nil {
			t.Fatal("expected timeout error on slow rqlite server, got nil")
		}
	})
}
