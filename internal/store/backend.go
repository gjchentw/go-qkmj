package store

import (
	"context"
	"encoding/json"
	"fmt"
	"strconv"
	"strings"
	"time"
)

type stmt struct {
	query string
	args  []any
}

type execResult struct {
	lastInsertID int64
	rowsAffected int64
}

type executor interface {
	Query(ctx context.Context, s stmt) ([][]any, error)
	ExecBatch(ctx context.Context, stmts []stmt) ([]execResult, error)
	Close() error
}

func toInt64(v any) int64 {
	switch x := v.(type) {
	case int64:
		return x
	case int:
		return int64(x)
	case int32:
		return int64(x)
	case uint64:
		if x > 1<<63-1 {
			return 0
		}
		return int64(x)
	case float64:
		return int64(x)
	case json.Number:
		n, err := x.Int64()
		if err != nil {
			return 0
		}
		return n
	case string:
		n, err := strconv.ParseInt(x, 10, 64)
		if err != nil {
			return 0
		}
		return n
	case []byte:
		n, err := strconv.ParseInt(string(x), 10, 64)
		if err != nil {
			return 0
		}
		return n
	default:
		return 0
	}
}

func toString(v any) string {
	switch x := v.(type) {
	case nil:
		return ""
	case string:
		return x
	case []byte:
		return string(x)
	default:
		return fmt.Sprint(x)
	}
}

func parseOptionalTime(v any) (ts time.Time, ok bool, err error) {
	raw := toString(v)
	if raw == "" {
		return time.Time{}, false, nil
	}
	parsed, err := time.Parse(time.RFC3339Nano, raw)
	if err != nil {
		return time.Time{}, false, fmt.Errorf("parse timestamp %q: %w", raw, err)
	}
	return parsed.UTC(), true, nil
}

func toRequiredTime(v any) (time.Time, error) {
	raw := toString(v)
	parsed, err := time.Parse(time.RFC3339Nano, raw)
	if err != nil {
		return time.Time{}, fmt.Errorf("parse timestamp %q: %w", raw, err)
	}
	return parsed.UTC(), nil
}

func formatTimeUTC(t time.Time) string {
	return t.UTC().Format(time.RFC3339Nano)
}

func formatOptionalTimeUTC(t *time.Time) any {
	if t == nil {
		return nil
	}
	return formatTimeUTC(*t)
}

func isUniqueConstraintErr(err error) bool {
	if err == nil {
		return false
	}
	msg := strings.ToLower(err.Error())
	return strings.Contains(msg, "unique constraint failed") ||
		strings.Contains(msg, "constraint failed: unique") ||
		strings.Contains(msg, "constraint failed: primary key")
}
