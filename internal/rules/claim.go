package rules

import (
	"errors"
	"fmt"
	"slices"
)

// Sentinel errors returned by ClaimWindow operations.
var (
	ErrClaimNotPending     = errors.New("seat is not pending a claim decision")
	ErrClaimAlreadyDecided = errors.New("seat has already submitted a claim decision")
	ErrInvalidClaimOption  = errors.New("claim option is not available for seat")
)

// SeatClaimOptions describes the legal non-pass claims offered to a single seat in a ClaimWindow.
type SeatClaimOptions struct {
	Seat          Seat
	Options       []ClaimKind
	ChowOptions   []ChowOption
	FlowerWinKind WinKind
	FlowerPayer   Seat
}

// ClaimResolution is the final public arbitration result once all prompted seats have decided.
type ClaimResolution struct {
	Kind           ClaimKind
	Winner         Seat
	Chow           ChowOption
	FlowerWinKind  WinKind
	FlowerPayer    Seat
	PassedWinSeats []Seat
}

type seatDecision struct {
	decided bool
	claim   ClaimKind
	chow    ChowOption
}

// ClaimWindow manages parallel waiting, optional two-step chow selection, and
// priority/head-bump arbitration for Discard, RobKong, and Flower claim windows.
type ClaimWindow struct {
	kind           ClaimWindowKind
	triggerSeat    Seat
	triggerTile    Tile
	promptOrder    []Seat
	optionsBySeat  map[Seat]SeatClaimOptions
	decisions      map[Seat]seatDecision
	waitingChowFor Seat
}

// ComputeChowOptions returns all canonical 3-tile sequences that concealed tiles can form with target.
func ComputeChowOptions(concealed []Tile, target Tile) []ChowOption {
	if !target.Valid() || !target.IsNumber() {
		return nil
	}
	var has [NonFlowerMax + 1]bool
	for _, t := range concealed {
		if t.Valid() && !t.IsFlower() {
			has[t] = true
		}
	}

	rank := target.Rank()
	var opts []ChowOption
	for _, delta := range [3]Tile{2, 1, 0} {
		startRank := rank - int(delta)
		if startRank < 1 || startRank > 7 {
			continue
		}
		base := target - delta
		t0, t1, t2 := base, base+1, base+2
		if (t0 == target || has[t0]) && (t1 == target || has[t1]) && (t2 == target || has[t2]) {
			opts = append(opts, ChowOption{Tiles: [3]Tile{t0, t1, t2}})
		}
	}
	return opts
}

// NewClaimWindow constructs a ClaimWindow containing only seats with at least one legal option.
func NewClaimWindow(
	kind ClaimWindowKind,
	triggerSeat Seat,
	triggerTile Tile,
	seatOptions []SeatClaimOptions,
) *ClaimWindow {
	w := &ClaimWindow{
		kind:          kind,
		triggerSeat:   triggerSeat,
		triggerTile:   triggerTile,
		optionsBySeat: make(map[Seat]SeatClaimOptions, len(seatOptions)),
		decisions:     make(map[Seat]seatDecision, len(seatOptions)),
	}
	for _, so := range seatOptions {
		if !so.Seat.Valid() || len(so.Options) == 0 {
			continue
		}
		copied := SeatClaimOptions{
			Seat:          so.Seat,
			Options:       slices.Clone(so.Options),
			ChowOptions:   slices.Clone(so.ChowOptions),
			FlowerWinKind: so.FlowerWinKind,
			FlowerPayer:   so.FlowerPayer,
		}
		w.promptOrder = append(w.promptOrder, so.Seat)
		w.optionsBySeat[so.Seat] = copied
	}
	return w
}

// Kind returns the window trigger category.
func (w *ClaimWindow) Kind() ClaimWindowKind {
	return w.kind
}

// TriggerSeat returns the seat whose discard, added kong, or flower triggered the window.
func (w *ClaimWindow) TriggerSeat() Seat {
	return w.triggerSeat
}

// TriggerTile returns the tile associated with this window.
func (w *ClaimWindow) TriggerTile() Tile {
	return w.triggerTile
}

// HasPrompts reports whether at least one seat was offered a claim option.
func (w *ClaimWindow) HasPrompts() bool {
	return len(w.promptOrder) > 0
}

// WaitingChowSeat returns the seat currently waiting in WaitChowChoice, or SeatUnknown.
func (w *ClaimWindow) WaitingChowSeat() Seat {
	return w.waitingChowFor
}

// Prompts returns private ClaimPromptEvent messages for every prompted seat.
func (w *ClaimWindow) Prompts() []Event {
	events := make([]Event, 0, len(w.promptOrder))
	for _, seat := range w.promptOrder {
		so := w.optionsBySeat[seat]
		events = append(events, ClaimPromptEvent{
			Seat:          seat,
			WindowKind:    w.kind,
			TriggerSeat:   w.triggerSeat,
			TriggerTile:   w.triggerTile,
			Options:       slices.Clone(so.Options),
			ChowOptions:   slices.Clone(so.ChowOptions),
			FlowerWinKind: so.FlowerWinKind,
		})
	}
	return events
}

// IsComplete reports whether all prompted seats have finalized their decisions.
func (w *ClaimWindow) IsComplete() bool {
	if w.waitingChowFor != SeatUnknown {
		return false
	}
	for _, seat := range w.promptOrder {
		if !w.decisions[seat].decided {
			return false
		}
	}
	return true
}

// Submit records an immutable claim decision for seat.
// When seat selects ClaimChow with multiple choices and an empty ChowOption, it transitions
// seat into WaitChowChoice and returns a private ChowChoicePromptEvent.
// When all prompted seats have finalized, Submit returns the resolved ClaimResolution.
func (w *ClaimWindow) Submit(seat Seat, claim ClaimKind, chow ChowOption) ([]Event, *ClaimResolution, error) {
	so, ok := w.optionsBySeat[seat]
	if !ok {
		return nil, nil, fmt.Errorf("seat %v: %w", seat, ErrClaimNotPending)
	}
	if w.decisions[seat].decided {
		return nil, nil, fmt.Errorf("seat %v: %w", seat, ErrClaimAlreadyDecided)
	}
	if w.waitingChowFor == seat {
		return w.submitChowChoice(seat, so, claim, chow)
	}
	if !claim.Valid() {
		return nil, nil, fmt.Errorf("claim %d: %w", claim, ErrInvalidClaimOption)
	}
	if claim != ClaimPass && !slices.Contains(so.Options, claim) {
		return nil, nil, fmt.Errorf("seat %v cannot claim %d: %w", seat, claim, ErrInvalidClaimOption)
	}
	if claim == ClaimChow {
		return w.submitChow(seat, so, chow)
	}

	w.decisions[seat] = seatDecision{decided: true, claim: claim}
	return nil, w.maybeResolve(), nil
}

func (w *ClaimWindow) submitChow(seat Seat, so SeatClaimOptions, chow ChowOption) ([]Event, *ClaimResolution, error) {
	if chow == (ChowOption{}) {
		if len(so.ChowOptions) == 1 {
			w.decisions[seat] = seatDecision{decided: true, claim: ClaimChow, chow: so.ChowOptions[0]}
			return nil, w.maybeResolve(), nil
		}
		w.waitingChowFor = seat
		prompt := ChowChoicePromptEvent{
			Seat:        seat,
			TriggerSeat: w.triggerSeat,
			TriggerTile: w.triggerTile,
			Choices:     slices.Clone(so.ChowOptions),
		}
		return []Event{prompt}, nil, nil
	}
	if !slices.Contains(so.ChowOptions, chow) {
		return nil, nil, fmt.Errorf("chow %+v not offered: %w", chow, ErrInvalidClaimOption)
	}
	w.decisions[seat] = seatDecision{decided: true, claim: ClaimChow, chow: chow}
	return nil, w.maybeResolve(), nil
}

func (w *ClaimWindow) submitChowChoice(
	seat Seat,
	so SeatClaimOptions,
	claim ClaimKind,
	chow ChowOption,
) ([]Event, *ClaimResolution, error) {
	if claim != ClaimChow || !slices.Contains(so.ChowOptions, chow) {
		return nil, nil, fmt.Errorf("expected valid chow choice: %w", ErrInvalidClaimOption)
	}
	w.waitingChowFor = SeatUnknown
	w.decisions[seat] = seatDecision{decided: true, claim: ClaimChow, chow: chow}
	return nil, w.maybeResolve(), nil
}

func (w *ClaimWindow) maybeResolve() *ClaimResolution {
	if !w.IsComplete() {
		return nil
	}
	res := w.resolve()
	return &res
}

func (w *ClaimWindow) resolve() ClaimResolution {
	res := ClaimResolution{
		Kind:           ClaimPass,
		PassedWinSeats: w.collectPassedWinSeats(),
	}

	// Evaluate candidates in clockwise order from triggerSeat.Next() so head-bump (截胡)
	// naturally selects the closest clockwise claimant on equal priority.
	order := w.arbitrationSeatOrder()
	for _, seat := range order {
		dec, ok := w.decisions[seat]
		if !ok || !dec.decided || dec.claim == ClaimPass {
			continue
		}
		if dec.claim > res.Kind {
			so := w.optionsBySeat[seat]
			res.Kind = dec.claim
			res.Winner = seat
			res.Chow = dec.chow
			res.FlowerWinKind = so.FlowerWinKind
			res.FlowerPayer = so.FlowerPayer
		}
	}
	return res
}

func (w *ClaimWindow) arbitrationSeatOrder() []Seat {
	if !w.triggerSeat.Valid() {
		return slices.Clone(w.promptOrder)
	}
	order := make([]Seat, 0, 4)
	s := w.triggerSeat.Next()
	for range 4 {
		order = append(order, s)
		s = s.Next()
	}
	return order
}

func (w *ClaimWindow) collectPassedWinSeats() []Seat {
	if w.kind == ClaimWindowFlower {
		return nil
	}
	var passed []Seat
	for _, seat := range w.promptOrder {
		so := w.optionsBySeat[seat]
		if !slices.Contains(so.Options, ClaimWin) {
			continue
		}
		if w.decisions[seat].claim != ClaimWin {
			passed = append(passed, seat)
		}
	}
	return passed
}
