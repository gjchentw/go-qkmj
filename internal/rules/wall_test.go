package rules_test

import (
	"errors"
	"math/rand/v2"
	"slices"
	"testing"

	"github.com/gjchentw/go-qkmj/internal/rules"
)

func TestNewWall_NilSourceRejected(t *testing.T) {
	t.Parallel()

	w, err := rules.NewWall(nil)
	if !errors.Is(err, rules.ErrNilRandSource) {
		t.Fatalf("NewWall(nil) err = %v, want ErrNilRandSource", err)
	}
	if w != nil {
		t.Fatalf("NewWall(nil) returned non-nil wall: %v", w)
	}
}

func TestNewWall_DeterministicShuffleAndComposition(t *testing.T) {
	t.Parallel()

	w1, err := rules.NewWall(rand.NewPCG(42, 99))
	if err != nil {
		t.Fatalf("NewWall(w1) err = %v", err)
	}
	w2, err := rules.NewWall(rand.NewPCG(42, 99))
	if err != nil {
		t.Fatalf("NewWall(w2) err = %v", err)
	}
	w3, err := rules.NewWall(rand.NewPCG(43, 99))
	if err != nil {
		t.Fatalf("NewWall(w3) err = %v", err)
	}

	if got := w1.Remaining(); got != rules.TotalDeckTiles {
		t.Fatalf("Remaining() = %d, want %d", got, rules.TotalDeckTiles)
	}
	if got := w1.DrawableRemaining(); got != rules.TotalDeckTiles-rules.ReserveWallTiles {
		t.Fatalf("DrawableRemaining() = %d, want %d", got, rules.TotalDeckTiles-rules.ReserveWallTiles)
	}

	seq1 := drawAll(t, w1)
	seq2 := drawAll(t, w2)
	seq3 := drawAll(t, w3)

	if !slices.Equal(seq1, seq2) {
		t.Fatal("walls created with identical rand.Source seeds must produce identical draw sequences")
	}
	if slices.Equal(seq1, seq3) {
		t.Fatal("walls created with different rand.Source seeds should differ")
	}
}

func TestWall_DealAndReserveProtection(t *testing.T) {
	t.Parallel()

	w, err := rules.NewWall(rand.NewPCG(1, 2))
	if err != nil {
		t.Fatalf("NewWall err = %v", err)
	}

	// Deal 16 tiles to each of 4 players plus 1 extra tile to dealer (65 total).
	for range 65 {
		tile, derr := w.Draw()
		if derr != nil {
			t.Fatalf("unexpected draw error during deal: %v", derr)
		}
		if !tile.Valid() {
			t.Fatalf("dealt invalid tile: %v", tile)
		}
	}

	if got := w.DrawableRemaining(); got != 63 {
		t.Fatalf("DrawableRemaining() after initial deal = %d, want 63", got)
	}
	if got := w.Remaining(); got != 79 {
		t.Fatalf("Remaining() after initial deal = %d, want 79", got)
	}

	// Draw remaining 63 drawable tiles.
	for range 63 {
		if _, derr := w.Draw(); derr != nil {
			t.Fatalf("unexpected draw error before wall exhaustion: %v", derr)
		}
	}

	if got := w.DrawableRemaining(); got != 0 {
		t.Fatalf("DrawableRemaining() when exhausted = %d, want 0", got)
	}
	if got := w.Remaining(); got != rules.ReserveWallTiles {
		t.Fatalf("Remaining() when exhausted = %d, want %d", got, rules.ReserveWallTiles)
	}

	// Attempting to draw from reserve tiles must fail cleanly.
	tile, err := w.Draw()
	if !errors.Is(err, rules.ErrWallExhausted) {
		t.Fatalf("Draw() at reserve boundary err = %v, want ErrWallExhausted", err)
	}
	if tile != rules.TileInvalid {
		t.Fatalf("Draw() at reserve boundary tile = %v, want TileInvalid", tile)
	}
}

func TestNewWallFromTiles_Validation(t *testing.T) {
	t.Parallel()

	validDeck := rules.FullDeck()
	w, err := rules.NewWallFromTiles(validDeck)
	if err != nil {
		t.Fatalf("NewWallFromTiles(FullDeck) err = %v", err)
	}
	// Mutating caller slice must not affect wall.
	validDeck[0] = rules.FlowerBamboo
	first, err := w.Draw()
	if err != nil {
		t.Fatalf("Draw() err = %v", err)
	}
	if first != rules.Wan1 {
		t.Fatalf("first tile = %v, want Wan1 (wall must copy input slice)", first)
	}

	badLen := rules.FullDeck()[:100]
	if _, err := rules.NewWallFromTiles(badLen); !errors.Is(err, rules.ErrInvalidWall) {
		t.Fatalf("short deck err = %v, want ErrInvalidWall", err)
	}

	dupFlower := rules.FullDeck()
	dupFlower[0] = rules.FlowerSpring
	if _, err := rules.NewWallFromTiles(dupFlower); !errors.Is(err, rules.ErrInvalidWall) {
		t.Fatalf("duplicate flower deck err = %v, want ErrInvalidWall", err)
	}
}

func drawAll(t *testing.T, w *rules.Wall) []rules.Tile {
	t.Helper()
	out := make([]rules.Tile, 0, w.DrawableRemaining())
	for w.DrawableRemaining() > 0 {
		tile, err := w.Draw()
		if err != nil {
			t.Fatalf("unexpected draw err: %v", err)
		}
		out = append(out, tile)
	}
	return out
}
