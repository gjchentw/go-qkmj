package rules_test

import (
	"errors"
	"math/rand/v2"
	"slices"
	"testing"
	"time"

	"github.com/gjchentw/go-qkmj/internal/rules"
)

func defaultBalances() map[rules.Seat]int {
	return map[rules.Seat]int{
		rules.Seat1: 40000,
		rules.Seat2: 40000,
		rules.Seat3: 40000,
		rules.Seat4: 40000,
	}
}

// buildDeterministicWall constructs a valid 144-tile Wall where the first 65 tiles deal
// hands[dealer] (16), hands[dealer.Next()] (16), hands[dealer.Opposite()] (16),
// hands[dealer.Prev()] (16), and dealerExtra (1), followed by drawQueue, and padded with
// all remaining tiles from FullDeck() to reach 144 tiles.
func buildDeterministicWall(
	t *testing.T,
	dealer rules.Seat,
	hands map[rules.Seat][]rules.Tile,
	dealerExtra rules.Tile,
	drawQueue []rules.Tile,
) *rules.Wall {
	t.Helper()

	var used [rules.TileMax + 1]int
	tiles := make([]rules.Tile, 0, rules.TotalDeckTiles)
	appendTiles := func(ts ...rules.Tile) {
		for _, tile := range ts {
			used[tile]++
			maxCount := 4
			if tile.IsFlower() {
				maxCount = 1
			}
			if used[tile] > maxCount {
				t.Fatalf("tile %v used %d times (max %d)", tile, used[tile], maxCount)
			}
			tiles = append(tiles, tile)
		}
	}

	seat := dealer
	for range 4 {
		h := hands[seat]
		if len(h) != rules.WaitingHandSize {
			t.Fatalf("seat %v hand len = %d, want %d", seat, len(h), rules.WaitingHandSize)
		}
		appendTiles(h...)
		seat = seat.Next()
	}
	appendTiles(dealerExtra)
	appendTiles(drawQueue...)

	for _, tile := range rules.FullDeck() {
		maxCount := 4
		if tile.IsFlower() {
			maxCount = 1
		}
		if used[tile] < maxCount {
			used[tile]++
			tiles = append(tiles, tile)
		}
	}
	if len(tiles) != rules.TotalDeckTiles {
		t.Fatalf("built wall len = %d, want %d", len(tiles), rules.TotalDeckTiles)
	}
	w, err := rules.NewWallFromTiles(tiles)
	if err != nil {
		t.Fatalf("NewWallFromTiles: %v", err)
	}
	return w
}

func nonWinningHandsNoFlowers() (map[rules.Seat][]rules.Tile, rules.Tile) {
	hands := map[rules.Seat][]rules.Tile{
		rules.Seat1: {
			rules.Wan1, rules.Wan3, rules.Wan5, rules.Wan7, rules.Wan9,
			rules.Suo1, rules.Suo3, rules.Suo5, rules.Suo7, rules.Suo9,
			rules.Tong1, rules.Tong3, rules.Tong5, rules.Tong7, rules.Tong9,
			rules.EastWind,
		},
		rules.Seat2: {
			rules.Wan2, rules.Wan4, rules.Wan6, rules.Wan8,
			rules.Suo2, rules.Suo4, rules.Suo6, rules.Suo8,
			rules.Tong2, rules.Tong4, rules.Tong6, rules.Tong8,
			rules.SouthWind, rules.WestWind, rules.NorthWind, rules.RedDragon,
		},
		rules.Seat3: {
			rules.Wan1, rules.Wan4, rules.Wan7, rules.Wan9,
			rules.Suo1, rules.Suo4, rules.Suo7, rules.Suo9,
			rules.Tong1, rules.Tong4, rules.Tong7, rules.Tong9,
			rules.EastWind, rules.SouthWind, rules.WhiteDragon, rules.GreenDragon,
		},
		rules.Seat4: {
			rules.Wan2, rules.Wan5, rules.Wan8,
			rules.Suo2, rules.Suo5, rules.Suo8,
			rules.Tong2, rules.Tong5, rules.Tong8,
			rules.WestWind, rules.NorthWind, rules.RedDragon, rules.WhiteDragon,
			rules.GreenDragon, rules.Wan3, rules.Suo3,
		},
	}
	return hands, rules.Tong3
}

func TestNewHandEngine_ValidationAndOptions(t *testing.T) {
	t.Parallel()

	rng := rand.NewPCG(1, 2)
	validWall, err := rules.NewWall(rand.NewPCG(3, 4))
	if err != nil {
		t.Fatalf("NewWall: %v", err)
	}
	usedWall, err := rules.NewWall(rand.NewPCG(5, 6))
	if err != nil {
		t.Fatalf("NewWall: %v", err)
	}
	if _, err := usedWall.Draw(); err != nil {
		t.Fatalf("Draw: %v", err)
	}

	tests := []struct {
		name string
		cfg  rules.HandConfig
		opts []rules.HandOption
	}{
		{
			name: "invalid round wind",
			cfg: rules.HandConfig{
				RoundWind: rules.WindUnknown,
				Dealer:    rules.Seat1,
				Balances:  defaultBalances(),
				Base:      rules.DefaultBase,
				PerFan:    rules.DefaultPerFan,
				Rand:      rng,
			},
		},
		{
			name: "invalid dealer",
			cfg: rules.HandConfig{
				RoundWind: rules.WindEast,
				Dealer:    rules.SeatUnknown,
				Balances:  defaultBalances(),
				Base:      rules.DefaultBase,
				PerFan:    rules.DefaultPerFan,
				Rand:      rng,
			},
		},
		{
			name: "negative dealer streak",
			cfg: rules.HandConfig{
				RoundWind:    rules.WindEast,
				Dealer:       rules.Seat1,
				DealerStreak: -1,
				Balances:     defaultBalances(),
				Base:         rules.DefaultBase,
				PerFan:       rules.DefaultPerFan,
				Rand:         rng,
			},
		},
		{
			name: "negative stakes",
			cfg: rules.HandConfig{
				RoundWind: rules.WindEast,
				Dealer:    rules.Seat1,
				Balances:  defaultBalances(),
				Base:      -1,
				PerFan:    rules.DefaultPerFan,
				Rand:      rng,
			},
		},
		{
			name: "missing balance seat",
			cfg: rules.HandConfig{
				RoundWind: rules.WindEast,
				Dealer:    rules.Seat1,
				Balances:  map[rules.Seat]int{rules.Seat1: 40000},
				Base:      rules.DefaultBase,
				PerFan:    rules.DefaultPerFan,
				Rand:      rng,
			},
		},
		{
			name: "nil rand without wall and door overrides",
			cfg: rules.HandConfig{
				RoundWind: rules.WindEast,
				Dealer:    rules.Seat1,
				Balances:  defaultBalances(),
				Base:      rules.DefaultBase,
				PerFan:    rules.DefaultPerFan,
			},
		},
		{
			name: "WithWall nil rejected",
			cfg: rules.HandConfig{
				RoundWind: rules.WindEast,
				Dealer:    rules.Seat1,
				Balances:  defaultBalances(),
				Base:      rules.DefaultBase,
				PerFan:    rules.DefaultPerFan,
				Rand:      rng,
			},
			opts: []rules.HandOption{rules.WithWall(nil)},
		},
		{
			name: "WithWall partially drawn rejected",
			cfg: rules.HandConfig{
				RoundWind: rules.WindEast,
				Dealer:    rules.Seat1,
				Balances:  defaultBalances(),
				Base:      rules.DefaultBase,
				PerFan:    rules.DefaultPerFan,
				Rand:      rng,
			},
			opts: []rules.HandOption{rules.WithWall(usedWall)},
		},
		{
			name: "WithDoorSeat invalid rejected",
			cfg: rules.HandConfig{
				RoundWind: rules.WindEast,
				Dealer:    rules.Seat1,
				Balances:  defaultBalances(),
				Base:      rules.DefaultBase,
				PerFan:    rules.DefaultPerFan,
				Rand:      rng,
			},
			opts: []rules.HandOption{rules.WithWall(validWall), rules.WithDoorSeat(rules.SeatUnknown)},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			if _, err := rules.NewHandEngine(tt.cfg, tt.opts...); !errors.Is(err, rules.ErrInvalidHandConfig) {
				t.Errorf("NewHandEngine() err = %v, want ErrInvalidHandConfig", err)
			}
		})
	}
}

func TestHandEngine_Start_DoorSeatWindsAndDealIsolation(t *testing.T) {
	t.Parallel()

	hands, extra := nonWinningHandsNoFlowers()
	wall := buildDeterministicWall(t, rules.Seat1, hands, extra, nil)

	eng, err := rules.NewHandEngine(
		rules.HandConfig{
			RoundWind:    rules.WindEast,
			Dealer:       rules.Seat1,
			DealerStreak: 2,
			Balances:     defaultBalances(),
			Base:         rules.DefaultBase,
			PerFan:       rules.DefaultPerFan,
		},
		rules.WithWall(wall),
		rules.WithDoorSeat(rules.Seat3),
	)
	if err != nil {
		t.Fatalf("NewHandEngine: %v", err)
	}
	if eng.Phase() != rules.HandPhaseNotStarted {
		t.Fatalf("initial Phase() = %v, want HandPhaseNotStarted", eng.Phase())
	}

	events, err := eng.Start()
	if err != nil {
		t.Fatalf("Start: %v", err)
	}
	if eng.Phase() != rules.HandPhasePlaying {
		t.Fatalf("Phase() after Start = %v, want HandPhasePlaying", eng.Phase())
	}
	if eng.TurnPhase() != rules.PhaseWaitDiscard {
		t.Fatalf("TurnPhase() = %v, want PhaseWaitDiscard (dealer has no self options)", eng.TurnPhase())
	}

	// Double Start must be rejected.
	if _, err := eng.Start(); !errors.Is(err, rules.ErrInvalidPhase) {
		t.Fatalf("second Start() err = %v, want ErrInvalidPhase", err)
	}

	// Event 0: HandStartedEvent with Seat3 East, Seat4 South, Seat1 West, Seat2 North.
	started, ok := events[0].(rules.HandStartedEvent)
	if !ok {
		t.Fatalf("events[0] = %T, want HandStartedEvent", events[0])
	}
	if started.DoorSeat != rules.Seat3 || started.Dealer != rules.Seat1 || started.DealerStreak != 2 {
		t.Fatalf("HandStartedEvent = %+v, unexpected metadata", started)
	}
	wantWinds := map[rules.Seat]rules.Wind{
		rules.Seat3: rules.WindEast,
		rules.Seat4: rules.WindSouth,
		rules.Seat1: rules.WindWest,
		rules.Seat2: rules.WindNorth,
	}
	for seat, wantWind := range wantWinds {
		if got := started.SeatWinds[seat]; got != wantWind {
			t.Errorf("SeatWinds[%v] = %v, want %v", seat, got, wantWind)
		}
	}

	// Event 1: HandDealtPublicEvent with counts only and DrawableRemaining == 63.
	dealtPub, ok := events[1].(rules.HandDealtPublicEvent)
	if !ok {
		t.Fatalf("events[1] = %T, want HandDealtPublicEvent", events[1])
	}
	if dealtPub.DrawableRemaining != 63 {
		t.Errorf("DrawableRemaining = %d, want 63", dealtPub.DrawableRemaining)
	}
	if dealtPub.HandCounts[rules.Seat1] != 17 || dealtPub.HandCounts[rules.Seat2] != 16 ||
		dealtPub.HandCounts[rules.Seat3] != 16 || dealtPub.HandCounts[rules.Seat4] != 16 {
		t.Errorf("HandCounts = %+v, want Seat1=17 others=16", dealtPub.HandCounts)
	}

	// Events 2..5: Four private HandDealtEvent, one per seat with exact hand length.
	seenPrivate := make(map[rules.Seat]int)
	for _, ev := range events[2:6] {
		dealt, ok := ev.(rules.HandDealtEvent)
		if !ok {
			t.Fatalf("expected HandDealtEvent, got %T", ev)
		}
		if dealt.Audience() != rules.AudiencePrivate || dealt.TargetSeat() != dealt.Seat {
			t.Fatalf("invalid private event routing: %+v", dealt)
		}
		seenPrivate[dealt.Seat] = len(dealt.Tiles)
	}
	if seenPrivate[rules.Seat1] != 17 || seenPrivate[rules.Seat2] != 16 ||
		seenPrivate[rules.Seat3] != 16 || seenPrivate[rules.Seat4] != 16 {
		t.Errorf("private dealt lengths = %+v, want Seat1=17 others=16", seenPrivate)
	}
}

func TestHandEngine_Start_OpeningFlowersChainedAndOrder(t *testing.T) {
	t.Parallel()

	hands, extra := nonWinningHandsNoFlowers()
	// Give Dealer (Seat2) Spring, and Seat3 Summer; supplement for Seat2's Spring is Chrysanthemum -> then Wan6.
	// Wait: make sure Wan6 isn't already used 4 times; buildDeterministicWall checks counts.
	hands[rules.Seat2][0] = rules.FlowerSpring
	hands[rules.Seat3][0] = rules.FlowerSummer

	wall := buildDeterministicWall(t, rules.Seat2, hands, extra, []rules.Tile{
		rules.FlowerChrysanthemum, // Seat2 supplement for Spring -> chains!
		rules.Wan6,                // Seat2 supplement for Chrysanthemum -> non-flower
		rules.Suo6,                // Seat3 supplement for Summer -> non-flower
	})

	eng, err := rules.NewHandEngine(
		rules.HandConfig{
			RoundWind: rules.WindEast,
			Dealer:    rules.Seat2,
			Balances:  defaultBalances(),
			Base:      rules.DefaultBase,
			PerFan:    rules.DefaultPerFan,
		},
		rules.WithWall(wall),
		rules.WithDoorSeat(rules.Seat1),
	)
	if err != nil {
		t.Fatalf("NewHandEngine: %v", err)
	}

	events, err := eng.Start()
	if err != nil {
		t.Fatalf("Start: %v", err)
	}

	var exposed []rules.FlowerExposedEvent
	var privDraws []rules.TileDrawnEvent
	for _, ev := range events {
		switch e := ev.(type) {
		case rules.FlowerExposedEvent:
			exposed = append(exposed, e)
		case rules.TileDrawnEvent:
			privDraws = append(privDraws, e)
		case rules.ClaimPromptEvent:
			t.Fatalf("unexpected ClaimPromptEvent during opening flower replacement: %+v", e)
		}
	}

	if len(exposed) != 3 {
		t.Fatalf("exposed flower count = %d, want 3 (%+v)", len(exposed), exposed)
	}
	if exposed[0].Seat != rules.Seat2 || exposed[0].Flower != rules.FlowerSpring ||
		exposed[1].Seat != rules.Seat2 || exposed[1].Flower != rules.FlowerChrysanthemum ||
		exposed[2].Seat != rules.Seat3 || exposed[2].Flower != rules.FlowerSummer {
		t.Fatalf("exposed sequence = %+v, want Seat2(Spring), Seat2(Chrysanthemum), Seat3(Summer)", exposed)
	}
	if len(privDraws) != 3 ||
		privDraws[0].Seat != rules.Seat2 || privDraws[0].Tile != rules.FlowerChrysanthemum ||
		privDraws[1].Seat != rules.Seat2 || privDraws[1].Tile != rules.Wan6 ||
		privDraws[2].Seat != rules.Seat3 || privDraws[2].Tile != rules.Suo6 {
		t.Fatalf("private supplement sequence = %+v", privDraws)
	}
	if eng.DrawableRemaining() != 60 {
		t.Errorf("DrawableRemaining = %d, want 60 (63 - 3 supplements)", eng.DrawableRemaining())
	}
}

func TestHandEngine_Start_OpeningEightFlowersOpensClaimWindowAfterAllReplaced(t *testing.T) {
	t.Parallel()

	hands, extra := nonWinningHandsNoFlowers()
	allFlowers := []rules.Tile{
		rules.FlowerSpring, rules.FlowerSummer, rules.FlowerAutumn, rules.FlowerWinter,
		rules.FlowerPlum, rules.FlowerOrchid, rules.FlowerChrysanthemum, rules.FlowerBamboo,
	}
	// Put all 8 flowers into Seat2's initial hand so Seat2 collects all 8 during opening replacement.
	copy(hands[rules.Seat2], allFlowers)
	wall := buildDeterministicWall(t, rules.Seat1, hands, extra, nil)

	eng, err := rules.NewHandEngine(
		rules.HandConfig{
			RoundWind: rules.WindEast,
			Dealer:    rules.Seat1,
			Balances:  defaultBalances(),
			Base:      rules.DefaultBase,
			PerFan:    rules.DefaultPerFan,
		},
		rules.WithWall(wall),
		rules.WithDoorSeat(rules.Seat1),
	)
	if err != nil {
		t.Fatalf("NewHandEngine: %v", err)
	}

	events, err := eng.Start()
	if err != nil {
		t.Fatalf("Start: %v", err)
	}

	if eng.TurnPhase() != rules.PhaseWaitClaims {
		t.Fatalf("TurnPhase() = %v, want PhaseWaitClaims for Eight Immortals", eng.TurnPhase())
	}

	var flowerPrompts []rules.ClaimPromptEvent
	for _, ev := range events {
		if cp, ok := ev.(rules.ClaimPromptEvent); ok {
			flowerPrompts = append(flowerPrompts, cp)
		}
	}
	if len(flowerPrompts) != 1 || flowerPrompts[0].Seat != rules.Seat2 || flowerPrompts[0].FlowerWinKind != rules.WinEightImmortals {
		t.Fatalf("flower prompts = %+v, want single Seat2 WinEightImmortals prompt", flowerPrompts)
	}
}

func TestHandEngine_Start_DealerFirstHandTianHu(t *testing.T) {
	t.Parallel()

	hands, _ := nonWinningHandsNoFlowers()
	// Give Dealer (Seat1) a winning 17-tile hand (5 sets + pair).
	hands[rules.Seat1] = []rules.Tile{
		rules.Wan1, rules.Wan2, rules.Wan3,
		rules.Wan4, rules.Wan5, rules.Wan6,
		rules.Suo1, rules.Suo2, rules.Suo3,
		rules.Tong1, rules.Tong2, rules.Tong3,
		rules.Tong7, rules.Tong8, rules.Tong9,
		rules.Wan9,
	}
	extra := rules.Wan9

	wall := buildDeterministicWall(t, rules.Seat1, hands, extra, nil)
	eng, err := rules.NewHandEngine(
		rules.HandConfig{
			RoundWind: rules.WindEast,
			Dealer:    rules.Seat1,
			Balances:  defaultBalances(),
			Base:      rules.DefaultBase,
			PerFan:    rules.DefaultPerFan,
		},
		rules.WithWall(wall),
		rules.WithDoorSeat(rules.Seat1),
	)
	if err != nil {
		t.Fatalf("NewHandEngine: %v", err)
	}

	events, err := eng.Start()
	if err != nil {
		t.Fatalf("Start: %v", err)
	}
	if eng.TurnPhase() != rules.PhaseWaitSelfDeclare {
		t.Fatalf("TurnPhase() = %v, want PhaseWaitSelfDeclare", eng.TurnPhase())
	}

	idx := slices.IndexFunc(events, func(ev rules.Event) bool {
		sp, ok := ev.(rules.SelfDeclarePromptEvent)
		return ok && sp.Seat == rules.Seat1 && sp.CanSelfWin
	})
	if idx < 0 {
		t.Fatalf("expected SelfDeclarePromptEvent with CanSelfWin=true for dealer, got %+v", events)
	}
}

func TestHandEngine_Start_RandomDoorSeatUsesInjectedRNG(t *testing.T) {
	t.Parallel()

	rng := rand.NewPCG(42, 99)
	seenDoors := make(map[rules.Seat]bool)
	for range 20 {
		eng, err := rules.NewHandEngine(rules.HandConfig{
			RoundWind: rules.WindEast,
			Dealer:    rules.Seat1,
			Balances:  defaultBalances(),
			Base:      rules.DefaultBase,
			PerFan:    rules.DefaultPerFan,
			Rand:      rng,
		})
		if err != nil {
			t.Fatalf("NewHandEngine: %v", err)
		}
		events, err := eng.Start()
		if err != nil {
			t.Fatalf("Start: %v", err)
		}
		started, ok := events[0].(rules.HandStartedEvent)
		if !ok {
			t.Fatalf("events[0] = %T, want HandStartedEvent", events[0])
		}
		if !started.DoorSeat.Valid() {
			t.Fatalf("invalid DoorSeat %v", started.DoorSeat)
		}
		seenDoors[started.DoorSeat] = true
	}
	if len(seenDoors) < 2 {
		t.Fatalf("expected random door seat variation across 20 hands, only saw %+v", seenDoors)
	}
}

func TestHandEngine_Apply_WinRevealsSettlesAndWaitsForAllReady(t *testing.T) {
	t.Parallel()

	hands, _ := nonWinningHandsNoFlowers()
	// Dealer Seat1 has TianHu (self-draw win on opening 17 tiles).
	hands[rules.Seat1] = []rules.Tile{
		rules.Wan1, rules.Wan2, rules.Wan3,
		rules.Wan4, rules.Wan5, rules.Wan6,
		rules.Suo1, rules.Suo2, rules.Suo3,
		rules.Tong1, rules.Tong2, rules.Tong3,
		rules.Tong7, rules.Tong8, rules.Tong9,
		rules.Wan9,
	}
	wall := buildDeterministicWall(t, rules.Seat1, hands, rules.Wan9, nil)

	eng, err := rules.NewHandEngine(
		rules.HandConfig{
			RoundWind: rules.WindEast,
			Dealer:    rules.Seat1,
			Balances:  defaultBalances(),
			Base:      rules.DefaultBase,
			PerFan:    rules.DefaultPerFan,
		},
		rules.WithWall(wall),
		rules.WithDoorSeat(rules.Seat1),
	)
	if err != nil {
		t.Fatalf("NewHandEngine: %v", err)
	}
	if _, err := eng.Start(); err != nil {
		t.Fatalf("Start: %v", err)
	}

	events, err := eng.Apply(rules.PlayerAction{
		Seat:        rules.Seat1,
		Kind:        rules.ActionSelfDeclare,
		SelfDeclare: rules.SelfDeclareSelfWin,
	}, 2*time.Second)
	if err != nil {
		t.Fatalf("Apply SelfWin: %v", err)
	}
	if eng.Phase() != rules.HandPhaseWaitReady {
		t.Fatalf("Phase() = %v, want HandPhaseWaitReady", eng.Phase())
	}
	if !eng.Settled() {
		t.Fatal("Settled() = false, want true")
	}

	// Verify event order: HandEndedEvent -> HandsRevealedEvent -> HandSettledEvent -> WaitReadyEvent.
	if len(events) != 4 {
		t.Fatalf("win end events len = %d, want 4 (%+v)", len(events), events)
	}
	if ended, ok := events[0].(rules.HandEndedEvent); !ok || ended.Reason != rules.HandEndWin {
		t.Fatalf("events[0] = %+v, want HandEndedEvent(HandEndWin)", events[0])
	}
	revealed, ok := events[1].(rules.HandsRevealedEvent)
	if !ok || len(revealed.Seats) != 4 {
		t.Fatalf("events[1] = %+v, want HandsRevealedEvent with 4 seats", events[1])
	}
	settled, ok := events[2].(rules.HandSettledEvent)
	if !ok {
		t.Fatalf("events[2] = %T, want HandSettledEvent", events[2])
	}
	if settled.Reason != rules.HandEndWin || settled.Score == nil || !settled.IncrementPlayedCount {
		t.Fatalf("HandSettledEvent = %+v, expected win settlement with IncrementPlayedCount=true", settled)
	}
	if settled.Balances[rules.Seat1].Delta <= 0 || settled.Balances[rules.Seat2].Delta >= 0 {
		t.Fatalf("unexpected settlement deltas: %+v", settled.Balances)
	}
	for _, s := range rules.AllSeats {
		bc := settled.Balances[s]
		if bc.Before != 40000 || bc.After != bc.Before+bc.Delta {
			t.Errorf("seat %v balance change inconsistent: %+v", s, bc)
		}
		if eng.Balances()[s] != bc.After {
			t.Errorf("eng.Balances()[%v] = %d, want %d", s, eng.Balances()[s], bc.After)
		}
	}
	if _, ok := events[3].(rules.WaitReadyEvent); !ok {
		t.Fatalf("events[3] = %T, want WaitReadyEvent", events[3])
	}

	// Non-ready actions must be rejected during WaitReady.
	if _, err := eng.Apply(rules.PlayerAction{Seat: rules.Seat1, Kind: rules.ActionDraw}, 0); !errors.Is(err, rules.ErrInvalidPhase) {
		t.Errorf("ActionDraw in WaitReady err = %v, want ErrInvalidPhase", err)
	}

	// Confirm Seat1, duplicate Seat1 rejected, then confirm Seat2, Seat3, Seat4.
	evs, err := eng.Apply(rules.PlayerAction{Seat: rules.Seat1, Kind: rules.ActionReady}, 0)
	if err != nil {
		t.Fatalf("Seat1 Ready: %v", err)
	}
	if len(evs) != 1 {
		t.Fatalf("ready events = %+v, want 1 PlayerReadyEvent", evs)
	}
	if _, err := eng.Apply(rules.PlayerAction{Seat: rules.Seat1, Kind: rules.ActionReady}, 0); !errors.Is(err, rules.ErrAlreadyReady) {
		t.Errorf("duplicate Ready err = %v, want ErrAlreadyReady", err)
	}
	for _, s := range []rules.Seat{rules.Seat2, rules.Seat3} {
		if _, err := eng.Apply(rules.PlayerAction{Seat: s, Kind: rules.ActionReady}, 0); err != nil {
			t.Fatalf("seat %v Ready: %v", s, err)
		}
		if eng.Phase() != rules.HandPhaseWaitReady {
			t.Fatalf("Phase() after %v ready = %v, want HandPhaseWaitReady until all 4 ready", s, eng.Phase())
		}
	}
	if _, err := eng.Apply(rules.PlayerAction{Seat: rules.Seat4, Kind: rules.ActionReady}, 0); err != nil {
		t.Fatalf("Seat4 Ready: %v", err)
	}
	if eng.Phase() != rules.HandPhaseFinished {
		t.Fatalf("Phase() after all 4 ready = %v, want HandPhaseFinished", eng.Phase())
	}
}

func TestHandEngine_Apply_ExhaustedDrawRevealsConcealedKongAndWaitsAllFour(t *testing.T) {
	t.Parallel()

	hands, _ := nonWinningHandsNoFlowers()
	// Give Dealer Seat1 four EastWinds so Seat1 declares a concealed kong on turn 1,
	// then drive all remaining drawable tiles to exhaustion.
	hands[rules.Seat1] = []rules.Tile{
		rules.EastWind, rules.EastWind, rules.EastWind, rules.EastWind,
		rules.Wan1, rules.Wan4, rules.Wan7,
		rules.Suo1, rules.Suo4, rules.Suo7,
		rules.Tong1, rules.Tong4, rules.Tong7,
		rules.Wan9, rules.Suo9, rules.Tong9,
	}
	hands[rules.Seat2] = []rules.Tile{
		rules.Wan2, rules.Wan5, rules.Wan8,
		rules.Suo2, rules.Suo5, rules.Suo8,
		rules.Tong2, rules.Tong5, rules.Tong8,
		rules.SouthWind, rules.WestWind, rules.NorthWind,
		rules.RedDragon, rules.WhiteDragon, rules.GreenDragon, rules.Wan1,
	}
	hands[rules.Seat3] = []rules.Tile{
		rules.Wan3, rules.Wan6, rules.Wan9,
		rules.Suo3, rules.Suo6, rules.Suo9,
		rules.Tong3, rules.Tong6, rules.Tong9,
		rules.SouthWind, rules.WestWind, rules.NorthWind,
		rules.RedDragon, rules.WhiteDragon, rules.GreenDragon, rules.Suo1,
	}
	hands[rules.Seat4] = []rules.Tile{
		rules.Wan2, rules.Wan5, rules.Wan8,
		rules.Suo2, rules.Suo5, rules.Suo8,
		rules.Tong2, rules.Tong5, rules.Tong8,
		rules.SouthWind, rules.WestWind, rules.NorthWind,
		rules.RedDragon, rules.WhiteDragon, rules.GreenDragon, rules.Tong1,
	}
	// Make sure all remaining wall tiles are drawn cleanly without winning.
	wall := buildDeterministicWall(t, rules.Seat1, hands, rules.Wan3, nil)

	eng, err := rules.NewHandEngine(
		rules.HandConfig{
			RoundWind: rules.WindEast,
			Dealer:    rules.Seat1,
			Balances:  defaultBalances(),
			Base:      rules.DefaultBase,
			PerFan:    rules.DefaultPerFan,
		},
		rules.WithWall(wall),
		rules.WithDoorSeat(rules.Seat1),
	)
	if err != nil {
		t.Fatalf("NewHandEngine: %v", err)
	}
	startEvs, err := eng.Start()
	if err != nil {
		t.Fatalf("Start: %v", err)
	}
	// Track private hands so test helper always discards a tile owned by the current turn seat.
	privateHands := make(map[rules.Seat][]rules.Tile, 4)
	trackEvents := func(evs []rules.Event) {
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
			}
		}
	}
	trackEvents(startEvs)

	// Step 1: Seat1 declares concealed kong of EastWind.
	kongEvs, err := eng.Apply(rules.PlayerAction{
		Seat:        rules.Seat1,
		Kind:        rules.ActionSelfDeclare,
		SelfDeclare: rules.SelfDeclareConcealedKong,
		Tile:        rules.EastWind,
	}, 0)
	if err != nil {
		t.Fatalf("Seat1 concealed kong: %v", err)
	}
	for range 4 {
		idx := slices.Index(privateHands[rules.Seat1], rules.EastWind)
		privateHands[rules.Seat1] = slices.Delete(privateHands[rules.Seat1], idx, idx+1)
	}
	// Verify concealed kong faces were hidden at declaration time.
	foundMasked := false
	for _, ev := range kongEvs {
		if md, ok := ev.(rules.MeldDeclaredEvent); ok && md.Meld.Kind == rules.MeldConcealedKong && len(md.Meld.Tiles) == 0 {
			foundMasked = true
		}
	}
	if !foundMasked {
		t.Fatalf("expected masked MeldDeclaredEvent on concealed kong, got %+v", kongEvs)
	}

	// Drive to exhaustion by drawing, passing all self/claim prompts, and discarding the newest tile.
	var lastEvs []rules.Event
	pendingClaims := make(map[rules.Seat]bool)
	for guard := 0; eng.Phase() == rules.HandPhasePlaying && guard < 400; guard++ {
		var evs []rules.Event
		switch eng.TurnPhase() {
		case rules.PhaseWaitDraw:
			evs, err = eng.Apply(rules.PlayerAction{Seat: eng.CurrentTurn(), Kind: rules.ActionDraw}, 0)
		case rules.PhaseWaitSelfDeclare:
			evs, err = eng.Apply(rules.PlayerAction{Seat: eng.CurrentTurn(), Kind: rules.ActionSelfDeclare, SelfDeclare: rules.SelfDeclarePass}, 0)
		case rules.PhaseWaitDiscard:
			seat := eng.CurrentTurn()
			h := privateHands[seat]
			tile := h[len(h)-1]
			evs, err = eng.Apply(rules.PlayerAction{Seat: seat, Kind: rules.ActionDiscard, Tile: tile}, time.Second)
		case rules.PhaseWaitClaims:
			var nextSeat rules.Seat
			for _, s := range rules.AllSeats {
				if pendingClaims[s] {
					nextSeat = s
					delete(pendingClaims, s)
					break
				}
			}
			evs, err = eng.Apply(rules.PlayerAction{Seat: nextSeat, Kind: rules.ActionClaim, Claim: rules.ClaimPass}, 0)
		default:
			t.Fatalf("unexpected TurnPhase %v", eng.TurnPhase())
		}
		if err != nil {
			t.Fatalf("step %d (turnPhase=%v): %v", guard, eng.TurnPhase(), err)
		}
		trackEvents(evs)
		for _, ev := range evs {
			if cp, ok := ev.(rules.ClaimPromptEvent); ok {
				pendingClaims[cp.Seat] = true
			}
		}
		lastEvs = evs
	}

	if eng.Phase() != rules.HandPhaseWaitReady {
		t.Fatalf("Phase() = %v, want HandPhaseWaitReady after exhaustion", eng.Phase())
	}
	reason, score := eng.HandResult()
	if reason != rules.HandEndExhausted || score != nil {
		t.Fatalf("HandResult = (%v, %+v), want (HandEndExhausted, nil)", reason, score)
	}

	// Verify HandsRevealedEvent exposes Seat1's concealed kong faces (4x EastWind)
	// and HandSettledEvent has zero deltas and IncrementPlayedCount=false.
	var revealed *rules.HandsRevealedEvent
	var settled *rules.HandSettledEvent
	for _, ev := range lastEvs {
		switch e := ev.(type) {
		case rules.HandsRevealedEvent:
			revealed = &e
		case rules.HandSettledEvent:
			settled = &e
		}
	}
	if revealed == nil {
		t.Fatal("missing HandsRevealedEvent on exhausted draw")
	}
	s1Melds := revealed.Seats[rules.Seat1].Melds
	if len(s1Melds) != 1 || s1Melds[0].Kind != rules.MeldConcealedKong || len(s1Melds[0].Tiles) != 4 || s1Melds[0].Tiles[0] != rules.EastWind {
		t.Fatalf("Seat1 revealed melds = %+v, want unmasked 4x EastWind concealed kong", s1Melds)
	}
	if settled == nil || settled.Reason != rules.HandEndExhausted || settled.IncrementPlayedCount {
		t.Fatalf("settled = %+v, want HandEndExhausted with IncrementPlayedCount=false", settled)
	}
	for _, s := range rules.AllSeats {
		if settled.Balances[s].Delta != 0 || settled.Balances[s].After != 40000 {
			t.Errorf("exhausted draw changed seat %v balance: %+v", s, settled.Balances[s])
		}
	}

	// Exhausted draw must ALSO wait for all four seats (not only host Seat1).
	if _, err := eng.Apply(rules.PlayerAction{Seat: rules.Seat1, Kind: rules.ActionReady}, 0); err != nil {
		t.Fatalf("Seat1 Ready: %v", err)
	}
	if eng.Phase() != rules.HandPhaseWaitReady {
		t.Fatalf("Phase() after only host ready on draw = %v, want HandPhaseWaitReady", eng.Phase())
	}
	for _, s := range []rules.Seat{rules.Seat2, rules.Seat3, rules.Seat4} {
		if _, err := eng.Apply(rules.PlayerAction{Seat: s, Kind: rules.ActionReady}, 0); err != nil {
			t.Fatalf("seat %v Ready: %v", s, err)
		}
	}
	if eng.Phase() != rules.HandPhaseFinished {
		t.Fatalf("Phase() after all 4 ready = %v, want HandPhaseFinished", eng.Phase())
	}
}

func TestHandEngine_Abort_PlayingVsWaitReady(t *testing.T) {
	t.Parallel()

	t.Run("abort during playing emits no reveal or settlement", func(t *testing.T) {
		t.Parallel()

		hands, extra := nonWinningHandsNoFlowers()
		wall := buildDeterministicWall(t, rules.Seat1, hands, extra, nil)
		eng, err := rules.NewHandEngine(
			rules.HandConfig{
				RoundWind: rules.WindEast,
				Dealer:    rules.Seat1,
				Balances:  defaultBalances(),
				Base:      rules.DefaultBase,
				PerFan:    rules.DefaultPerFan,
			},
			rules.WithWall(wall),
			rules.WithDoorSeat(rules.Seat1),
		)
		if err != nil {
			t.Fatalf("NewHandEngine: %v", err)
		}
		if _, err := eng.Start(); err != nil {
			t.Fatalf("Start: %v", err)
		}

		evs, err := eng.Abort(rules.Seat3)
		if err != nil {
			t.Fatalf("Abort: %v", err)
		}
		if eng.Phase() != rules.HandPhaseAborted {
			t.Fatalf("Phase() = %v, want HandPhaseAborted", eng.Phase())
		}
		if eng.Settled() {
			t.Fatal("Settled() = true after playing abort, want false")
		}
		for _, ev := range evs {
			switch ev.(type) {
			case rules.HandsRevealedEvent, rules.HandSettledEvent:
				t.Fatalf("unexpected reveal/settlement event on abort: %T", ev)
			}
		}
		if _, err := eng.Apply(rules.PlayerAction{Seat: rules.Seat1, Kind: rules.ActionDiscard, Tile: rules.Wan1}, 0); !errors.Is(err, rules.ErrInvalidPhase) {
			t.Errorf("Apply after Abort err = %v, want ErrInvalidPhase", err)
		}
	})

	t.Run("abort during WaitReady preserves settled balances", func(t *testing.T) {
		t.Parallel()

		hands, _ := nonWinningHandsNoFlowers()
		hands[rules.Seat1] = []rules.Tile{
			rules.Wan1, rules.Wan2, rules.Wan3,
			rules.Wan4, rules.Wan5, rules.Wan6,
			rules.Suo1, rules.Suo2, rules.Suo3,
			rules.Tong1, rules.Tong2, rules.Tong3,
			rules.Tong7, rules.Tong8, rules.Tong9,
			rules.Wan9,
		}
		wall := buildDeterministicWall(t, rules.Seat1, hands, rules.Wan9, nil)
		eng, err := rules.NewHandEngine(
			rules.HandConfig{
				RoundWind: rules.WindEast,
				Dealer:    rules.Seat1,
				Balances:  defaultBalances(),
				Base:      rules.DefaultBase,
				PerFan:    rules.DefaultPerFan,
			},
			rules.WithWall(wall),
			rules.WithDoorSeat(rules.Seat1),
		)
		if err != nil {
			t.Fatalf("NewHandEngine: %v", err)
		}
		if _, err := eng.Start(); err != nil {
			t.Fatalf("Start: %v", err)
		}
		if _, err := eng.Apply(rules.PlayerAction{
			Seat:        rules.Seat1,
			Kind:        rules.ActionSelfDeclare,
			SelfDeclare: rules.SelfDeclareSelfWin,
		}, 0); err != nil {
			t.Fatalf("Apply SelfWin: %v", err)
		}
		afterWin := eng.Balances()

		if _, err := eng.Abort(rules.Seat2); err != nil {
			t.Fatalf("Abort in WaitReady: %v", err)
		}
		if eng.Phase() != rules.HandPhaseAborted || !eng.Settled() {
			t.Fatalf("Phase=%v Settled=%v, want Aborted & Settled=true", eng.Phase(), eng.Settled())
		}
		for _, s := range rules.AllSeats {
			if eng.Balances()[s] != afterWin[s] {
				t.Errorf("balance for %v rolled back: got %d, want %d", s, eng.Balances()[s], afterWin[s])
			}
		}
	})
}

func TestHandEngine_SpecialDraws_RevealZeroDeltaAndWaitReady(t *testing.T) {
	t.Parallel()

	t.Run("nine terminals draw reveals hands with zero delta and requires all 4 ready", func(t *testing.T) {
		t.Parallel()

		hands, extra := nonWinningHandsNoFlowers()
		// Seat1 already has 11 distinct terminals/honors in nonWinningHandsNoFlowers():
		// Wan1, Wan9, Suo1, Suo9, Tong1, Tong9, EastWind -> add 2 more honors so >= 9 distinct types.
		hands[rules.Seat1][2] = rules.SouthWind
		hands[rules.Seat1][3] = rules.WestWind
		hands[rules.Seat1][7] = rules.NorthWind
		wall := buildDeterministicWall(t, rules.Seat1, hands, extra, nil)

		eng, err := rules.NewHandEngine(
			rules.HandConfig{
				RoundWind: rules.WindEast,
				Dealer:    rules.Seat1,
				Balances:  defaultBalances(),
				Base:      rules.DefaultBase,
				PerFan:    rules.DefaultPerFan,
			},
			rules.WithWall(wall),
			rules.WithDoorSeat(rules.Seat1),
		)
		if err != nil {
			t.Fatalf("NewHandEngine: %v", err)
		}
		if _, err := eng.Start(); err != nil {
			t.Fatalf("Start: %v", err)
		}

		evs, err := eng.Apply(rules.PlayerAction{
			Seat:        rules.Seat1,
			Kind:        rules.ActionSelfDeclare,
			SelfDeclare: rules.SelfDeclareNineTerminals,
		}, time.Second)
		if err != nil {
			t.Fatalf("SelfDeclareNineTerminals: %v", err)
		}
		if eng.Phase() != rules.HandPhaseWaitReady || !eng.Settled() {
			t.Fatalf("Phase=%v Settled=%v, want WaitReady & true", eng.Phase(), eng.Settled())
		}
		reason, score := eng.HandResult()
		if reason != rules.HandEndNineTerminals || score != nil {
			t.Fatalf("HandResult() = (%v, %+v), want (HandEndNineTerminals, nil)", reason, score)
		}

		var (
			foundReveal  bool
			foundSettled bool
			foundWait    bool
		)
		for _, ev := range evs {
			switch e := ev.(type) {
			case rules.HandsRevealedEvent:
				foundReveal = true
				for _, s := range rules.AllSeats {
					if len(e.Seats[s].Concealed) == 0 {
						t.Errorf("HandsRevealedEvent missing concealed hand for %v", s)
					}
				}
			case rules.HandSettledEvent:
				foundSettled = true
				if e.Reason != rules.HandEndNineTerminals || e.Score != nil || e.IncrementPlayedCount {
					t.Fatalf("unexpected HandSettledEvent: %+v", e)
				}
				for _, s := range rules.AllSeats {
					if e.Balances[s].Delta != 0 || e.Balances[s].After != 40000 {
						t.Errorf("seat %v balance change = %+v, want delta=0 after=40000", s, e.Balances[s])
					}
				}
			case rules.WaitReadyEvent:
				foundWait = true
			}
		}
		if !foundReveal || !foundSettled || !foundWait {
			t.Fatalf("missing lifecycle events: reveal=%v settled=%v wait=%v", foundReveal, foundSettled, foundWait)
		}

		// First 3 seats ready keeps WaitReady; duplicate ready & gameplay actions rejected.
		for _, s := range []rules.Seat{rules.Seat1, rules.Seat2, rules.Seat3} {
			if _, err := eng.Apply(rules.PlayerAction{Seat: s, Kind: rules.ActionReady}, 0); err != nil {
				t.Fatalf("ActionReady(%v): %v", s, err)
			}
		}
		if eng.Phase() != rules.HandPhaseWaitReady {
			t.Fatalf("Phase after 3 ready = %v, want HandPhaseWaitReady", eng.Phase())
		}
		if _, err := eng.Apply(rules.PlayerAction{Seat: rules.Seat1, Kind: rules.ActionReady}, 0); !errors.Is(err, rules.ErrAlreadyReady) {
			t.Fatalf("duplicate ready err = %v, want ErrAlreadyReady", err)
		}
		if _, err := eng.Apply(rules.PlayerAction{Seat: rules.Seat4, Kind: rules.ActionDraw}, 0); !errors.Is(err, rules.ErrInvalidPhase) {
			t.Fatalf("draw in WaitReady err = %v, want ErrInvalidPhase", err)
		}
		if _, err := eng.Apply(rules.PlayerAction{Seat: rules.Seat4, Kind: rules.ActionReady}, 0); err != nil {
			t.Fatalf("ActionReady(Seat4): %v", err)
		}
		if eng.Phase() != rules.HandPhaseFinished {
			t.Fatalf("Phase after 4 ready = %v, want HandPhaseFinished", eng.Phase())
		}
	})

	t.Run("four kongs draw reveals concealed kong tiles with zero delta", func(t *testing.T) {
		t.Parallel()

		hands := map[rules.Seat][]rules.Tile{
			rules.Seat1: {
				rules.Wan1, rules.Wan1, rules.Wan1, rules.Wan1,
				rules.Wan5, rules.Wan5, rules.Wan5, rules.Wan5,
				rules.Wan9, rules.Wan9, rules.Wan9, rules.Wan9,
				rules.Tong2, rules.Tong5, rules.Tong8, rules.EastWind,
			},
			rules.Seat2: {
				rules.Suo1, rules.Suo1, rules.Suo1, rules.Suo1,
				rules.Suo3, rules.Suo5, rules.Suo7, rules.Suo9,
				rules.Tong1, rules.Tong4, rules.Tong7, rules.Tong9,
				rules.SouthWind, rules.WestWind, rules.NorthWind, rules.RedDragon,
			},
			rules.Seat3: {
				rules.Wan2, rules.Wan4, rules.Wan6, rules.Wan8,
				rules.Suo2, rules.Suo4, rules.Suo6, rules.Suo8,
				rules.Tong2, rules.Tong4, rules.Tong6, rules.Tong8,
				rules.EastWind, rules.SouthWind, rules.WhiteDragon, rules.GreenDragon,
			},
			rules.Seat4: {
				rules.Wan2, rules.Wan4, rules.Wan6, rules.Wan8,
				rules.Suo2, rules.Suo4, rules.Suo6, rules.Suo8,
				rules.Tong2, rules.Tong4, rules.Tong6, rules.Tong8,
				rules.WestWind, rules.NorthWind, rules.RedDragon, rules.WhiteDragon,
			},
		}
		wall := buildDeterministicWall(t, rules.Seat1, hands, rules.GreenDragon, []rules.Tile{
			rules.Wan3,        // Seat1 kong #1 supplement
			rules.Wan7,        // Seat1 kong #2 supplement
			rules.Suo3,        // Seat1 kong #3 supplement
			rules.Tong3,       // Seat2 turn draw
			rules.GreenDragon, // Seat2 kong #4 supplement
		})

		eng, err := rules.NewHandEngine(
			rules.HandConfig{
				RoundWind: rules.WindEast,
				Dealer:    rules.Seat1,
				Balances:  defaultBalances(),
				Base:      rules.DefaultBase,
				PerFan:    rules.DefaultPerFan,
			},
			rules.WithWall(wall),
			rules.WithDoorSeat(rules.Seat1),
		)
		if err != nil {
			t.Fatalf("NewHandEngine: %v", err)
		}
		if _, err := eng.Start(); err != nil {
			t.Fatalf("Start: %v", err)
		}

		// Seat1 declares 3 concealed kongs and discards GreenDragon.
		for _, kTile := range []rules.Tile{rules.Wan1, rules.Wan5, rules.Wan9} {
			if _, err := eng.Apply(rules.PlayerAction{
				Seat:        rules.Seat1,
				Kind:        rules.ActionSelfDeclare,
				SelfDeclare: rules.SelfDeclareConcealedKong,
				Tile:        kTile,
			}, 0); err != nil {
				t.Fatalf("Seat1 concealed kong %v: %v", kTile, err)
			}
			if _, err := eng.Apply(rules.PlayerAction{Seat: rules.Seat1, Kind: rules.ActionDraw}, 0); err != nil {
				t.Fatalf("Seat1 supplement draw: %v", err)
			}
		}
		if _, err := eng.Apply(rules.PlayerAction{Seat: rules.Seat1, Kind: rules.ActionDiscard, Tile: rules.GreenDragon}, 0); err != nil {
			t.Fatalf("Seat1 discard: %v", err)
		}

		// Seat2 draws, declares 4th concealed kong (Suo1), supplements, and discards safe GreenDragon -> HandEndFourKongs!
		if _, err := eng.Apply(rules.PlayerAction{Seat: rules.Seat2, Kind: rules.ActionDraw}, 0); err != nil {
			t.Fatalf("Seat2 draw: %v", err)
		}
		if _, err := eng.Apply(rules.PlayerAction{
			Seat:        rules.Seat2,
			Kind:        rules.ActionSelfDeclare,
			SelfDeclare: rules.SelfDeclareConcealedKong,
			Tile:        rules.Suo1,
		}, 0); err != nil {
			t.Fatalf("Seat2 concealed kong Suo1: %v", err)
		}
		if _, err := eng.Apply(rules.PlayerAction{Seat: rules.Seat2, Kind: rules.ActionDraw}, 0); err != nil {
			t.Fatalf("Seat2 supplement draw: %v", err)
		}
		evs, err := eng.Apply(rules.PlayerAction{Seat: rules.Seat2, Kind: rules.ActionDiscard, Tile: rules.GreenDragon}, 0)
		if err != nil {
			t.Fatalf("Seat2 safe discard: %v", err)
		}

		if eng.Phase() != rules.HandPhaseWaitReady {
			t.Fatalf("Phase = %v, want HandPhaseWaitReady", eng.Phase())
		}
		var revealEv *rules.HandsRevealedEvent
		var settledEv *rules.HandSettledEvent
		for _, ev := range evs {
			switch e := ev.(type) {
			case rules.HandsRevealedEvent:
				cp := e
				revealEv = &cp
			case rules.HandSettledEvent:
				cp := e
				settledEv = &cp
			}
		}
		if revealEv == nil || settledEv == nil {
			t.Fatalf("expected HandsRevealedEvent & HandSettledEvent, got %+v", evs)
		}
		if settledEv.Reason != rules.HandEndFourKongs || settledEv.IncrementPlayedCount || settledEv.Score != nil {
			t.Fatalf("unexpected HandSettledEvent: %+v", settledEv)
		}
		if len(revealEv.Seats[rules.Seat1].Melds) != 3 || len(revealEv.Seats[rules.Seat2].Melds) != 1 {
			t.Fatalf("revealed concealed kongs mismatch: seat1=%+v seat2=%+v", revealEv.Seats[rules.Seat1].Melds, revealEv.Seats[rules.Seat2].Melds)
		}
		m2 := revealEv.Seats[rules.Seat2].Melds[0]
		if m2.Kind != rules.MeldConcealedKong || len(m2.Tiles) != 4 || m2.Tiles[0] != rules.Suo1 {
			t.Fatalf("expected Seat2 concealed kong Suo1 revealed with 4 tiles, got %+v", m2)
		}
	})
}
