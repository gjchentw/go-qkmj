package rules

import (
	"errors"
	"fmt"
	"math/rand/v2"
	"slices"
)

// Sentinel errors returned by Wall operations.
var (
	ErrNilRandSource = errors.New("rand source is nil")
	ErrWallExhausted = errors.New("wall has no drawable tiles remaining")
	ErrInvalidWall   = errors.New("invalid wall tile composition")
)

// Wall holds a shuffled 144-tile Mahjong wall while protecting the trailing 16 reserve tiles.
type Wall struct {
	tiles []Tile
	next  int
}

// NewWall creates a freshly shuffled 144-tile wall using the injected rand.Source.
// Global randomness is never used so callers control entropy and deterministic replay.
func NewWall(src rand.Source) (*Wall, error) {
	if src == nil {
		return nil, ErrNilRandSource
	}
	deck := FullDeck()
	//nolint:gosec // G404: rand.Source is injected by callers so production supplies crypto-seeded ChaCha8 while tests stay deterministic.
	rng := rand.New(src)
	rng.Shuffle(len(deck), func(i, j int) {
		deck[i], deck[j] = deck[j], deck[i]
	})
	return &Wall{tiles: deck}, nil
}

// NewWallFromTiles creates a wall from an explicit 144-tile sequence after validating
// that it contains the exact Taiwan 16-tile Mahjong deck composition.
func NewWallFromTiles(tiles []Tile) (*Wall, error) {
	if len(tiles) != TotalDeckTiles {
		return nil, fmt.Errorf("expected %d tiles, got %d: %w", TotalDeckTiles, len(tiles), ErrInvalidWall)
	}
	var counts [TileMax + 1]int
	for _, t := range tiles {
		if !t.Valid() {
			return nil, fmt.Errorf("tile %v: %w", t, ErrInvalidWall)
		}
		counts[t]++
	}
	for t := Wan1; t <= TileMax; t++ {
		want := 4
		if t.IsFlower() {
			want = 1
		}
		if counts[t] != want {
			return nil, fmt.Errorf("tile %s count %d, want %d: %w", t, counts[t], want, ErrInvalidWall)
		}
	}
	return &Wall{tiles: slices.Clone(tiles)}, nil
}

// Remaining returns the total number of undrawn tiles including the 16 reserve tiles.
func (w *Wall) Remaining() int {
	return len(w.tiles) - w.next
}

// DrawableRemaining returns the public count of tiles that can still be drawn (Remaining - 16).
func (w *Wall) DrawableRemaining() int {
	return max(0, w.Remaining()-ReserveWallTiles)
}

// Draw removes and returns the next drawable tile from the wall.
// It returns ErrWallExhausted when DrawableRemaining() == 0 and never draws reserve tiles.
func (w *Wall) Draw() (Tile, error) {
	if w.DrawableRemaining() == 0 {
		return TileInvalid, ErrWallExhausted
	}
	t := w.tiles[w.next]
	w.next++
	return t, nil
}
