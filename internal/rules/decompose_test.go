package rules_test

import (
	"testing"

	"github.com/gjchentw/go-qkmj/internal/rules"
)

func TestCanWin_BasicHands(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name      string
		concealed []rules.Tile
		melds     []rules.Meld
		winTile   rules.Tile
		want      bool
	}{
		{
			name: "standard 5 groups plus pair all concealed",
			concealed: []rules.Tile{
				rules.Wan1, rules.Wan2, rules.Wan3,
				rules.Wan4, rules.Wan5, rules.Wan6,
				rules.Suo7, rules.Suo8, rules.Suo9,
				rules.Tong2, rules.Tong3, rules.Tong4,
				rules.Tong6, rules.Tong7,
				rules.WhiteDragon, rules.WhiteDragon,
			},
			winTile: rules.Tong8,
			want:    true,
		},
		{
			name: "standard hand with 1 exposed pong",
			concealed: []rules.Tile{
				rules.Wan1, rules.Wan2, rules.Wan3,
				rules.Wan4, rules.Wan5, rules.Wan6,
				rules.Suo9, rules.Suo9,
				rules.Tong2, rules.Tong3, rules.Tong4,
				rules.Tong6, rules.Tong7,
			},
			melds: []rules.Meld{
				{Kind: rules.MeldPong, Tiles: []rules.Tile{rules.Suo2, rules.Suo2, rules.Suo2}},
			},
			winTile: rules.Tong8,
			want:    true,
		},
		{
			name: "honor tiles cannot form sequence",
			concealed: []rules.Tile{
				rules.Wan1, rules.Wan2, rules.Wan3,
				rules.Wan4, rules.Wan5, rules.Wan6,
				rules.Suo7, rules.Suo8, rules.Suo9,
				rules.Tong2, rules.Tong3, rules.Tong4,
				rules.EastWind, rules.SouthWind,
				rules.WhiteDragon, rules.WhiteDragon,
			},
			winTile: rules.WestWind,
			want:    false,
		},
		{
			name: "cross suit sequence rejected",
			concealed: []rules.Tile{
				rules.Wan1, rules.Wan2, rules.Wan3,
				rules.Wan4, rules.Wan5, rules.Wan6,
				rules.Suo7, rules.Suo8, rules.Suo9,
				rules.Tong2, rules.Tong3, rules.Tong4,
				rules.Wan8, rules.Wan9,
				rules.WhiteDragon, rules.WhiteDragon,
			},
			winTile: rules.Suo1,
			want:    false,
		},
		{
			name: "flower tile in concealed hand rejected",
			concealed: []rules.Tile{
				rules.Wan1, rules.Wan2, rules.Wan3,
				rules.Wan4, rules.Wan5, rules.Wan6,
				rules.Suo7, rules.Suo8, rules.Suo9,
				rules.Tong2, rules.Tong3, rules.Tong4,
				rules.Tong6, rules.Tong7,
				rules.FlowerSpring, rules.FlowerSpring,
			},
			winTile: rules.Tong8,
			want:    false,
		},
		{
			name: "wrong tile count rejected",
			concealed: []rules.Tile{
				rules.Wan1, rules.Wan2, rules.Wan3,
				rules.WhiteDragon, rules.WhiteDragon,
			},
			winTile: rules.Wan4,
			want:    false,
		},
		{
			name: "five melds single wait (all-claim)",
			concealed: []rules.Tile{
				rules.Wan5,
			},
			melds: []rules.Meld{
				{Kind: rules.MeldChow, Tiles: []rules.Tile{rules.Wan2, rules.Wan3, rules.Wan4}},
				{Kind: rules.MeldChow, Tiles: []rules.Tile{rules.Tong5, rules.Tong6, rules.Tong7}},
				{Kind: rules.MeldPong, Tiles: []rules.Tile{rules.Suo2, rules.Suo2, rules.Suo2}},
				{Kind: rules.MeldChow, Tiles: []rules.Tile{rules.Suo3, rules.Suo4, rules.Suo5}},
				{Kind: rules.MeldPong, Tiles: []rules.Tile{rules.Tong9, rules.Tong9, rules.Tong9}},
			},
			winTile: rules.Wan5,
			want:    true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			got := rules.CanWin(tt.concealed, tt.melds, tt.winTile)
			if got != tt.want {
				t.Fatalf("CanWin() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestDecompose_MultipleDecompositionsAndWinRoles(t *testing.T) {
	t.Parallel()

	// Three copies each of Wan1, Wan2, Wan3 can decompose as 3 triplets OR 3 sequences.
	concealed := []rules.Tile{
		rules.Wan1, rules.Wan1, rules.Wan1,
		rules.Wan2, rules.Wan2, rules.Wan2,
		rules.Wan3, rules.Wan3, rules.Wan3,
		rules.Suo4, rules.Suo5, rules.Suo6,
		rules.Tong7, rules.Tong8,
		rules.RedDragon, rules.RedDragon,
	}
	decomps := rules.Decompose(concealed, nil, rules.Tong9)
	if len(decomps) != 2 {
		t.Fatalf("len(Decompose) = %d, want 2 (triplets vs sequences)", len(decomps))
	}

	hasTriplets := false
	hasSequences := false
	for _, d := range decomps {
		wanTriplets := 0
		wanSeqs := 0
		for _, g := range d.Groups {
			if g.Base.Suit() == rules.SuitWan {
				if g.Kind == rules.GroupTriplet {
					wanTriplets++
				}
				if g.Kind == rules.GroupSequence {
					wanSeqs++
				}
			}
		}
		if wanTriplets == 3 {
			hasTriplets = true
		}
		if wanSeqs == 3 {
			hasSequences = true
		}
		if !d.IsTwoSidedWait(rules.Tong9) {
			t.Errorf("winning Tong9 on 7-8 Tong must be a two-sided wait")
		}
	}
	if !hasTriplets || !hasSequences {
		t.Fatalf("expected both triplet and sequence decompositions, got triplets=%v seqs=%v", hasTriplets, hasSequences)
	}
}

func TestDecompose_TwoSidedWaitEdgeCases(t *testing.T) {
	t.Parallel()

	base := []rules.Tile{
		rules.Wan2, rules.Wan3, rules.Wan4,
		rules.Wan5, rules.Wan6, rules.Wan7,
		rules.Tong3, rules.Tong4, rules.Tong5,
		rules.Tong6, rules.Tong7, rules.Tong8,
		rules.Tong9, rules.Tong9,
	}

	tests := []struct {
		name      string
		waitTiles []rules.Tile
		winTile   rules.Tile
		twoSided  bool
	}{
		{name: "2-3 waiting 4 is two-sided", waitTiles: []rules.Tile{rules.Suo2, rules.Suo3}, winTile: rules.Suo4, twoSided: true},
		{name: "2-3 waiting 1 is two-sided", waitTiles: []rules.Tile{rules.Suo2, rules.Suo3}, winTile: rules.Suo1, twoSided: true},
		{name: "7-8 waiting 9 is two-sided", waitTiles: []rules.Tile{rules.Suo7, rules.Suo8}, winTile: rules.Suo9, twoSided: true},
		{name: "7-8 waiting 6 is two-sided", waitTiles: []rules.Tile{rules.Suo7, rules.Suo8}, winTile: rules.Suo6, twoSided: true},
		{name: "1-2 waiting 3 is edge wait", waitTiles: []rules.Tile{rules.Suo1, rules.Suo2}, winTile: rules.Suo3, twoSided: false},
		{name: "8-9 waiting 7 is edge wait", waitTiles: []rules.Tile{rules.Suo8, rules.Suo9}, winTile: rules.Suo7, twoSided: false},
		{name: "2-4 waiting 3 is middle wait", waitTiles: []rules.Tile{rules.Suo2, rules.Suo4}, winTile: rules.Suo3, twoSided: false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			hand := append(append([]rules.Tile(nil), base...), tt.waitTiles...)
			decomps := rules.Decompose(hand, nil, tt.winTile)
			if len(decomps) == 0 {
				t.Fatalf("expected valid decomposition for %s", tt.name)
			}
			anyTwoSided := false
			for _, d := range decomps {
				if d.IsTwoSidedWait(tt.winTile) {
					anyTwoSided = true
				}
			}
			if anyTwoSided != tt.twoSided {
				t.Errorf("IsTwoSidedWait = %v, want %v", anyTwoSided, tt.twoSided)
			}
		})
	}
}

func TestDecompose_NoDuplicateTripletSequenceOrdering(t *testing.T) {
	t.Parallel()

	// 4x Wan1 + Wan2 + Wan3 yields Triplet(Wan1) + Seq(Wan1) exactly once (not twice).
	concealed := []rules.Tile{
		rules.Wan1, rules.Wan1, rules.Wan1, rules.Wan1, rules.Wan2, rules.Wan3,
		rules.Suo4, rules.Suo5, rules.Suo6,
		rules.Tong2, rules.Tong3, rules.Tong4,
		rules.Tong6, rules.Tong7,
		rules.WhiteDragon, rules.WhiteDragon,
	}
	decomps := rules.Decompose(concealed, nil, rules.Tong8)
	if len(decomps) != 1 {
		t.Fatalf("len(Decompose) = %d, want 1 (deduplicated canonical order)", len(decomps))
	}
}
