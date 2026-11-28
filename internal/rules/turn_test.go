package rules_test

import (
	"errors"
	"slices"
	"testing"
	"time"

	"github.com/gjchentw/go-qkmj/internal/rules"
)

// makeTurnTestWall builds a valid 144-tile wall whose initial drawable sequence starts with frontTiles.
// It removes hands + flowers + frontTiles from a canonical deck, then pads the tail so total == 144.
// Wait: Wall only needs the remaining undrawn tiles (NewWallFromTiles requires 144 tiles, so we place
// dummy dealt tiles at the front and draw them before handing the wall to TurnEngine!).
func makePreparedWall(t *testing.T, hands, flowers map[rules.Seat][]rules.Tile, nextDraws []rules.Tile) *rules.Wall {
	t.Helper()

	var used [rules.TileMax + 1]int
	var dealt []rules.Tile
	for _, s := range rules.AllSeats {
		for _, tile := range hands[s] {
			used[tile]++
			dealt = append(dealt, tile)
		}
		for _, fl := range flowers[s] {
			used[fl]++
			dealt = append(dealt, fl)
		}
	}
	for _, tile := range nextDraws {
		used[tile]++
	}

	var rest []rules.Tile
	for tile := rules.Wan1; tile <= rules.TileMax; tile++ {
		total := 4
		if tile.IsFlower() {
			total = 1
		}
		if used[tile] > total {
			t.Fatalf("tile %s used %d times (max %d)", tile, used[tile], total)
		}
		for range total - used[tile] {
			rest = append(rest, tile)
		}
	}

	full := make([]rules.Tile, 0, rules.TotalDeckTiles)
	full = append(full, dealt...)
	full = append(full, nextDraws...)
	full = append(full, rest...)

	w, err := rules.NewWallFromTiles(full)
	if err != nil {
		t.Fatalf("NewWallFromTiles err = %v", err)
	}
	for range len(dealt) {
		if _, err := w.Draw(); err != nil {
			t.Fatalf("advance dealt tile err = %v", err)
		}
	}
	return w
}

func defaultTurnHands(t *testing.T) map[rules.Seat][]rules.Tile {
	t.Helper()
	return map[rules.Seat][]rules.Tile{
		rules.Seat1: mustTiles(t, "一萬 二萬 三萬 四萬 五萬 六萬 七索 八索 九索 二筒 三筒 四筒 六筒 七筒 東風 南風 西風"),
		rules.Seat2: mustTiles(t, "一萬 二萬 三萬 四萬 五萬 六萬 一索 二索 三索 四索 五索 六索 一筒 二筒 三筒 北風"),
		rules.Seat3: mustTiles(t, "七萬 八萬 九萬 七萬 八萬 九萬 四索 五索 六索 七索 八索 九索 四筒 五筒 六筒 紅中"),
		rules.Seat4: mustTiles(t, "一萬 二萬 三萬 四萬 五萬 六萬 一索 二索 三索 四索 五索 六索 七筒 八筒 九筒 白板"),
	}
}

func defaultSeatWinds() map[rules.Seat]rules.Wind {
	return map[rules.Seat]rules.Wind{
		rules.Seat1: rules.WindEast,
		rules.Seat2: rules.WindSouth,
		rules.Seat3: rules.WindWest,
		rules.Seat4: rules.WindNorth,
	}
}

func TestTurnEngine_DealerFirstHandHeavenlyWin(t *testing.T) {
	t.Parallel()

	hands := defaultTurnHands(t)
	// Give dealer (Seat1) a winning 17-tile hand immediately.
	hands[rules.Seat1] = mustTiles(t, "一萬 二萬 三萬 四萬 五萬 六萬 七索 八索 九索 二筒 三筒 四筒 六筒 七筒 白板 白板 八筒")
	hands[rules.Seat4] = mustTiles(t, "一萬 二萬 三萬 四萬 五萬 六萬 一索 二索 三索 四索 五索 六索 七筒 八筒 九筒 青發")
	wall := makePreparedWall(t, hands, nil, nil)

	eng, err := rules.NewTurnEngine(rules.TurnConfig{
		Wall:      wall,
		RoundWind: rules.WindEast,
		SeatWinds: defaultSeatWinds(),
		Dealer:    rules.Seat1,
		Hands:     hands,
	})
	if err != nil {
		t.Fatalf("NewTurnEngine err = %v", err)
	}

	events := eng.Start()
	if eng.Phase() != rules.PhaseWaitSelfDeclare {
		t.Fatalf("Phase() = %v, want PhaseWaitSelfDeclare", eng.Phase())
	}
	if !hasSelfDeclarePrompt(events, rules.Seat1, true) {
		t.Fatalf("expected private SelfDeclarePromptEvent(CanSelfWin=true) for Seat1, got %+v", events)
	}

	winEvents, err := eng.Apply(rules.PlayerAction{
		Seat:        rules.Seat1,
		Kind:        rules.ActionSelfDeclare,
		SelfDeclare: rules.SelfDeclareSelfWin,
	}, 0)
	if err != nil {
		t.Fatalf("Apply self win err = %v", err)
	}
	if eng.Phase() != rules.PhaseHandEnded {
		t.Fatalf("Phase() = %v, want PhaseHandEnded", eng.Phase())
	}
	endEv := findHandEndedEvent(t, winEvents)
	if endEv.Reason != rules.HandEndWin || endEv.Score == nil {
		t.Fatalf("HandEndedEvent = %+v, want HandEndWin with score", endEv)
	}
	if endEv.Score.TotalFan != 20 {
		t.Fatalf("Heavenly win TotalFan = %d, want 20 (fans: %+v)", endEv.Score.TotalFan, endEv.Score.Fans)
	}
}

func TestTurnEngine_DrawAutoMultiFlowersAndThinkingTime(t *testing.T) {
	t.Parallel()

	hands := defaultTurnHands(t)
	wall := makePreparedWall(t, hands, nil, []rules.Tile{
		rules.FlowerPlum,
		rules.FlowerBamboo,
		rules.GreenDragon,
	})
	startDrawable := wall.DrawableRemaining()

	eng, err := rules.NewTurnEngine(rules.TurnConfig{
		Wall:      wall,
		RoundWind: rules.WindEast,
		SeatWinds: defaultSeatWinds(),
		Dealer:    rules.Seat1,
		Hands:     hands,
	})
	if err != nil {
		t.Fatalf("NewTurnEngine err = %v", err)
	}
	eng.Start()
	if eng.Phase() != rules.PhaseWaitDiscard {
		t.Fatalf("Phase() = %v, want PhaseWaitDiscard", eng.Phase())
	}

	// Dealer discards WestWind after thinking 7 seconds -> nobody claims -> Seat2 WaitDraw.
	discEvents, err := eng.Apply(rules.PlayerAction{
		Seat: rules.Seat1,
		Kind: rules.ActionDiscard,
		Tile: rules.WestWind,
	}, 7*time.Second)
	if err != nil {
		t.Fatalf("Seat1 discard err = %v", err)
	}
	if eng.ThinkingTime(rules.Seat1) != 7*time.Second {
		t.Fatalf("ThinkingTime(Seat1) = %v, want 7s", eng.ThinkingTime(rules.Seat1))
	}
	if !hasDiscardedEvent(discEvents, rules.Seat1, rules.WestWind, 7*time.Second) {
		t.Fatalf("missing DiscardedEvent with 7s thinking time in %+v", discEvents)
	}
	if eng.Phase() != rules.PhaseWaitDraw || eng.CurrentTurn() != rules.Seat2 {
		t.Fatalf("after discard: phase=%v turn=%v, want WaitDraw Seat2", eng.Phase(), eng.CurrentTurn())
	}

	// Seat2 draws -> gets FlowerPlum, auto-supplements FlowerBamboo, then GreenDragon.
	drawEvents, err := eng.Apply(rules.PlayerAction{
		Seat: rules.Seat2,
		Kind: rules.ActionDraw,
	}, 0)
	if err != nil {
		t.Fatalf("Seat2 draw err = %v", err)
	}
	if got := eng.DrawableRemaining(); got != startDrawable-3 {
		t.Fatalf("DrawableRemaining() = %d, want %d (minus 2 flowers + 1 non-flower)", got, startDrawable-3)
	}
	if got := eng.Flowers(rules.Seat2); !slices.Equal(got, []rules.Tile{rules.FlowerPlum, rules.FlowerBamboo}) {
		t.Fatalf("Flowers(Seat2) = %v, want [梅１ 竹４]", got)
	}
	if eng.Phase() != rules.PhaseWaitDiscard {
		t.Fatalf("Phase() = %v, want PhaseWaitDiscard", eng.Phase())
	}
	flowerCount := 0
	for _, ev := range drawEvents {
		if _, ok := ev.(rules.FlowerExposedEvent); ok {
			flowerCount++
		}
	}
	if flowerCount != 2 {
		t.Fatalf("expected 2 FlowerExposedEvent, got %d", flowerCount)
	}
}

func TestTurnEngine_FullHandConcealedKongManualSupplementAndConsecutiveKong(t *testing.T) {
	t.Parallel()

	hands := defaultTurnHands(t)
	// Seat2 already has 4x Suo1 and 3x Tong1 in hand; draws Tong2 (unrelated) -> must still be offered concealed kong Suo1!
	// Then on manual supplement draws 4th Tong1 -> must be offered consecutive concealed kong Tong1!
	hands[rules.Seat1] = mustTiles(t, "一萬 二萬 三萬 四萬 五萬 六萬 七索 八索 九索 二筒 三筒 四筒 六筒 七筒 東風 南風 西風")
	hands[rules.Seat2] = mustTiles(t, "一索 一索 一索 一索 一筒 一筒 一筒 二萬 三萬 四萬 五萬 六萬 七萬 八萬 九萬 北風")
	hands[rules.Seat3] = mustTiles(t, "七萬 八萬 九萬 七萬 八萬 九萬 四索 五索 六索 七索 八索 九索 四筒 五筒 六筒 紅中")
	hands[rules.Seat4] = mustTiles(t, "二索 三索 四索 五索 六索 七索 八索 九索 三筒 四筒 五筒 六筒 七筒 八筒 九筒 白板")

	wall := makePreparedWall(t, hands, nil, []rules.Tile{
		rules.Tong2,       // Seat2 normal draw (not part of Suo1 quad!)
		rules.Tong1,       // Seat2 manual supplement after Suo1 concealed kong -> enables 2nd concealed kong!
		rules.GreenDragon, // Seat2 manual supplement after Tong1 concealed kong
	})

	eng, err := rules.NewTurnEngine(rules.TurnConfig{
		Wall:      wall,
		RoundWind: rules.WindEast,
		SeatWinds: defaultSeatWinds(),
		Dealer:    rules.Seat1,
		Hands:     hands,
	})
	if err != nil {
		t.Fatalf("NewTurnEngine err = %v", err)
	}
	eng.Start()

	// Seat1 discards WestWind -> Seat2 draws Tong2.
	if _, err := eng.Apply(rules.PlayerAction{Seat: rules.Seat1, Kind: rules.ActionDiscard, Tile: rules.WestWind}, 0); err != nil {
		t.Fatalf("Seat1 discard err = %v", err)
	}
	evDraw, err := eng.Apply(rules.PlayerAction{Seat: rules.Seat2, Kind: rules.ActionDraw}, 0)
	if err != nil {
		t.Fatalf("Seat2 draw err = %v", err)
	}
	if eng.Phase() != rules.PhaseWaitSelfDeclare {
		t.Fatalf("Phase() = %v, want PhaseWaitSelfDeclare (pre-existing Suo1 quad)", eng.Phase())
	}
	sp := findSelfDeclarePrompt(t, evDraw, rules.Seat2)
	if !slices.Contains(sp.ConcealedKongTiles, rules.Suo1) {
		t.Fatalf("ConcealedKongTiles = %v, want Suo1", sp.ConcealedKongTiles)
	}

	// Seat2 declares concealed kong Suo1 -> public meld hides tiles & Seat2 enters WaitDraw for manual supplement.
	evKong1, err := eng.Apply(rules.PlayerAction{
		Seat:        rules.Seat2,
		Kind:        rules.ActionSelfDeclare,
		SelfDeclare: rules.SelfDeclareConcealedKong,
		Tile:        rules.Suo1,
	}, 0)
	if err != nil {
		t.Fatalf("concealed kong 1 err = %v", err)
	}
	if eng.Phase() != rules.PhaseWaitDraw || eng.CurrentTurn() != rules.Seat2 {
		t.Fatalf("after concealed kong: phase=%v turn=%v, want WaitDraw Seat2", eng.Phase(), eng.CurrentTurn())
	}
	for _, ev := range evKong1 {
		if me, ok := ev.(rules.MeldDeclaredEvent); ok {
			if len(me.Meld.Tiles) != 0 {
				t.Fatalf("concealed kong public event leaked tiles: %v", me.Meld.Tiles)
			}
		}
	}

	// Seat2 sends manual ActionDraw -> gets 4th Tong1 -> offered consecutive concealed kong Tong1!
	evSupp1, err := eng.Apply(rules.PlayerAction{Seat: rules.Seat2, Kind: rules.ActionDraw}, 0)
	if err != nil {
		t.Fatalf("manual supplement 1 err = %v", err)
	}
	if eng.Phase() != rules.PhaseWaitSelfDeclare {
		t.Fatalf("Phase() = %v, want PhaseWaitSelfDeclare for consecutive kong", eng.Phase())
	}
	sp2 := findSelfDeclarePrompt(t, evSupp1, rules.Seat2)
	if !slices.Contains(sp2.ConcealedKongTiles, rules.Tong1) {
		t.Fatalf("consecutive ConcealedKongTiles = %v, want Tong1", sp2.ConcealedKongTiles)
	}

	// Declare 2nd concealed kong Tong1 -> manual supplement -> WaitDiscard.
	if _, err := eng.Apply(rules.PlayerAction{
		Seat:        rules.Seat2,
		Kind:        rules.ActionSelfDeclare,
		SelfDeclare: rules.SelfDeclareConcealedKong,
		Tile:        rules.Tong1,
	}, 0); err != nil {
		t.Fatalf("concealed kong 2 err = %v", err)
	}
	if _, err := eng.Apply(rules.PlayerAction{Seat: rules.Seat2, Kind: rules.ActionDraw}, 0); err != nil {
		t.Fatalf("manual supplement 2 err = %v", err)
	}
	if eng.Phase() != rules.PhaseWaitDiscard {
		t.Fatalf("Phase() = %v, want PhaseWaitDiscard", eng.Phase())
	}
	if len(eng.Melds(rules.Seat2)) != 2 {
		t.Fatalf("len(Melds(Seat2)) = %d, want 2", len(eng.Melds(rules.Seat2)))
	}
}

func TestTurnEngine_AddedKongRobKongWin(t *testing.T) {
	t.Parallel()

	// Seat1 discards Suo6; Seat2 pongs Suo6; Seat2 discards; Seat3 & Seat4 turn; Seat2 later draws 4th Suo6 and adds kong;
	// Seat4 is waiting on Suo6 and robs the kong!
	hands := map[rules.Seat][]rules.Tile{
		rules.Seat1: mustTiles(t, "一萬 二萬 三萬 四萬 五萬 六萬 七萬 八萬 九萬 二筒 三筒 四筒 五筒 七筒 東風 南風 六索"),
		rules.Seat2: mustTiles(t, "六索 六索 一萬 二萬 三萬 四萬 五萬 六萬 七萬 八萬 九萬 二筒 三筒 四筒 西風 北風"),
		rules.Seat3: mustTiles(t, "一索 二索 三索 四索 五索 七索 八索 九索 一筒 二筒 三筒 四筒 五筒 七筒 八筒 紅中"),
		rules.Seat4: mustTiles(t, "二萬 三萬 四萬 五筒 六筒 七筒 三索 四索 五索 七索 八索 九索 九筒 九筒 一索 一索"),
	}
	// Make Seat4 wait on Suo6: 234W 567T 345S 78S(wait 6/9S) 99T + 111S = 16 tiles!
	hands[rules.Seat4] = mustTiles(t, "二萬 三萬 四萬 五筒 六筒 七筒 三索 四索 五索 七索 八索 九筒 九筒 一索 一索 一索")

	wall := makePreparedWall(t, hands, nil, []rules.Tile{
		rules.GreenDragon, // Seat3 draw
		rules.WhiteDragon, // Seat4 draw
		rules.EastWind,    // Seat1 draw
		rules.Suo6,        // Seat2 draws 4th Suo6 -> added kong!
	})

	eng, err := rules.NewTurnEngine(rules.TurnConfig{
		Wall:      wall,
		RoundWind: rules.WindEast,
		SeatWinds: defaultSeatWinds(),
		Dealer:    rules.Seat1,
		Hands:     hands,
	})
	if err != nil {
		t.Fatalf("NewTurnEngine err = %v", err)
	}
	eng.Start()

	// Seat1 discards Suo6 -> Seat2 can pong, Seat4 can win (Seat4 passes to let Seat2 pong for test setup? No!
	// If Seat4 passes on Suo6, Seat4 would enter 0.95m passed-win and couldn't rob kong until drawing a non-winning tile!
	// Even better: Seat4 draws WhiteDragon (non-winning tile!) which CLEARS Seat4's passed-win before Seat2's added kong!)
	if _, err := eng.Apply(rules.PlayerAction{Seat: rules.Seat1, Kind: rules.ActionDiscard, Tile: rules.Suo6}, 0); err != nil {
		t.Fatalf("Seat1 discard Suo6 err = %v", err)
	}
	if _, err := eng.Apply(rules.PlayerAction{Seat: rules.Seat2, Kind: rules.ActionClaim, Claim: rules.ClaimPong}, 0); err != nil {
		t.Fatalf("Seat2 pong err = %v", err)
	}
	if _, err := eng.Apply(rules.PlayerAction{Seat: rules.Seat4, Kind: rules.ActionClaim, Claim: rules.ClaimPass}, 0); err != nil {
		t.Fatalf("Seat4 pass err = %v", err)
	}
	if !eng.PassedWin(rules.Seat4) {
		t.Fatal("Seat4 passed win on Suo6 so must be in passed-win state")
	}

	// Seat2 discards NorthWind -> Seat3 draws & discards -> Seat4 draws WhiteDragon (clears passed-win!) & discards -> Seat1 draws & discards -> Seat2 draws 4th Suo6!
	for _, act := range []rules.PlayerAction{
		{Seat: rules.Seat2, Kind: rules.ActionDiscard, Tile: rules.NorthWind},
		{Seat: rules.Seat3, Kind: rules.ActionDraw},
		{Seat: rules.Seat3, Kind: rules.ActionDiscard, Tile: rules.GreenDragon},
		{Seat: rules.Seat4, Kind: rules.ActionDraw},
		{Seat: rules.Seat4, Kind: rules.ActionDiscard, Tile: rules.WhiteDragon},
		{Seat: rules.Seat1, Kind: rules.ActionDraw},
		{Seat: rules.Seat1, Kind: rules.ActionDiscard, Tile: rules.EastWind},
		{Seat: rules.Seat2, Kind: rules.ActionDraw},
	} {
		if _, err := eng.Apply(act, 0); err != nil {
			t.Fatalf("Apply(%+v) err = %v", act, err)
		}
	}

	if eng.PassedWin(rules.Seat4) {
		t.Fatal("Seat4 drew non-winning WhiteDragon so passed-win must be cleared")
	}
	if eng.Phase() != rules.PhaseWaitSelfDeclare || eng.CurrentTurn() != rules.Seat2 {
		t.Fatalf("phase=%v turn=%v, want WaitSelfDeclare Seat2", eng.Phase(), eng.CurrentTurn())
	}

	// Seat2 declares added kong Suo6 -> opens ClaimWindowRobKong prompting Seat4!
	evAddKong, err := eng.Apply(rules.PlayerAction{
		Seat:        rules.Seat2,
		Kind:        rules.ActionSelfDeclare,
		SelfDeclare: rules.SelfDeclareAddedKong,
		Tile:        rules.Suo6,
	}, 0)
	if err != nil {
		t.Fatalf("Seat2 added kong err = %v", err)
	}
	if eng.Phase() != rules.PhaseWaitClaims {
		t.Fatalf("Phase() = %v, want PhaseWaitClaims (RobKong window)", eng.Phase())
	}
	if !hasClaimPrompt(evAddKong, rules.Seat4, rules.ClaimWindowRobKong, rules.ClaimWin) {
		t.Fatalf("expected ClaimWindowRobKong prompt for Seat4, got %+v", evAddKong)
	}

	// Seat4 robs the kong -> HandEndWin with WinRobKong, payer = Seat2!
	evRob, err := eng.Apply(rules.PlayerAction{
		Seat:  rules.Seat4,
		Kind:  rules.ActionClaim,
		Claim: rules.ClaimWin,
	}, 0)
	if err != nil {
		t.Fatalf("Seat4 rob kong err = %v", err)
	}
	if eng.Phase() != rules.PhaseHandEnded {
		t.Fatalf("Phase() = %v, want PhaseHandEnded", eng.Phase())
	}
	endEv := findHandEndedEvent(t, evRob)
	if endEv.Reason != rules.HandEndWin || endEv.Score == nil {
		t.Fatalf("unexpected HandEndedEvent: %+v", endEv)
	}
	if endEv.Score.WinKind != rules.WinRobKong || endEv.Score.Winner != rules.Seat4 || endEv.Score.Payer != rules.Seat2 {
		t.Fatalf("Score = %+v, want WinRobKong winner=Seat4 payer=Seat2", endEv.Score)
	}
	assertHasFan(t, *endEv.Score, rules.FanRobKong, 1)
}

func TestTurnEngine_NegativeThinkTimeRejected(t *testing.T) {
	t.Parallel()

	hands := defaultTurnHands(t)
	wall := makePreparedWall(t, hands, nil, nil)
	eng, err := rules.NewTurnEngine(rules.TurnConfig{
		Wall:      wall,
		RoundWind: rules.WindEast,
		SeatWinds: defaultSeatWinds(),
		Dealer:    rules.Seat1,
		Hands:     hands,
	})
	if err != nil {
		t.Fatalf("NewTurnEngine err = %v", err)
	}
	eng.Start()

	_, err = eng.Apply(rules.PlayerAction{Seat: rules.Seat1, Kind: rules.ActionDiscard, Tile: rules.WestWind}, -time.Second)
	if !errors.Is(err, rules.ErrInvalidThinkTime) {
		t.Fatalf("negative think time err = %v, want ErrInvalidThinkTime", err)
	}
	// State must remain unchanged after rejected action.
	if eng.Phase() != rules.PhaseWaitDiscard || len(eng.Discards()) != 0 {
		t.Fatalf("state mutated after rejected negative think time: phase=%v discards=%d", eng.Phase(), len(eng.Discards()))
	}
}

func TestTurnEngine_Strict095mPassedWin_SelfWinAndDrawCycle(t *testing.T) {
	t.Parallel()

	// Seat1 starts with winning 17 tiles; declines self-win -> enters passed-win.
	// Seat2 is waiting on Tong8; Seat1 discards Tong8 -> Seat2 declines win -> enters passed-win.
	// Seat2 draws FlowerSpring (stays passed-win) -> auto-supplements Tong8 (winning tile! still no self-win, stays passed-win!).
	// Next round Seat2 draws GreenDragon (non-flower, non-winning) -> passed-win CLEARS with private notification!
	hands := map[rules.Seat][]rules.Tile{
		rules.Seat1: mustTiles(t, "一萬 二萬 三萬 四萬 五萬 六萬 七索 八索 九索 二筒 三筒 四筒 六筒 七筒 白板 白板 八筒"),
		rules.Seat2: mustTiles(t, "一萬 二萬 三萬 四萬 五萬 六萬 七索 八索 九索 二筒 三筒 四筒 六筒 七筒 紅中 紅中"),
		rules.Seat3: mustTiles(t, "七萬 八萬 九萬 七萬 八萬 九萬 一索 二索 三索 四索 五索 六索 一筒 二筒 三筒 東風"),
		rules.Seat4: mustTiles(t, "一索 二索 三索 四索 五索 六索 七萬 八萬 九萬 一筒 二筒 三筒 五筒 五筒 南風 西風"),
	}
	wall := makePreparedWall(t, hands, nil, []rules.Tile{
		rules.FlowerSpring, // Seat2 draw #1: flower -> auto-supplement
		rules.Tong8,        // Seat2 supplement #1: winning tile while passed-win -> must NOT offer self-win!
		rules.NorthWind,    // Seat3 draw (non-winning)
		rules.SouthWind,    // Seat4 draw
		rules.WestWind,     // Seat1 draw
		rules.GreenDragon,  // Seat2 draw #2: non-flower non-winning -> clears passed-win!
	})

	eng, err := rules.NewTurnEngine(rules.TurnConfig{
		Wall:      wall,
		RoundWind: rules.WindEast,
		SeatWinds: defaultSeatWinds(),
		Dealer:    rules.Seat1,
		Hands:     hands,
	})
	if err != nil {
		t.Fatalf("NewTurnEngine err = %v", err)
	}
	eng.Start()

	// Seat1 declines heavenly self-win -> enters passed-win.
	evPassSelf, err := eng.Apply(rules.PlayerAction{
		Seat:        rules.Seat1,
		Kind:        rules.ActionSelfDeclare,
		SelfDeclare: rules.SelfDeclarePass,
	}, 0)
	if err != nil {
		t.Fatalf("Seat1 decline self win err = %v", err)
	}
	if !eng.PassedWin(rules.Seat1) || !hasPassedWinEvent(evPassSelf, rules.Seat1, true) {
		t.Fatalf("Seat1 must enter passed-win with private event, got %+v", evPassSelf)
	}

	// Seat1 discards Tong8 -> Seat2 offered Win, Seat2 passes -> Seat2 enters passed-win.
	if _, err := eng.Apply(rules.PlayerAction{Seat: rules.Seat1, Kind: rules.ActionDiscard, Tile: rules.Tong8}, 0); err != nil {
		t.Fatalf("Seat1 discard Tong8 err = %v", err)
	}
	evPass2, err := eng.Apply(rules.PlayerAction{Seat: rules.Seat2, Kind: rules.ActionClaim, Claim: rules.ClaimPass}, 0)
	if err != nil {
		t.Fatalf("Seat2 pass win err = %v", err)
	}
	if !eng.PassedWin(rules.Seat2) || !hasPassedWinEvent(evPass2, rules.Seat2, true) {
		t.Fatalf("Seat2 must enter passed-win with private event, got %+v", evPass2)
	}

	// Seat2 draws FlowerSpring -> auto-supplements winning tile Tong8 -> MUST NOT offer self-win and MUST stay passed-win!
	evDrawWinWhilePassed, err := eng.Apply(rules.PlayerAction{Seat: rules.Seat2, Kind: rules.ActionDraw}, 0)
	if err != nil {
		t.Fatalf("Seat2 draw err = %v", err)
	}
	if eng.Phase() != rules.PhaseWaitDiscard {
		t.Fatalf("Phase() = %v, want PhaseWaitDiscard (self-win suppressed during passed-win)", eng.Phase())
	}
	if !eng.PassedWin(rules.Seat2) {
		t.Fatal("Seat2 drew winning tile while passed-win; passed-win MUST remain active")
	}
	if hasPassedWinEvent(evDrawWinWhilePassed, rules.Seat2, false) {
		t.Fatal("must not emit passed-win clear when drawing a winning tile")
	}

	// Complete the cycle until Seat2 draws non-winning GreenDragon.
	for _, act := range []rules.PlayerAction{
		{Seat: rules.Seat2, Kind: rules.ActionDiscard, Tile: rules.Tong8},
		{Seat: rules.Seat3, Kind: rules.ActionDraw},
		{Seat: rules.Seat3, Kind: rules.ActionDiscard, Tile: rules.NorthWind},
		{Seat: rules.Seat4, Kind: rules.ActionDraw},
		{Seat: rules.Seat4, Kind: rules.ActionDiscard, Tile: rules.SouthWind},
		{Seat: rules.Seat1, Kind: rules.ActionDraw},
		{Seat: rules.Seat1, Kind: rules.ActionDiscard, Tile: rules.WestWind},
	} {
		if _, err := eng.Apply(act, 0); err != nil {
			t.Fatalf("Apply(%+v) err = %v", act, err)
		}
	}

	evClear, err := eng.Apply(rules.PlayerAction{Seat: rules.Seat2, Kind: rules.ActionDraw}, 0)
	if err != nil {
		t.Fatalf("Seat2 second draw err = %v", err)
	}
	if eng.PassedWin(rules.Seat2) || !hasPassedWinEvent(evClear, rules.Seat2, false) {
		t.Fatalf("Seat2 passed-win must clear on non-winning non-flower draw, events=%+v", evClear)
	}
}

func TestTurnEngine_FlowerClaims_SevenRobOne_And_DeclineEightImmortals(t *testing.T) {
	t.Parallel()

	t.Run("seven rob one mid-hand ignores passed-win and payer is 1-flower holder", func(t *testing.T) {
		t.Parallel()
		hands := defaultTurnHands(t)
		flowers := map[rules.Seat][]rules.Tile{
			rules.Seat2: mustTiles(t, "春１ 夏２ 秋３ 冬４ 梅１ 蘭２ 菊３"),
		}
		wall := makePreparedWall(t, hands, flowers, []rules.Tile{
			rules.GreenDragon,  // Seat2 draw
			rules.FlowerBamboo, // Seat3 draws 8th flower -> triggers SevenRobOne for Seat2!
		})

		eng, err := rules.NewTurnEngine(rules.TurnConfig{
			Wall:      wall,
			RoundWind: rules.WindEast,
			SeatWinds: defaultSeatWinds(),
			Dealer:    rules.Seat1,
			Hands:     hands,
			Flowers:   flowers,
		})
		if err != nil {
			t.Fatalf("NewTurnEngine err = %v", err)
		}
		eng.Start()

		for _, act := range []rules.PlayerAction{
			{Seat: rules.Seat1, Kind: rules.ActionDiscard, Tile: rules.WestWind},
			{Seat: rules.Seat2, Kind: rules.ActionDraw},
			{Seat: rules.Seat2, Kind: rules.ActionDiscard, Tile: rules.GreenDragon},
		} {
			if _, err := eng.Apply(act, 0); err != nil {
				t.Fatalf("Apply(%+v) err = %v", act, err)
			}
		}

		evDraw3, err := eng.Apply(rules.PlayerAction{Seat: rules.Seat3, Kind: rules.ActionDraw}, 0)
		if err != nil {
			t.Fatalf("Seat3 draw 8th flower err = %v", err)
		}
		if eng.Phase() != rules.PhaseWaitClaims {
			t.Fatalf("Phase() = %v, want PhaseWaitClaims (SevenRobOne)", eng.Phase())
		}
		if !hasClaimPrompt(evDraw3, rules.Seat2, rules.ClaimWindowFlower, rules.ClaimWin) {
			t.Fatalf("expected Flower claim prompt for Seat2, got %+v", evDraw3)
		}

		evWin, err := eng.Apply(rules.PlayerAction{Seat: rules.Seat2, Kind: rules.ActionClaim, Claim: rules.ClaimWin}, 0)
		if err != nil {
			t.Fatalf("Seat2 claim SevenRobOne err = %v", err)
		}
		endEv := findHandEndedEvent(t, evWin)
		if endEv.Score == nil || endEv.Score.WinKind != rules.WinSevenRobOne || endEv.Score.Winner != rules.Seat2 || endEv.Score.Payer != rules.Seat3 {
			t.Fatalf("unexpected SevenRobOne score: %+v", endEv.Score)
		}
		if endEv.Score.TotalFan != 8 {
			t.Fatalf("TotalFan = %d, want 8", endEv.Score.TotalFan)
		}
	})

	t.Run("declining eight immortals resumes supplement without re-prompting", func(t *testing.T) {
		t.Parallel()
		hands := defaultTurnHands(t)
		flowers := map[rules.Seat][]rules.Tile{
			rules.Seat2: mustTiles(t, "春１ 夏２ 秋３ 冬４ 梅１ 蘭２ 菊３"),
		}
		wall := makePreparedWall(t, hands, flowers, []rules.Tile{
			rules.FlowerBamboo, // Seat2 draws 8th flower -> EightImmortals prompt
			rules.GreenDragon,  // Supplement after declining EightImmortals
		})

		eng, err := rules.NewTurnEngine(rules.TurnConfig{
			Wall:      wall,
			RoundWind: rules.WindEast,
			SeatWinds: defaultSeatWinds(),
			Dealer:    rules.Seat1,
			Hands:     hands,
			Flowers:   flowers,
		})
		if err != nil {
			t.Fatalf("NewTurnEngine err = %v", err)
		}
		eng.Start()

		if _, err := eng.Apply(rules.PlayerAction{Seat: rules.Seat1, Kind: rules.ActionDiscard, Tile: rules.WestWind}, 0); err != nil {
			t.Fatalf("Seat1 discard err = %v", err)
		}
		if _, err := eng.Apply(rules.PlayerAction{Seat: rules.Seat2, Kind: rules.ActionDraw}, 0); err != nil {
			t.Fatalf("Seat2 draw 8th flower err = %v", err)
		}
		if eng.Phase() != rules.PhaseWaitClaims {
			t.Fatalf("Phase() = %v, want PhaseWaitClaims (EightImmortals)", eng.Phase())
		}

		// Decline EightImmortals -> auto-supplements GreenDragon and enters WaitDiscard without passed-win.
		if _, err := eng.Apply(rules.PlayerAction{Seat: rules.Seat2, Kind: rules.ActionClaim, Claim: rules.ClaimPass}, 0); err != nil {
			t.Fatalf("Seat2 decline EightImmortals err = %v", err)
		}
		if eng.Phase() != rules.PhaseWaitDiscard || eng.CurrentTurn() != rules.Seat2 {
			t.Fatalf("after decline: phase=%v turn=%v, want WaitDiscard Seat2", eng.Phase(), eng.CurrentTurn())
		}
		if eng.PassedWin(rules.Seat2) {
			t.Fatal("declining flower claim must NOT trigger passed-win")
		}
	})
}

func TestTurnEngine_ClaimTransitions_SkipSeat_RemoveRiver_And_KongSupplementWin(t *testing.T) {
	t.Parallel()

	t.Run("pong skips intermediate seat and removes tile from river", func(t *testing.T) {
		t.Parallel()
		hands := defaultTurnHands(t)
		hands[rules.Seat1] = mustTiles(t, "一萬 二萬 三萬 四萬 五萬 六萬 七索 八索 九索 二筒 三筒 四筒 六筒 七筒 東風 南風 紅中")
		hands[rules.Seat3] = mustTiles(t, "七萬 八萬 九萬 七萬 八萬 九萬 四索 五索 六索 七索 八索 九索 四筒 西風 紅中 紅中")
		wall := makePreparedWall(t, hands, nil, nil)

		eng, err := rules.NewTurnEngine(rules.TurnConfig{
			Wall:      wall,
			RoundWind: rules.WindEast,
			SeatWinds: defaultSeatWinds(),
			Dealer:    rules.Seat1,
			Hands:     hands,
		})
		if err != nil {
			t.Fatalf("NewTurnEngine err = %v", err)
		}
		eng.Start()

		// Seat1 discards RedDragon -> river has 1 tile -> Seat3 pongs -> river emptied, Seat3 discards, Seat4 draws!
		if _, err := eng.Apply(rules.PlayerAction{Seat: rules.Seat1, Kind: rules.ActionDiscard, Tile: rules.RedDragon}, 0); err != nil {
			t.Fatalf("Seat1 discard err = %v", err)
		}
		if len(eng.Discards()) != 1 {
			t.Fatalf("len(Discards()) = %d, want 1", len(eng.Discards()))
		}
		if _, err := eng.Apply(rules.PlayerAction{Seat: rules.Seat3, Kind: rules.ActionClaim, Claim: rules.ClaimPong}, 0); err != nil {
			t.Fatalf("Seat3 pong err = %v", err)
		}
		if len(eng.Discards()) != 0 {
			t.Fatalf("claimed tile must be removed from river, got %+v", eng.Discards())
		}
		if eng.Phase() != rules.PhaseWaitDiscard || eng.CurrentTurn() != rules.Seat3 {
			t.Fatalf("after pong: phase=%v turn=%v, want WaitDiscard Seat3", eng.Phase(), eng.CurrentTurn())
		}

		// Seat3 discards WestWind (nobody claims) -> Seat4 gets WaitDraw (Seat2 skipped!).
		if _, err := eng.Apply(rules.PlayerAction{Seat: rules.Seat3, Kind: rules.ActionDiscard, Tile: rules.WestWind}, 0); err != nil {
			t.Fatalf("Seat3 discard err = %v", err)
		}
		if eng.Phase() != rules.PhaseWaitDraw || eng.CurrentTurn() != rules.Seat4 {
			t.Fatalf("after Seat3 discard: phase=%v turn=%v, want WaitDraw Seat4 (Seat2 skipped)", eng.Phase(), eng.CurrentTurn())
		}
	})

	t.Run("exposed kong followed by supplement self-draw scores KongFlowerWin", func(t *testing.T) {
		t.Parallel()
		hands := defaultTurnHands(t)
		hands[rules.Seat1] = mustTiles(t, "一萬 二萬 三萬 四萬 五萬 六萬 七索 八索 九索 二筒 三筒 四筒 六筒 七筒 東風 南風 紅中")
		// Seat3 has 3x RedDragon + 4 complete sets + waiting pair/sequence on Tong8!
		hands[rules.Seat3] = mustTiles(t, "紅中 紅中 紅中 一萬 二萬 三萬 四萬 五萬 六萬 四索 五索 六索 九索 九索 六筒 七筒")
		wall := makePreparedWall(t, hands, nil, []rules.Tile{
			rules.Tong8, // Seat3 supplement draw after exposed kong -> 自摸 + 杠上開花!
		})

		eng, err := rules.NewTurnEngine(rules.TurnConfig{
			Wall:      wall,
			RoundWind: rules.WindEast,
			SeatWinds: defaultSeatWinds(),
			Dealer:    rules.Seat1,
			Hands:     hands,
		})
		if err != nil {
			t.Fatalf("NewTurnEngine err = %v", err)
		}
		eng.Start()

		if _, err := eng.Apply(rules.PlayerAction{Seat: rules.Seat1, Kind: rules.ActionDiscard, Tile: rules.RedDragon}, 0); err != nil {
			t.Fatalf("Seat1 discard err = %v", err)
		}
		if _, err := eng.Apply(rules.PlayerAction{Seat: rules.Seat3, Kind: rules.ActionClaim, Claim: rules.ClaimKong}, 0); err != nil {
			t.Fatalf("Seat3 exposed kong err = %v", err)
		}
		if eng.Phase() != rules.PhaseWaitDraw || eng.CurrentTurn() != rules.Seat3 {
			t.Fatalf("after exposed kong: phase=%v turn=%v, want WaitDraw Seat3", eng.Phase(), eng.CurrentTurn())
		}
		if _, err := eng.Apply(rules.PlayerAction{Seat: rules.Seat3, Kind: rules.ActionDraw}, 0); err != nil {
			t.Fatalf("Seat3 manual supplement draw err = %v", err)
		}
		evWin, err := eng.Apply(rules.PlayerAction{Seat: rules.Seat3, Kind: rules.ActionSelfDeclare, SelfDeclare: rules.SelfDeclareSelfWin}, 0)
		if err != nil {
			t.Fatalf("Seat3 self win err = %v", err)
		}
		endEv := findHandEndedEvent(t, evWin)
		if endEv.Score == nil {
			t.Fatal("expected non-nil score")
		}
		assertHasFan(t, *endEv.Score, rules.FanKongFlowerWin, 1)
		assertHasFan(t, *endEv.Score, rules.FanSelfDraw, 1)
		assertHasFan(t, *endEv.Score, rules.FanRedDragon, 1)
	})
}

func TestTurnEngine_LastDrawableTile_OnlyWinAndFlowerExhaustion(t *testing.T) {
	t.Parallel()

	t.Run("zero drawable remaining suppresses pong and ends exhausted", func(t *testing.T) {
		t.Parallel()
		hands := defaultTurnHands(t)
		hands[rules.Seat1] = mustTiles(t, "一萬 二萬 三萬 四萬 五萬 六萬 七索 八索 九索 二筒 三筒 四筒 六筒 七筒 東風 南風 北風")
		hands[rules.Seat2] = mustTiles(t, "一萬 二萬 三萬 四萬 五萬 六萬 一索 二索 三索 四索 五索 六索 一筒 二筒 北風 北風")
		wall := makePreparedWall(t, hands, nil, nil)
		// Drain wall until DrawableRemaining == 0.
		for wall.DrawableRemaining() > 0 {
			if _, err := wall.Draw(); err != nil {
				t.Fatalf("drain err = %v", err)
			}
		}

		eng, err := rules.NewTurnEngine(rules.TurnConfig{
			Wall:      wall,
			RoundWind: rules.WindEast,
			SeatWinds: defaultSeatWinds(),
			Dealer:    rules.Seat1,
			Hands:     hands,
		})
		if err != nil {
			t.Fatalf("NewTurnEngine err = %v", err)
		}
		eng.Start()

		// Seat1 discards NorthWind (Seat2 has a pair of NorthWind, cannot win) -> must NOT ask Pong, ends exhausted!
		ev, err := eng.Apply(rules.PlayerAction{Seat: rules.Seat1, Kind: rules.ActionDiscard, Tile: rules.NorthWind}, 0)
		if err != nil {
			t.Fatalf("discard at 0 drawable err = %v", err)
		}
		if eng.Phase() != rules.PhaseHandEnded {
			t.Fatalf("Phase() = %v, want PhaseHandEnded", eng.Phase())
		}
		endEv := findHandEndedEvent(t, ev)
		if endEv.Reason != rules.HandEndExhausted {
			t.Fatalf("Reason = %v, want HandEndExhausted", endEv.Reason)
		}
	})

	t.Run("zero drawable remaining allows river bottom win (河底撈魚)", func(t *testing.T) {
		t.Parallel()
		hands := defaultTurnHands(t)
		hands[rules.Seat1] = mustTiles(t, "一萬 二萬 三萬 四萬 五萬 六萬 七索 八索 九索 二筒 三筒 四筒 六筒 七筒 東風 南風 八筒")
		hands[rules.Seat2] = mustTiles(t, "一萬 二萬 三萬 四萬 五萬 六萬 七索 八索 九索 二筒 三筒 四筒 六筒 七筒 白板 白板")
		hands[rules.Seat4] = mustTiles(t, "一索 二索 三索 四索 五索 六索 七萬 八萬 九萬 一筒 二筒 三筒 五筒 五筒 青發 青發")
		wall := makePreparedWall(t, hands, nil, nil)
		for wall.DrawableRemaining() > 0 {
			if _, err := wall.Draw(); err != nil {
				t.Fatalf("drain err = %v", err)
			}
		}

		eng, err := rules.NewTurnEngine(rules.TurnConfig{
			Wall:      wall,
			RoundWind: rules.WindEast,
			SeatWinds: defaultSeatWinds(),
			Dealer:    rules.Seat1,
			Hands:     hands,
		})
		if err != nil {
			t.Fatalf("NewTurnEngine err = %v", err)
		}
		eng.Start()

		if _, err := eng.Apply(rules.PlayerAction{Seat: rules.Seat1, Kind: rules.ActionDiscard, Tile: rules.Tong8}, 0); err != nil {
			t.Fatalf("Seat1 discard err = %v", err)
		}
		if eng.Phase() != rules.PhaseWaitClaims {
			t.Fatalf("Phase() = %v, want PhaseWaitClaims for river-bottom win", eng.Phase())
		}
		evWin, err := eng.Apply(rules.PlayerAction{Seat: rules.Seat2, Kind: rules.ActionClaim, Claim: rules.ClaimWin}, 0)
		if err != nil {
			t.Fatalf("Seat2 win err = %v", err)
		}
		endEv := findHandEndedEvent(t, evWin)
		if endEv.Score == nil {
			t.Fatal("expected score on river-bottom win")
		}
		assertHasFan(t, *endEv.Score, rules.FanLastTileDiscard, 1)
	})

	t.Run("last drawable tile is flower ends exhausted without touching 16 reserve tiles", func(t *testing.T) {
		t.Parallel()
		hands := defaultTurnHands(t)
		probeWall := makePreparedWall(t, hands, nil, []rules.Tile{rules.FlowerSpring})
		filler := make([]rules.Tile, 0, 62)
		for range 63 {
			tile, err := probeWall.Draw()
			if err != nil {
				t.Fatalf("collect filler err = %v", err)
			}
			if tile != rules.FlowerSpring && len(filler) < 62 {
				filler = append(filler, tile)
			}
		}
		filler = append(filler, rules.FlowerSpring)
		wall3 := makePreparedWall(t, hands, nil, filler)
		for range 62 {
			if _, err := wall3.Draw(); err != nil {
				t.Fatalf("drain 62 err = %v", err)
			}
		}
		if wall3.DrawableRemaining() != 1 {
			t.Fatalf("DrawableRemaining() = %d, want 1", wall3.DrawableRemaining())
		}

		eng, err := rules.NewTurnEngine(rules.TurnConfig{
			Wall:      wall3,
			RoundWind: rules.WindEast,
			SeatWinds: defaultSeatWinds(),
			Dealer:    rules.Seat1,
			Hands:     hands,
		})
		if err != nil {
			t.Fatalf("NewTurnEngine err = %v", err)
		}
		eng.Start()

		if _, err := eng.Apply(rules.PlayerAction{Seat: rules.Seat1, Kind: rules.ActionDiscard, Tile: rules.WestWind}, 0); err != nil {
			t.Fatalf("Seat1 discard err = %v", err)
		}
		evDraw, err := eng.Apply(rules.PlayerAction{Seat: rules.Seat2, Kind: rules.ActionDraw}, 0)
		if err != nil {
			t.Fatalf("Seat2 draw last flower err = %v", err)
		}
		if eng.Phase() != rules.PhaseHandEnded {
			t.Fatalf("Phase() = %v, want PhaseHandEnded", eng.Phase())
		}
		if wall3.Remaining() != rules.ReserveWallTiles {
			t.Fatalf("reserve tiles touched! Remaining() = %d, want %d", wall3.Remaining(), rules.ReserveWallTiles)
		}
		endEv := findHandEndedEvent(t, evDraw)
		if endEv.Reason != rules.HandEndExhausted {
			t.Fatalf("Reason = %v, want HandEndExhausted", endEv.Reason)
		}
	})
}

func TestTurnEngine_IllegalActionsDoNotMutateState(t *testing.T) {
	t.Parallel()

	hands := defaultTurnHands(t)
	wall := makePreparedWall(t, hands, nil, nil)
	eng, err := rules.NewTurnEngine(rules.TurnConfig{
		Wall:      wall,
		RoundWind: rules.WindEast,
		SeatWinds: defaultSeatWinds(),
		Dealer:    rules.Seat1,
		Hands:     hands,
	})
	if err != nil {
		t.Fatalf("NewTurnEngine err = %v", err)
	}

	// Applying before Start() must fail.
	if _, err := eng.Apply(rules.PlayerAction{Seat: rules.Seat1, Kind: rules.ActionDiscard, Tile: rules.WestWind}, 0); !errors.Is(err, rules.ErrInvalidPhase) {
		t.Fatalf("before Start err = %v, want ErrInvalidPhase", err)
	}
	eng.Start()

	// Wrong seat discard must fail without mutation.
	if _, err := eng.Apply(rules.PlayerAction{Seat: rules.Seat2, Kind: rules.ActionDiscard, Tile: rules.NorthWind}, 0); !errors.Is(err, rules.ErrNotYourTurn) {
		t.Fatalf("wrong seat discard err = %v, want ErrNotYourTurn", err)
	}
	// Tile not in Seat1's hand must fail without mutation.
	if _, err := eng.Apply(rules.PlayerAction{Seat: rules.Seat1, Kind: rules.ActionDiscard, Tile: rules.RedDragon}, 0); !errors.Is(err, rules.ErrTileNotInHand) {
		t.Fatalf("missing tile discard err = %v, want ErrTileNotInHand", err)
	}
	// Draw in WaitDiscard must fail.
	if _, err := eng.Apply(rules.PlayerAction{Seat: rules.Seat1, Kind: rules.ActionDraw}, 0); !errors.Is(err, rules.ErrInvalidPhase) {
		t.Fatalf("draw in WaitDiscard err = %v, want ErrInvalidPhase", err)
	}
	if eng.Phase() != rules.PhaseWaitDiscard || len(eng.Concealed(rules.Seat1)) != 17 || len(eng.Discards()) != 0 {
		t.Fatalf("state mutated after illegal actions: phase=%v handLen=%d discards=%d", eng.Phase(), len(eng.Concealed(rules.Seat1)), len(eng.Discards()))
	}
}

func TestTurnEngine_TwoStepChowChoiceAndUnrobbedAddedKong(t *testing.T) {
	t.Parallel()

	t.Run("two-step chow choice through TurnEngine.Apply", func(t *testing.T) {
		t.Parallel()
		hands := defaultTurnHands(t)
		hands[rules.Seat1] = mustTiles(t, "一萬 二萬 三萬 四萬 五萬 六萬 七索 八索 九索 二筒 三筒 四筒 六筒 七筒 東風 南風 四萬")
		// Seat2 has 2,3,5 Wan -> two chow options for Wan4: 234 and 345!
		hands[rules.Seat2] = mustTiles(t, "二萬 三萬 五萬 一索 二索 三索 四索 五索 六索 一筒 二筒 三筒 七筒 八筒 九筒 北風")
		wall := makePreparedWall(t, hands, nil, nil)

		eng, err := rules.NewTurnEngine(rules.TurnConfig{
			Wall:      wall,
			RoundWind: rules.WindEast,
			SeatWinds: defaultSeatWinds(),
			Dealer:    rules.Seat1,
			Hands:     hands,
		})
		if err != nil {
			t.Fatalf("NewTurnEngine err = %v", err)
		}
		eng.Start()

		if _, err := eng.Apply(rules.PlayerAction{Seat: rules.Seat1, Kind: rules.ActionDiscard, Tile: rules.Wan4}, 0); err != nil {
			t.Fatalf("Seat1 discard Wan4 err = %v", err)
		}
		// Step 1: Seat2 submits ClaimChow with empty ChowOption -> enters PhaseWaitChowChoice!
		evStep1, err := eng.Apply(rules.PlayerAction{Seat: rules.Seat2, Kind: rules.ActionClaim, Claim: rules.ClaimChow}, 0)
		if err != nil {
			t.Fatalf("Seat2 step 1 chow err = %v", err)
		}
		if eng.Phase() != rules.PhaseWaitChowChoice {
			t.Fatalf("Phase() = %v, want PhaseWaitChowChoice", eng.Phase())
		}
		if len(evStep1) != 1 {
			t.Fatalf("expected 1 ChowChoicePromptEvent, got %+v", evStep1)
		}
		// Step 2: Seat2 selects 345 Wan -> meld declared, river emptied, Seat2 enters WaitDiscard!
		choice345 := rules.ChowOption{Tiles: [3]rules.Tile{rules.Wan3, rules.Wan4, rules.Wan5}}
		if _, err := eng.Apply(rules.PlayerAction{
			Seat:  rules.Seat2,
			Kind:  rules.ActionClaim,
			Claim: rules.ClaimChow,
			Chow:  choice345,
		}, 0); err != nil {
			t.Fatalf("Seat2 step 2 chow err = %v", err)
		}
		if eng.Phase() != rules.PhaseWaitDiscard || eng.CurrentTurn() != rules.Seat2 {
			t.Fatalf("after chow: phase=%v turn=%v, want WaitDiscard Seat2", eng.Phase(), eng.CurrentTurn())
		}
		if len(eng.Discards()) != 0 {
			t.Fatalf("claimed chow tile must be removed from river, got %+v", eng.Discards())
		}
		melds := eng.Melds(rules.Seat2)
		if len(melds) != 1 || melds[0].Kind != rules.MeldChow || melds[0].BaseTile() != rules.Wan3 {
			t.Fatalf("Seat2 melds = %+v, want Chow starting at Wan3", melds)
		}
	})

	t.Run("unrobbed added kong upgrades pong and enters manual WaitDraw", func(t *testing.T) {
		t.Parallel()
		hands := defaultTurnHands(t)
		hands[rules.Seat1] = mustTiles(t, "一萬 二萬 三萬 四萬 五萬 六萬 七索 八索 九索 二筒 三筒 四筒 六筒 七筒 東風 南風 紅中")
		hands[rules.Seat3] = mustTiles(t, "七萬 八萬 九萬 七萬 八萬 九萬 四索 五索 六索 七索 八索 九索 四筒 西風 紅中 紅中")
		wall := makePreparedWall(t, hands, nil, []rules.Tile{
			rules.GreenDragon, // Seat4 draw (non-winning)
			rules.GreenDragon, // Seat1 draw (unclaimable by Seat2)
			rules.WestWind,    // Seat2 draw (non-winning)
			rules.RedDragon,   // Seat3 draws 4th RedDragon -> added kong (no robber!)
			rules.Tong9,       // Seat3 manual supplement after added kong
		})

		eng, err := rules.NewTurnEngine(rules.TurnConfig{
			Wall:      wall,
			RoundWind: rules.WindEast,
			SeatWinds: defaultSeatWinds(),
			Dealer:    rules.Seat1,
			Hands:     hands,
		})
		if err != nil {
			t.Fatalf("NewTurnEngine err = %v", err)
		}
		eng.Start()

		for _, act := range []rules.PlayerAction{
			{Seat: rules.Seat1, Kind: rules.ActionDiscard, Tile: rules.RedDragon},
			{Seat: rules.Seat3, Kind: rules.ActionClaim, Claim: rules.ClaimPong},
			{Seat: rules.Seat3, Kind: rules.ActionDiscard, Tile: rules.WestWind},
			{Seat: rules.Seat4, Kind: rules.ActionDraw},
			{Seat: rules.Seat4, Kind: rules.ActionDiscard, Tile: rules.GreenDragon},
			{Seat: rules.Seat1, Kind: rules.ActionDraw},
			{Seat: rules.Seat1, Kind: rules.ActionDiscard, Tile: rules.GreenDragon},
			{Seat: rules.Seat2, Kind: rules.ActionDraw},
			{Seat: rules.Seat2, Kind: rules.ActionDiscard, Tile: rules.WestWind},
			{Seat: rules.Seat3, Kind: rules.ActionDraw},
		} {
			if _, err := eng.Apply(act, 0); err != nil {
				t.Fatalf("Apply(%+v) err = %v", act, err)
			}
		}

		if eng.Phase() != rules.PhaseWaitSelfDeclare || eng.CurrentTurn() != rules.Seat3 {
			t.Fatalf("phase=%v turn=%v, want WaitSelfDeclare Seat3", eng.Phase(), eng.CurrentTurn())
		}
		if _, err := eng.Apply(rules.PlayerAction{
			Seat:        rules.Seat3,
			Kind:        rules.ActionSelfDeclare,
			SelfDeclare: rules.SelfDeclareAddedKong,
			Tile:        rules.RedDragon,
		}, 0); err != nil {
			t.Fatalf("Seat3 added kong err = %v", err)
		}
		if eng.Phase() != rules.PhaseWaitDraw || eng.CurrentTurn() != rules.Seat3 {
			t.Fatalf("after unrobbed added kong: phase=%v turn=%v, want WaitDraw Seat3", eng.Phase(), eng.CurrentTurn())
		}
		melds := eng.Melds(rules.Seat3)
		if len(melds) != 1 || melds[0].Kind != rules.MeldAddedKong || len(melds[0].Tiles) != 4 {
			t.Fatalf("Seat3 melds = %+v, want upgraded MeldAddedKong", melds)
		}
	})

	t.Run("opening 8 flowers prompts on Start and HandResult reports winner", func(t *testing.T) {
		t.Parallel()
		hands := defaultTurnHands(t)
		flowers := map[rules.Seat][]rules.Tile{
			rules.Seat2: mustTiles(t, "春１ 夏２ 秋３ 冬４ 梅１ 蘭２ 菊３ 竹４"),
		}
		wall := makePreparedWall(t, hands, flowers, nil)

		eng, err := rules.NewTurnEngine(rules.TurnConfig{
			Wall:      wall,
			RoundWind: rules.WindEast,
			SeatWinds: defaultSeatWinds(),
			Dealer:    rules.Seat1,
			Hands:     hands,
			Flowers:   flowers,
		})
		if err != nil {
			t.Fatalf("NewTurnEngine err = %v", err)
		}
		evStart := eng.Start()
		if eng.Phase() != rules.PhaseWaitClaims {
			t.Fatalf("Phase() = %v, want PhaseWaitClaims on opening 8 flowers", eng.Phase())
		}
		if !hasClaimPrompt(evStart, rules.Seat2, rules.ClaimWindowFlower, rules.ClaimWin) {
			t.Fatalf("expected opening flower claim prompt for Seat2, got %+v", evStart)
		}
		if _, err := eng.Apply(rules.PlayerAction{Seat: rules.Seat2, Kind: rules.ActionClaim, Claim: rules.ClaimWin}, 0); err != nil {
			t.Fatalf("opening EightImmortals win err = %v", err)
		}
		reason, score := eng.HandResult()
		if reason != rules.HandEndWin || score == nil || score.WinKind != rules.WinEightImmortals {
			t.Fatalf("HandResult() = (%v, %+v), want HandEndWin WinEightImmortals", reason, score)
		}
	})
}

func hasPassedWinEvent(events []rules.Event, seat rules.Seat, passed bool) bool {
	for _, ev := range events {
		if pe, ok := ev.(rules.PassedWinChangedEvent); ok && pe.Seat == seat && pe.Passed == passed {
			return true
		}
	}
	return false
}

func hasSelfDeclarePrompt(events []rules.Event, seat rules.Seat, canWin bool) bool {
	for _, ev := range events {
		if sp, ok := ev.(rules.SelfDeclarePromptEvent); ok && sp.Seat == seat && sp.CanSelfWin == canWin {
			return true
		}
	}
	return false
}

func findSelfDeclarePrompt(t *testing.T, events []rules.Event, seat rules.Seat) rules.SelfDeclarePromptEvent {
	t.Helper()
	for _, ev := range events {
		if sp, ok := ev.(rules.SelfDeclarePromptEvent); ok && sp.Seat == seat {
			return sp
		}
	}
	t.Fatalf("SelfDeclarePromptEvent for %v not found in %+v", seat, events)
	return rules.SelfDeclarePromptEvent{}
}

func hasClaimPrompt(events []rules.Event, seat rules.Seat, kind rules.ClaimWindowKind, opt rules.ClaimKind) bool {
	for _, ev := range events {
		if cp, ok := ev.(rules.ClaimPromptEvent); ok && cp.Seat == seat && cp.WindowKind == kind && slices.Contains(cp.Options, opt) {
			return true
		}
	}
	return false
}

func hasDiscardedEvent(events []rules.Event, seat rules.Seat, tile rules.Tile, elapsed time.Duration) bool {
	for _, ev := range events {
		if de, ok := ev.(rules.DiscardedEvent); ok && de.Seat == seat && de.Tile == tile && de.ThinkingTime == elapsed {
			return true
		}
	}
	return false
}

func findHandEndedEvent(t *testing.T, events []rules.Event) rules.HandEndedEvent {
	t.Helper()
	for _, ev := range events {
		if he, ok := ev.(rules.HandEndedEvent); ok {
			return he
		}
	}
	t.Fatalf("HandEndedEvent not found in %+v", events)
	return rules.HandEndedEvent{}
}
