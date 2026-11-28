package rules_test

import (
	"testing"
	"time"

	"github.com/gjchentw/go-qkmj/internal/rules"
)

func TestEvents_AudienceAndTargetSeat(t *testing.T) {
	t.Parallel()

	publicEvents := []struct {
		name string
		ev   rules.Event
	}{
		{name: "TurnChangedEvent", ev: rules.TurnChangedEvent{Seat: rules.Seat1, WaitingForDraw: true}},
		{name: "TileDrawnPublicEvent", ev: rules.TileDrawnPublicEvent{Seat: rules.Seat2, DrawableRemaining: 62}},
		{name: "FlowerExposedEvent", ev: rules.FlowerExposedEvent{Seat: rules.Seat3, Flower: rules.FlowerSpring, DrawableRemaining: 61}},
		{name: "MeldDeclaredEvent", ev: rules.NewMeldDeclaredEvent(rules.Seat2, rules.Meld{Kind: rules.MeldPong, Tiles: []rules.Tile{rules.Wan1, rules.Wan1, rules.Wan1}}, true)},
		{name: "DiscardedEvent", ev: rules.DiscardedEvent{Seat: rules.Seat1, Tile: rules.Suo5, ThinkingTime: 3 * time.Second}},
		{name: "HandEndedEvent", ev: rules.HandEndedEvent{Reason: rules.HandEndExhausted}},
		{name: "MatchStartedEvent", ev: rules.MatchStartedEvent{RoundWind: rules.WindEast, Dealer: rules.Seat1, DealerStreak: 0}},
		{name: "HandStartedEvent", ev: rules.HandStartedEvent{RoundWind: rules.WindEast, Dealer: rules.Seat1, DoorSeat: rules.Seat3}},
		{name: "HandDealtPublicEvent", ev: rules.HandDealtPublicEvent{DrawableRemaining: 63}},
		{name: "HandsRevealedEvent", ev: rules.HandsRevealedEvent{}},
		{name: "HandSettledEvent", ev: rules.HandSettledEvent{Reason: rules.HandEndExhausted}},
		{name: "WaitReadyEvent", ev: rules.WaitReadyEvent{}},
		{name: "PlayerReadyEvent", ev: rules.PlayerReadyEvent{Seat: rules.Seat2}},
		{name: "MatchProgressedEvent", ev: rules.MatchProgressedEvent{RoundWind: rules.WindSouth, Dealer: rules.Seat1, DealerStreak: 0}},
		{name: "MatchAbortedEvent", ev: rules.MatchAbortedEvent{Leaver: rules.Seat4, HandAborted: true}},
		{name: "RiichiDeclaredEvent", ev: rules.RiichiDeclaredEvent{Seat: rules.Seat2, IsTianTing: true}},
		{name: "RoundCycleCompletedEvent", ev: rules.RoundCycleCompletedEvent{
			CycleNumber: 1,
			Elapsed:     10 * time.Minute,
			Stats:       map[rules.Seat]rules.SeatCycleStats{rules.Seat1: {NetDelta: 1500, SelfDrawWins: 1}},
		}},
	}

	for _, tt := range publicEvents {
		t.Run("public_"+tt.name, func(t *testing.T) {
			t.Parallel()

			if got := tt.ev.Audience(); got != rules.AudiencePublic {
				t.Errorf("Audience() = %v, want AudiencePublic", got)
			}
			if got := tt.ev.TargetSeat(); got != rules.SeatUnknown {
				t.Errorf("TargetSeat() = %v, want SeatUnknown", got)
			}
		})
	}

	privateEvents := []struct {
		name     string
		ev       rules.Event
		wantSeat rules.Seat
	}{
		{name: "DrawPromptEvent", ev: rules.DrawPromptEvent{Seat: rules.Seat1}, wantSeat: rules.Seat1},
		{name: "TileDrawnEvent", ev: rules.TileDrawnEvent{Seat: rules.Seat2, Tile: rules.Tong7}, wantSeat: rules.Seat2},
		{name: "SelfDeclarePromptEvent", ev: rules.SelfDeclarePromptEvent{
			Seat:               rules.Seat3,
			CanSelfWin:         true,
			CanNineTerminals:   true,
			CanRiichi:          true,
			RiichiDiscardTiles: []rules.Tile{rules.Wan1},
		}, wantSeat: rules.Seat3},
		{name: "ClaimPromptEvent", ev: rules.ClaimPromptEvent{Seat: rules.Seat4, WindowKind: rules.ClaimWindowDiscard, Options: []rules.ClaimKind{rules.ClaimPong}}, wantSeat: rules.Seat4},
		{name: "ChowChoicePromptEvent", ev: rules.ChowChoicePromptEvent{Seat: rules.Seat2}, wantSeat: rules.Seat2},
		{name: "PassedWinChangedEvent", ev: rules.PassedWinChangedEvent{Seat: rules.Seat3, Passed: true}, wantSeat: rules.Seat3},
		{name: "HandDealtEvent", ev: rules.HandDealtEvent{Seat: rules.Seat1, Tiles: []rules.Tile{rules.Wan1}}, wantSeat: rules.Seat1},
	}

	for _, tt := range privateEvents {
		t.Run("private_"+tt.name, func(t *testing.T) {
			t.Parallel()

			if got := tt.ev.Audience(); got != rules.AudiencePrivate {
				t.Errorf("Audience() = %v, want AudiencePrivate", got)
			}
			if got := tt.ev.TargetSeat(); got != tt.wantSeat {
				t.Errorf("TargetSeat() = %v, want %v", got, tt.wantSeat)
			}
		})
	}
}

func TestHandEndReason_IsDraw(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name   string
		reason rules.HandEndReason
		want   bool
	}{
		{name: "unknown", reason: rules.HandEndUnknown, want: false},
		{name: "win", reason: rules.HandEndWin, want: false},
		{name: "aborted", reason: rules.HandEndAborted, want: false},
		{name: "exhausted", reason: rules.HandEndExhausted, want: true},
		{name: "four_winds", reason: rules.HandEndFourWinds, want: true},
		{name: "four_kongs", reason: rules.HandEndFourKongs, want: true},
		{name: "triple_ron", reason: rules.HandEndTripleRon, want: true},
		{name: "nine_terminals", reason: rules.HandEndNineTerminals, want: true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			if got := tt.reason.IsDraw(); got != tt.want {
				t.Errorf("HandEndReason(%d).IsDraw() = %v, want %v", tt.reason, got, tt.want)
			}
		})
	}
}

func TestNewMeldDeclaredEvent_HidesConcealedKongFaces(t *testing.T) {
	t.Parallel()

	ck := rules.Meld{
		Kind:  rules.MeldConcealedKong,
		Tiles: []rules.Tile{rules.RedDragon, rules.RedDragon, rules.RedDragon, rules.RedDragon},
	}
	ev := rules.NewMeldDeclaredEvent(rules.Seat2, ck, false)
	if ev.Audience() != rules.AudiencePublic {
		t.Fatalf("Audience() = %v, want AudiencePublic", ev.Audience())
	}
	if ev.Meld.Kind != rules.MeldConcealedKong {
		t.Fatalf("Meld.Kind = %v, want MeldConcealedKong", ev.Meld.Kind)
	}
	if len(ev.Meld.Tiles) != 0 {
		t.Fatalf("public concealed kong event must hide tile faces, got %v", ev.Meld.Tiles)
	}
}

func TestNewHandsRevealedEvent_RevealsConcealedKongsAndClones(t *testing.T) {
	t.Parallel()

	concealed := map[rules.Seat][]rules.Tile{
		rules.Seat1: {rules.Wan1, rules.Wan2},
		rules.Seat2: {rules.Suo3},
		rules.Seat3: {rules.Tong4},
		rules.Seat4: {rules.EastWind},
	}
	melds := map[rules.Seat][]rules.Meld{
		rules.Seat2: {
			{
				Kind:  rules.MeldConcealedKong,
				Tiles: []rules.Tile{rules.WhiteDragon, rules.WhiteDragon, rules.WhiteDragon, rules.WhiteDragon},
			},
		},
	}

	ev := rules.NewHandsRevealedEvent(concealed, melds)
	// Mutating inputs after construction must not affect event payload.
	concealed[rules.Seat1][0] = rules.Wan9
	melds[rules.Seat2][0].Tiles[0] = rules.GreenDragon

	if ev.Audience() != rules.AudiencePublic {
		t.Fatalf("Audience() = %v, want AudiencePublic", ev.Audience())
	}
	s1 := ev.Seats[rules.Seat1]
	if len(s1.Concealed) != 2 || s1.Concealed[0] != rules.Wan1 {
		t.Fatalf("Seat1 concealed = %v, want defensive copy starting with Wan1", s1.Concealed)
	}
	s2 := ev.Seats[rules.Seat2]
	if len(s2.Melds) != 1 || s2.Melds[0].Kind != rules.MeldConcealedKong || len(s2.Melds[0].Tiles) != 4 || s2.Melds[0].Tiles[0] != rules.WhiteDragon {
		t.Fatalf("Seat2 revealed concealed kong = %+v, want 4x WhiteDragon", s2.Melds)
	}
}
