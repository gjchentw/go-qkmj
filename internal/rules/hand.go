package rules

import (
	"errors"
	"fmt"
	"maps"
	"math/rand/v2"
	"slices"
	"time"
)

// Sentinel errors returned by HandEngine operations.
var (
	ErrInvalidHandConfig = errors.New("invalid hand configuration")
	ErrAlreadyReady      = errors.New("seat has already confirmed ready")
)

// HandPhase identifies the lifecycle phase of a single hand.
type HandPhase uint8

const (
	HandPhaseUnknown HandPhase = iota
	HandPhaseNotStarted
	HandPhasePlaying
	HandPhaseWaitReady
	HandPhaseFinished
	HandPhaseAborted
)

// HandConfig specifies the required deterministic inputs to start a single hand.
type HandConfig struct {
	RoundWind    Wind
	Dealer       Seat
	DealerStreak int
	Balances     map[Seat]int
	Base         int
	PerFan       int
	Rand         rand.Source
}

type handOptions struct {
	wall     *Wall
	doorSeat Seat
}

// HandOption configures optional overrides on NewHandEngine with construction-time validation.
type HandOption func(*handOptions) error

// WithWall injects a freshly constructed 144-tile wall instead of shuffling from HandConfig.Rand.
func WithWall(w *Wall) HandOption {
	return func(o *handOptions) error {
		if w == nil || w.Remaining() != TotalDeckTiles {
			return fmt.Errorf("injected wall must be non-nil with %d tiles: %w", TotalDeckTiles, ErrInvalidHandConfig)
		}
		o.wall = w
		return nil
	}
}

// WithDoorSeat overrides the random door seat selection (the seat assigned WindEast).
func WithDoorSeat(seat Seat) HandOption {
	return func(o *handOptions) error {
		if !seat.Valid() {
			return fmt.Errorf("door seat %v invalid: %w", seat, ErrInvalidHandConfig)
		}
		o.doorSeat = seat
		return nil
	}
}

// HandEngine drives a single hand's full lifecycle: shuffle, door-seat wind assignment,
// deal, opening flower replacement, TurnEngine gameplay, reveal, settlement, and WaitReady.
type HandEngine struct {
	roundWind    Wind
	dealer       Seat
	dealerStreak int
	base         int
	perFan       int
	rng          rand.Source
	optWall      *Wall
	optDoorSeat  Seat

	phase     HandPhase
	doorSeat  Seat
	seatWinds map[Seat]Wind
	turn      *TurnEngine

	balancesBefore [5]int
	balancesAfter  [5]int
	settled        bool
	endReason      HandEndReason
	endScore       *ScoreResult
	ready          [5]bool
}

// NewHandEngine validates cfg and opts, returning a HandEngine positioned at HandPhaseNotStarted.
func NewHandEngine(cfg HandConfig, opts ...HandOption) (*HandEngine, error) {
	var o handOptions
	for _, opt := range opts {
		if opt == nil {
			return nil, fmt.Errorf("nil hand option: %w", ErrInvalidHandConfig)
		}
		if err := opt(&o); err != nil {
			return nil, err
		}
	}
	if err := validateHandConfig(cfg, o); err != nil {
		return nil, err
	}

	eng := &HandEngine{
		roundWind:    cfg.RoundWind,
		dealer:       cfg.Dealer,
		dealerStreak: cfg.DealerStreak,
		base:         cfg.Base,
		perFan:       cfg.PerFan,
		rng:          cfg.Rand,
		optWall:      o.wall,
		optDoorSeat:  o.doorSeat,
		phase:        HandPhaseNotStarted,
	}
	for _, s := range AllSeats {
		eng.balancesBefore[s] = cfg.Balances[s]
		eng.balancesAfter[s] = cfg.Balances[s]
	}
	return eng, nil
}

func validateHandConfig(cfg HandConfig, o handOptions) error {
	if !cfg.RoundWind.Valid() || !cfg.Dealer.Valid() || cfg.DealerStreak < 0 {
		return fmt.Errorf("roundWind=%v dealer=%v streak=%d: %w", cfg.RoundWind, cfg.Dealer, cfg.DealerStreak, ErrInvalidHandConfig)
	}
	if cfg.Base < 0 || cfg.PerFan < 0 {
		return fmt.Errorf("base=%d perFan=%d: %w", cfg.Base, cfg.PerFan, ErrInvalidHandConfig)
	}
	for _, s := range AllSeats {
		if _, ok := cfg.Balances[s]; !ok {
			return fmt.Errorf("missing balance for seat %v: %w", s, ErrInvalidHandConfig)
		}
	}
	if cfg.Rand == nil && (o.wall == nil || o.doorSeat == SeatUnknown) {
		return fmt.Errorf("rand source required unless both wall and door seat are provided: %w", ErrInvalidHandConfig)
	}
	return nil
}

// Start runs the automated hand opening sequence (shuffle, door-seat wind assignment, deal,
// clockwise opening flower replacement, and handoff to TurnEngine.Start).
func (h *HandEngine) Start() ([]Event, error) {
	if h.phase != HandPhaseNotStarted {
		return nil, fmt.Errorf("hand phase %d: %w", h.phase, ErrInvalidPhase)
	}

	wall, err := h.prepareWall()
	if err != nil {
		return nil, err
	}
	doorSeat := h.pickDoorSeat()
	seatWinds := assignSeatWinds(doorSeat)

	var events []Event
	events = append(events, HandStartedEvent{
		RoundWind:    h.roundWind,
		Dealer:       h.dealer,
		DealerStreak: h.dealerStreak,
		DoorSeat:     doorSeat,
		SeatWinds:    maps.Clone(seatWinds),
	})

	hands, dealEvents, err := dealInitialHands(wall, h.dealer)
	if err != nil {
		return nil, err
	}
	events = append(events, dealEvents...)

	flowers, flowerEvents, err := replaceOpeningFlowers(wall, h.dealer, hands)
	if err != nil {
		return nil, err
	}
	events = append(events, flowerEvents...)

	turn, err := NewTurnEngine(TurnConfig{
		Wall:         wall,
		RoundWind:    h.roundWind,
		SeatWinds:    seatWinds,
		Dealer:       h.dealer,
		DealerStreak: h.dealerStreak,
		Hands:        hands,
		Flowers:      flowers,
		Base:         h.base,
		PerFan:       h.perFan,
	})
	if err != nil {
		return nil, err
	}

	h.doorSeat = doorSeat
	h.seatWinds = seatWinds
	h.turn = turn
	h.phase = HandPhasePlaying

	turnEvents := turn.Start()
	events = append(events, turnEvents...)
	return events, nil
}

func (h *HandEngine) prepareWall() (*Wall, error) {
	if h.optWall != nil {
		return h.optWall, nil
	}
	return NewWall(h.rng)
}

func (h *HandEngine) pickDoorSeat() Seat {
	if h.optDoorSeat.Valid() {
		return h.optDoorSeat
	}
	// Non-cryptographic RNG is intentionally injected via rand.Source for deterministic Mahjong rules replay.
	rng := rand.New(h.rng) //nolint:gosec // deterministic game RNG injected via rand.Source
	return AllSeats[rng.IntN(len(AllSeats))]
}

func assignSeatWinds(doorSeat Seat) map[Seat]Wind {
	winds := make(map[Seat]Wind, len(AllSeats))
	s := doorSeat
	w := WindEast
	for range AllSeats {
		winds[s] = w
		s = s.Next()
		w = w.Next()
	}
	return winds
}

func dealInitialHands(wall *Wall, dealer Seat) (map[Seat][]Tile, []Event, error) {
	hands := make(map[Seat][]Tile, len(AllSeats))
	counts := make(map[Seat]int, len(AllSeats))

	s := dealer
	for range AllSeats {
		capHint := WaitingHandSize
		if s == dealer {
			capHint = DrawnHandSize
		}
		hand := make([]Tile, 0, capHint)
		for range WaitingHandSize {
			t, err := wall.Draw()
			if err != nil {
				return nil, nil, fmt.Errorf("deal seat %v: %w", s, err)
			}
			hand = append(hand, t)
		}
		hands[s] = hand
		counts[s] = len(hand)
		s = s.Next()
	}

	extra, err := wall.Draw()
	if err != nil {
		return nil, nil, fmt.Errorf("deal dealer extra tile: %w", err)
	}
	hands[dealer] = append(hands[dealer], extra)
	counts[dealer] = len(hands[dealer])

	events := make([]Event, 0, 1+len(AllSeats))
	events = append(events, HandDealtPublicEvent{
		HandCounts:        counts,
		DrawableRemaining: wall.DrawableRemaining(),
	})
	s = dealer
	for range AllSeats {
		events = append(events, HandDealtEvent{
			Seat:  s,
			Tiles: slices.Clone(hands[s]),
		})
		s = s.Next()
	}
	return hands, events, nil
}

func replaceOpeningFlowers(wall *Wall, dealer Seat, hands map[Seat][]Tile) (map[Seat][]Tile, []Event, error) {
	flowers := make(map[Seat][]Tile, len(AllSeats))
	var events []Event

	s := dealer
	for range AllSeats {
		hand := hands[s]
		for i := range hand {
			for hand[i].IsFlower() {
				fl := hand[i]
				flowers[s] = append(flowers[s], fl)
				rep, err := wall.Draw()
				if err != nil {
					return nil, nil, fmt.Errorf("opening flower supplement for seat %v: %w", s, err)
				}
				events = append(events,
					FlowerExposedEvent{
						Seat:              s,
						Flower:            fl,
						DrawableRemaining: wall.DrawableRemaining(),
					},
					TileDrawnEvent{
						Seat:         s,
						Tile:         rep,
						IsSupplement: true,
					},
				)
				hand[i] = rep
			}
		}
		hands[s] = hand
		s = s.Next()
	}
	return flowers, events, nil
}

// Apply executes one player action within the active hand (delegating gameplay actions to
// TurnEngine and handling ActionReady during HandPhaseWaitReady). On error, no state is mutated.
func (h *HandEngine) Apply(action PlayerAction, thinkElapsed time.Duration) ([]Event, error) {
	if thinkElapsed < 0 {
		return nil, ErrInvalidThinkTime
	}
	if err := action.Validate(); err != nil {
		return nil, err
	}

	switch h.phase {
	case HandPhasePlaying:
		events, err := h.turn.Apply(action, thinkElapsed)
		if err != nil {
			return nil, err
		}
		if h.turn.Phase() == PhaseHandEnded {
			events = append(events, h.finalizeHandEnd()...)
		}
		return events, nil
	case HandPhaseWaitReady:
		return h.applyReady(action)
	case HandPhaseUnknown, HandPhaseNotStarted, HandPhaseFinished, HandPhaseAborted:
		return nil, fmt.Errorf("hand phase %d: %w", h.phase, ErrInvalidPhase)
	default:
		return nil, fmt.Errorf("hand phase %d: %w", h.phase, ErrInvalidPhase)
	}
}

func (h *HandEngine) applyReady(action PlayerAction) ([]Event, error) {
	if action.Kind != ActionReady {
		return nil, fmt.Errorf("expected ActionReady in WaitReady: %w", ErrInvalidPhase)
	}
	if h.ready[action.Seat] {
		return nil, fmt.Errorf("seat %v: %w", action.Seat, ErrAlreadyReady)
	}
	h.ready[action.Seat] = true
	if h.allReady() {
		h.phase = HandPhaseFinished
	}
	return []Event{PlayerReadyEvent{Seat: action.Seat}}, nil
}

func (h *HandEngine) allReady() bool {
	for _, s := range AllSeats {
		if !h.ready[s] {
			return false
		}
	}
	return true
}

func (h *HandEngine) finalizeHandEnd() []Event {
	reason, score := h.turn.HandResult()

	concealed := make(map[Seat][]Tile, len(AllSeats))
	melds := make(map[Seat][]Meld, len(AllSeats))
	for _, s := range AllSeats {
		concealed[s] = h.turn.Concealed(s)
		melds[s] = h.turn.Melds(s)
	}
	revealedEv := NewHandsRevealedEvent(concealed, melds)

	balChanges := make(map[Seat]SeatBalanceChange, len(AllSeats))
	for _, s := range AllSeats {
		before := h.balancesBefore[s]
		delta := 0
		if reason == HandEndWin && score != nil {
			delta = score.Deltas[s]
		}
		after := before + delta
		h.balancesAfter[s] = after
		balChanges[s] = SeatBalanceChange{
			Before: before,
			Delta:  delta,
			After:  after,
		}
	}

	h.settled = true
	h.endReason = reason
	h.endScore = score
	h.phase = HandPhaseWaitReady

	return []Event{
		revealedEv,
		HandSettledEvent{
			Reason:               reason,
			Score:                score,
			Balances:             balChanges,
			IncrementPlayedCount: reason == HandEndWin,
		},
		WaitReadyEvent{},
	}
}

// Abort terminates an active or WaitReady hand because a player left the table.
// During HandPhasePlaying, no hands are revealed and no settlement occurs.
// During HandPhaseWaitReady, the already-completed settlement is preserved.
func (h *HandEngine) Abort(leaver Seat) ([]Event, error) {
	if !leaver.Valid() {
		return nil, fmt.Errorf("leaver seat %v: %w", leaver, ErrInvalidAction)
	}
	switch h.phase {
	case HandPhasePlaying:
		h.phase = HandPhaseAborted
		h.endReason = HandEndAborted
		h.endScore = nil
		return []Event{HandEndedEvent{Reason: HandEndAborted}}, nil
	case HandPhaseWaitReady:
		h.phase = HandPhaseAborted
		return nil, nil
	case HandPhaseUnknown, HandPhaseNotStarted, HandPhaseFinished, HandPhaseAborted:
		return nil, fmt.Errorf("hand phase %d: %w", h.phase, ErrInvalidPhase)
	default:
		return nil, fmt.Errorf("hand phase %d: %w", h.phase, ErrInvalidPhase)
	}
}

// Phase returns the current HandPhase of the hand.
func (h *HandEngine) Phase() HandPhase { return h.phase }

// TurnPhase returns the underlying TurnEngine phase, or PhaseNotStarted before Start().
func (h *HandEngine) TurnPhase() TurnPhase {
	if h.turn == nil {
		return PhaseNotStarted
	}
	return h.turn.Phase()
}

// CurrentTurn returns the active turn seat, or SeatUnknown before Start().
func (h *HandEngine) CurrentTurn() Seat {
	if h.turn == nil {
		return SeatUnknown
	}
	return h.turn.CurrentTurn()
}

// DrawableRemaining returns the remaining drawable tiles in the active wall.
func (h *HandEngine) DrawableRemaining() int {
	if h.turn == nil {
		return 0
	}
	return h.turn.DrawableRemaining()
}

// DoorSeat returns the seat assigned WindEast for this hand.
func (h *HandEngine) DoorSeat() Seat { return h.doorSeat }

// SeatWinds returns a defensive copy of the hand's per-seat winds.
func (h *HandEngine) SeatWinds() map[Seat]Wind {
	return maps.Clone(h.seatWinds)
}

// Balances returns a defensive copy of the four seats' current (or post-settlement) balances.
func (h *HandEngine) Balances() map[Seat]int {
	out := make(map[Seat]int, len(AllSeats))
	for _, s := range AllSeats {
		out[s] = h.balancesAfter[s]
	}
	return out
}

// Settled reports whether this hand has completed reveal and settlement.
func (h *HandEngine) Settled() bool { return h.settled }

// HandResult returns the hand's termination reason and optional winning score result.
func (h *HandEngine) HandResult() (HandEndReason, *ScoreResult) {
	return h.endReason, h.endScore
}

// ReadySeats returns a defensive copy of which seats have confirmed ActionReady.
func (h *HandEngine) ReadySeats() map[Seat]bool {
	out := make(map[Seat]bool, len(AllSeats))
	for _, s := range AllSeats {
		out[s] = h.ready[s]
	}
	return out
}
