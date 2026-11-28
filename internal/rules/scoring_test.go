package rules_test

import (
	"errors"
	"testing"

	"github.com/gjchentw/go-qkmj/internal/rules"
)

func TestScore_ValidationErrors(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name    string
		ctx     rules.ScoreContext
		wantErr error
	}{
		{
			name:    "unknown win kind rejected",
			ctx:     rules.ScoreContext{Winner: rules.Seat1, Dealer: rules.Seat1, RoundWind: rules.WindEast, SeatWind: rules.WindEast},
			wantErr: rules.ErrInvalidScoreContext,
		},
		{
			name: "discard win without payer rejected",
			ctx: rules.ScoreContext{
				WinKind:   rules.WinDiscard,
				Winner:    rules.Seat2,
				Dealer:    rules.Seat1,
				RoundWind: rules.WindEast,
				SeatWind:  rules.WindSouth,
			},
			wantErr: rules.ErrInvalidScoreContext,
		},
		{
			name: "negative base rejected",
			ctx: rules.ScoreContext{
				WinKind:   rules.WinEightImmortals,
				Winner:    rules.Seat1,
				Dealer:    rules.Seat1,
				RoundWind: rules.WindEast,
				SeatWind:  rules.WindEast,
				Base:      -100,
				PerFan:    200,
			},
			wantErr: rules.ErrInvalidScoreContext,
		},
		{
			name: "duplicate flower rejected",
			ctx: rules.ScoreContext{
				WinKind:   rules.WinEightImmortals,
				Winner:    rules.Seat1,
				Dealer:    rules.Seat1,
				RoundWind: rules.WindEast,
				SeatWind:  rules.WindEast,
				Flowers:   []rules.Tile{rules.FlowerSpring, rules.FlowerSpring},
			},
			wantErr: rules.ErrInvalidScoreContext,
		},
		{
			name: "non-winning hand rejected",
			ctx: rules.ScoreContext{
				Concealed: []rules.Tile{
					rules.Wan1, rules.Wan2, rules.Wan3,
					rules.Wan4, rules.Wan5, rules.Wan6,
					rules.Suo7, rules.Suo8, rules.Suo9,
					rules.Tong2, rules.Tong3, rules.Tong4,
					rules.EastWind, rules.SouthWind,
					rules.WhiteDragon, rules.WhiteDragon,
				},
				WinTile:   rules.WestWind,
				WinKind:   rules.WinSelfDraw,
				Winner:    rules.Seat2,
				Dealer:    rules.Seat1,
				RoundWind: rules.WindEast,
				SeatWind:  rules.WindNorth,
			},
			wantErr: rules.ErrNotWinningHand,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			_, err := rules.Score(tt.ctx)
			if !errors.Is(err, tt.wantErr) {
				t.Fatalf("Score() err = %v, want %v", err, tt.wantErr)
			}
		})
	}
}

func TestScore_HighestDecompositionChosen(t *testing.T) {
	t.Parallel()

	// 1,2,3 Wan x3 can be 3 triplets (三暗刻 2 + 門清 1 = 3) OR 3 sequences (一杯口 1 + 門清 1 = 2).
	// Scoring engine must choose the 3-triplet decomposition (3 fan).
	res, err := rules.Score(rules.ScoreContext{
		Concealed: []rules.Tile{
			rules.Wan1, rules.Wan1, rules.Wan1,
			rules.Wan2, rules.Wan2, rules.Wan2,
			rules.Wan3, rules.Wan3, rules.Wan3,
			rules.Suo4, rules.Suo5, rules.Suo6,
			rules.Tong7, rules.Tong8,
			rules.RedDragon, rules.RedDragon,
		},
		WinTile:   rules.Tong9,
		WinKind:   rules.WinDiscard,
		Winner:    rules.Seat2,
		Payer:     rules.Seat3,
		Dealer:    rules.Seat1,
		RoundWind: rules.WindEast,
		SeatWind:  rules.WindNorth,
	})
	if err != nil {
		t.Fatalf("Score() unexpected err = %v", err)
	}
	if res.TotalFan != 3 {
		t.Fatalf("TotalFan = %d, want 3 (fans: %+v)", res.TotalFan, res.Fans)
	}
	assertHasFan(t, res, rules.FanThreeConcealedTriplets, 2)
	assertHasFan(t, res, rules.FanConcealed, 1)
}

func TestScore_SpecialWinsAndRobKong(t *testing.T) {
	t.Parallel()

	t.Run("rob kong", func(t *testing.T) {
		t.Parallel()
		res, err := rules.Score(rules.ScoreContext{
			Concealed: []rules.Tile{
				rules.Wan1, rules.Wan2, rules.Wan3,
				rules.Wan4, rules.Wan5, rules.Wan6,
				rules.Suo9, rules.Suo9,
				rules.Tong2, rules.Tong3, rules.Tong4,
				rules.Tong6, rules.Tong7,
			},
			Melds: []rules.Meld{
				{Kind: rules.MeldPong, Tiles: []rules.Tile{rules.Suo2, rules.Suo2, rules.Suo2}},
			},
			WinTile:   rules.Tong8,
			WinKind:   rules.WinRobKong,
			Winner:    rules.Seat4,
			Payer:     rules.Seat2,
			Dealer:    rules.Seat1,
			RoundWind: rules.WindEast,
			SeatWind:  rules.WindWest,
		})
		if err != nil {
			t.Fatalf("Score() err = %v", err)
		}
		if res.TotalFan != 1 {
			t.Fatalf("TotalFan = %d, want 1 (fans: %+v)", res.TotalFan, res.Fans)
		}
		assertHasFan(t, res, rules.FanRobKong, 1)
		// Base 500 + 1*200 = 700 paid by Seat2.
		if res.Deltas[rules.Seat2] != -700 || res.Deltas[rules.Seat4] != 700 {
			t.Fatalf("Deltas = %+v, want Seat2=-700 Seat4=+700", res.Deltas)
		}
	})

	t.Run("seven rob one by non-dealer against dealer", func(t *testing.T) {
		t.Parallel()
		res, err := rules.Score(rules.ScoreContext{
			WinKind:      rules.WinSevenRobOne,
			Winner:       rules.Seat2,
			Payer:        rules.Seat1,
			Dealer:       rules.Seat1,
			DealerStreak: 1,
			RoundWind:    rules.WindEast,
			SeatWind:     rules.WindSouth,
		})
		if err != nil {
			t.Fatalf("Score() err = %v", err)
		}
		if res.TotalFan != 8 {
			t.Fatalf("TotalFan = %d, want 8", res.TotalFan)
		}
		if res.DealerExtraFan != 3 {
			t.Fatalf("DealerExtraFan = %d, want 3 (1 + 2*1)", res.DealerExtraFan)
		}
		// Dealer pays 500 + (8 + 3) * 200 = 2700.
		if res.Deltas[rules.Seat1] != -2700 || res.Deltas[rules.Seat2] != 2700 {
			t.Fatalf("Deltas = %+v, want Seat1=-2700 Seat2=+2700", res.Deltas)
		}
	})

	t.Run("eight immortals by dealer with streak 1", func(t *testing.T) {
		t.Parallel()
		res, err := rules.Score(rules.ScoreContext{
			WinKind:      rules.WinEightImmortals,
			Winner:       rules.Seat1,
			Dealer:       rules.Seat1,
			DealerStreak: 1,
			RoundWind:    rules.WindEast,
			SeatWind:     rules.WindEast,
		})
		if err != nil {
			t.Fatalf("Score() err = %v", err)
		}
		// 16 + 1 (莊家) + 2 (連１拉１) = 19 fan; each of 3 non-dealers pays 500 + 19*200 = 4300.
		if res.TotalFan != 19 {
			t.Fatalf("TotalFan = %d, want 19 (fans: %+v)", res.TotalFan, res.Fans)
		}
		if res.DealerExtraFan != 0 {
			t.Fatalf("DealerExtraFan = %d, want 0 when dealer is winner", res.DealerExtraFan)
		}
		if res.Deltas[rules.Seat2] != -4300 || res.Deltas[rules.Seat3] != -4300 ||
			res.Deltas[rules.Seat4] != -4300 || res.Deltas[rules.Seat1] != 12900 {
			t.Fatalf("Deltas = %+v, want -4300 each, +12900 for Seat1", res.Deltas)
		}
	})
}

func TestScore_Payments(t *testing.T) {
	t.Parallel()

	t.Run("non-dealer self draw 5 fan with dealer streak 1", func(t *testing.T) {
		t.Parallel()
		// Concealed self-draw (3) + 三色同順 (2) = 5 fan.
		res, err := rules.Score(rules.ScoreContext{
			Concealed: []rules.Tile{
				rules.Wan2, rules.Wan3, rules.Wan4,
				rules.Wan6, rules.Wan7, rules.Wan8,
				rules.Suo2, rules.Suo3, rules.Suo4,
				rules.Suo5, rules.Suo6,
				rules.Tong2, rules.Tong3, rules.Tong4,
				rules.WhiteDragon, rules.WhiteDragon,
			},
			WinTile:      rules.Suo7,
			WinKind:      rules.WinSelfDraw,
			Winner:       rules.Seat2,
			Dealer:       rules.Seat1,
			DealerStreak: 1,
			RoundWind:    rules.WindEast,
			SeatWind:     rules.WindNorth,
		})
		if err != nil {
			t.Fatalf("Score() err = %v", err)
		}
		if res.TotalFan != 5 {
			t.Fatalf("TotalFan = %d, want 5 (fans: %+v)", res.TotalFan, res.Fans)
		}
		// Dealer pays 500 + (5 + 1 + 2) * 200 = 2100; others pay 500 + 5 * 200 = 1500; winner gets 5100.
		if res.Deltas[rules.Seat1] != -2100 || res.Deltas[rules.Seat3] != -1500 ||
			res.Deltas[rules.Seat4] != -1500 || res.Deltas[rules.Seat2] != 5100 {
			t.Fatalf("Deltas = %+v, want Seat1=-2100 Seat3=-1500 Seat4=-1500 Seat2=+5100", res.Deltas)
		}
	})

	t.Run("non-dealer wins on dealer discard marks dealer extra fan", func(t *testing.T) {
		t.Parallel()
		// 門清 (1) + 一條龍 (2) = 3 fan; dealer streak 0 -> dealer pays 4 fan = 1300.
		res, err := rules.Score(rules.ScoreContext{
			Concealed: []rules.Tile{
				rules.Wan1, rules.Wan2, rules.Wan3,
				rules.Wan4, rules.Wan5, rules.Wan6,
				rules.Wan7, rules.Wan8, rules.Wan9,
				rules.Suo5, rules.Suo6,
				rules.Tong2, rules.Tong3, rules.Tong4,
				rules.WhiteDragon, rules.WhiteDragon,
			},
			WinTile:      rules.Suo7,
			WinKind:      rules.WinDiscard,
			Winner:       rules.Seat2,
			Payer:        rules.Seat1,
			Dealer:       rules.Seat1,
			DealerStreak: 0,
			RoundWind:    rules.WindEast,
			SeatWind:     rules.WindNorth,
		})
		if err != nil {
			t.Fatalf("Score() err = %v", err)
		}
		if res.TotalFan != 3 {
			t.Fatalf("TotalFan = %d, want 3 (fans: %+v)", res.TotalFan, res.Fans)
		}
		if res.DealerExtraFan != 1 {
			t.Fatalf("DealerExtraFan = %d, want 1", res.DealerExtraFan)
		}
		if res.Deltas[rules.Seat1] != -1300 || res.Deltas[rules.Seat2] != 1300 ||
			res.Deltas[rules.Seat3] != 0 || res.Deltas[rules.Seat4] != 0 {
			t.Fatalf("Deltas = %+v, want Seat1=-1300 Seat2=+1300 others=0", res.Deltas)
		}
	})
}

func assertHasFan(t *testing.T, res rules.ScoreResult, id rules.FanID, wantPoints int) {
	t.Helper()
	for _, f := range res.Fans {
		if f.ID == id {
			if f.Points != wantPoints {
				t.Errorf("fan %v (%s) points = %d, want %d", id, f.Name, f.Points, wantPoints)
			}
			return
		}
	}
	t.Errorf("missing expected fan %v (%d pts) in %+v", id, wantPoints, res.Fans)
}
