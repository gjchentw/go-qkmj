package store

import (
	"errors"
	"path/filepath"
	"testing"
	"time"
)

func TestSQLiteExecutor_MigrationsSeedIdempotencyAndRollback(t *testing.T) {
	t.Parallel()

	ctx := t.Context()
	fixedNow := func() time.Time { return time.Date(2026, 11, 28, 12, 0, 0, 0, time.UTC) }

	t.Run("empty dsn rejected", func(t *testing.T) {
		t.Parallel()
		if _, err := newSQLiteExecutor(ctx, ""); !errors.Is(err, ErrInvalidConfig) {
			t.Fatalf("newSQLiteExecutor(\"\") err = %v, want ErrInvalidConfig", err)
		}
	})

	t.Run("file sqlite enables WAL foreign_keys and is idempotent across reopen", func(t *testing.T) {
		t.Parallel()
		dbPath := filepath.Join(t.TempDir(), "qkmj.db")

		for attempt := range 2 {
			exec, err := newSQLiteExecutor(ctx, dbPath)
			if err != nil {
				t.Fatalf("attempt %d newSQLiteExecutor: %v", attempt, err)
			}
			if err := migrateAndSeed(ctx, exec, fixedNow); err != nil {
				_ = exec.Close()
				t.Fatalf("attempt %d migrateAndSeed: %v", attempt, err)
			}

			// Verify PRAGMA foreign_keys = 1 and journal_mode = wal.
			fkRows, err := exec.Query(ctx, stmt{query: "PRAGMA foreign_keys"})
			if err != nil || len(fkRows) != 1 || toInt64(fkRows[0][0]) != 1 {
				_ = exec.Close()
				t.Fatalf("PRAGMA foreign_keys = %v, err=%v; want 1", fkRows, err)
			}
			jmRows, err := exec.Query(ctx, stmt{query: "PRAGMA journal_mode"})
			if err != nil || len(jmRows) != 1 || toString(jmRows[0][0]) != "wal" {
				_ = exec.Close()
				t.Fatalf("PRAGMA journal_mode = %v, err=%v; want wal", jmRows, err)
			}

			// Verify exactly 1 migration applied and 10 bot accounts seeded.
			migRows, err := exec.Query(ctx, stmt{query: "SELECT COUNT(*) FROM schema_migrations"})
			if err != nil || toInt64(migRows[0][0]) != 1 {
				_ = exec.Close()
				t.Fatalf("schema_migrations count = %v, err=%v; want 1", migRows, err)
			}
			botRows, err := exec.Query(ctx, stmt{
				query: `SELECT a.username, a.balance, a.password_hash, r.role_name
					FROM accounts a
					JOIN account_roles r ON r.account_id = a.id
					WHERE r.role_name = ?
					ORDER BY a.username_lower`,
				args: []any{RoleBot.String()},
			})
			if err != nil || len(botRows) != BotAccountCount {
				_ = exec.Close()
				t.Fatalf("bot accounts len = %d, err=%v; want %d", len(botRows), err, BotAccountCount)
			}
			if toString(botRows[0][0]) != "QKMJ0" || toInt64(botRows[0][1]) != InitialBalance || toString(botRows[0][2]) != BotUnmatchableHash {
				_ = exec.Close()
				t.Fatalf("first bot row = %+v, want QKMJ0/%d/!", botRows[0], InitialBalance)
			}
			if err := exec.Close(); err != nil {
				t.Fatalf("Close: %v", err)
			}
		}
	})

	t.Run("ExecBatch rolls back entire transaction when second statement fails", func(t *testing.T) {
		t.Parallel()
		exec, err := newSQLiteExecutor(ctx, ":memory:")
		if err != nil {
			t.Fatalf("newSQLiteExecutor(:memory:): %v", err)
		}
		defer func() { _ = exec.Close() }()

		if err := migrateAndSeed(ctx, exec, fixedNow); err != nil {
			t.Fatalf("migrateAndSeed: %v", err)
		}

		// Batch inserts a valid settlement then violates FK by inserting a role for non-existent account 999999.
		_, err = exec.ExecBatch(ctx, []stmt{
			{
				query: "INSERT INTO hand_settlements (hand_id, node_name, end_reason, entries_json, settled_at) VALUES (?, ?, ?, ?, ?)",
				args:  []any{"h-rollback", "node-1", "win", "[]", fixedNow().Format(time.RFC3339Nano)},
			},
			{
				query: "INSERT INTO account_roles (account_id, role_name) VALUES (?, ?)",
				args:  []any{int64(999999), RolePlayer.String()},
			},
		})
		if err == nil {
			t.Fatal("expected ExecBatch error on FK violation, got nil")
		}

		rows, err := exec.Query(ctx, stmt{query: "SELECT COUNT(*) FROM hand_settlements WHERE hand_id = ?", args: []any{"h-rollback"}})
		if err != nil {
			t.Fatalf("verify rollback query: %v", err)
		}
		if got := toInt64(rows[0][0]); got != 0 {
			t.Fatalf("hand_settlements count after rollback = %d, want 0", got)
		}
	})
}
