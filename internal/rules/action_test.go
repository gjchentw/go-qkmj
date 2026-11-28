package rules_test

import (
	"errors"
	"testing"

	"github.com/gjchentw/go-qkmj/internal/rules"
)

func TestPlayerAction_Validate(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name    string
		action  rules.PlayerAction
		wantErr error
	}{
		{
			name:   "valid draw",
			action: rules.PlayerAction{Seat: rules.Seat1, Kind: rules.ActionDraw},
		},
		{
			name:   "valid discard",
			action: rules.PlayerAction{Seat: rules.Seat2, Kind: rules.ActionDiscard, Tile: rules.Suo5},
		},
		{
			name:   "valid self declare pass",
			action: rules.PlayerAction{Seat: rules.Seat1, Kind: rules.ActionSelfDeclare, SelfDeclare: rules.SelfDeclarePass},
		},
		{
			name:   "valid self declare self win",
			action: rules.PlayerAction{Seat: rules.Seat1, Kind: rules.ActionSelfDeclare, SelfDeclare: rules.SelfDeclareSelfWin},
		},
		{
			name: "valid self declare concealed kong",
			action: rules.PlayerAction{
				Seat:        rules.Seat3,
				Kind:        rules.ActionSelfDeclare,
				SelfDeclare: rules.SelfDeclareConcealedKong,
				Tile:        rules.Wan4,
			},
		},
		{
			name: "valid self declare riichi with discard tile",
			action: rules.PlayerAction{
				Seat:        rules.Seat2,
				Kind:        rules.ActionSelfDeclare,
				SelfDeclare: rules.SelfDeclareRiichi,
				Tile:        rules.Suo9,
			},
		},
		{
			name: "valid self declare nine terminals",
			action: rules.PlayerAction{
				Seat:        rules.Seat1,
				Kind:        rules.ActionSelfDeclare,
				SelfDeclare: rules.SelfDeclareNineTerminals,
			},
		},
		{
			name: "valid claim chow with option",
			action: rules.PlayerAction{
				Seat:  rules.Seat2,
				Kind:  rules.ActionClaim,
				Claim: rules.ClaimChow,
				Chow:  rules.ChowOption{Tiles: [3]rules.Tile{rules.Wan2, rules.Wan3, rules.Wan4}},
			},
		},
		{
			name:   "valid claim pong",
			action: rules.PlayerAction{Seat: rules.Seat4, Kind: rules.ActionClaim, Claim: rules.ClaimPong},
		},
		{
			name:   "valid ready",
			action: rules.PlayerAction{Seat: rules.Seat1, Kind: rules.ActionReady},
		},
		{
			name:    "invalid zero seat",
			action:  rules.PlayerAction{Kind: rules.ActionDraw},
			wantErr: rules.ErrInvalidAction,
		},
		{
			name:    "invalid zero action kind",
			action:  rules.PlayerAction{Seat: rules.Seat1},
			wantErr: rules.ErrInvalidAction,
		},
		{
			name:    "discard flower rejected",
			action:  rules.PlayerAction{Seat: rules.Seat1, Kind: rules.ActionDiscard, Tile: rules.FlowerSpring},
			wantErr: rules.ErrInvalidAction,
		},
		{
			name:    "discard missing tile rejected",
			action:  rules.PlayerAction{Seat: rules.Seat1, Kind: rules.ActionDiscard},
			wantErr: rules.ErrInvalidAction,
		},
		{
			name: "concealed kong missing tile rejected",
			action: rules.PlayerAction{
				Seat:        rules.Seat1,
				Kind:        rules.ActionSelfDeclare,
				SelfDeclare: rules.SelfDeclareConcealedKong,
			},
			wantErr: rules.ErrInvalidAction,
		},
		{
			name: "riichi missing discard tile rejected",
			action: rules.PlayerAction{
				Seat:        rules.Seat1,
				Kind:        rules.ActionSelfDeclare,
				SelfDeclare: rules.SelfDeclareRiichi,
			},
			wantErr: rules.ErrInvalidAction,
		},
		{
			name: "riichi flower discard tile rejected",
			action: rules.PlayerAction{
				Seat:        rules.Seat1,
				Kind:        rules.ActionSelfDeclare,
				SelfDeclare: rules.SelfDeclareRiichi,
				Tile:        rules.FlowerSpring,
			},
			wantErr: rules.ErrInvalidAction,
		},
		{
			name: "claim chow invalid option rejected",
			action: rules.PlayerAction{
				Seat:  rules.Seat2,
				Kind:  rules.ActionClaim,
				Claim: rules.ClaimChow,
				Chow:  rules.ChowOption{Tiles: [3]rules.Tile{rules.EastWind, rules.SouthWind, rules.WestWind}},
			},
			wantErr: rules.ErrInvalidAction,
		},
		{
			name:    "claim unknown rejected",
			action:  rules.PlayerAction{Seat: rules.Seat2, Kind: rules.ActionClaim},
			wantErr: rules.ErrInvalidAction,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			err := tt.action.Validate()
			if tt.wantErr != nil {
				if !errors.Is(err, tt.wantErr) {
					t.Fatalf("Validate() err = %v, want %v", err, tt.wantErr)
				}
				return
			}
			if err != nil {
				t.Fatalf("Validate() unexpected err = %v", err)
			}
		})
	}
}

func TestClaimKind_PriorityOrder(t *testing.T) {
	t.Parallel()

	if rules.ClaimPass >= rules.ClaimChow ||
		rules.ClaimChow >= rules.ClaimPong ||
		rules.ClaimPong >= rules.ClaimKong ||
		rules.ClaimKong >= rules.ClaimWin {
		t.Fatal("ClaimKind priority must satisfy Pass < Chow < Pong < Kong < Win")
	}
}

func TestProjectMeld_HidesConcealedKongUntilRevealed(t *testing.T) {
	t.Parallel()

	ck := rules.Meld{
		Kind:  rules.MeldConcealedKong,
		Tiles: []rules.Tile{rules.Wan7, rules.Wan7, rules.Wan7, rules.Wan7},
	}
	hidden := rules.ProjectMeld(ck, false)
	if hidden.Kind != rules.MeldConcealedKong {
		t.Fatalf("hidden.Kind = %v, want MeldConcealedKong", hidden.Kind)
	}
	if len(hidden.Tiles) != 0 {
		t.Fatalf("hidden concealed kong must not expose tile faces, got %v", hidden.Tiles)
	}

	revealed := rules.ProjectMeld(ck, true)
	if len(revealed.Tiles) != 4 || revealed.Tiles[0] != rules.Wan7 {
		t.Fatalf("revealed concealed kong tiles = %v, want 4x Wan7", revealed.Tiles)
	}
	// Mutating projected slice must not mutate original meld.
	revealed.Tiles[0] = rules.Tong1
	if ck.Tiles[0] != rules.Wan7 {
		t.Fatal("ProjectMeld must return defensive copy of tiles")
	}

	exposed := rules.Meld{
		Kind:     rules.MeldPong,
		Tiles:    []rules.Tile{rules.RedDragon, rules.RedDragon, rules.RedDragon},
		FromSeat: rules.Seat3,
	}
	pub := rules.ProjectMeld(exposed, false)
	if len(pub.Tiles) != 3 || pub.FromSeat != rules.Seat3 {
		t.Fatalf("exposed meld projection = %+v, want 3 tiles from Seat3", pub)
	}
}
