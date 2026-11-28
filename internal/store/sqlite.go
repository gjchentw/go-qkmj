package store

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"strings"

	// Register pure-Go SQLite database/sql driver.
	_ "modernc.org/sqlite"
)

type sqliteExecutor struct {
	db *sql.DB
}

var _ executor = (*sqliteExecutor)(nil)

func newSQLiteExecutor(ctx context.Context, dsn string) (*sqliteExecutor, error) {
	dsn = strings.TrimSpace(dsn)
	if dsn == "" {
		return nil, fmt.Errorf("sqlite dsn must not be empty: %w", ErrInvalidConfig)
	}

	db, err := sql.Open("sqlite", dsn)
	if err != nil {
		return nil, fmt.Errorf("open sqlite %q: %w", dsn, err)
	}
	// SQLite allows one writer at a time; pinning to 1 open connection ensures
	// per-connection PRAGMAs (foreign_keys, busy_timeout) and :memory: databases stay consistent.
	db.SetMaxOpenConns(1)
	db.SetMaxIdleConns(1)

	if err := db.PingContext(ctx); err != nil {
		_ = db.Close()
		return nil, fmt.Errorf("ping sqlite: %w", err)
	}

	pragmas := []string{
		"PRAGMA foreign_keys = ON",
		"PRAGMA busy_timeout = 5000",
	}
	if !isMemorySQLiteDSN(dsn) {
		pragmas = append(pragmas, "PRAGMA journal_mode = WAL")
	}
	for _, p := range pragmas {
		if _, err := db.ExecContext(ctx, p); err != nil {
			_ = db.Close()
			return nil, fmt.Errorf("apply %q: %w", p, err)
		}
	}

	return &sqliteExecutor{db: db}, nil
}

func isMemorySQLiteDSN(dsn string) bool {
	lower := strings.ToLower(dsn)
	return lower == ":memory:" || strings.Contains(lower, "mode=memory")
}

func (e *sqliteExecutor) Query(ctx context.Context, s stmt) ([][]any, error) {
	rows, err := e.db.QueryContext(ctx, s.query, s.args...)
	if err != nil {
		return nil, fmt.Errorf("sqlite query: %w", err)
	}
	defer func() { _ = rows.Close() }()

	cols, err := rows.Columns()
	if err != nil {
		return nil, fmt.Errorf("sqlite columns: %w", err)
	}
	colCount := len(cols)

	var out [][]any
	for rows.Next() {
		vals := make([]any, colCount)
		ptrs := make([]any, colCount)
		for i := range colCount {
			ptrs[i] = &vals[i]
		}
		if err := rows.Scan(ptrs...); err != nil {
			return nil, fmt.Errorf("sqlite scan: %w", err)
		}
		out = append(out, vals)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("sqlite rows: %w", err)
	}
	return out, nil
}

func (e *sqliteExecutor) ExecBatch(ctx context.Context, stmts []stmt) (results []execResult, retErr error) {
	if len(stmts) == 0 {
		return nil, nil
	}

	tx, err := e.db.BeginTx(ctx, nil)
	if err != nil {
		return nil, fmt.Errorf("sqlite begin tx: %w", err)
	}
	defer func() {
		if rbErr := tx.Rollback(); rbErr != nil && !errors.Is(rbErr, sql.ErrTxDone) {
			retErr = errors.Join(retErr, fmt.Errorf("sqlite rollback tx: %w", rbErr))
		}
	}()

	results = make([]execResult, 0, len(stmts))
	for i, s := range stmts {
		res, execErr := tx.ExecContext(ctx, s.query, s.args...)
		if execErr != nil {
			return nil, fmt.Errorf("sqlite exec stmt[%d]: %w", i, execErr)
		}
		var er execResult
		if lastID, idErr := res.LastInsertId(); idErr == nil {
			er.lastInsertID = lastID
		}
		if affected, affErr := res.RowsAffected(); affErr == nil {
			er.rowsAffected = affected
		}
		results = append(results, er)
	}

	if err := tx.Commit(); err != nil {
		return nil, fmt.Errorf("sqlite commit tx: %w", err)
	}
	return results, nil
}

func (e *sqliteExecutor) Close() error {
	return e.db.Close()
}
