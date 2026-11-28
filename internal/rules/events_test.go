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
		{name: "SelfDeclarePromptEvent", ev: rules.SelfDeclarePromptEvent{Seat: rules.Seat3, CanSelfWin: true}, wantSeat: rules.Seat3},
		{name: "ClaimPromptEvent", ev: rules.ClaimPromptEvent{Seat: rules.Seat4, WindowKind: rules.ClaimWindowDiscard, Options: []rules.ClaimKind{rules.ClaimPong}}, wantSeat: rules.Seat4},
		{name: "ChowChoicePromptEvent", ev: rules.ChowChoicePromptEvent{Seat: rules.Seat2}, wantSeat: rules.Seat2},
		{name: "PassedWinChangedEvent", ev: rules.PassedWinChangedEvent{Seat: rules.Seat3, Passed: true}, wantSeat: rules.Seat3},
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
