package rules

import (
	"errors"
	"fmt"
	"slices"
)

// ErrInvalidAction is returned when a PlayerAction fails structural validation.
var ErrInvalidAction = errors.New("invalid player action")

// ActionKind enumerates the five player input actions accepted by the rules engine.
type ActionKind uint8

const (
	ActionUnknown ActionKind = iota
	ActionDraw
	ActionDiscard
	ActionSelfDeclare
	ActionClaim
	ActionReady
)

// Valid reports whether k is one of the five defined action kinds.
func (k ActionKind) Valid() bool {
	return k >= ActionDraw && k <= ActionReady
}

// SelfDeclareKind enumerates declarations a player can make on their own turn after drawing.
type SelfDeclareKind uint8

const (
	SelfDeclareUnknown SelfDeclareKind = iota
	SelfDeclarePass
	SelfDeclareSelfWin
	SelfDeclareConcealedKong
	SelfDeclareAddedKong
	SelfDeclareRiichi
	SelfDeclareNineTerminals
)

// Valid reports whether k is a valid self-declaration kind.
func (k SelfDeclareKind) Valid() bool {
	return k >= SelfDeclarePass && k <= SelfDeclareNineTerminals
}

// ClaimKind enumerates responses inside a claim arbitration window,
// ordered strictly by ascending arbitration priority (Pass < Chow < Pong < Kong < Win).
type ClaimKind uint8

const (
	ClaimUnknown ClaimKind = iota
	ClaimPass
	ClaimChow
	ClaimPong
	ClaimKong
	ClaimWin
)

// Valid reports whether k is a valid claim kind.
func (k ClaimKind) Valid() bool {
	return k >= ClaimPass && k <= ClaimWin
}

// ChowOption identifies a concrete 3-tile sequence chosen when claiming a chow.
type ChowOption struct {
	Tiles [3]Tile
}

// Validate checks that ChowOption represents a valid consecutive 3-tile number sequence.
func (c ChowOption) Validate() error {
	return validateChow(c.Tiles[:])
}

// PlayerAction is the strongly-typed player input delivered to the rules FSM.
type PlayerAction struct {
	Seat        Seat
	Kind        ActionKind
	Tile        Tile
	SelfDeclare SelfDeclareKind
	Claim       ClaimKind
	Chow        ChowOption
}

// Validate checks that the action carries structurally valid fields for its Kind.
func (a PlayerAction) Validate() error {
	if !a.Seat.Valid() {
		return fmt.Errorf("seat %v: %w", a.Seat, ErrInvalidAction)
	}
	if !a.Kind.Valid() {
		return fmt.Errorf("kind %d: %w", a.Kind, ErrInvalidAction)
	}

	switch a.Kind {
	case ActionDraw, ActionReady:
		return nil
	case ActionDiscard:
		if !a.Tile.Valid() || a.Tile.IsFlower() {
			return fmt.Errorf("discard tile %v: %w", a.Tile, ErrInvalidAction)
		}
		return nil
	case ActionSelfDeclare:
		return a.validateSelfDeclare()
	case ActionClaim:
		return a.validateClaim()
	case ActionUnknown:
		return ErrInvalidAction
	default:
		return ErrInvalidAction
	}
}

func (a PlayerAction) validateSelfDeclare() error {
	if !a.SelfDeclare.Valid() {
		return fmt.Errorf("self declare %d: %w", a.SelfDeclare, ErrInvalidAction)
	}
	if a.SelfDeclare == SelfDeclareConcealedKong || a.SelfDeclare == SelfDeclareAddedKong || a.SelfDeclare == SelfDeclareRiichi {
		if !a.Tile.Valid() || a.Tile.IsFlower() {
			return fmt.Errorf("self declare tile %v: %w", a.Tile, ErrInvalidAction)
		}
	}
	return nil
}

func (a PlayerAction) validateClaim() error {
	if !a.Claim.Valid() {
		return fmt.Errorf("claim %d: %w", a.Claim, ErrInvalidAction)
	}
	if a.Claim == ClaimChow && a.Chow != (ChowOption{}) {
		if err := a.Chow.Validate(); err != nil {
			return fmt.Errorf("chow option: %w: %w", err, ErrInvalidAction)
		}
	}
	return nil
}

// DiscardRecord records a publicly discarded tile and its discarding seat.
type DiscardRecord struct {
	Seat Seat
	Tile Tile
}

// PublicMeld is the visibility-safe projection of a Meld delivered to players.
// For concealed kongs before hand reveal, Tiles is empty so faces never leak.
type PublicMeld struct {
	Kind     MeldKind
	Tiles    []Tile
	FromSeat Seat
}

// ProjectMeld converts a Meld into its visibility-safe PublicMeld representation.
// Concealed kong faces are omitted unless revealConcealed is true (e.g. hand end or owner view).
func ProjectMeld(m Meld, revealConcealed bool) PublicMeld {
	pm := PublicMeld{
		Kind:     m.Kind,
		FromSeat: m.FromSeat,
	}
	if m.Kind == MeldConcealedKong && !revealConcealed {
		return pm
	}
	pm.Tiles = slices.Clone(m.Tiles)
	return pm
}

// SeatPublicView captures publicly visible per-seat information during a hand.
type SeatPublicView struct {
	Seat           Seat
	SeatWind       Wind
	ConcealedCount int
	Melds          []PublicMeld
	Flowers        []Tile
}

// PlayerPrivateView captures information visible exclusively to the owning seat.
type PlayerPrivateView struct {
	Seat        Seat
	Concealed   []Tile
	LastDrawn   Tile
	PassedWin   bool
	SelfPrompt  []SelfDeclareKind
	Claims      []ClaimKind
	ChowChoices []ChowOption
}
