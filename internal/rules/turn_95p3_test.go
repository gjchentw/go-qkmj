package rules_test

import (
	"errors"
	"slices"
	"testing"
	"time"

	"github.com/gjchentw/go-qkmj/internal/rules"
)

func nonTenpaiTurnHands() map[rules.Seat][]rules.Tile {
	return map[rules.Seat][]rules.Tile{
		rules.Seat1: {
			rules.Wan2, rules.Wan3, rules.Wan5, rules.Wan7, rules.Wan8,
			rules.Suo2, rules.Suo3, rules.Suo5, rules.Suo7, rules.Suo8,
			rules.Tong2, rules.Tong3, rules.Tong5, rules.Tong7, rules.Tong8,
			rules.EastWind, rules.SouthWind,
		},
		rules.Seat2: {
			rules.Wan2, rules.Wan4, rules.Wan6, rules.Wan8,
			rules.Suo2, rules.Suo4, rules.Suo6, rules.Suo8,
			rules.Tong2, rules.Tong4, rules.Tong6, rules.Tong8,
			rules.EastWind, rules.WestWind, rules.NorthWind, rules.RedDragon,
		},
		rules.Seat3: {
			rules.Wan2, rules.Wan4, rules.Wan6, rules.Wan8,
			rules.Suo2, rules.Suo4, rules.Suo6, rules.Suo8,
			rules.Tong2, rules.Tong4, rules.Tong6, rules.Tong8,
			rules.EastWind, rules.SouthWind, rules.WhiteDragon, rules.GreenDragon,
		},
		rules.Seat4: {
			rules.Wan2, rules.Wan5, rules.Wan8,
			rules.Suo2, rules.Suo5, rules.Suo8,
			rules.Tong2, rules.Tong5, rules.Tong8,
			rules.EastWind, rules.WestWind, rules.NorthWind,
			rules.RedDragon, rules.WhiteDragon, rules.GreenDragon, rules.Wan3,
		},
	}
}

func TestTurnEngine_FourWindsDraw(t *testing.T) {
	t.Parallel()

	hands := nonTenpaiTurnHands()
	wall := makePreparedWall(t, hands, nil, []rules.Tile{rules.Wan1, rules.Suo1, rules.Tong1})
	eng, err := rules.NewTurnEngine(rules.TurnConfig{
		Wall:      wall,
		RoundWind: rules.WindEast,
		SeatWinds: defaultSeatWinds(),
		Dealer:    rules.Seat1,
		Hands:     hands,
		Base:      rules.DefaultBase,
		PerFan:    rules.DefaultPerFan,
	})
	if err != nil {
		t.Fatalf("NewTurnEngine: %v", err)
	}
	eng.Start()

	// Seat1 discards EastWind; Seat2 draws and discards EastWind; Seat3 draws and discards EastWind;
	// Seat4 draws and discards EastWind -> Four Winds Draw!
	if _, err := eng.Apply(rules.PlayerAction{Seat: rules.Seat1, Kind: rules.ActionDiscard, Tile: rules.EastWind}, 0); err != nil {
		t.Fatalf("Seat1 discard EastWind: %v", err)
	}
	for _, seat := range []rules.Seat{rules.Seat2, rules.Seat3} {
		if _, err := eng.Apply(rules.PlayerAction{Seat: seat, Kind: rules.ActionDraw}, 0); err != nil {
			t.Fatalf("seat %v draw: %v", seat, err)
		}
		if _, err := eng.Apply(rules.PlayerAction{Seat: seat, Kind: rules.ActionDiscard, Tile: rules.EastWind}, 0); err != nil {
			t.Fatalf("seat %v discard EastWind: %v", seat, err)
		}
	}
	if _, err := eng.Apply(rules.PlayerAction{Seat: rules.Seat4, Kind: rules.ActionDraw}, 0); err != nil {
		t.Fatalf("Seat4 draw: %v", err)
	}
	evs, err := eng.Apply(rules.PlayerAction{Seat: rules.Seat4, Kind: rules.ActionDiscard, Tile: rules.EastWind}, 0)
	if err != nil {
		t.Fatalf("Seat4 discard EastWind: %v", err)
	}
	if eng.Phase() != rules.PhaseHandEnded {
		t.Fatalf("Phase() = %v, want PhaseHandEnded", eng.Phase())
	}
	he := findHandEndedEvent(t, evs)
	if he.Reason != rules.HandEndFourWinds || he.Score != nil {
		t.Fatalf("HandEndedEvent = %+v, want HandEndFourWinds with nil score", he)
	}
}

func TestTurnEngine_FourKongsDraw_MultiSeatVsSingleSeat(t *testing.T) {
	t.Parallel()

	t.Run("two seats accumulate 4 kongs and safe discard ends in HandEndFourKongs", func(t *testing.T) {
		t.Parallel()
		hands := nonTenpaiTurnHands()
		// Seat1 has 2 concealed kongs (Wan1 x4, Wan9 x4); Seat2 has 2 concealed kongs (Suo1 x4, Suo9 x4).
		hands[rules.Seat1] = []rules.Tile{
			rules.Wan1, rules.Wan1, rules.Wan1, rules.Wan1,
			rules.Wan9, rules.Wan9, rules.Wan9, rules.Wan9,
			rules.Suo3, rules.Suo5, rules.Suo7,
			rules.Tong2, rules.Tong4, rules.Tong6, rules.Tong8,
			rules.EastWind, rules.SouthWind,
		}
		hands[rules.Seat2] = []rules.Tile{
			rules.Suo1, rules.Suo1, rules.Suo1, rules.Suo1,
			rules.Suo9, rules.Suo9, rules.Suo9, rules.Suo9,
			rules.Wan2, rules.Wan4, rules.Wan6,
			rules.Tong1, rules.Tong3, rules.Tong5, rules.WestWind, rules.NorthWind,
		}
		hands[rules.Seat3] = []rules.Tile{
			rules.Wan3, rules.Wan5, rules.Wan7,
			rules.Suo2, rules.Suo4, rules.Suo6, rules.Suo8,
			rules.Tong2, rules.Tong4, rules.Tong6, rules.Tong8,
			rules.EastWind, rules.SouthWind, rules.RedDragon, rules.WhiteDragon, rules.GreenDragon,
		}
		hands[rules.Seat4] = []rules.Tile{
			rules.Wan3, rules.Wan5, rules.Wan7,
			rules.Suo2, rules.Suo4, rules.Suo6, rules.Suo8,
			rules.Tong1, rules.Tong3, rules.Tong5, rules.Tong7,
			rules.WestWind, rules.NorthWind, rules.RedDragon, rules.WhiteDragon, rules.GreenDragon,
		}

		wall := makePreparedWall(t, hands, nil, []rules.Tile{
			rules.Tong7, // Seat1 kong1 supplement
			rules.Tong9, // Seat1 kong2 supplement
			rules.Wan8,  // Seat2 regular draw
			rules.Tong9, // Seat2 kong3 supplement
			rules.Wan8,  // Seat2 kong4 supplement
		})
		eng, err := rules.NewTurnEngine(rules.TurnConfig{
			Wall:      wall,
			RoundWind: rules.WindEast,
			SeatWinds: defaultSeatWinds(),
			Dealer:    rules.Seat1,
			Hands:     hands,
			Base:      rules.DefaultBase,
			PerFan:    rules.DefaultPerFan,
		})
		if err != nil {
			t.Fatalf("NewTurnEngine: %v", err)
		}
		eng.Start()

		// Seat1 declares 2 concealed kongs, then discards SouthWind.
		for _, kTile := range []rules.Tile{rules.Wan1, rules.Wan9} {
			if _, err := eng.Apply(rules.PlayerAction{
				Seat:        rules.Seat1,
				Kind:        rules.ActionSelfDeclare,
				SelfDeclare: rules.SelfDeclareConcealedKong,
				Tile:        kTile,
			}, 0); err != nil {
				t.Fatalf("Seat1 kong %v: %v", kTile, err)
			}
			if _, err := eng.Apply(rules.PlayerAction{Seat: rules.Seat1, Kind: rules.ActionDraw}, 0); err != nil {
				t.Fatalf("Seat1 supplement draw: %v", err)
			}
		}
		if _, err := eng.Apply(rules.PlayerAction{Seat: rules.Seat1, Kind: rules.ActionDiscard, Tile: rules.SouthWind}, 0); err != nil {
			t.Fatalf("Seat1 discard: %v", err)
		}

		// Seat2 draws and declares 2 concealed kongs (total 4 across 2 seats).
		if _, err := eng.Apply(rules.PlayerAction{Seat: rules.Seat2, Kind: rules.ActionDraw}, 0); err != nil {
			t.Fatalf("Seat2 draw: %v", err)
		}
		for _, kTile := range []rules.Tile{rules.Suo1, rules.Suo9} {
			if _, err := eng.Apply(rules.PlayerAction{
				Seat:        rules.Seat2,
				Kind:        rules.ActionSelfDeclare,
				SelfDeclare: rules.SelfDeclareConcealedKong,
				Tile:        kTile,
			}, 0); err != nil {
				t.Fatalf("Seat2 kong %v: %v", kTile, err)
			}
			if _, err := eng.Apply(rules.PlayerAction{Seat: rules.Seat2, Kind: rules.ActionDraw}, 0); err != nil {
				t.Fatalf("Seat2 supplement draw: %v", err)
			}
		}

		// Seat2 discards safe WestWind -> HandEndFourKongs!
		evs, err := eng.Apply(rules.PlayerAction{Seat: rules.Seat2, Kind: rules.ActionDiscard, Tile: rules.WestWind}, 0)
		if err != nil {
			t.Fatalf("Seat2 safe discard after 4th kong: %v", err)
		}
		if eng.Phase() != rules.PhaseHandEnded {
			t.Fatalf("Phase() = %v, want PhaseHandEnded", eng.Phase())
		}
		he := findHandEndedEvent(t, evs)
		if he.Reason != rules.HandEndFourKongs || he.Score != nil {
			t.Fatalf("HandEndedEvent = %+v, want HandEndFourKongs", he)
		}
	})

	t.Run("single seat with 4 kongs does not trigger Four Kongs draw", func(t *testing.T) {
		t.Parallel()
		hands := nonTenpaiTurnHands()
		hands[rules.Seat1] = []rules.Tile{
			rules.Wan1, rules.Wan1, rules.Wan1, rules.Wan1,
			rules.Wan9, rules.Wan9, rules.Wan9, rules.Wan9,
			rules.Suo1, rules.Suo1, rules.Suo1, rules.Suo1,
			rules.Suo9, rules.Suo9, rules.Suo9, rules.Suo9,
			rules.EastWind,
		}
		hands[rules.Seat2] = []rules.Tile{
			rules.Wan2, rules.Wan4, rules.Wan6, rules.Wan8,
			rules.Suo2, rules.Suo4, rules.Suo6, rules.Suo8,
			rules.Tong2, rules.Tong4, rules.Tong6, rules.Tong8,
			rules.WestWind, rules.NorthWind, rules.RedDragon, rules.WhiteDragon,
		}
		hands[rules.Seat3] = []rules.Tile{
			rules.Wan3, rules.Wan5, rules.Wan7,
			rules.Suo3, rules.Suo5, rules.Suo7,
			rules.Tong1, rules.Tong3, rules.Tong5, rules.Tong7, rules.Tong9,
			rules.SouthWind, rules.WestWind, rules.NorthWind, rules.RedDragon, rules.GreenDragon,
		}
		hands[rules.Seat4] = []rules.Tile{
			rules.Wan2, rules.Wan5, rules.Wan8,
			rules.Suo2, rules.Suo5, rules.Suo8,
			rules.Tong2, rules.Tong5, rules.Tong8,
			rules.SouthWind, rules.NorthWind, rules.RedDragon, rules.WhiteDragon, rules.GreenDragon,
			rules.Wan3, rules.Suo3,
		}

		wall := makePreparedWall(t, hands, nil, []rules.Tile{
			rules.Tong1, rules.Tong3, rules.Tong5, rules.Tong7,
		})
		eng, err := rules.NewTurnEngine(rules.TurnConfig{
			Wall:      wall,
			RoundWind: rules.WindEast,
			SeatWinds: defaultSeatWinds(),
			Dealer:    rules.Seat1,
			Hands:     hands,
			Base:      rules.DefaultBase,
			PerFan:    rules.DefaultPerFan,
		})
		if err != nil {
			t.Fatalf("NewTurnEngine: %v", err)
		}
		eng.Start()

		for _, kTile := range []rules.Tile{rules.Wan1, rules.Wan9, rules.Suo1, rules.Suo9} {
			if _, err := eng.Apply(rules.PlayerAction{
				Seat:        rules.Seat1,
				Kind:        rules.ActionSelfDeclare,
				SelfDeclare: rules.SelfDeclareConcealedKong,
				Tile:        kTile,
			}, 0); err != nil {
				t.Fatalf("Seat1 kong %v: %v", kTile, err)
			}
			if _, err := eng.Apply(rules.PlayerAction{Seat: rules.Seat1, Kind: rules.ActionDraw}, 0); err != nil {
				t.Fatalf("Seat1 draw: %v", err)
			}
		}
		if _, err := eng.Apply(rules.PlayerAction{Seat: rules.Seat1, Kind: rules.ActionDiscard, Tile: rules.EastWind}, 0); err != nil {
			t.Fatalf("Seat1 discard: %v", err)
		}
		if eng.Phase() != rules.PhaseWaitDraw || eng.CurrentTurn() != rules.Seat2 {
			t.Fatalf("single-seat 4 kongs should continue normally, got phase=%v turn=%v", eng.Phase(), eng.CurrentTurn())
		}
	})
}

func TestTurnEngine_NineTerminalsDraw(t *testing.T) {
	t.Parallel()

	hands := nonTenpaiTurnHands()
	// Give Dealer Seat1 9 distinct terminals/honors on opening 17 tiles.
	hands[rules.Seat1] = []rules.Tile{
		rules.Wan1, rules.Wan9, rules.Suo1, rules.Suo9, rules.Tong1, rules.Tong9,
		rules.EastWind, rules.SouthWind, rules.RedDragon,
		rules.Wan3, rules.Wan5, rules.Suo3, rules.Suo5, rules.Tong3, rules.Tong5, rules.Wan7, rules.Suo7,
	}
	wall := makePreparedWall(t, hands, nil, nil)
	eng, err := rules.NewTurnEngine(rules.TurnConfig{
		Wall:      wall,
		RoundWind: rules.WindEast,
		SeatWinds: defaultSeatWinds(),
		Dealer:    rules.Seat1,
		Hands:     hands,
		Base:      rules.DefaultBase,
		PerFan:    rules.DefaultPerFan,
	})
	if err != nil {
		t.Fatalf("NewTurnEngine: %v", err)
	}
	startEvs := eng.Start()
	prompt := findSelfDeclarePrompt(t, startEvs, rules.Seat1)
	if !prompt.CanNineTerminals {
		t.Fatalf("expected CanNineTerminals=true on dealer opening hand, got %+v", prompt)
	}

	evs, err := eng.Apply(rules.PlayerAction{
		Seat:        rules.Seat1,
		Kind:        rules.ActionSelfDeclare,
		SelfDeclare: rules.SelfDeclareNineTerminals,
	}, 0)
	if err != nil {
		t.Fatalf("Apply NineTerminals: %v", err)
	}
	he := findHandEndedEvent(t, evs)
	if he.Reason != rules.HandEndNineTerminals || he.Score != nil {
		t.Fatalf("HandEndedEvent = %+v, want HandEndNineTerminals", he)
	}
}

func TestTurnEngine_TripleRonDraw(t *testing.T) {
	t.Parallel()

	// All 3 non-dealer seats wait on RedDragon; Dealer Seat1 discards RedDragon -> Triple Ron draw!
	hands := map[rules.Seat][]rules.Tile{
		rules.Seat1: {
			rules.Wan1, rules.Wan3, rules.Wan5, rules.Wan7, rules.Wan9,
			rules.Suo1, rules.Suo3, rules.Suo5, rules.Suo7, rules.Suo9,
			rules.Tong1, rules.Tong3, rules.Tong5, rules.Tong7, rules.Tong9,
			rules.EastWind, rules.RedDragon,
		},
		rules.Seat2: {
			rules.Wan1, rules.Wan2, rules.Wan3,
			rules.Wan4, rules.Wan5, rules.Wan6,
			rules.Suo2, rules.Suo3, rules.Suo4,
			rules.Tong2, rules.Tong3, rules.Tong4,
			rules.Tong7, rules.Tong8, rules.Tong9,
			rules.RedDragon,
		},
		rules.Seat3: {
			rules.Wan1, rules.Wan2, rules.Wan3,
			rules.Wan4, rules.Wan5, rules.Wan6,
			rules.Suo2, rules.Suo3, rules.Suo4,
			rules.Tong2, rules.Tong3, rules.Tong4,
			rules.Tong7, rules.Tong8, rules.Tong9,
			rules.RedDragon,
		},
		rules.Seat4: {
			rules.Wan2, rules.Wan3, rules.Wan4,
			rules.Wan5, rules.Wan6, rules.Wan7,
			rules.Suo4, rules.Suo5, rules.Suo6,
			rules.Tong1, rules.Tong2, rules.Tong3,
			rules.Tong6, rules.Tong7, rules.Tong8,
			rules.RedDragon,
		},
	}
	wall := makePreparedWall(t, hands, nil, nil)
	eng, err := rules.NewTurnEngine(rules.TurnConfig{
		Wall:      wall,
		RoundWind: rules.WindEast,
		SeatWinds: defaultSeatWinds(),
		Dealer:    rules.Seat1,
		Hands:     hands,
		Base:      rules.DefaultBase,
		PerFan:    rules.DefaultPerFan,
	})
	if err != nil {
		t.Fatalf("NewTurnEngine: %v", err)
	}
	eng.Start()

	if _, err := eng.Apply(rules.PlayerAction{Seat: rules.Seat1, Kind: rules.ActionDiscard, Tile: rules.RedDragon}, 0); err != nil {
		t.Fatalf("Seat1 discard RedDragon: %v", err)
	}
	for _, s := range []rules.Seat{rules.Seat2, rules.Seat3} {
		if _, err := eng.Apply(rules.PlayerAction{Seat: s, Kind: rules.ActionClaim, Claim: rules.ClaimWin}, 0); err != nil {
			t.Fatalf("seat %v claim win: %v", s, err)
		}
		if eng.Phase() != rules.PhaseWaitClaims {
			t.Fatalf("after %v win, phase = %v, want PhaseWaitClaims until 3rd seat decides", s, eng.Phase())
		}
	}
	evs, err := eng.Apply(rules.PlayerAction{Seat: rules.Seat4, Kind: rules.ActionClaim, Claim: rules.ClaimWin}, 0)
	if err != nil {
		t.Fatalf("Seat4 claim win: %v", err)
	}
	he := findHandEndedEvent(t, evs)
	if he.Reason != rules.HandEndTripleRon || he.Score != nil {
		t.Fatalf("HandEndedEvent = %+v, want HandEndTripleRon", he)
	}
}

func TestTurnEngine_Riichi_TianTing_LockedDiscard_And_PermanentPassedWin(t *testing.T) {
	t.Parallel()

	t.Run("first-round unbroken riichi marks TianTing and scores 8 fan without revealing opponent hands", func(t *testing.T) {
		t.Parallel()
		hands := nonTenpaiTurnHands()
		// Seat2 is 16 tiles: 4 complete sets + WhiteDragon pair + Suo5, Suo6 (tenpai waiting Suo4/Suo7).
		hands[rules.Seat2] = []rules.Tile{
			rules.Wan1, rules.Wan2, rules.Wan3,
			rules.Wan4, rules.Wan5, rules.Wan6,
			rules.Tong1, rules.Tong2, rules.Tong3,
			rules.Tong7, rules.Tong8, rules.Tong9,
			rules.WhiteDragon, rules.WhiteDragon,
			rules.Suo5, rules.Suo6,
		}
		// Seat1 discards SouthWind; Seat2 draws NorthWind (17 tiles, can Riichi discarding NorthWind);
		// Seat3 draws Wan1 and discards Suo7 -> Seat2 wins with TianTing (8 fan)!
		hands[rules.Seat3][0] = rules.Suo7
		wall := makePreparedWall(t, hands, nil, []rules.Tile{
			rules.NorthWind, // Seat2 draw
			rules.Wan1,      // Seat3 draw
		})
		eng, err := rules.NewTurnEngine(rules.TurnConfig{
			Wall:      wall,
			RoundWind: rules.WindEast,
			SeatWinds: defaultSeatWinds(),
			Dealer:    rules.Seat1,
			Hands:     hands,
			Base:      rules.DefaultBase,
			PerFan:    rules.DefaultPerFan,
		})
		if err != nil {
			t.Fatalf("NewTurnEngine: %v", err)
		}
		eng.Start()

		if _, err := eng.Apply(rules.PlayerAction{Seat: rules.Seat1, Kind: rules.ActionDiscard, Tile: rules.SouthWind}, 0); err != nil {
			t.Fatalf("Seat1 discard: %v", err)
		}
		drawEvs, err := eng.Apply(rules.PlayerAction{Seat: rules.Seat2, Kind: rules.ActionDraw}, 0)
		if err != nil {
			t.Fatalf("Seat2 draw: %v", err)
		}
		prompt := findSelfDeclarePrompt(t, drawEvs, rules.Seat2)
		if !prompt.CanRiichi || !slices.Contains(prompt.RiichiDiscardTiles, rules.NorthWind) {
			t.Fatalf("expected CanRiichi with NorthWind, got %+v", prompt)
		}

		riichiEvs, err := eng.Apply(rules.PlayerAction{
			Seat:        rules.Seat2,
			Kind:        rules.ActionSelfDeclare,
			SelfDeclare: rules.SelfDeclareRiichi,
			Tile:        rules.NorthWind,
		}, 3*time.Second)
		if err != nil {
			t.Fatalf("Seat2 Riichi: %v", err)
		}
		if !eng.Riichi(rules.Seat2) || !eng.TianTing(rules.Seat2) {
			t.Fatalf("expected Riichi=true TianTing=true for Seat2, got (%v, %v)", eng.Riichi(rules.Seat2), eng.TianTing(rules.Seat2))
		}
		foundRiichi := false
		for _, ev := range riichiEvs {
			switch e := ev.(type) {
			case rules.RiichiDeclaredEvent:
				if e.Seat == rules.Seat2 && e.IsTianTing {
					foundRiichi = true
				}
			case rules.HandsRevealedEvent:
				t.Fatal("Riichi MUST NOT reveal opponent hands")
			}
		}
		if !foundRiichi {
			t.Fatalf("missing RiichiDeclaredEvent(Seat2, IsTianTing=true) in %+v", riichiEvs)
		}
		if !hasDiscardedEvent(riichiEvs, rules.Seat2, rules.NorthWind, 3*time.Second) {
			t.Fatalf("expected DiscardedEvent(Seat2, NorthWind, 3s) in %+v", riichiEvs)
		}

		// Seat3 draws Wan1 and discards Suo7 -> Seat2 claims Win -> scores FanTianTing (8)!
		if _, err := eng.Apply(rules.PlayerAction{Seat: rules.Seat3, Kind: rules.ActionDraw}, 0); err != nil {
			t.Fatalf("Seat3 draw: %v", err)
		}
		if _, err := eng.Apply(rules.PlayerAction{Seat: rules.Seat3, Kind: rules.ActionDiscard, Tile: rules.Suo7}, 0); err != nil {
			t.Fatalf("Seat3 discard Suo7: %v", err)
		}
		winEvs, err := eng.Apply(rules.PlayerAction{Seat: rules.Seat2, Kind: rules.ActionClaim, Claim: rules.ClaimWin}, 0)
		if err != nil {
			t.Fatalf("Seat2 win: %v", err)
		}
		he := findHandEndedEvent(t, winEvs)
		if he.Reason != rules.HandEndWin || he.Score == nil {
			t.Fatalf("expected win result, got %+v", he)
		}
		assertHasFan(t, *he.Score, rules.FanTianTing, 8)
		for _, f := range he.Score.Fans {
			if f.ID == rules.FanRiichi {
				t.Fatalf("TianTing must replace FanRiichi, got %+v", he.Score.Fans)
			}
		}
	})

	t.Run("mid-hand riichi locks discard to drawn tile suppresses pong and makes passed-win permanent", func(t *testing.T) {
		t.Parallel()
		hands := nonTenpaiTurnHands()
		// Seat1 starts 1-away from tenpai (not tenpai yet on turn 1), discards SouthWind,
		// then on turn 2 draws Suo6 to reach tenpai and declares mid-hand Riichi (IsTianTing=false)!
		hands[rules.Seat1] = []rules.Tile{
			rules.Wan1, rules.Wan2, rules.Wan3,
			rules.Wan4, rules.Wan5, rules.Wan6,
			rules.Tong1, rules.Tong2, rules.Tong3,
			rules.Tong7, rules.Tong8, rules.Tong9,
			rules.WhiteDragon, rules.WhiteDragon,
			rules.Suo5, rules.SouthWind, rules.EastWind,
		}
		// Give Seat2 WhiteDragon (matches Seat1's pair) and Suo7 (Seat1's win tile) to discard later.
		hands[rules.Seat2][0] = rules.WhiteDragon
		hands[rules.Seat2][1] = rules.Suo7
		// Give Seat3 Suo4 (Seat1's other win tile), replace Seat3's Suo8 so Seat3 cannot Chow Suo7,
		// and replace Seat3's WhiteDragon so total WhiteDragon <= 4.
		hands[rules.Seat3][0] = rules.Suo4
		hands[rules.Seat3][7] = rules.Tong1
		hands[rules.Seat3][14] = rules.Wan1

		wall := makePreparedWall(t, hands, nil, []rules.Tile{
			rules.Wan9, // Seat2 turn 1 draw
			rules.Suo9, // Seat3 turn 1 draw
			rules.Suo1, // Seat4 turn 1 draw
			rules.Suo6, // Seat1 turn 2 draw -> now tenpai discarding EastWind!
			rules.Wan9, // Seat2 turn 2 draw
			rules.Suo9, // Seat3 turn 2 draw
			rules.Suo1, // Seat4 turn 2 draw
			rules.Suo1, // Seat1 turn 3 draw -> non-winning non-flower tile (must stay locked & stay passed-win!)
			rules.Wan9, // Seat2 turn 3 draw
			rules.Suo9, // Seat3 turn 3 draw -> Seat3 discards Suo4 (Seat1 must NOT be prompted to win!)
		})
		eng, err := rules.NewTurnEngine(rules.TurnConfig{
			Wall:      wall,
			RoundWind: rules.WindEast,
			SeatWinds: defaultSeatWinds(),
			Dealer:    rules.Seat1,
			Hands:     hands,
			Base:      rules.DefaultBase,
			PerFan:    rules.DefaultPerFan,
		})
		if err != nil {
			t.Fatalf("NewTurnEngine: %v", err)
		}
		eng.Start()

		// Round 1: Seat1 discards SouthWind; Seats 2,3,4 draw & discard their newly drawn tiles.
		if _, err := eng.Apply(rules.PlayerAction{Seat: rules.Seat1, Kind: rules.ActionDiscard, Tile: rules.SouthWind}, 0); err != nil {
			t.Fatalf("Seat1 turn1 discard: %v", err)
		}
		for _, pair := range []struct {
			seat rules.Seat
			tile rules.Tile
		}{
			{rules.Seat2, rules.Wan9},
			{rules.Seat3, rules.Suo9},
			{rules.Seat4, rules.Suo1},
		} {
			if _, err := eng.Apply(rules.PlayerAction{Seat: pair.seat, Kind: rules.ActionDraw}, 0); err != nil {
				t.Fatalf("%v draw: %v", pair.seat, err)
			}
			if _, err := eng.Apply(rules.PlayerAction{Seat: pair.seat, Kind: rules.ActionDiscard, Tile: pair.tile}, 0); err != nil {
				t.Fatalf("%v discard: %v", pair.seat, err)
			}
		}

		// Round 2: Seat1 draws Suo6 and declares mid-hand Riichi discarding EastWind (IsTianTing=false).
		if _, err := eng.Apply(rules.PlayerAction{Seat: rules.Seat1, Kind: rules.ActionDraw}, 0); err != nil {
			t.Fatalf("Seat1 turn2 draw: %v", err)
		}
		if _, err := eng.Apply(rules.PlayerAction{
			Seat:        rules.Seat1,
			Kind:        rules.ActionSelfDeclare,
			SelfDeclare: rules.SelfDeclareRiichi,
			Tile:        rules.EastWind,
		}, 0); err != nil {
			t.Fatalf("Seat1 Riichi: %v", err)
		}
		if !eng.Riichi(rules.Seat1) || eng.TianTing(rules.Seat1) {
			t.Fatalf("expected mid-hand Riichi=true TianTing=false, got (%v, %v)", eng.Riichi(rules.Seat1), eng.TianTing(rules.Seat1))
		}

		// Seat2 draws and discards Suo7 (Seat1's win tile) -> Seat1 declines (enters PERMANENT passed-win!).
		if _, err := eng.Apply(rules.PlayerAction{Seat: rules.Seat2, Kind: rules.ActionDraw}, 0); err != nil {
			t.Fatalf("Seat2 turn2 draw: %v", err)
		}
		discEvs, err := eng.Apply(rules.PlayerAction{Seat: rules.Seat2, Kind: rules.ActionDiscard, Tile: rules.Suo7}, 0)
		if err != nil {
			t.Fatalf("Seat2 discard Suo7: %v", err)
		}
		if !hasClaimPrompt(discEvs, rules.Seat1, rules.ClaimWindowDiscard, rules.ClaimWin) {
			t.Fatalf("expected Win prompt for Seat1 on Suo7, got %+v", discEvs)
		}
		if _, err := eng.Apply(rules.PlayerAction{Seat: rules.Seat1, Kind: rules.ActionClaim, Claim: rules.ClaimPass}, 0); err != nil {
			t.Fatalf("Seat1 pass win in Riichi: %v", err)
		}
		if !eng.PassedWin(rules.Seat1) {
			t.Fatal("Seat1 must enter passed-win after declining win in Riichi")
		}

		// Seat3 & Seat4 draw and discard.
		for _, pair := range []struct {
			seat rules.Seat
			tile rules.Tile
		}{
			{rules.Seat3, rules.Suo9},
			{rules.Seat4, rules.Suo1},
		} {
			if _, err := eng.Apply(rules.PlayerAction{Seat: pair.seat, Kind: rules.ActionDraw}, 0); err != nil {
				t.Fatalf("%v draw: %v", pair.seat, err)
			}
			if _, err := eng.Apply(rules.PlayerAction{Seat: pair.seat, Kind: rules.ActionDiscard, Tile: pair.tile}, 0); err != nil {
				t.Fatalf("%v discard: %v", pair.seat, err)
			}
		}

		// Round 3: Seat1 draws non-winning Suo1 -> passed-win MUST NOT clear in Riichi!
		if _, err := eng.Apply(rules.PlayerAction{Seat: rules.Seat1, Kind: rules.ActionDraw}, 0); err != nil {
			t.Fatalf("Seat1 turn3 draw: %v", err)
		}
		if !eng.PassedWin(rules.Seat1) {
			t.Fatal("passed-win MUST stay permanent in Riichi even after drawing non-winning non-flower tile")
		}
		// Seat1 tries to discard Wan1 instead of newly drawn Suo1 -> ErrRiichiLocked!
		if _, err := eng.Apply(rules.PlayerAction{Seat: rules.Seat1, Kind: rules.ActionDiscard, Tile: rules.Wan1}, 0); !errors.Is(err, rules.ErrRiichiLocked) {
			t.Fatalf("discarding non-drawn tile in Riichi err = %v, want ErrRiichiLocked", err)
		}
		// Discarding Suo1 succeeds.
		if _, err := eng.Apply(rules.PlayerAction{Seat: rules.Seat1, Kind: rules.ActionDiscard, Tile: rules.Suo1}, 0); err != nil {
			t.Fatalf("Seat1 discard drawn Suo1: %v", err)
		}

		// Seat2 discards WhiteDragon (matches Seat1's pair) -> Seat1 MUST NOT get Pong prompt while in Riichi!
		if _, err := eng.Apply(rules.PlayerAction{Seat: rules.Seat2, Kind: rules.ActionDraw}, 0); err != nil {
			t.Fatalf("Seat2 turn3 draw: %v", err)
		}
		wdEvs, err := eng.Apply(rules.PlayerAction{Seat: rules.Seat2, Kind: rules.ActionDiscard, Tile: rules.WhiteDragon}, 0)
		if err != nil {
			t.Fatalf("Seat2 discard WhiteDragon: %v", err)
		}
		if hasClaimPrompt(wdEvs, rules.Seat1, rules.ClaimWindowDiscard, rules.ClaimPong) {
			t.Fatalf("Riichi seat must NOT be offered Pong, got %+v", wdEvs)
		}

		// Seat3 draws and discards Suo4 (Seat1's win tile) -> Seat1 MUST NOT be prompted to win (permanent passed-win!).
		if _, err := eng.Apply(rules.PlayerAction{Seat: rules.Seat3, Kind: rules.ActionDraw}, 0); err != nil {
			t.Fatalf("Seat3 turn3 draw: %v", err)
		}
		suo4Evs, err := eng.Apply(rules.PlayerAction{Seat: rules.Seat3, Kind: rules.ActionDiscard, Tile: rules.Suo4}, 0)
		if err != nil {
			t.Fatalf("Seat3 discard Suo4: %v", err)
		}
		if hasClaimPrompt(suo4Evs, rules.Seat1, rules.ClaimWindowDiscard, rules.ClaimWin) {
			t.Fatalf("permanently passed-win Riichi seat must NOT be offered Win, got %+v", suo4Evs)
		}
	})

	t.Run("mid-hand riichi win scores FanRiichi 1 and suppresses concealed kong", func(t *testing.T) {
		t.Parallel()
		hands := nonTenpaiTurnHands()
		// Seat1 holds 3x Wan1 (can form concealed kong if drawing 4th Wan1) and becomes tenpai on turn 2 drawing Suo6.
		hands[rules.Seat1] = []rules.Tile{
			rules.Wan1, rules.Wan1, rules.Wan1,
			rules.Wan4, rules.Wan5, rules.Wan6,
			rules.Tong1, rules.Tong2, rules.Tong3,
			rules.Tong7, rules.Tong8, rules.Tong9,
			rules.WhiteDragon, rules.WhiteDragon,
			rules.Suo5, rules.SouthWind, rules.EastWind,
		}
		wall := makePreparedWall(t, hands, nil, []rules.Tile{
			rules.Wan9, // Seat2 turn 1
			rules.Suo9, // Seat3 turn 1
			rules.Suo1, // Seat4 turn 1
			rules.Suo6, // Seat1 turn 2 -> declares mid-hand Riichi discarding EastWind
			rules.Wan9, // Seat2 turn 2
			rules.Suo9, // Seat3 turn 2
			rules.Suo1, // Seat4 turn 2
			rules.Wan1, // Seat1 turn 3 -> 4th Wan1! Concealed kong MUST be suppressed in Riichi!
			rules.Wan9, // Seat2 turn 3
			rules.Suo9, // Seat3 turn 3
			rules.Suo1, // Seat4 turn 3
			rules.Suo7, // Seat1 turn 4 -> Self-draw win with FanRiichi (1), NOT FanTianTing!
		})
		eng, err := rules.NewTurnEngine(rules.TurnConfig{
			Wall:      wall,
			RoundWind: rules.WindEast,
			SeatWinds: defaultSeatWinds(),
			Dealer:    rules.Seat1,
			Hands:     hands,
			Base:      rules.DefaultBase,
			PerFan:    rules.DefaultPerFan,
		})
		if err != nil {
			t.Fatalf("NewTurnEngine: %v", err)
		}
		eng.Start()

		// Round 1
		if _, err := eng.Apply(rules.PlayerAction{Seat: rules.Seat1, Kind: rules.ActionDiscard, Tile: rules.SouthWind}, 0); err != nil {
			t.Fatalf("Seat1 discard SouthWind: %v", err)
		}
		for _, pair := range []struct {
			seat rules.Seat
			tile rules.Tile
		}{
			{rules.Seat2, rules.Wan9},
			{rules.Seat3, rules.Suo9},
			{rules.Seat4, rules.Suo1},
		} {
			if _, err := eng.Apply(rules.PlayerAction{Seat: pair.seat, Kind: rules.ActionDraw}, 0); err != nil {
				t.Fatalf("%v draw: %v", pair.seat, err)
			}
			if _, err := eng.Apply(rules.PlayerAction{Seat: pair.seat, Kind: rules.ActionDiscard, Tile: pair.tile}, 0); err != nil {
				t.Fatalf("%v discard: %v", pair.seat, err)
			}
		}

		// Round 2: Seat1 declares mid-hand Riichi
		if _, err := eng.Apply(rules.PlayerAction{Seat: rules.Seat1, Kind: rules.ActionDraw}, 0); err != nil {
			t.Fatalf("Seat1 turn2 draw: %v", err)
		}
		riichiEvs, err := eng.Apply(rules.PlayerAction{
			Seat:        rules.Seat1,
			Kind:        rules.ActionSelfDeclare,
			SelfDeclare: rules.SelfDeclareRiichi,
			Tile:        rules.EastWind,
		}, 0)
		if err != nil {
			t.Fatalf("Seat1 Riichi: %v", err)
		}
		foundMidRiichi := false
		for _, ev := range riichiEvs {
			if re, ok := ev.(rules.RiichiDeclaredEvent); ok && re.Seat == rules.Seat1 && !re.IsTianTing {
				foundMidRiichi = true
			}
		}
		if !foundMidRiichi {
			t.Fatalf("expected RiichiDeclaredEvent(Seat1, IsTianTing=false), got %+v", riichiEvs)
		}

		for _, pair := range []struct {
			seat rules.Seat
			tile rules.Tile
		}{
			{rules.Seat2, rules.Wan9},
			{rules.Seat3, rules.Suo9},
			{rules.Seat4, rules.Suo1},
		} {
			if _, err := eng.Apply(rules.PlayerAction{Seat: pair.seat, Kind: rules.ActionDraw}, 0); err != nil {
				t.Fatalf("%v draw: %v", pair.seat, err)
			}
			if _, err := eng.Apply(rules.PlayerAction{Seat: pair.seat, Kind: rules.ActionDiscard, Tile: pair.tile}, 0); err != nil {
				t.Fatalf("%v discard: %v", pair.seat, err)
			}
		}

		// Round 3: Seat1 draws 4th Wan1 -> MUST go directly to PhaseWaitDiscard (no concealed kong prompt!)
		if _, err := eng.Apply(rules.PlayerAction{Seat: rules.Seat1, Kind: rules.ActionDraw}, 0); err != nil {
			t.Fatalf("Seat1 turn3 draw Wan1: %v", err)
		}
		if eng.Phase() != rules.PhaseWaitDiscard {
			t.Fatalf("Phase() = %v, want PhaseWaitDiscard (concealed kong suppressed in Riichi)", eng.Phase())
		}
		if _, err := eng.Apply(rules.PlayerAction{Seat: rules.Seat1, Kind: rules.ActionDiscard, Tile: rules.Wan1}, 0); err != nil {
			t.Fatalf("Seat1 discard drawn Wan1: %v", err)
		}

		for _, pair := range []struct {
			seat rules.Seat
			tile rules.Tile
		}{
			{rules.Seat2, rules.Wan9},
			{rules.Seat3, rules.Suo9},
			{rules.Seat4, rules.Suo1},
		} {
			if _, err := eng.Apply(rules.PlayerAction{Seat: pair.seat, Kind: rules.ActionDraw}, 0); err != nil {
				t.Fatalf("%v draw: %v", pair.seat, err)
			}
			if _, err := eng.Apply(rules.PlayerAction{Seat: pair.seat, Kind: rules.ActionDiscard, Tile: pair.tile}, 0); err != nil {
				t.Fatalf("%v discard: %v", pair.seat, err)
			}
		}

		// Round 4: Seat1 self-draws Suo7 -> wins with FanRiichi (1) and NOT FanTianTing!
		if _, err := eng.Apply(rules.PlayerAction{Seat: rules.Seat1, Kind: rules.ActionDraw}, 0); err != nil {
			t.Fatalf("Seat1 turn4 draw Suo7: %v", err)
		}
		winEvs, err := eng.Apply(rules.PlayerAction{
			Seat:        rules.Seat1,
			Kind:        rules.ActionSelfDeclare,
			SelfDeclare: rules.SelfDeclareSelfWin,
		}, 0)
		if err != nil {
			t.Fatalf("Seat1 self-win: %v", err)
		}
		he := findHandEndedEvent(t, winEvs)
		if he.Reason != rules.HandEndWin || he.Score == nil {
			t.Fatalf("expected HandEndWin, got %+v", he)
		}
		assertHasFan(t, *he.Score, rules.FanRiichi, 1)
		for _, f := range he.Score.Fans {
			if f.ID == rules.FanTianTing {
				t.Fatalf("mid-hand Riichi must NOT score FanTianTing, got %+v", he.Score.Fans)
			}
		}
	})
}
