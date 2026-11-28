package store

import (
	"context"
	"fmt"
	"strings"
	"time"
)

var permanentBanUntil = time.Date(9999, 12, 31, 23, 59, 59, 0, time.UTC)

// BootstrapAdmin grants RoleAdmin to an existing human account during initial deployment bootstrap.
// The bootstrap grant is idempotent and recorded in moderation_logs.
func (s *Store) BootstrapAdmin(ctx context.Context, username string) error {
	_, lower, err := ValidateUsername(username)
	if err != nil {
		return err
	}
	target, err := s.loadAccountRowByLower(ctx, lower)
	if err != nil {
		return err
	}
	if target.account.IsBot() {
		return fmt.Errorf("cannot bootstrap bot account %q as admin: %w", target.account.Username, ErrInvalidRole)
	}
	if target.account.HasRole(RoleAdmin) {
		return nil
	}

	nowStr := formatTimeUTC(s.nowUTC())
	if _, err := s.exec.ExecBatch(ctx, []stmt{
		{
			query: "INSERT OR IGNORE INTO account_roles (account_id, role_name) VALUES (?, ?)",
			args:  []any{target.account.ID, RoleAdmin.String()},
		},
		{
			query: `INSERT INTO moderation_logs
				(actor_account_id, target_account_id, action, reason, expires_at, created_at)
				VALUES (?, ?, ?, ?, ?, ?)`,
			args: []any{target.account.ID, target.account.ID, ModActionAssignRole.String(), "bootstrap:admin", nil, nowStr},
		},
	}); err != nil {
		return fmt.Errorf("bootstrap admin %q: %w", target.account.Username, err)
	}
	return nil
}

// AssignRole grants role to targetUsername after verifying actorUsername holds PermRoleAssign.
func (s *Store) AssignRole(ctx context.Context, actorUsername, targetUsername string, role Role, reason string) error {
	if !role.Valid() || role == RoleBot {
		return fmt.Errorf("role %v cannot be manually assigned: %w", role, ErrInvalidRole)
	}
	actor, target, err := s.authorizeModeration(ctx, actorUsername, targetUsername, PermRoleAssign)
	if err != nil {
		return err
	}
	if target.IsBot() {
		return fmt.Errorf("cannot modify roles on bot account %q: %w", target.Username, ErrInvalidRole)
	}

	nowStr := formatTimeUTC(s.nowUTC())
	auditReason := fmt.Sprintf("role=%s %s", role.String(), strings.TrimSpace(reason))
	if _, err := s.exec.ExecBatch(ctx, []stmt{
		{
			query: "INSERT OR IGNORE INTO account_roles (account_id, role_name) VALUES (?, ?)",
			args:  []any{target.ID, role.String()},
		},
		{
			query: `INSERT INTO moderation_logs
				(actor_account_id, target_account_id, action, reason, expires_at, created_at)
				VALUES (?, ?, ?, ?, ?, ?)`,
			args: []any{actor.ID, target.ID, ModActionAssignRole.String(), strings.TrimSpace(auditReason), nil, nowStr},
		},
	}); err != nil {
		return fmt.Errorf("assign role %s to %q: %w", role, target.Username, err)
	}
	return nil
}

// RevokeRole removes role from targetUsername after verifying actorUsername holds PermRoleAssign.
func (s *Store) RevokeRole(ctx context.Context, actorUsername, targetUsername string, role Role, reason string) error {
	if !role.Valid() || role == RoleBot {
		return fmt.Errorf("role %v cannot be manually revoked: %w", role, ErrInvalidRole)
	}
	actor, target, err := s.authorizeModeration(ctx, actorUsername, targetUsername, PermRoleAssign)
	if err != nil {
		return err
	}

	nowStr := formatTimeUTC(s.nowUTC())
	auditReason := fmt.Sprintf("role=%s %s", role.String(), strings.TrimSpace(reason))
	if _, err := s.exec.ExecBatch(ctx, []stmt{
		{
			query: "DELETE FROM account_roles WHERE account_id = ? AND role_name = ?",
			args:  []any{target.ID, role.String()},
		},
		{
			query: `INSERT INTO moderation_logs
				(actor_account_id, target_account_id, action, reason, expires_at, created_at)
				VALUES (?, ?, ?, ?, ?, ?)`,
			args: []any{actor.ID, target.ID, ModActionRevokeRole.String(), strings.TrimSpace(auditReason), nil, nowStr},
		},
	}); err != nil {
		return fmt.Errorf("revoke role %s from %q: %w", role, target.Username, err)
	}
	return nil
}

// BanAccount bans targetUsername until until (or permanently if until is nil) and writes an
// immutable moderation_logs entry in the same atomic batch. Requires actor PermUserBan.
func (s *Store) BanAccount(ctx context.Context, actorUsername, targetUsername, reason string, until *time.Time) error {
	actor, target, err := s.authorizeModeration(ctx, actorUsername, targetUsername, PermUserBan)
	if err != nil {
		return err
	}
	if target.IsBot() {
		return fmt.Errorf("cannot ban bot account %q: %w", target.Username, ErrPermissionDenied)
	}

	effectiveUntil := permanentBanUntil
	if until != nil {
		effectiveUntil = until.UTC()
	}
	reason = strings.TrimSpace(reason)
	nowStr := formatTimeUTC(s.nowUTC())
	untilStr := formatTimeUTC(effectiveUntil)

	if _, err := s.exec.ExecBatch(ctx, []stmt{
		{
			query: "UPDATE accounts SET banned_until = ?, banned_reason = ? WHERE id = ?",
			args:  []any{untilStr, reason, target.ID},
		},
		{
			query: `INSERT INTO moderation_logs
				(actor_account_id, target_account_id, action, reason, expires_at, created_at)
				VALUES (?, ?, ?, ?, ?, ?)`,
			args: []any{actor.ID, target.ID, ModActionBan.String(), reason, formatOptionalTimeUTC(until), nowStr},
		},
	}); err != nil {
		return fmt.Errorf("ban account %q: %w", target.Username, err)
	}
	return nil
}

// UnbanAccount lifts an active ban on targetUsername and writes an immutable moderation_logs entry.
// Requires actor PermUserBan.
func (s *Store) UnbanAccount(ctx context.Context, actorUsername, targetUsername, reason string) error {
	actor, target, err := s.authorizeModeration(ctx, actorUsername, targetUsername, PermUserBan)
	if err != nil {
		return err
	}

	reason = strings.TrimSpace(reason)
	nowStr := formatTimeUTC(s.nowUTC())
	if _, err := s.exec.ExecBatch(ctx, []stmt{
		{
			query: "UPDATE accounts SET banned_until = NULL, banned_reason = '' WHERE id = ?",
			args:  []any{target.ID},
		},
		{
			query: `INSERT INTO moderation_logs
				(actor_account_id, target_account_id, action, reason, expires_at, created_at)
				VALUES (?, ?, ?, ?, ?, ?)`,
			args: []any{actor.ID, target.ID, ModActionUnban.String(), reason, nil, nowStr},
		},
	}); err != nil {
		return fmt.Errorf("unban account %q: %w", target.Username, err)
	}
	return nil
}

// ListModerationLogs returns recent immutable moderation_logs entries targeting targetUsername,
// ordered newest first.
func (s *Store) ListModerationLogs(ctx context.Context, targetUsername string, limit int) ([]ModerationLog, error) {
	if limit <= 0 {
		limit = 20
	}
	_, lower, err := ValidateUsername(targetUsername)
	if err != nil {
		return nil, err
	}
	target, err := s.loadAccountRowByLower(ctx, lower)
	if err != nil {
		return nil, err
	}

	rows, err := s.exec.Query(ctx, stmt{
		query: `SELECT m.id, m.actor_account_id, a.username, m.target_account_id, t.username,
				m.action, m.reason, m.expires_at, m.created_at
			FROM moderation_logs m
			JOIN accounts a ON a.id = m.actor_account_id
			JOIN accounts t ON t.id = m.target_account_id
			WHERE m.target_account_id = ?
			ORDER BY m.id DESC
			LIMIT ?`,
		args: []any{target.account.ID, limit},
	})
	if err != nil {
		return nil, fmt.Errorf("list moderation logs for %q: %w", target.account.Username, err)
	}

	out := make([]ModerationLog, 0, len(rows))
	for _, r := range rows {
		act, err := ParseModerationAction(toString(r[5]))
		if err != nil {
			return nil, err
		}
		var expiresAt *time.Time
		if ts, ok, tsErr := parseOptionalTime(r[7]); tsErr != nil {
			return nil, tsErr
		} else if ok {
			expiresAt = &ts
		}
		createdAt, err := toRequiredTime(r[8])
		if err != nil {
			return nil, err
		}
		out = append(out, ModerationLog{
			ID:              toInt64(r[0]),
			ActorAccountID:  toInt64(r[1]),
			ActorUsername:   toString(r[2]),
			TargetAccountID: toInt64(r[3]),
			TargetUsername:  toString(r[4]),
			Action:          act,
			Reason:          toString(r[6]),
			ExpiresAt:       expiresAt,
			CreatedAt:       createdAt,
		})
	}
	return out, nil
}

func (s *Store) authorizeModeration(ctx context.Context, actorUsername, targetUsername string, required Permission) (actor, target Account, err error) {
	_, actorLower, err := ValidateUsername(actorUsername)
	if err != nil {
		return Account{}, Account{}, err
	}
	_, targetLower, err := ValidateUsername(targetUsername)
	if err != nil {
		return Account{}, Account{}, err
	}

	actorRow, err := s.loadAccountRowByLower(ctx, actorLower)
	if err != nil {
		return Account{}, Account{}, err
	}
	if !actorRow.account.HasPermission(required) {
		return Account{}, Account{}, fmt.Errorf("actor %q lacks %s: %w", actorRow.account.Username, required, ErrPermissionDenied)
	}
	targetRow, err := s.loadAccountRowByLower(ctx, targetLower)
	if err != nil {
		return Account{}, Account{}, err
	}
	return actorRow.account, targetRow.account, nil
}
