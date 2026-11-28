package rules_test

import (
	"errors"
	"testing"

	"github.com/gjchentw/go-qkmj/internal/rules"
)

func TestTile_ZeroValueInvalid(t *testing.T) {
	t.Parallel()

	var zero rules.Tile
	if zero.Valid() {
		t.Fatal("zero Tile must be invalid")
	}
	if zero.Suit() != rules.SuitUnknown {
		t.Fatalf("zero Tile suit = %v, want SuitUnknown", zero.Suit())
	}
	if zero.Rank() != 0 {
		t.Fatalf("zero Tile rank = %d, want 0", zero.Rank())
	}
	if zero.IsFlower() || zero.IsHonor() || zero.IsNumber() || zero.IsTerminal() || zero.IsSimple() {
		t.Fatal("zero Tile must return false for all classification predicates")
	}

	var zeroSuit rules.Suit
	if zeroSuit.Valid() {
		t.Fatal("zero Suit must be invalid")
	}
	var zeroWind rules.Wind
	if zeroWind.Valid() {
		t.Fatal("zero Wind must be invalid")
	}
	var zeroSeat rules.Seat
	if zeroSeat.Valid() {
		t.Fatal("zero Seat must be invalid")
	}
	var zeroMeld rules.MeldKind
	if zeroMeld.Valid() {
		t.Fatal("zero MeldKind must be invalid")
	}
}

func TestTile_ClassificationAndNames(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name       string
		tile       rules.Tile
		wantName   string
		wantSuit   rules.Suit
		wantRank   int
		isNumber   bool
		isHonor    bool
		isTerminal bool
		isSimple   bool
		isFlower   bool
		flowerWind rules.Wind
	}{
		{
			name:       "one wan terminal",
			tile:       rules.Wan1,
			wantName:   "一萬",
			wantSuit:   rules.SuitWan,
			wantRank:   1,
			isNumber:   true,
			isTerminal: true,
		},
		{
			name:     "five suo simple",
			tile:     rules.Suo5,
			wantName: "五索",
			wantSuit: rules.SuitSuo,
			wantRank: 5,
			isNumber: true,
			isSimple: true,
		},
		{
			name:       "nine tong terminal",
			tile:       rules.Tong9,
			wantName:   "九筒",
			wantSuit:   rules.SuitTong,
			wantRank:   9,
			isNumber:   true,
			isTerminal: true,
		},
		{
			name:     "east wind honor",
			tile:     rules.EastWind,
			wantName: "東風",
			wantSuit: rules.SuitWind,
			wantRank: 1,
			isHonor:  true,
		},
		{
			name:     "red dragon honor",
			tile:     rules.RedDragon,
			wantName: "紅中",
			wantSuit: rules.SuitDragon,
			isHonor:  true,
		},
		{
			name:     "white dragon honor",
			tile:     rules.WhiteDragon,
			wantName: "白板",
			wantSuit: rules.SuitDragon,
			isHonor:  true,
		},
		{
			name:     "green dragon honor",
			tile:     rules.GreenDragon,
			wantName: "青發",
			wantSuit: rules.SuitDragon,
			isHonor:  true,
		},
		{
			name:       "spring flower matches east",
			tile:       rules.FlowerSpring,
			wantName:   "春１",
			wantSuit:   rules.SuitFlower,
			wantRank:   1,
			isFlower:   true,
			flowerWind: rules.WindEast,
		},
		{
			name:       "orchid flower matches south",
			tile:       rules.FlowerOrchid,
			wantName:   "蘭２",
			wantSuit:   rules.SuitFlower,
			wantRank:   2,
			isFlower:   true,
			flowerWind: rules.WindSouth,
		},
		{
			name:       "chrysanthemum flower matches west",
			tile:       rules.FlowerChrysanthemum,
			wantName:   "菊３",
			wantSuit:   rules.SuitFlower,
			wantRank:   3,
			isFlower:   true,
			flowerWind: rules.WindWest,
		},
		{
			name:       "bamboo flower matches north",
			tile:       rules.FlowerBamboo,
			wantName:   "竹４",
			wantSuit:   rules.SuitFlower,
			wantRank:   4,
			isFlower:   true,
			flowerWind: rules.WindNorth,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			if !tt.tile.Valid() {
				t.Fatalf("tile %v should be valid", tt.tile)
			}
			if got := tt.tile.String(); got != tt.wantName {
				t.Errorf("String() = %q, want %q", got, tt.wantName)
			}
			parsed, err := rules.ParseTile(tt.wantName)
			if err != nil || parsed != tt.tile {
				t.Errorf("ParseTile(%q) = (%v, %v), want (%v, nil)", tt.wantName, parsed, err, tt.tile)
			}
			if got := tt.tile.Suit(); got != tt.wantSuit {
				t.Errorf("Suit() = %v, want %v", got, tt.wantSuit)
			}
			if got := tt.tile.Rank(); got != tt.wantRank {
				t.Errorf("Rank() = %d, want %d", got, tt.wantRank)
			}
			if got := tt.tile.IsNumber(); got != tt.isNumber {
				t.Errorf("IsNumber() = %v, want %v", got, tt.isNumber)
			}
			if got := tt.tile.IsHonor(); got != tt.isHonor {
				t.Errorf("IsHonor() = %v, want %v", got, tt.isHonor)
			}
			if got := tt.tile.IsTerminal(); got != tt.isTerminal {
				t.Errorf("IsTerminal() = %v, want %v", got, tt.isTerminal)
			}
			if got := tt.tile.IsSimple(); got != tt.isSimple {
				t.Errorf("IsSimple() = %v, want %v", got, tt.isSimple)
			}
			if got := tt.tile.IsFlower(); got != tt.isFlower {
				t.Errorf("IsFlower() = %v, want %v", got, tt.isFlower)
			}
			if got := tt.tile.FlowerWind(); got != tt.flowerWind {
				t.Errorf("FlowerWind() = %v, want %v", got, tt.flowerWind)
			}
		})
	}
}

func TestFullDeck_Composition(t *testing.T) {
	t.Parallel()

	deck := rules.FullDeck()
	if len(deck) != rules.TotalDeckTiles {
		t.Fatalf("len(FullDeck()) = %d, want %d", len(deck), rules.TotalDeckTiles)
	}

	counts := make(map[rules.Tile]int)
	for _, tile := range deck {
		if !tile.Valid() {
			t.Fatalf("deck contains invalid tile: %v", tile)
		}
		counts[tile]++
	}

	for tile := rules.Wan1; tile <= rules.TileMax; tile++ {
		want := 4
		if tile.IsFlower() {
			want = 1
		}
		if counts[tile] != want {
			t.Errorf("count[%s] = %d, want %d", tile, counts[tile], want)
		}
	}
}

func TestSeat_RotationOrder(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name     string
		seat     rules.Seat
		next     rules.Seat
		prev     rules.Seat
		opposite rules.Seat
	}{
		{name: "seat 1", seat: rules.Seat1, next: rules.Seat2, prev: rules.Seat4, opposite: rules.Seat3},
		{name: "seat 2", seat: rules.Seat2, next: rules.Seat3, prev: rules.Seat1, opposite: rules.Seat4},
		{name: "seat 3", seat: rules.Seat3, next: rules.Seat4, prev: rules.Seat2, opposite: rules.Seat1},
		{name: "seat 4", seat: rules.Seat4, next: rules.Seat1, prev: rules.Seat3, opposite: rules.Seat2},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			if got := tt.seat.Next(); got != tt.next {
				t.Errorf("Next() = %v, want %v", got, tt.next)
			}
			if got := tt.seat.Prev(); got != tt.prev {
				t.Errorf("Prev() = %v, want %v", got, tt.prev)
			}
			if got := tt.seat.Opposite(); got != tt.opposite {
				t.Errorf("Opposite() = %v, want %v", got, tt.opposite)
			}
		})
	}
}

func TestMeld_ValidationAndEquivalentCount(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name            string
		meld            rules.Meld
		wantErr         error
		breaksConcealed bool
	}{
		{
			name: "valid chow",
			meld: rules.Meld{
				Kind:     rules.MeldChow,
				Tiles:    []rules.Tile{rules.Suo2, rules.Suo3, rules.Suo4},
				FromSeat: rules.Seat1,
			},
			breaksConcealed: true,
		},
		{
			name: "valid pong",
			meld: rules.Meld{
				Kind:     rules.MeldPong,
				Tiles:    []rules.Tile{rules.RedDragon, rules.RedDragon, rules.RedDragon},
				FromSeat: rules.Seat3,
			},
			breaksConcealed: true,
		},
		{
			name: "valid concealed kong keeps concealed",
			meld: rules.Meld{
				Kind:  rules.MeldConcealedKong,
				Tiles: []rules.Tile{rules.Wan5, rules.Wan5, rules.Wan5, rules.Wan5},
			},
			breaksConcealed: false,
		},
		{
			name: "valid added kong breaks concealed",
			meld: rules.Meld{
				Kind:     rules.MeldAddedKong,
				Tiles:    []rules.Tile{rules.Tong8, rules.Tong8, rules.Tong8, rules.Tong8},
				FromSeat: rules.Seat2,
			},
			breaksConcealed: true,
		},
		{
			name: "honor chow rejected",
			meld: rules.Meld{
				Kind:  rules.MeldChow,
				Tiles: []rules.Tile{rules.EastWind, rules.SouthWind, rules.WestWind},
			},
			wantErr: rules.ErrInvalidMeld,
		},
		{
			name: "cross suit chow rejected",
			meld: rules.Meld{
				Kind:  rules.MeldChow,
				Tiles: []rules.Tile{rules.Wan8, rules.Wan9, rules.Suo1},
			},
			wantErr: rules.ErrInvalidMeld,
		},
		{
			name: "flower in pong rejected",
			meld: rules.Meld{
				Kind:  rules.MeldPong,
				Tiles: []rules.Tile{rules.FlowerSpring, rules.FlowerSpring, rules.FlowerSpring},
			},
			wantErr: rules.ErrInvalidMeld,
		},
		{
			name: "kong with 3 tiles rejected",
			meld: rules.Meld{
				Kind:  rules.MeldExposedKong,
				Tiles: []rules.Tile{rules.Wan1, rules.Wan1, rules.Wan1},
			},
			wantErr: rules.ErrInvalidMeld,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			err := tt.meld.Validate()
			if tt.wantErr != nil {
				if !errors.Is(err, tt.wantErr) {
					t.Fatalf("Validate() err = %v, want %v", err, tt.wantErr)
				}
				return
			}
			if err != nil {
				t.Fatalf("Validate() unexpected err = %v", err)
			}
			if got := tt.meld.EquivalentTileCount(); got != 3 {
				t.Errorf("EquivalentTileCount() = %d, want 3", got)
			}
			if got := tt.meld.Kind.BreaksConcealed(); got != tt.breaksConcealed {
				t.Errorf("BreaksConcealed() = %v, want %v", got, tt.breaksConcealed)
			}
		})
	}
}

func TestValidateHandCount(t *testing.T) {
	t.Parallel()

	kong := rules.Meld{
		Kind:  rules.MeldConcealedKong,
		Tiles: []rules.Tile{rules.Wan1, rules.Wan1, rules.Wan1, rules.Wan1},
	}
	pong := rules.Meld{
		Kind:  rules.MeldPong,
		Tiles: []rules.Tile{rules.Suo2, rules.Suo2, rules.Suo2},
	}

	if err := rules.ValidateHandCount(16, nil, false); err != nil {
		t.Fatalf("16 concealed waiting should be valid: %v", err)
	}
	if err := rules.ValidateHandCount(14, []rules.Meld{pong}, true); err != nil {
		t.Fatalf("14 concealed + 1 pong after claim should be valid (17): %v", err)
	}
	if err := rules.ValidateHandCount(14, []rules.Meld{kong}, true); err != nil {
		t.Fatalf("14 concealed + 1 kong after draw should be valid (17): %v", err)
	}
	if err := rules.ValidateHandCount(15, []rules.Meld{pong}, true); !errors.Is(err, rules.ErrInvalidHandCount) {
		t.Fatalf("15 concealed + 1 pong (18) err = %v, want ErrInvalidHandCount", err)
	}
}
