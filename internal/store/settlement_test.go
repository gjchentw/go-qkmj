package store

import (
	"errors"
	"testing"
)

func TestStore_SettleHand_And_RecordAbandonedHand(t *testing.T) {
	t.Parallel()

	runDualBackend(t, func(t *testing.T, s *Store, _ *fakeClock) {
		t.Helper()
		ctx := t.Context()
		for _, name := range []string{"Alice", "Bob", "Carol"} {
			if _, err := s.CreateAccount(ctx, name, "password123"); err != nil {
				t.Fatalf("CreateAccount(%s): %v", name, err)
			}
		}

		// 1. Validation failures do not mutate state.
		badSettlements := []HandSettlement{
			{HandID: "", NodeName: "node-1", EndReason: "win", Entries: make([]SettlementEntry, 4)},
			{HandID: "h-bad-node", NodeName: "", EndReason: "win", Entries: make([]SettlementEntry, 4)},
			{HandID: "h-bad-len", NodeName: "node-1", EndReason: "win", Entries: []SettlementEntry{{Username: "Alice"}}},
			{
				HandID: "h-bad-dup", NodeName: "node-1", EndReason: "win",
				Entries: []SettlementEntry{
					{Username: "Alice", Delta: 1000, IncrementGames: true},
					{Username: "alice", Delta: -1000, IncrementGames: true},
					{Username: "Bob", Delta: 0, IncrementGames: true},
					{Username: "QKMJ0", Delta: 0, IncrementGames: true},
				},
			},
			{
				HandID: "h-bad-sum", NodeName: "node-1", EndReason: "win",
				Entries: []SettlementEntry{
					{Username: "Alice", Delta: 1500, IncrementGames: true},
					{Username: "Bob", Delta: -1000, IncrementGames: true},
					{Username: "Carol", Delta: 0, IncrementGames: true},
					{Username: "QKMJ0", Delta: 0, IncrementGames: true},
				},
			},
			{
				HandID: "h-bad-draw", NodeName: "node-1", EndReason: "four_winds", IsDraw: true,
				Entries: []SettlementEntry{
					{Username: "Alice", Delta: 500, Draw: true},
					{Username: "Bob", Delta: -500, Draw: true},
					{Username: "Carol", Delta: 0, Draw: true},
					{Username: "QKMJ0", Delta: 0, Draw: true},
				},
			},
		}
		for _, bs := range badSettlements {
			if err := s.SettleHand(ctx, bs); !errors.Is(err, ErrInvalidSettlement) {
				t.Errorf("SettleHand(%s) err = %v, want ErrInvalidSettlement", bs.HandID, err)
			}
		}

		// Non-existent account fails fast with ErrAccountNotFound.
		err := s.SettleHand(ctx, HandSettlement{
			HandID: "h-missing-user", NodeName: "node-1", EndReason: "win",
			Entries: []SettlementEntry{
				{Username: "Alice", Delta: 1000, IncrementGames: true, DiscardWin: true},
				{Username: "GhostUser", Delta: -1000, IncrementGames: true, DealIn: true},
				{Username: "Carol", Delta: 0, IncrementGames: true},
				{Username: "QKMJ0", Delta: 0, IncrementGames: true},
			},
		})
		if !errors.Is(err, ErrAccountNotFound) {
			t.Fatalf("SettleHand with missing user err = %v, want ErrAccountNotFound", err)
		}

		// 2. Valid win settlement across 3 humans + QKMJ0 bot.
		winHand := HandSettlement{
			HandID:    "h-101",
			NodeName:  "brisk-falcon",
			EndReason: "win",
			Entries: []SettlementEntry{
				{Username: "Alice", Delta: 1500, IncrementGames: true, DiscardWin: true},
				{Username: "Bob", Delta: -1500, IncrementGames: true, DealIn: true},
				{Username: "Carol", Delta: 0, IncrementGames: true},
				{Username: "QKMJ0", Delta: 0, IncrementGames: true},
			},
		}
		if err := s.SettleHand(ctx, winHand); err != nil {
			t.Fatalf("SettleHand(h-101): %v", err)
		}

		// 3. Duplicate HandID MUST be rejected idempotently without double-updating balances.
		if err := s.SettleHand(ctx, winHand); !errors.Is(err, ErrDuplicateSettlement) {
			t.Fatalf("duplicate SettleHand(h-101) err = %v, want ErrDuplicateSettlement", err)
		}

		// 4. Second hand from another node (QKMJ0 self-draw wins!) verifies relative additive deltas & bot real-account audit.
		botWinHand := HandSettlement{
			HandID:    "h-102",
			NodeName:  "calm-otter",
			EndReason: "win",
			Entries: []SettlementEntry{
				{Username: "QKMJ0", Delta: 3000, IncrementGames: true, SelfDrawWin: true},
				{Username: "Alice", Delta: -1000, IncrementGames: true},
				{Username: "Bob", Delta: -1000, IncrementGames: true},
				{Username: "Carol", Delta: -1000, IncrementGames: true},
			},
		}
		if err := s.SettleHand(ctx, botWinHand); err != nil {
			t.Fatalf("SettleHand(h-102): %v", err)
		}

		// 5. Special draw settlement increments Draws only (balance & GamesPlayed unchanged).
		drawHand := HandSettlement{
			HandID:    "h-103",
			NodeName:  "calm-otter",
			EndReason: "four_winds",
			IsDraw:    true,
			Entries: []SettlementEntry{
				{Username: "Alice", Draw: true},
				{Username: "Bob", Draw: true},
				{Username: "Carol", Draw: true},
				{Username: "QKMJ0", Draw: true},
			},
		}
		if err := s.SettleHand(ctx, drawHand); err != nil {
			t.Fatalf("SettleHand(h-103 draw): %v", err)
		}

		// Verify balances & lifetime stats across all 4 accounts (including QKMJ0).
		mustGet := func(name string) Account {
			t.Helper()
			acc, getErr := s.GetAccount(ctx, name)
			if getErr != nil {
				t.Fatalf("GetAccount(%s): %v", name, getErr)
			}
			return acc
		}
		alice := mustGet("Alice")
		bob := mustGet("Bob")
		carol := mustGet("Carol")
		bot0 := mustGet("QKMJ0")

		if alice.Balance != 20500 || alice.GamesPlayed != 2 || alice.DiscardWins != 1 || alice.Draws != 1 {
			t.Errorf("Alice = %+v, want balance=20500 games=2 discardWins=1 draws=1", alice)
		}
		if bob.Balance != 17500 || bob.GamesPlayed != 2 || bob.DealIns != 1 || bob.Draws != 1 {
			t.Errorf("Bob = %+v, want balance=17500 games=2 dealIns=1 draws=1", bob)
		}
		if carol.Balance != 19000 || carol.GamesPlayed != 2 || carol.Draws != 1 {
			t.Errorf("Carol = %+v, want balance=19000 games=2 draws=1", carol)
		}
		if bot0.Balance != 23000 || bot0.GamesPlayed != 2 || bot0.SelfDrawWins != 1 || bot0.Draws != 1 {
			t.Errorf("QKMJ0 = %+v, want balance=23000 games=2 selfDrawWins=1 draws=1", bot0)
		}

		// Verify hand_settlements recorded node_name and entries_json.
		rec, err := s.GetSettlement(ctx, "h-101")
		if err != nil {
			t.Fatalf("GetSettlement(h-101): %v", err)
		}
		if rec.NodeName != "brisk-falcon" || rec.EndReason != "win" || len(rec.Entries) != 4 {
			t.Fatalf("GetSettlement(h-101) = %+v", rec)
		}

		// 6. RecordAbandonedHand increments abandoned_hands only.
		if err := s.RecordAbandonedHand(ctx, "Bob"); err != nil {
			t.Fatalf("RecordAbandonedHand(Bob): %v", err)
		}
		bobAfter := mustGet("Bob")
		if bobAfter.AbandonedHands != 1 || bobAfter.Balance != 17500 || bobAfter.GamesPlayed != 2 {
			t.Fatalf("Bob after abandon = %+v, want abandoned=1 balance=17500 games=2", bobAfter)
		}
		if err := s.RecordAbandonedHand(ctx, "NoSuchUser"); !errors.Is(err, ErrAccountNotFound) {
			t.Fatalf("RecordAbandonedHand(unknown) err = %v, want ErrAccountNotFound", err)
		}
	})
}
