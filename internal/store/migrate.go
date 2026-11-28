package store

import (
	"cmp"
	"context"
	"embed"
	"fmt"
	"io/fs"
	"path"
	"slices"
	"strconv"
	"strings"
	"time"
)

//go:embed migrations/*.sql
var migrationsFS embed.FS

type sqlMigration struct {
	version int
	name    string
	stmts   []string
}

func loadEmbeddedMigrations() ([]sqlMigration, error) {
	entries, err := fs.ReadDir(migrationsFS, "migrations")
	if err != nil {
		return nil, fmt.Errorf("read embedded migrations: %w", err)
	}

	migs := make([]sqlMigration, 0, len(entries))
	for _, entry := range entries {
		if entry.IsDir() || !strings.HasSuffix(entry.Name(), ".sql") {
			continue
		}
		verStr, _, ok := strings.Cut(entry.Name(), "_")
		if !ok {
			return nil, fmt.Errorf("invalid migration filename %q: %w", entry.Name(), ErrInvalidConfig)
		}
		ver, err := strconv.Atoi(verStr)
		if err != nil || ver <= 0 {
			return nil, fmt.Errorf("invalid migration version %q: %w", entry.Name(), ErrInvalidConfig)
		}
		raw, err := fs.ReadFile(migrationsFS, path.Join("migrations", entry.Name()))
		if err != nil {
			return nil, fmt.Errorf("read migration %q: %w", entry.Name(), err)
		}
		migs = append(migs, sqlMigration{
			version: ver,
			name:    entry.Name(),
			stmts:   splitSQLStatements(string(raw)),
		})
	}

	slices.SortFunc(migs, func(a, b sqlMigration) int {
		return cmp.Compare(a.version, b.version)
	})
	return migs, nil
}

func splitSQLStatements(sqlText string) []string {
	var out []string
	for part := range strings.SplitSeq(sqlText, ";") {
		trimmed := strings.TrimSpace(part)
		if trimmed == "" {
			continue
		}
		out = append(out, trimmed)
	}
	return out
}

func migrateAndSeed(ctx context.Context, exec executor, nowFn func() time.Time) error {
	if _, err := exec.ExecBatch(ctx, []stmt{{
		query: `CREATE TABLE IF NOT EXISTS schema_migrations (
			version INTEGER PRIMARY KEY,
			name TEXT NOT NULL,
			applied_at TEXT NOT NULL
		)`,
	}}); err != nil {
		return fmt.Errorf("ensure schema_migrations: %w", err)
	}

	appliedRows, err := exec.Query(ctx, stmt{query: "SELECT version FROM schema_migrations"})
	if err != nil {
		return fmt.Errorf("query applied migrations: %w", err)
	}
	applied := make(map[int]bool, len(appliedRows))
	for _, r := range appliedRows {
		applied[int(toInt64(r[0]))] = true
	}

	migs, err := loadEmbeddedMigrations()
	if err != nil {
		return err
	}

	nowStr := formatTimeUTC(nowFn())
	for _, m := range migs {
		if applied[m.version] {
			continue
		}
		batch := make([]stmt, 0, len(m.stmts)+1)
		for _, q := range m.stmts {
			batch = append(batch, stmt{query: q})
		}
		batch = append(batch, stmt{
			query: "INSERT INTO schema_migrations (version, name, applied_at) VALUES (?, ?, ?)",
			args:  []any{m.version, m.name, nowStr},
		})
		if _, err := exec.ExecBatch(ctx, batch); err != nil {
			return fmt.Errorf("apply migration %s: %w", m.name, err)
		}
	}

	return seedBuiltInBots(ctx, exec, nowStr)
}

func seedBuiltInBots(ctx context.Context, exec executor, nowStr string) error {
	batch := make([]stmt, 0, BotAccountCount*2)
	for i := range BotAccountCount {
		name := fmt.Sprintf("%s%d", BotUsernamePrefix, i)
		lower := strings.ToLower(name)
		batch = append(batch,
			stmt{
				query: `INSERT OR IGNORE INTO accounts
					(username, username_lower, password_hash, balance, created_at)
					VALUES (?, ?, ?, ?, ?)`,
				args: []any{name, lower, BotUnmatchableHash, InitialBalance, nowStr},
			},
			stmt{
				query: `INSERT OR IGNORE INTO account_roles (account_id, role_name)
					SELECT id, ? FROM accounts WHERE username_lower = ?`,
				args: []any{RoleBot.String(), lower},
			},
		)
	}
	if _, err := exec.ExecBatch(ctx, batch); err != nil {
		return fmt.Errorf("seed built-in bot accounts: %w", err)
	}
	return nil
}
