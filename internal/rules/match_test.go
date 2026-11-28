package rules_test

import (
	"errors"
	"math/rand/v2"
	"slices"
	"testing"
	"time"

	"github.com/gjchentw/go-qkmj/internal/rules"
)

func buildDealerTianHuWall(t *testing.T, dealer rules.Seat) *rules.Wall {
	t.Helper()
	hands, _ := nonWinningHandsNoFlowers()
	hands[dealer] = []rules.Tile{
		rules.Wan1, rules.Wan2, rules.Wan3,
		rules.Wan4, rules.Wan5, rules.Wan6,
		rules.Suo1, rules.Suo2, rules.Suo3,
		rules.Tong1, rules.Tong2, rules.Tong3,
		rules.Tong7, rules.Tong8, rules.Tong9,
		rules.Wan9,
	}
	return buildDeterministicWall(t, dealer, hands, rules.Wan9, nil)
}

func buildOpeningEightFlowersWall(t *testing.T, dealer, flowerWinner rules.Seat) *rules.Wall {
	t.Helper()
	hands, extra := nonWinningHandsNoFlowers()
	allFlowers := []rules.Tile{
		rules.FlowerSpring, rules.FlowerSummer, rules.FlowerAutumn, rules.FlowerWinter,
		rules.FlowerPlum, rules.FlowerOrchid, rules.FlowerChrysanthemum, rules.FlowerBamboo,
	}
	copy(hands[flowerWinner], allFlowers)
	return buildDeterministicWall(t, dealer, hands, extra, nil)
}

func buildExhaustedDrawWall(t *testing.T, dealer rules.Seat) *rules.Wall {
	t.Helper()
	hands := map[rules.Seat][]rules.Tile{
		rules.Seat1: {
			rules.Wan1, rules.Wan4, rules.Wan7, rules.Wan9,
			rules.Suo1, rules.Suo4, rules.Suo7, rules.Suo9,
			rules.Tong1, rules.Tong4, rules.Tong7, rules.Tong9,
			rules.EastWind, rules.EastWind, rules.EastWind, rules.Wan3,
		},
		rules.Seat2: {
			rules.Wan2, rules.Wan5, rules.Wan8,
			rules.Suo2, rules.Suo5, rules.Suo8,
			rules.Tong2, rules.Tong5, rules.Tong8,
			rules.SouthWind, rules.WestWind, rules.NorthWind,
			rules.RedDragon, rules.WhiteDragon, rules.GreenDragon, rules.Wan1,
		},
		rules.Seat3: {
			rules.Wan3, rules.Wan6, rules.Wan9,
			rules.Suo3, rules.Suo6, rules.Suo9,
			rules.Tong3, rules.Tong6, rules.Tong9,
			rules.SouthWind, rules.WestWind, rules.NorthWind,
			rules.RedDragon, rules.WhiteDragon, rules.GreenDragon, rules.Suo1,
		},
		rules.Seat4: {
			rules.Wan2, rules.Wan5, rules.Wan8,
			rules.Suo2, rules.Suo5, rules.Suo8,
			rules.Tong2, rules.Tong5, rules.Tong8,
			rules.SouthWind, rules.WestWind, rules.NorthWind,
			rules.RedDragon, rules.WhiteDragon, rules.GreenDragon, rules.Tong1,
		},
	}
	return buildDeterministicWall(t, dealer, hands, rules.EastWind, nil)
}

func readyAllSeats(t *testing.T, m *rules.MatchEngine) []rules.Event {
	t.Helper()
	var last []rules.Event
	for _, s := range rules.AllSeats {
		evs, err := m.Apply(rules.PlayerAction{Seat: s, Kind: rules.ActionReady}, 0)
		if err != nil {
			t.Fatalf("ActionReady(%v): %v", s, err)
		}
		last = evs
	}
	return last
}

func driveMatchHandToExhaustion(t *testing.T, m *rules.MatchEngine, startEvs []rules.Event) {
	t.Helper()

	privateHands := make(map[rules.Seat][]rules.Tile, 4)
	pendingClaims := make(map[rules.Seat]bool)
	track := func(evs []rules.Event) {
		for _, ev := range evs {
			switch e := ev.(type) {
			case rules.HandDealtEvent:
				privateHands[e.Seat] = slices.Clone(e.Tiles)
			case rules.FlowerExposedEvent:
				if idx := slices.Index(privateHands[e.Seat], e.Flower); idx >= 0 {
					privateHands[e.Seat] = slices.Delete(privateHands[e.Seat], idx, idx+1)
				}
			case rules.TileDrawnEvent:
				privateHands[e.Seat] = append(privateHands[e.Seat], e.Tile)
			case rules.DiscardedEvent:
				if idx := slices.Index(privateHands[e.Seat], e.Tile); idx >= 0 {
					privateHands[e.Seat] = slices.Delete(privateHands[e.Seat], idx, idx+1)
				}
			case rules.ClaimPromptEvent:
				pendingClaims[e.Seat] = true
			}
		}
	}
	track(startEvs)

	for guard := 0; m.HandPhase() == rules.HandPhasePlaying && guard < 400; guard++ {
		var (
			evs []rules.Event
			err error
		)
		switch m.TurnPhase() {
		case rules.PhaseWaitDraw:
			evs, err = m.Apply(rules.PlayerAction{Seat: m.CurrentTurn(), Kind: rules.ActionDraw}, 0)
		case rules.PhaseWaitSelfDeclare:
			evs, err = m.Apply(rules.PlayerAction{Seat: m.CurrentTurn(), Kind: rules.ActionSelfDeclare, SelfDeclare: rules.SelfDeclarePass}, 0)
		case rules.PhaseWaitDiscard:
			seat := m.CurrentTurn()
			h := privateHands[seat]
			evs, err = m.Apply(rules.PlayerAction{Seat: seat, Kind: rules.ActionDiscard, Tile: h[len(h)-1]}, time.Second)
		case rules.PhaseWaitClaims:
			var nextSeat rules.Seat
			for _, s := range rules.AllSeats {
				if pendingClaims[s] {
					nextSeat = s
					delete(pendingClaims, s)
					break
				}
			}
			evs, err = m.Apply(rules.PlayerAction{Seat: nextSeat, Kind: rules.ActionClaim, Claim: rules.ClaimPass}, 0)
		default:
			t.Fatalf("unexpected TurnPhase %v", m.TurnPhase())
		}
		if err != nil {
			t.Fatalf("exhaust step %d (turnPhase=%v): %v", guard, m.TurnPhase(), err)
		}
		track(evs)
	}
	if m.HandPhase() != rules.HandPhaseWaitReady {
		t.Fatalf("HandPhase = %v, want HandPhaseWaitReady after exhaustion", m.HandPhase())
	}
}

func TestNewMatchEngine_ValidationAndOptions(t *testing.T) {
	t.Parallel()

	rng := rand.NewPCG(1, 2)
	tests := []struct {
		name string
		rng  rand.Source
		opts []rules.MatchOption
	}{
		{
			name: "nil rng without factory overrides",
			rng:  nil,
		},
		{
			name: "negative base stake",
			rng:  rng,
			opts: []rules.MatchOption{rules.WithStakes(-1, 200)},
		},
		{
			name: "negative per-fan stake",
			rng:  rng,
			opts: []rules.MatchOption{rules.WithStakes(500, -1)},
		},
		{
			name: "nil wall factory",
			rng:  rng,
			opts: []rules.MatchOption{rules.WithWallFactory(nil)},
		},
		{
			name: "nil door seat picker",
			rng:  rng,
			opts: []rules.MatchOption{rules.WithDoorSeatPicker(nil)},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			if _, err := rules.NewMatchEngine(tt.rng, tt.opts...); !errors.Is(err, rules.ErrInvalidMatchConfig) {
				t.Errorf("NewMatchEngine() err = %v, want ErrInvalidMatchConfig", err)
			}
		})
	}
}

func TestMatchEngine_StartMatch_InitialStateAndErrors(t *testing.T) {
	t.Parallel()

	m, err := rules.NewMatchEngine(rand.NewPCG(7, 8))
	if err != nil {
		t.Fatalf("NewMatchEngine: %v", err)
	}
	if m.Phase() != rules.MatchPhaseGathering {
		t.Fatalf("initial Phase() = %v, want MatchPhaseGathering", m.Phase())
	}
	if _, err := m.Apply(rules.PlayerAction{Seat: rules.Seat1, Kind: rules.ActionDraw}, 0); !errors.Is(err, rules.ErrInvalidPhase) {
		t.Errorf("Apply in Gathering err = %v, want ErrInvalidPhase", err)
	}
	if _, err := m.StartMatch(map[rules.Seat]int{rules.Seat1: 40000}); !errors.Is(err, rules.ErrInvalidMatchConfig) {
		t.Errorf("StartMatch with missing seats err = %v, want ErrInvalidMatchConfig", err)
	}

	events, err := m.StartMatch(defaultBalances())
	if err != nil {
		t.Fatalf("StartMatch: %v", err)
	}
	if m.Phase() != rules.MatchPhaseInHand || m.RoundWind() != rules.WindEast || m.Dealer() != rules.Seat1 || m.DealerStreak() != 0 {
		t.Fatalf("unexpected match start state: phase=%v wind=%v dealer=%v streak=%d", m.Phase(), m.RoundWind(), m.Dealer(), m.DealerStreak())
	}
	started, ok := events[0].(rules.MatchStartedEvent)
	if !ok || started.RoundWind != rules.WindEast || started.Dealer != rules.Seat1 || started.DealerStreak != 0 {
		t.Fatalf("events[0] = %+v, want MatchStartedEvent(East, Seat1, 0)", events[0])
	}
	if _, ok := events[1].(rules.HandStartedEvent); !ok {
		t.Fatalf("events[1] = %T, want HandStartedEvent", events[1])
	}

	// Duplicate StartMatch while in hand must fail without mutating state.
	if _, err := m.StartMatch(defaultBalances()); !errors.Is(err, rules.ErrInvalidPhase) {
		t.Errorf("second StartMatch err = %v, want ErrInvalidPhase", err)
	}
}

func TestMatchEngine_Progression_DealerStreakDrawRotationAndEndlessWrap(t *testing.T) {
	t.Parallel()

	// Plan 18 hands:
	//  Hand 0 (East, Seat1, 0): Dealer Seat1 wins TianHu -> streak 1
	//  Hand 1 (East, Seat1, 1): Exhausted draw -> streak 2
	//  Hands 2..17 (16 rotations: East 1->2->3->4, South 1->2->3->4, West 1->2->3->4, North 1->2->3->4 -> East 1!):
	//    Each hand has non-dealer (dealer.Next()) win opening Eight Immortals -> passes deal clockwise and wraps North->East!
	handIdx := 0
	currentDealer := rules.Seat1
	wallFactory := func(_ rand.Source) (*rules.Wall, error) {
		defer func() { handIdx++ }()
		switch handIdx {
		case 0:
			return buildDealerTianHuWall(t, rules.Seat1), nil
		case 1:
			return buildExhaustedDrawWall(t, rules.Seat1), nil
		default:
			winner := currentDealer.Next()
			w := buildOpeningEightFlowersWall(t, currentDealer, winner)
			currentDealer = winner
			return w, nil
		}
	}
	doorCalls := 0
	doorPicker := func(_ rand.Source) rules.Seat {
		doorCalls++
		return rules.AllSeats[(doorCalls-1)%len(rules.AllSeats)]
	}

	m, err := rules.NewMatchEngine(
		nil,
		rules.WithStakes(rules.DefaultBase, rules.DefaultPerFan),
		rules.WithWallFactory(wallFactory),
		rules.WithDoorSeatPicker(doorPicker),
	)
	if err != nil {
		t.Fatalf("NewMatchEngine: %v", err)
	}

	if _, err := m.StartMatch(defaultBalances()); err != nil {
		t.Fatalf("StartMatch: %v", err)
	}

	// Hand 0: Dealer Seat1 wins TianHu.
	if _, err := m.Apply(rules.PlayerAction{
		Seat:        rules.Seat1,
		Kind:        rules.ActionSelfDeclare,
		SelfDeclare: rules.SelfDeclareSelfWin,
	}, 0); err != nil {
		t.Fatalf("Hand 0 TianHu: %v", err)
	}
	// Balances must already reflect Hand 0 settlement BEFORE WaitReady completes.
	balAfterHand0 := m.Balances()
	if balAfterHand0[rules.Seat1] <= 40000 || balAfterHand0[rules.Seat2] >= 40000 {
		t.Fatalf("balances not updated immediately on settlement: %+v", balAfterHand0)
	}
	hand1StartEvs := readyAllSeats(t, m)
	if m.RoundWind() != rules.WindEast || m.Dealer() != rules.Seat1 || m.DealerStreak() != 1 {
		t.Fatalf("after dealer win: wind=%v dealer=%v streak=%d, want East/Seat1/1", m.RoundWind(), m.Dealer(), m.DealerStreak())
	}
	if m.DoorSeat() != rules.Seat2 {
		t.Errorf("door seat on streak hand = %v, want Seat2 (re-rolled)", m.DoorSeat())
	}

	// Hand 1: Exhausted draw increments dealer streak to 2 and leaves balances unchanged.
	driveMatchHandToExhaustion(t, m, hand1StartEvs)
	if m.Balances()[rules.Seat1] != balAfterHand0[rules.Seat1] {
		t.Fatalf("exhausted draw modified balance: got %d, want %d", m.Balances()[rules.Seat1], balAfterHand0[rules.Seat1])
	}
	readyAllSeats(t, m)
	if m.RoundWind() != rules.WindEast || m.Dealer() != rules.Seat1 || m.DealerStreak() != 2 {
		t.Fatalf("after draw: wind=%v dealer=%v streak=%d, want East/Seat1/2", m.RoundWind(), m.Dealer(), m.DealerStreak())
	}

	// Hands 2..17: 16 consecutive non-dealer Eight Immortals wins rotate through all 4 winds
	// and wrap from North Seat4 back to East Seat1 (endless match!).
	expectedSequence := []struct {
		wind   rules.Wind
		dealer rules.Seat
	}{
		{rules.WindEast, rules.Seat2},
		{rules.WindEast, rules.Seat3},
		{rules.WindEast, rules.Seat4},
		{rules.WindSouth, rules.Seat1},
		{rules.WindSouth, rules.Seat2},
		{rules.WindSouth, rules.Seat3},
		{rules.WindSouth, rules.Seat4},
		{rules.WindWest, rules.Seat1},
		{rules.WindWest, rules.Seat2},
		{rules.WindWest, rules.Seat3},
		{rules.WindWest, rules.Seat4},
		{rules.WindNorth, rules.Seat1},
		{rules.WindNorth, rules.Seat2},
		{rules.WindNorth, rules.Seat3},
		{rules.WindNorth, rules.Seat4},
		{rules.WindEast, rules.Seat1}, // North -> East endless wrap!
	}

	for i, want := range expectedSequence {
		winner := m.Dealer().Next()
		if _, err := m.Apply(rules.PlayerAction{
			Seat:  winner,
			Kind:  rules.ActionClaim,
			Claim: rules.ClaimWin,
		}, 0); err != nil {
			t.Fatalf("rotation %d (%v wins Eight Immortals): %v", i, winner, err)
		}
		evs := readyAllSeats(t, m)
		if m.RoundWind() != want.wind || m.Dealer() != want.dealer || m.DealerStreak() != 0 {
			t.Fatalf("rotation %d: got (%v, %v, streak=%d), want (%v, %v, streak=0)",
				i, m.RoundWind(), m.Dealer(), m.DealerStreak(), want.wind, want.dealer)
		}
		idx := slices.IndexFunc(evs, func(ev rules.Event) bool {
			mp, ok := ev.(rules.MatchProgressedEvent)
			return ok && mp.RoundWind == want.wind && mp.Dealer == want.dealer && mp.DealerStreak == 0
		})
		if idx < 0 {
			t.Fatalf("rotation %d: missing MatchProgressedEvent(%v, %v, 0) in %+v", i, want.wind, want.dealer, evs)
		}
	}
}

func TestMatchEngine_PlayerLeft_PlayingAbortResetsAndPreservesPriorSettlement(t *testing.T) {
	t.Parallel()

	// Advance to South Wind (4 rotations of East), then Seat3 leaves mid-hand in South Wind.
	handNum := 0
	nextDealer := rules.Seat1
	wallFactory := func(_ rand.Source) (*rules.Wall, error) {
		defer func() { handNum++ }()
		if handNum < 4 {
			winner := nextDealer.Next()
			w := buildOpeningEightFlowersWall(t, nextDealer, winner)
			nextDealer = winner
			return w, nil
		}
		// South hand and restarted East hand: plain non-winning wall.
		hands, extra := nonWinningHandsNoFlowers()
		return buildDeterministicWall(t, rules.Seat1, hands, extra, nil), nil
	}

	m, err := rules.NewMatchEngine(
		nil,
		rules.WithWallFactory(wallFactory),
		rules.WithDoorSeatPicker(func(_ rand.Source) rules.Seat { return rules.Seat1 }),
	)
	if err != nil {
		t.Fatalf("NewMatchEngine: %v", err)
	}

	// PlayerLeft while gathering must fail.
	if _, err := m.PlayerLeft(rules.Seat1); !errors.Is(err, rules.ErrInvalidPhase) {
		t.Errorf("PlayerLeft in Gathering err = %v, want ErrInvalidPhase", err)
	}

	if _, err := m.StartMatch(defaultBalances()); err != nil {
		t.Fatalf("StartMatch: %v", err)
	}
	for range 4 {
		winner := m.Dealer().Next()
		if _, err := m.Apply(rules.PlayerAction{Seat: winner, Kind: rules.ActionClaim, Claim: rules.ClaimWin}, 0); err != nil {
			t.Fatalf("claim win: %v", err)
		}
		readyAllSeats(t, m)
	}
	if m.RoundWind() != rules.WindSouth || m.Dealer() != rules.Seat1 {
		t.Fatalf("expected South Seat1 before leave, got (%v, %v)", m.RoundWind(), m.Dealer())
	}
	settledBeforeLeave := m.Balances()

	// Invalid leaver rejected without mutating state.
	if _, err := m.PlayerLeft(rules.SeatUnknown); !errors.Is(err, rules.ErrInvalidAction) {
		t.Errorf("PlayerLeft(SeatUnknown) err = %v, want ErrInvalidAction", err)
	}

	evs, err := m.PlayerLeft(rules.Seat3)
	if err != nil {
		t.Fatalf("PlayerLeft(Seat3): %v", err)
	}
	if m.Phase() != rules.MatchPhaseGathering || m.HandPhase() != rules.HandPhaseUnknown {
		t.Fatalf("after PlayerLeft: phase=%v handPhase=%v, want Gathering/Unknown", m.Phase(), m.HandPhase())
	}
	if m.RoundWind() != rules.WindUnknown || m.Dealer() != rules.SeatUnknown || m.DealerStreak() != 0 {
		t.Fatalf("after PlayerLeft: wind=%v dealer=%v streak=%d, want reset", m.RoundWind(), m.Dealer(), m.DealerStreak())
	}
	for _, ev := range evs {
		switch ev.(type) {
		case rules.HandsRevealedEvent, rules.HandSettledEvent:
			t.Fatalf("mid-hand PlayerLeft must not reveal or settle, got %T", ev)
		}
	}
	if idx := slices.IndexFunc(evs, func(ev rules.Event) bool {
		ma, ok := ev.(rules.MatchAbortedEvent)
		return ok && ma.Leaver == rules.Seat3 && ma.HandAborted
	}); idx < 0 {
		t.Fatalf("missing MatchAbortedEvent(Seat3, HandAborted=true) in %+v", evs)
	}

	// Previously settled balances must remain intact.
	for _, s := range rules.AllSeats {
		if m.Balances()[s] != settledBeforeLeave[s] {
			t.Errorf("seat %v balance changed on mid-hand abort: got %d, want %d", s, m.Balances()[s], settledBeforeLeave[s])
		}
	}

	// Re-starting match after gathering resets to East Wind, Seat1 dealer, streak 0.
	if _, err := m.StartMatch(settledBeforeLeave); err != nil {
		t.Fatalf("restart StartMatch: %v", err)
	}
	if m.RoundWind() != rules.WindEast || m.Dealer() != rules.Seat1 || m.DealerStreak() != 0 {
		t.Fatalf("restarted match state = (%v, %v, %d), want (East, Seat1, 0)", m.RoundWind(), m.Dealer(), m.DealerStreak())
	}
}

func TestMatchEngine_PlayerLeft_WaitReadyPreservesSettlement(t *testing.T) {
	t.Parallel()

	m, err := rules.NewMatchEngine(
		nil,
		rules.WithWallFactory(func(_ rand.Source) (*rules.Wall, error) {
			return buildDealerTianHuWall(t, rules.Seat1), nil
		}),
		rules.WithDoorSeatPicker(func(_ rand.Source) rules.Seat { return rules.Seat1 }),
	)
	if err != nil {
		t.Fatalf("NewMatchEngine: %v", err)
	}
	if _, err := m.StartMatch(defaultBalances()); err != nil {
		t.Fatalf("StartMatch: %v", err)
	}
	if _, err := m.Apply(rules.PlayerAction{
		Seat:        rules.Seat1,
		Kind:        rules.ActionSelfDeclare,
		SelfDeclare: rules.SelfDeclareSelfWin,
	}, 0); err != nil {
		t.Fatalf("TianHu: %v", err)
	}
	afterSettle := m.Balances()
	if afterSettle[rules.Seat1] <= 40000 {
		t.Fatalf("expected Seat1 positive delta, got %+v", afterSettle)
	}

	evs, err := m.PlayerLeft(rules.Seat4)
	if err != nil {
		t.Fatalf("PlayerLeft in WaitReady: %v", err)
	}
	if m.Phase() != rules.MatchPhaseGathering {
		t.Fatalf("Phase() = %v, want MatchPhaseGathering", m.Phase())
	}
	if len(evs) != 1 {
		t.Fatalf("expected single MatchAbortedEvent, got %+v", evs)
	}
	ma, ok := evs[0].(rules.MatchAbortedEvent)
	if !ok || ma.Leaver != rules.Seat4 || ma.HandAborted {
		t.Fatalf("events[0] = %+v, want MatchAbortedEvent{Leaver: Seat4, HandAborted: false}", evs[0])
	}
	for _, s := range rules.AllSeats {
		if m.Balances()[s] != afterSettle[s] {
			t.Errorf("WaitReady leave rolled back seat %v: got %d, want %d", s, m.Balances()[s], afterSettle[s])
		}
	}
}

func TestMatchEngine_NegativeBalancePlayerContinuesMatch(t *testing.T) {
	t.Parallel()

	call := 0
	m, err := rules.NewMatchEngine(
		nil,
		rules.WithWallFactory(func(_ rand.Source) (*rules.Wall, error) {
			defer func() { call++ }()
			if call == 0 {
				return buildDealerTianHuWall(t, rules.Seat1), nil
			}
			hands, extra := nonWinningHandsNoFlowers()
			return buildDeterministicWall(t, rules.Seat1, hands, extra, nil), nil
		}),
		rules.WithDoorSeatPicker(func(_ rand.Source) rules.Seat { return rules.Seat1 }),
	)
	if err != nil {
		t.Fatalf("NewMatchEngine: %v", err)
	}

	lowBalances := map[rules.Seat]int{
		rules.Seat1: 1000,
		rules.Seat2: 1000,
		rules.Seat3: 1000,
		rules.Seat4: 1000,
	}
	if _, err := m.StartMatch(lowBalances); err != nil {
		t.Fatalf("StartMatch: %v", err)
	}
	if _, err := m.Apply(rules.PlayerAction{
		Seat:        rules.Seat1,
		Kind:        rules.ActionSelfDeclare,
		SelfDeclare: rules.SelfDeclareSelfWin,
	}, 0); err != nil {
		t.Fatalf("TianHu: %v", err)
	}
	if m.Balances()[rules.Seat2] >= 0 {
		t.Fatalf("expected Seat2 balance to drop negative after TianHu, got %d", m.Balances()[rules.Seat2])
	}

	// All 4 seats confirm Ready -> next hand starts normally despite negative balances!
	readyAllSeats(t, m)
	if m.Phase() != rules.MatchPhaseInHand || m.HandPhase() != rules.HandPhasePlaying || m.DealerStreak() != 1 {
		t.Fatalf("expected match to continue with negative balance: phase=%v handPhase=%v streak=%d",
			m.Phase(), m.HandPhase(), m.DealerStreak())
	}
}

func TestMatchEngine_Apply_AtomicRollbackOnNextHandFactoryError(t *testing.T) {
	t.Parallel()

	errBoom := errors.New("injected wall factory failure")
	calls := 0
	m, err := rules.NewMatchEngine(
		nil,
		rules.WithWallFactory(func(_ rand.Source) (*rules.Wall, error) {
			calls++
			if calls == 1 {
				return buildDealerTianHuWall(t, rules.Seat1), nil
			}
			return nil, errBoom
		}),
		rules.WithDoorSeatPicker(func(_ rand.Source) rules.Seat { return rules.Seat1 }),
	)
	if err != nil {
		t.Fatalf("NewMatchEngine: %v", err)
	}
	if _, err := m.StartMatch(defaultBalances()); err != nil {
		t.Fatalf("StartMatch: %v", err)
	}
	if _, err := m.Apply(rules.PlayerAction{
		Seat:        rules.Seat1,
		Kind:        rules.ActionSelfDeclare,
		SelfDeclare: rules.SelfDeclareSelfWin,
	}, 0); err != nil {
		t.Fatalf("TianHu: %v", err)
	}

	for _, s := range []rules.Seat{rules.Seat1, rules.Seat2, rules.Seat3} {
		if _, err := m.Apply(rules.PlayerAction{Seat: s, Kind: rules.ActionReady}, 0); err != nil {
			t.Fatalf("ActionReady(%v): %v", s, err)
		}
	}

	// 4th ActionReady triggers next-hand creation, which fails via wallFactory.
	if _, err := m.Apply(rules.PlayerAction{Seat: rules.Seat4, Kind: rules.ActionReady}, 0); !errors.Is(err, errBoom) {
		t.Fatalf("4th ActionReady err = %v, want errBoom", err)
	}
	// State must roll back cleanly to HandPhaseWaitReady (not wedged at HandPhaseFinished).
	if m.HandPhase() != rules.HandPhaseWaitReady {
		t.Fatalf("HandPhase after factory error = %v, want HandPhaseWaitReady", m.HandPhase())
	}
	// Match must remain recoverable (e.g. PlayerLeft succeeds).
	if _, err := m.PlayerLeft(rules.Seat4); err != nil {
		t.Fatalf("PlayerLeft after failed next-hand start: %v", err)
	}
}
