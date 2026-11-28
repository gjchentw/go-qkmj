package rules

import (
	"errors"
	"fmt"
	"math/rand/v2"
	"time"
)

// ErrInvalidMatchConfig is returned when MatchEngine construction or StartMatch receives invalid arguments.
var ErrInvalidMatchConfig = errors.New("invalid match configuration")

// MatchPhase identifies the outermost phase of a 4-player match.
type MatchPhase uint8

const (
	MatchPhaseUnknown MatchPhase = iota
	MatchPhaseGathering
	MatchPhaseInHand
)

type matchOptions struct {
	base           int
	perFan         int
	wallFactory    func(rand.Source) (*Wall, error)
	doorSeatPicker func(rand.Source) Seat
}

// MatchOption configures optional behavior on NewMatchEngine with construction-time validation.
type MatchOption func(*matchOptions) error

// WithStakes overrides the default QKMJ baseline stakes (DefaultBase=500, DefaultPerFan=200).
func WithStakes(base, perFan int) MatchOption {
	return func(o *matchOptions) error {
		if base < 0 || perFan < 0 {
			return fmt.Errorf("stakes base=%d perFan=%d must be non-negative: %w", base, perFan, ErrInvalidMatchConfig)
		}
		o.base = base
		o.perFan = perFan
		return nil
	}
}

// WithWallFactory overrides how each hand's 144-tile wall is constructed.
func WithWallFactory(fn func(rand.Source) (*Wall, error)) MatchOption {
	return func(o *matchOptions) error {
		if fn == nil {
			return fmt.Errorf("wall factory must not be nil: %w", ErrInvalidMatchConfig)
		}
		o.wallFactory = fn
		return nil
	}
}

// WithDoorSeatPicker overrides how each hand's door seat (WindEast) is chosen.
func WithDoorSeatPicker(fn func(rand.Source) Seat) MatchOption {
	return func(o *matchOptions) error {
		if fn == nil {
			return fmt.Errorf("door seat picker must not be nil: %w", ErrInvalidMatchConfig)
		}
		o.doorSeatPicker = fn
		return nil
	}
}

// MatchEngine is the outermost pure finite state machine driving an endless multi-hand match
// across four seated players (East -> South -> West -> North -> East).
type MatchEngine struct {
	rng            rand.Source
	base           int
	perFan         int
	wallFactory    func(rand.Source) (*Wall, error)
	doorSeatPicker func(rand.Source) Seat

	phase        MatchPhase
	roundWind    Wind
	dealer       Seat
	dealerStreak int
	balances     [5]int
	hand         *HandEngine
	cycle        roundCycleTracker
}

type roundCycleTracker struct {
	cycleNumber int
	elapsed     time.Duration
	stats       [5]SeatCycleStats
}

func (t *roundCycleTracker) resetAll() {
	t.cycleNumber = 1
	t.elapsed = 0
	t.stats = [5]SeatCycleStats{}
}

func (t *roundCycleTracker) addThinkTime(d time.Duration) {
	t.elapsed += d
}

func (t *roundCycleTracker) recordSettlement(reason HandEndReason, score *ScoreResult) {
	if reason.IsDraw() {
		for _, s := range AllSeats {
			t.stats[s].Draws++
		}
		return
	}
	if reason != HandEndWin || score == nil {
		return
	}
	for _, s := range AllSeats {
		t.stats[s].HandsPlayed++
		t.stats[s].NetDelta += score.Deltas[s]
	}
	switch score.WinKind {
	case WinSelfDraw, WinEightImmortals:
		if score.Winner.Valid() {
			t.stats[score.Winner].SelfDrawWins++
		}
	case WinDiscard, WinRobKong, WinSevenRobOne:
		if score.Winner.Valid() {
			t.stats[score.Winner].DiscardWins++
		}
		if score.Payer.Valid() {
			t.stats[score.Payer].DealIns++
		}
	case WinUnknown:
	}
}

func (t *roundCycleTracker) completeCycle() RoundCycleCompletedEvent {
	statsMap := make(map[Seat]SeatCycleStats, len(AllSeats))
	for _, s := range AllSeats {
		statsMap[s] = t.stats[s]
	}
	ev := RoundCycleCompletedEvent{
		CycleNumber: t.cycleNumber,
		Elapsed:     t.elapsed,
		Stats:       statsMap,
	}
	t.cycleNumber++
	t.elapsed = 0
	t.stats = [5]SeatCycleStats{}
	return ev
}

// NewMatchEngine validates rng and opts, returning a MatchEngine in MatchPhaseGathering.
func NewMatchEngine(rng rand.Source, opts ...MatchOption) (*MatchEngine, error) {
	o := matchOptions{
		base:   DefaultBase,
		perFan: DefaultPerFan,
	}
	for _, opt := range opts {
		if opt == nil {
			return nil, fmt.Errorf("nil match option: %w", ErrInvalidMatchConfig)
		}
		if err := opt(&o); err != nil {
			return nil, err
		}
	}
	if rng == nil && (o.wallFactory == nil || o.doorSeatPicker == nil) {
		return nil, fmt.Errorf("rand source required unless both wall factory and door seat picker are provided: %w", ErrInvalidMatchConfig)
	}

	return &MatchEngine{
		rng:            rng,
		base:           o.base,
		perFan:         o.perFan,
		wallFactory:    o.wallFactory,
		doorSeatPicker: o.doorSeatPicker,
		phase:          MatchPhaseGathering,
	}, nil
}

// StartMatch starts a new match when all 4 players are seated (WindEast, Seat1 dealer, streak 0)
// and immediately runs the opening sequence of the first hand.
func (m *MatchEngine) StartMatch(balances map[Seat]int) ([]Event, error) {
	if m.phase != MatchPhaseGathering {
		return nil, fmt.Errorf("match phase %d: %w", m.phase, ErrInvalidPhase)
	}
	for _, s := range AllSeats {
		if _, ok := balances[s]; !ok {
			return nil, fmt.Errorf("missing balance for seat %v: %w", s, ErrInvalidMatchConfig)
		}
	}

	var initBalances [5]int
	for _, s := range AllSeats {
		initBalances[s] = balances[s]
	}
	hand, handEvents, err := m.buildAndStartHand(WindEast, Seat1, 0, initBalances)
	if err != nil {
		return nil, err
	}

	m.phase = MatchPhaseInHand
	m.roundWind = WindEast
	m.dealer = Seat1
	m.dealerStreak = 0
	m.balances = initBalances
	m.hand = hand
	m.cycle.resetAll()

	events := make([]Event, 0, 1+len(handEvents))
	events = append(events, MatchStartedEvent{
		RoundWind:    WindEast,
		Dealer:       Seat1,
		DealerStreak: 0,
	})
	events = append(events, handEvents...)
	return events, nil
}

// Apply delegates one player action to the active hand.
// When a hand settles, balances and 4-round cycle statistics are updated immediately before WaitReady finishes.
// When all four seats confirm ActionReady, the match advances dealer/streak/round wind (emitting
// RoundCycleCompletedEvent before MatchProgressedEvent on North->East wrap) and starts the next hand.
func (m *MatchEngine) Apply(action PlayerAction, thinkElapsed time.Duration) ([]Event, error) {
	if m.phase != MatchPhaseInHand || m.hand == nil {
		return nil, fmt.Errorf("match phase %d: %w", m.phase, ErrInvalidPhase)
	}

	wasSettled := m.hand.Settled()
	events, err := m.hand.Apply(action, thinkElapsed)
	if err != nil {
		return nil, err
	}

	if !wasSettled && m.hand.Settled() {
		for _, s := range AllSeats {
			m.balances[s] = m.hand.balancesAfter[s]
		}
		reason, score := m.hand.HandResult()
		m.cycle.recordSettlement(reason, score)
	}

	if m.hand.Phase() == HandPhaseFinished {
		reason, score := m.hand.HandResult()
		nextWind, nextDealer, nextStreak := advanceMatchProgress(m.roundWind, m.dealer, m.dealerStreak, reason, score)
		nextHand, nextHandEvents, err := m.buildAndStartHand(nextWind, nextDealer, nextStreak, m.balances)
		if err != nil {
			m.hand.ready[action.Seat] = false
			m.hand.phase = HandPhaseWaitReady
			return nil, err
		}
		m.cycle.addThinkTime(thinkElapsed)
		completedCycle := m.roundWind == WindNorth && nextWind == WindEast
		m.roundWind = nextWind
		m.dealer = nextDealer
		m.dealerStreak = nextStreak
		m.hand = nextHand

		if completedCycle {
			events = append(events, m.cycle.completeCycle())
		}
		events = append(events, MatchProgressedEvent{
			RoundWind:    nextWind,
			Dealer:       nextDealer,
			DealerStreak: nextStreak,
		})
		events = append(events, nextHandEvents...)
		return events, nil
	}

	m.cycle.addThinkTime(thinkElapsed)
	return events, nil
}

// PlayerLeft aborts an active match when any seated player leaves (either mid-hand or during WaitReady),
// returning the match to MatchPhaseGathering without rolling back already-settled balances.
func (m *MatchEngine) PlayerLeft(leaver Seat) ([]Event, error) {
	if !leaver.Valid() {
		return nil, fmt.Errorf("leaver seat %v: %w", leaver, ErrInvalidAction)
	}
	if m.phase != MatchPhaseInHand || m.hand == nil {
		return nil, fmt.Errorf("match phase %d: %w", m.phase, ErrInvalidPhase)
	}

	wasPlaying := m.hand.Phase() == HandPhasePlaying
	handEvents, err := m.hand.Abort(leaver)
	if err != nil {
		return nil, err
	}

	m.phase = MatchPhaseGathering
	m.roundWind = WindUnknown
	m.dealer = SeatUnknown
	m.dealerStreak = 0
	m.hand = nil
	m.cycle.resetAll()

	events := make([]Event, 0, len(handEvents)+1)
	events = append(events, handEvents...)
	events = append(events, MatchAbortedEvent{
		Leaver:      leaver,
		HandAborted: wasPlaying,
	})
	return events, nil
}

func (m *MatchEngine) buildAndStartHand(roundWind Wind, dealer Seat, streak int, balances [5]int) (*HandEngine, []Event, error) {
	balMap := make(map[Seat]int, len(AllSeats))
	for _, s := range AllSeats {
		balMap[s] = balances[s]
	}

	var opts []HandOption
	if m.wallFactory != nil {
		w, err := m.wallFactory(m.rng)
		if err != nil {
			return nil, nil, fmt.Errorf("wall factory: %w", err)
		}
		opts = append(opts, WithWall(w))
	}
	if m.doorSeatPicker != nil {
		opts = append(opts, WithDoorSeat(m.doorSeatPicker(m.rng)))
	}

	hand, err := NewHandEngine(HandConfig{
		RoundWind:    roundWind,
		Dealer:       dealer,
		DealerStreak: streak,
		Balances:     balMap,
		Base:         m.base,
		PerFan:       m.perFan,
		Rand:         m.rng,
	}, opts...)
	if err != nil {
		return nil, nil, err
	}
	events, err := hand.Start()
	if err != nil {
		return nil, nil, err
	}
	return hand, events, nil
}

func advanceMatchProgress(roundWind Wind, dealer Seat, streak int, reason HandEndReason, score *ScoreResult) (Wind, Seat, int) {
	if reason.IsDraw() {
		return roundWind, dealer, streak + 1
	}
	if reason == HandEndWin && score != nil && score.Winner == dealer {
		return roundWind, dealer, streak + 1
	}
	nextDealer := dealer.Next()
	nextWind := roundWind
	if nextDealer == Seat1 {
		nextWind = roundWind.Next()
	}
	return nextWind, nextDealer, 0
}

// Phase returns the current MatchPhase.
func (m *MatchEngine) Phase() MatchPhase { return m.phase }

// RoundWind returns the active round wind, or WindUnknown while gathering.
func (m *MatchEngine) RoundWind() Wind { return m.roundWind }

// Dealer returns the active dealer seat, or SeatUnknown while gathering.
func (m *MatchEngine) Dealer() Seat { return m.dealer }

// DealerStreak returns the current dealer's consecutive streak count.
func (m *MatchEngine) DealerStreak() int { return m.dealerStreak }

// Balances returns a defensive copy of the four seats' current balances.
func (m *MatchEngine) Balances() map[Seat]int {
	out := make(map[Seat]int, len(AllSeats))
	for _, s := range AllSeats {
		out[s] = m.balances[s]
	}
	return out
}

// HandPhase returns the active hand's HandPhase, or HandPhaseUnknown while gathering.
func (m *MatchEngine) HandPhase() HandPhase {
	if m.hand == nil {
		return HandPhaseUnknown
	}
	return m.hand.Phase()
}

// TurnPhase returns the active hand's TurnPhase, or PhaseUnknown while gathering.
func (m *MatchEngine) TurnPhase() TurnPhase {
	if m.hand == nil {
		return PhaseUnknown
	}
	return m.hand.TurnPhase()
}

// CurrentTurn returns the active hand's current turn seat, or SeatUnknown while gathering.
func (m *MatchEngine) CurrentTurn() Seat {
	if m.hand == nil {
		return SeatUnknown
	}
	return m.hand.CurrentTurn()
}

// DrawableRemaining returns the active hand's drawable wall count, or 0 while gathering.
func (m *MatchEngine) DrawableRemaining() int {
	if m.hand == nil {
		return 0
	}
	return m.hand.DrawableRemaining()
}

// DoorSeat returns the active hand's door seat (WindEast), or SeatUnknown while gathering.
func (m *MatchEngine) DoorSeat() Seat {
	if m.hand == nil {
		return SeatUnknown
	}
	return m.hand.DoorSeat()
}

// SeatWinds returns a defensive copy of the active hand's seat winds, or nil while gathering.
func (m *MatchEngine) SeatWinds() map[Seat]Wind {
	if m.hand == nil {
		return nil
	}
	return m.hand.SeatWinds()
}
