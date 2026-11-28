package store

import (
	"context"
	"encoding/json"
	"fmt"
	"maps"
)

// GetPreferences returns username's persisted UserPreferences, falling back to DefaultUserPreferences
// when none have been saved yet. Unknown JSON keys are silently ignored for forward compatibility.
func (s *Store) GetPreferences(ctx context.Context, username string) (UserPreferences, error) {
	_, lower, err := ValidateUsername(username)
	if err != nil {
		return UserPreferences{}, err
	}
	row, err := s.loadAccountRowByLower(ctx, lower)
	if err != nil {
		return UserPreferences{}, err
	}

	rows, err := s.exec.Query(ctx, stmt{
		query: "SELECT prefs_json FROM user_preferences WHERE account_id = ?",
		args:  []any{row.account.ID},
	})
	if err != nil {
		return UserPreferences{}, fmt.Errorf("query preferences for %q: %w", row.account.Username, err)
	}

	prefs := DefaultUserPreferences()
	if len(rows) == 0 {
		return prefs, nil
	}
	if err := json.Unmarshal([]byte(toString(rows[0][0])), &prefs); err != nil {
		return UserPreferences{}, fmt.Errorf("decode preferences for %q: %w", row.account.Username, err)
	}
	if prefs.FunctionKeys != nil {
		prefs.FunctionKeys = maps.Clone(prefs.FunctionKeys)
	}
	return prefs, nil
}

// SavePreferences persists username's UserPreferences atomically via upsert.
func (s *Store) SavePreferences(ctx context.Context, username string, prefs UserPreferences) error {
	_, lower, err := ValidateUsername(username)
	if err != nil {
		return err
	}
	row, err := s.loadAccountRowByLower(ctx, lower)
	if err != nil {
		return err
	}

	encoded, err := json.Marshal(prefs)
	if err != nil {
		return fmt.Errorf("marshal preferences for %q: %w", row.account.Username, err)
	}
	nowStr := formatTimeUTC(s.nowUTC())
	if _, err := s.exec.ExecBatch(ctx, []stmt{{
		query: `INSERT INTO user_preferences (account_id, prefs_json, updated_at)
			VALUES (?, ?, ?)
			ON CONFLICT(account_id) DO UPDATE SET
				prefs_json = excluded.prefs_json,
				updated_at = excluded.updated_at`,
		args: []any{row.account.ID, string(encoded), nowStr},
	}}); err != nil {
		return fmt.Errorf("save preferences for %q: %w", row.account.Username, err)
	}
	return nil
}
