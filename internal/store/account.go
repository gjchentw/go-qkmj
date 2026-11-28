package store

import (
	"cmp"
	"context"
	"fmt"
	"slices"
	"strings"
	"time"
)

const accountSelectColumns = `id, username, password_hash, balance, games_played,
	self_draw_wins, discard_wins, deal_ins, draws, abandoned_hands,
	login_count, last_login_at, last_node_name, banned_until, banned_reason, created_at`

type accountRow struct {
	account      Account
	passwordHash string
}

// CreateAccount registers a new human player account with QKMJ 95p3 initial balance (20000),
// Argon2id password hash, and default player role.
func (s *Store) CreateAccount(ctx context.Context, username, password string) (Account, error) {
	canonical, lower, err := ValidateRegistrationUsername(username)
	if err != nil {
		return Account{}, err
	}
	hash, err := HashPassword(password, s.cfg.argon2)
	if err != nil {
		return Account{}, err
	}

	nowStr := formatTimeUTC(s.nowUTC())
	_, err = s.exec.ExecBatch(ctx, []stmt{
		{
			query: `INSERT INTO accounts
				(username, username_lower, password_hash, balance, created_at)
				VALUES (?, ?, ?, ?, ?)`,
			args: []any{canonical, lower, hash, InitialBalance, nowStr},
		},
		{
			query: `INSERT INTO account_roles (account_id, role_name)
				SELECT id, ? FROM accounts WHERE username_lower = ?`,
			args: []any{RolePlayer.String(), lower},
		},
	})
	if err != nil {
		if isUniqueConstraintErr(err) {
			return Account{}, fmt.Errorf("username %q: %w", canonical, ErrUsernameTaken)
		}
		return Account{}, fmt.Errorf("create account %q: %w", canonical, err)
	}
	return s.GetAccount(ctx, canonical)
}

// Authenticate verifies credentials, blocks bot/reserved or actively banned accounts,
// atomically records login metadata (login_count, last_login_at, last_node_name),
// and applies QKMJ 95p3 login relief when balance < 20000 and games_played >= 16.
func (s *Store) Authenticate(ctx context.Context, username, password, nodeName string) (AuthResult, error) {
	_, lower, err := ValidateUsername(username)
	if err != nil {
		return AuthResult{}, err
	}
	if strings.HasPrefix(lower, ReservedPrefix) {
		return AuthResult{}, fmt.Errorf("username %q: %w", username, ErrBotLoginForbidden)
	}

	row, err := s.loadAccountRowByLower(ctx, lower)
	if err != nil {
		return AuthResult{}, err
	}
	if row.account.IsBot() {
		return AuthResult{}, fmt.Errorf("account %q: %w", row.account.Username, ErrBotLoginForbidden)
	}

	ok, err := VerifyPassword(password, row.passwordHash)
	if err != nil || !ok {
		return AuthResult{}, ErrInvalidCredentials
	}

	now := s.nowUTC()
	if row.account.IsBannedAt(now) {
		banUntil := row.account.BannedUntil
		if banUntil != nil && banUntil.Equal(permanentBanUntil) {
			banUntil = nil
		}
		return AuthResult{}, &AccountBannedError{
			Reason: row.account.BannedReason,
			Until:  banUntil,
		}
	}

	nodeName = strings.TrimSpace(nodeName)
	reliefCandidate := row.account.Balance < ReliefThreshold && row.account.GamesPlayed >= ReliefMinGames
	// Conditional SQL CASE guarantees concurrent multi-node SettleHand wins are never overwritten downward.
	updateStmt := stmt{
		query: `UPDATE accounts
			SET balance = CASE
					WHEN balance < ? AND games_played >= ? THEN ?
					ELSE balance
				END,
				login_count = login_count + 1,
				last_login_at = ?,
				last_node_name = ?
			WHERE id = ?`,
		args: []any{ReliefThreshold, ReliefMinGames, ReliefThreshold, formatTimeUTC(now), nodeName, row.account.ID},
	}
	if _, execErr := s.exec.ExecBatch(ctx, []stmt{updateStmt}); execErr != nil {
		return AuthResult{}, fmt.Errorf("record login for %q: %w", row.account.Username, execErr)
	}

	updated, err := s.loadAccountRowByLower(ctx, lower)
	if err != nil {
		return AuthResult{}, err
	}
	return AuthResult{
		Account:       updated.account,
		ReliefGranted: reliefCandidate && updated.account.Balance == ReliefThreshold,
	}, nil
}

// ChangePassword verifies oldPassword and replaces the stored Argon2id hash with newPassword.
func (s *Store) ChangePassword(ctx context.Context, username, oldPassword, newPassword string) error {
	_, lower, err := ValidateUsername(username)
	if err != nil {
		return err
	}
	if strings.HasPrefix(lower, ReservedPrefix) {
		return fmt.Errorf("username %q: %w", username, ErrBotLoginForbidden)
	}
	if pwErr := ValidatePassword(newPassword); pwErr != nil {
		return pwErr
	}

	row, err := s.loadAccountRowByLower(ctx, lower)
	if err != nil {
		return err
	}
	if row.account.IsBot() {
		return fmt.Errorf("account %q: %w", row.account.Username, ErrBotLoginForbidden)
	}

	ok, err := VerifyPassword(oldPassword, row.passwordHash)
	if err != nil || !ok {
		return ErrInvalidCredentials
	}

	newHash, err := HashPassword(newPassword, s.cfg.argon2)
	if err != nil {
		return err
	}
	if _, err := s.exec.ExecBatch(ctx, []stmt{{
		query: "UPDATE accounts SET password_hash = ? WHERE id = ?",
		args:  []any{newHash, row.account.ID},
	}}); err != nil {
		return fmt.Errorf("update password for %q: %w", row.account.Username, err)
	}
	return nil
}

// GetAccount fetches a public-safe Account (without password hash) by case-insensitive username.
func (s *Store) GetAccount(ctx context.Context, username string) (Account, error) {
	_, lower, err := ValidateUsername(username)
	if err != nil {
		return Account{}, err
	}
	row, err := s.loadAccountRowByLower(ctx, lower)
	if err != nil {
		return Account{}, err
	}
	return row.account, nil
}

// GetAccountRank calculates a player's global ranking among accounts with >= RankMinGames played.
// Equal balances share the same rank (1 + count of qualified accounts with strictly higher balance).
func (s *Store) GetAccountRank(ctx context.Context, username string) (AccountRank, error) {
	acc, err := s.GetAccount(ctx, username)
	if err != nil {
		return AccountRank{}, err
	}
	if acc.GamesPlayed < RankMinGames {
		return AccountRank{Qualified: false}, nil
	}

	rows, err := s.exec.Query(ctx, stmt{
		query: `SELECT
			(SELECT COUNT(*) FROM accounts WHERE games_played >= ? AND balance > ?) + 1,
			(SELECT COUNT(*) FROM accounts WHERE games_played >= ?)`,
		args: []any{RankMinGames, acc.Balance, RankMinGames},
	})
	if err != nil {
		return AccountRank{}, fmt.Errorf("query rank for %q: %w", acc.Username, err)
	}
	if len(rows) != 1 || len(rows[0]) < 2 {
		return AccountRank{}, fmt.Errorf("unexpected rank query result: %w", ErrBackendFailure)
	}
	return AccountRank{
		Qualified: true,
		Rank:      toInt64(rows[0][0]),
		Total:     toInt64(rows[0][1]),
	}, nil
}

// ListBotAccounts returns all built-in AI bot accounts (QKMJ0..QKMJ9) sorted by username.
func (s *Store) ListBotAccounts(ctx context.Context) ([]Account, error) {
	rows, err := s.exec.Query(ctx, stmt{
		query: `SELECT ` + accountSelectColumns + `
			FROM accounts a
			JOIN account_roles r ON r.account_id = a.id
			WHERE r.role_name = ?
			ORDER BY a.username_lower ASC`,
		args: []any{RoleBot.String()},
	})
	if err != nil {
		return nil, fmt.Errorf("list bot accounts: %w", err)
	}
	if len(rows) == 0 {
		return nil, nil
	}

	roleRows, err := s.exec.Query(ctx, stmt{
		query: `SELECT account_id, role_name FROM account_roles
			WHERE account_id IN (SELECT account_id FROM account_roles WHERE role_name = ?)`,
		args: []any{RoleBot.String()},
	})
	if err != nil {
		return nil, fmt.Errorf("load bot roles: %w", err)
	}
	rolesByID := make(map[int64][]Role, len(rows))
	for _, rr := range roleRows {
		role, err := ParseRole(toString(rr[1]))
		if err != nil {
			return nil, err
		}
		id := toInt64(rr[0])
		rolesByID[id] = append(rolesByID[id], role)
	}

	out := make([]Account, 0, len(rows))
	for _, r := range rows {
		ar, err := decodeAccountRow(r)
		if err != nil {
			return nil, err
		}
		roles := rolesByID[ar.account.ID]
		slices.SortFunc(roles, cmp.Compare)
		ar.account.Roles = roles
		out = append(out, ar.account)
	}
	return out, nil
}

func (s *Store) loadAccountRowByLower(ctx context.Context, lower string) (accountRow, error) {
	rows, err := s.exec.Query(ctx, stmt{
		query: `SELECT ` + accountSelectColumns + ` FROM accounts WHERE username_lower = ?`,
		args:  []any{lower},
	})
	if err != nil {
		return accountRow{}, fmt.Errorf("query account %q: %w", lower, err)
	}
	if len(rows) == 0 {
		return accountRow{}, fmt.Errorf("account %q: %w", lower, ErrAccountNotFound)
	}
	ar, err := decodeAccountRow(rows[0])
	if err != nil {
		return accountRow{}, err
	}
	roles, err := s.loadRolesByAccountID(ctx, ar.account.ID)
	if err != nil {
		return accountRow{}, err
	}
	ar.account.Roles = roles
	return ar, nil
}

func (s *Store) loadRolesByAccountID(ctx context.Context, accountID int64) ([]Role, error) {
	rows, err := s.exec.Query(ctx, stmt{
		query: "SELECT role_name FROM account_roles WHERE account_id = ?",
		args:  []any{accountID},
	})
	if err != nil {
		return nil, fmt.Errorf("query roles for account %d: %w", accountID, err)
	}
	roles := make([]Role, 0, len(rows))
	for _, r := range rows {
		role, err := ParseRole(toString(r[0]))
		if err != nil {
			return nil, err
		}
		roles = append(roles, role)
	}
	slices.SortFunc(roles, cmp.Compare)
	return roles, nil
}

func decodeAccountRow(vals []any) (accountRow, error) {
	if len(vals) < 16 {
		return accountRow{}, fmt.Errorf("account row has %d columns, want 16: %w", len(vals), ErrBackendFailure)
	}
	var lastLogin *time.Time
	if ts, ok, err := parseOptionalTime(vals[11]); err != nil {
		return accountRow{}, err
	} else if ok {
		lastLogin = &ts
	}
	var bannedUntil *time.Time
	if ts, ok, err := parseOptionalTime(vals[13]); err != nil {
		return accountRow{}, err
	} else if ok {
		bannedUntil = &ts
	}
	createdAt, err := toRequiredTime(vals[15])
	if err != nil {
		return accountRow{}, err
	}
	return accountRow{
		passwordHash: toString(vals[2]),
		account: Account{
			ID:             toInt64(vals[0]),
			Username:       toString(vals[1]),
			Balance:        toInt64(vals[3]),
			GamesPlayed:    toInt64(vals[4]),
			SelfDrawWins:   toInt64(vals[5]),
			DiscardWins:    toInt64(vals[6]),
			DealIns:        toInt64(vals[7]),
			Draws:          toInt64(vals[8]),
			AbandonedHands: toInt64(vals[9]),
			LoginCount:     toInt64(vals[10]),
			LastLoginAt:    lastLogin,
			LastNodeName:   toString(vals[12]),
			BannedUntil:    bannedUntil,
			BannedReason:   toString(vals[14]),
			CreatedAt:      createdAt,
		},
	}, nil
}
