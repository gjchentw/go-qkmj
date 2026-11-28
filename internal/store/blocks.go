package store

import (
	"context"
	"fmt"
)

// BlockUser adds targetUsername to username's personal blocklist (user_blocks).
// Blocking oneself is forbidden, target must exist, and entries are capped at cfg.maxBlocks.
func (s *Store) BlockUser(ctx context.Context, username, targetUsername string) error {
	_, ownerLower, err := ValidateUsername(username)
	if err != nil {
		return err
	}
	_, targetLower, err := ValidateUsername(targetUsername)
	if err != nil {
		return err
	}
	if ownerLower == targetLower {
		return ErrSelfBlock
	}

	owner, err := s.loadAccountRowByLower(ctx, ownerLower)
	if err != nil {
		return err
	}
	target, err := s.loadAccountRowByLower(ctx, targetLower)
	if err != nil {
		return err
	}

	// Idempotent check: if already blocked, return nil without consuming another slot.
	existsRows, err := s.exec.Query(ctx, stmt{
		query: "SELECT 1 FROM user_blocks WHERE account_id = ? AND blocked_account_id = ?",
		args:  []any{owner.account.ID, target.account.ID},
	})
	if err != nil {
		return fmt.Errorf("check existing block: %w", err)
	}
	if len(existsRows) > 0 {
		return nil
	}

	countRows, err := s.exec.Query(ctx, stmt{
		query: "SELECT COUNT(*) FROM user_blocks WHERE account_id = ?",
		args:  []any{owner.account.ID},
	})
	if err != nil {
		return fmt.Errorf("count blocks for %q: %w", owner.account.Username, err)
	}
	if len(countRows) > 0 && int(toInt64(countRows[0][0])) >= s.cfg.maxBlocks {
		return fmt.Errorf("block limit %d reached: %w", s.cfg.maxBlocks, ErrBlockLimitReached)
	}

	nowStr := formatTimeUTC(s.nowUTC())
	if _, err := s.exec.ExecBatch(ctx, []stmt{{
		query: "INSERT OR IGNORE INTO user_blocks (account_id, blocked_account_id, created_at) VALUES (?, ?, ?)",
		args:  []any{owner.account.ID, target.account.ID, nowStr},
	}}); err != nil {
		return fmt.Errorf("block user %q -> %q: %w", owner.account.Username, target.account.Username, err)
	}
	return nil
}

// UnblockUser removes targetUsername from username's personal blocklist.
func (s *Store) UnblockUser(ctx context.Context, username, targetUsername string) error {
	_, ownerLower, err := ValidateUsername(username)
	if err != nil {
		return err
	}
	_, targetLower, err := ValidateUsername(targetUsername)
	if err != nil {
		return err
	}
	owner, err := s.loadAccountRowByLower(ctx, ownerLower)
	if err != nil {
		return err
	}
	target, err := s.loadAccountRowByLower(ctx, targetLower)
	if err != nil {
		return err
	}

	results, err := s.exec.ExecBatch(ctx, []stmt{{
		query: "DELETE FROM user_blocks WHERE account_id = ? AND blocked_account_id = ?",
		args:  []any{owner.account.ID, target.account.ID},
	}})
	if err != nil {
		return fmt.Errorf("unblock user %q -> %q: %w", owner.account.Username, target.account.Username, err)
	}
	if len(results) == 0 || results[0].rowsAffected == 0 {
		return fmt.Errorf("%q is not blocked by %q: %w", target.account.Username, owner.account.Username, ErrNotBlocked)
	}
	return nil
}

// IsBlocked reports whether username has blocked targetUsername.
func (s *Store) IsBlocked(ctx context.Context, username, targetUsername string) (bool, error) {
	_, ownerLower, err := ValidateUsername(username)
	if err != nil {
		return false, err
	}
	_, targetLower, err := ValidateUsername(targetUsername)
	if err != nil {
		return false, err
	}
	if ownerLower == targetLower {
		return false, nil
	}

	rows, err := s.exec.Query(ctx, stmt{
		query: `SELECT 1
			FROM user_blocks b
			JOIN accounts o ON o.id = b.account_id
			JOIN accounts t ON t.id = b.blocked_account_id
			WHERE o.username_lower = ? AND t.username_lower = ?`,
		args: []any{ownerLower, targetLower},
	})
	if err != nil {
		return false, fmt.Errorf("check block %q -> %q: %w", ownerLower, targetLower, err)
	}
	return len(rows) > 0, nil
}

// ListBlockedUsers returns the canonical usernames blocked by username, sorted alphabetically.
func (s *Store) ListBlockedUsers(ctx context.Context, username string) ([]string, error) {
	_, ownerLower, err := ValidateUsername(username)
	if err != nil {
		return nil, err
	}
	owner, err := s.loadAccountRowByLower(ctx, ownerLower)
	if err != nil {
		return nil, err
	}

	rows, err := s.exec.Query(ctx, stmt{
		query: `SELECT t.username
			FROM user_blocks b
			JOIN accounts t ON t.id = b.blocked_account_id
			WHERE b.account_id = ?
			ORDER BY t.username_lower ASC`,
		args: []any{owner.account.ID},
	})
	if err != nil {
		return nil, fmt.Errorf("list blocked users for %q: %w", owner.account.Username, err)
	}
	out := make([]string, 0, len(rows))
	for _, r := range rows {
		out = append(out, toString(r[0]))
	}
	return out, nil
}
