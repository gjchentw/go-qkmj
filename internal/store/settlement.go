package store

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"time"
)

// SettlementRecord is the audited record persisted in hand_settlements for each settled hand.
type SettlementRecord struct {
	HandID    string
	NodeName  string
	EndReason string
	Entries   []SettlementEntry
	SettledAt time.Time
}

// SettleHand atomically inserts an idempotent hand_settlements audit record and applies
// relative additive balance/lifetime stat updates across all 4 seated accounts (humans and bots alike).
func (s *Store) SettleHand(ctx context.Context, req HandSettlement) error {
	normalized, err := validateHandSettlement(req)
	if err != nil {
		return err
	}

	accountIDs := make([]int64, len(normalized.Entries))
	for i, entry := range normalized.Entries {
		_, lower, uErr := ValidateUsername(entry.Username)
		if uErr != nil {
			return fmt.Errorf("settlement entry %d: %w", i, uErr)
		}
		row, loadErr := s.loadAccountRowByLower(ctx, lower)
		if loadErr != nil {
			return loadErr
		}
		accountIDs[i] = row.account.ID
		normalized.Entries[i].Username = row.account.Username
	}

	entriesJSON, err := json.Marshal(normalized.Entries)
	if err != nil {
		return fmt.Errorf("marshal settlement entries: %w", err)
	}

	nowStr := formatTimeUTC(s.nowUTC())
	batch := make([]stmt, 0, 1+len(normalized.Entries))
	batch = append(batch, stmt{
		query: `INSERT INTO hand_settlements (hand_id, node_name, end_reason, entries_json, settled_at)
			VALUES (?, ?, ?, ?, ?)`,
		args: []any{normalized.HandID, normalized.NodeName, normalized.EndReason, string(entriesJSON), nowStr},
	})
	for i, e := range normalized.Entries {
		batch = append(batch, stmt{
			query: `UPDATE accounts
				SET balance = balance + ?,
					games_played = games_played + ?,
					self_draw_wins = self_draw_wins + ?,
					discard_wins = discard_wins + ?,
					deal_ins = deal_ins + ?,
					draws = draws + ?
				WHERE id = ?`,
			args: []any{
				e.Delta,
				boolToInt64(e.IncrementGames),
				boolToInt64(e.SelfDrawWin),
				boolToInt64(e.DiscardWin),
				boolToInt64(e.DealIn),
				boolToInt64(e.Draw),
				accountIDs[i],
			},
		})
	}

	if _, execErr := s.exec.ExecBatch(ctx, batch); execErr != nil {
		if isUniqueConstraintErr(execErr) {
			return fmt.Errorf("hand_id %q: %w", normalized.HandID, ErrDuplicateSettlement)
		}
		return fmt.Errorf("settle hand %q: %w", normalized.HandID, execErr)
	}
	return nil
}

// GetSettlement retrieves an audited hand settlement record by its unique HandID.
func (s *Store) GetSettlement(ctx context.Context, handID string) (SettlementRecord, error) {
	handID = strings.TrimSpace(handID)
	if handID == "" {
		return SettlementRecord{}, fmt.Errorf("hand_id must not be empty: %w", ErrInvalidSettlement)
	}
	rows, err := s.exec.Query(ctx, stmt{
		query: "SELECT hand_id, node_name, end_reason, entries_json, settled_at FROM hand_settlements WHERE hand_id = ?",
		args:  []any{handID},
	})
	if err != nil {
		return SettlementRecord{}, fmt.Errorf("query settlement %q: %w", handID, err)
	}
	if len(rows) == 0 {
		return SettlementRecord{}, fmt.Errorf("settlement %q: %w", handID, ErrAccountNotFound)
	}
	r := rows[0]
	var entries []SettlementEntry
	if decErr := json.Unmarshal([]byte(toString(r[3])), &entries); decErr != nil {
		return SettlementRecord{}, fmt.Errorf("decode settlement entries: %w", decErr)
	}
	settledAt, err := toRequiredTime(r[4])
	if err != nil {
		return SettlementRecord{}, err
	}
	return SettlementRecord{
		HandID:    toString(r[0]),
		NodeName:  toString(r[1]),
		EndReason: toString(r[2]),
		Entries:   entries,
		SettledAt: settledAt,
	}, nil
}

// RecordAbandonedHand atomically increments an account's mid-hand abandonment counter.
func (s *Store) RecordAbandonedHand(ctx context.Context, username string) error {
	_, lower, err := ValidateUsername(username)
	if err != nil {
		return err
	}
	row, err := s.loadAccountRowByLower(ctx, lower)
	if err != nil {
		return err
	}
	if _, err := s.exec.ExecBatch(ctx, []stmt{{
		query: "UPDATE accounts SET abandoned_hands = abandoned_hands + 1 WHERE id = ?",
		args:  []any{row.account.ID},
	}}); err != nil {
		return fmt.Errorf("record abandoned hand for %q: %w", row.account.Username, err)
	}
	return nil
}

func validateHandSettlement(req HandSettlement) (HandSettlement, error) {
	req.HandID = strings.TrimSpace(req.HandID)
	req.NodeName = strings.TrimSpace(req.NodeName)
	req.EndReason = strings.TrimSpace(req.EndReason)
	if req.HandID == "" || req.NodeName == "" || req.EndReason == "" {
		return HandSettlement{}, fmt.Errorf("hand_id, node_name, and end_reason are required: %w", ErrInvalidSettlement)
	}
	if len(req.Entries) != 4 {
		return HandSettlement{}, fmt.Errorf("expected 4 seat entries, got %d: %w", len(req.Entries), ErrInvalidSettlement)
	}

	seen := make(map[string]bool, 4)
	var deltaSum int64
	entries := make([]SettlementEntry, len(req.Entries))
	for i, e := range req.Entries {
		canon, lower, err := ValidateUsername(e.Username)
		if err != nil {
			return HandSettlement{}, fmt.Errorf("entry[%d] username: %w", i, ErrInvalidSettlement)
		}
		if seen[lower] {
			return HandSettlement{}, fmt.Errorf("duplicate seat username %q: %w", canon, ErrInvalidSettlement)
		}
		seen[lower] = true
		if req.IsDraw && (e.Delta != 0 || e.IncrementGames || e.SelfDrawWin || e.DiscardWin || e.DealIn) {
			return HandSettlement{}, fmt.Errorf("draw settlement must have zero delta and no win flags: %w", ErrInvalidSettlement)
		}
		deltaSum += e.Delta
		e.Username = canon
		entries[i] = e
	}
	if deltaSum != 0 {
		return HandSettlement{}, fmt.Errorf("settlement deltas must sum to 0, got %d: %w", deltaSum, ErrInvalidSettlement)
	}
	req.Entries = entries
	return req, nil
}

func boolToInt64(v bool) int64 {
	if v {
		return 1
	}
	return 0
}
