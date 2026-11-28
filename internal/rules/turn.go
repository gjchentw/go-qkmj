package rules

import (
	"errors"
	"fmt"
	"slices"
	"time"
)

// Sentinel errors returned by TurnEngine operations.
var (
	ErrInvalidTurnConfig = errors.New("invalid turn engine configuration")
	ErrInvalidPhase      = errors.New("action not allowed in current turn phase")
	ErrNotYourTurn       = errors.New("action seat is not the active seat")
	ErrTileNotInHand     = errors.New("tile not found in concealed hand")
	ErrInvalidThinkTime  = errors.New("thinking time must be non-negative")
	ErrRiichiLocked      = errors.New("riichi hand can only discard the newly drawn tile")
)

// TurnPhase identifies the current waiting or terminal phase of a single hand's turn cycle.
type TurnPhase uint8

const (
	PhaseUnknown TurnPhase = iota
	PhaseNotStarted
	PhaseWaitDraw
	PhaseWaitSelfDeclare
	PhaseWaitDiscard
	PhaseWaitClaims
	PhaseWaitChowChoice
	PhaseHandEnded
)

// TurnConfig provides all deterministic starting state required to drive a hand's turn cycle
// (after initial deal and opening flower replacement).
type TurnConfig struct {
	Wall         *Wall
	RoundWind    Wind
	SeatWinds    map[Seat]Wind
	Dealer       Seat
	DealerStreak int
	Hands        map[Seat][]Tile
	Flowers      map[Seat][]Tile
	Base         int
	PerFan       int
}

type flowerResumeKind uint8

const (
	flowerResumeNone flowerResumeKind = iota
	flowerResumeDealerFirst
	flowerResumeSupplement
)

type flowerDeclineKey struct {
	owner Seat
	count int
}

// TurnEngine is the pure finite state machine driving a hand from dealer's first turn
// through draws, auto flower replacement, self-declarations, discards, claims, and hand end.
type TurnEngine struct {
	wall         *Wall
	roundWind    Wind
	seatWinds    [5]Wind
	dealer       Seat
	dealerStreak int
	base         int
	perFan       int

	phase                TurnPhase
	currentTurn          Seat
	concealed            [5][]Tile
	melds                [5][]Meld
	flowers              [5][]Tile
	discards             []DiscardRecord
	thinkingTime         [5]time.Duration
	passedWin            [5]bool
	riichi               [5]bool
	tianTing             [5]bool
	hasDiscarded         [5]bool
	anyMeldDeclared      bool
	pendingSupplement    bool
	afterSupplement      bool
	lastDrawnTile        Tile
	pendingAddedKong     Tile
	kongCount            int
	kongSeats            [5]bool
	pendingFourKongsDraw bool

	selfPrompt      SelfDeclarePromptEvent
	claimWindow     *ClaimWindow
	flowerResume    flowerResumeKind
	declinedFlowers map[flowerDeclineKey]bool

	endReason HandEndReason
	endScore  *ScoreResult
}

// NewTurnEngine validates cfg and constructs a TurnEngine positioned at PhaseNotStarted.
func NewTurnEngine(cfg TurnConfig) (*TurnEngine, error) {
	if err := validateTurnConfig(cfg); err != nil {
		return nil, err
	}

	eng := &TurnEngine{
		wall:            cfg.Wall,
		roundWind:       cfg.RoundWind,
		dealer:          cfg.Dealer,
		dealerStreak:    cfg.DealerStreak,
		base:            cfg.Base,
		perFan:          cfg.PerFan,
		phase:           PhaseNotStarted,
		currentTurn:     cfg.Dealer,
		declinedFlowers: make(map[flowerDeclineKey]bool),
	}
	for _, s := range AllSeats {
		eng.seatWinds[s] = cfg.SeatWinds[s]
		eng.concealed[s] = slices.Clone(cfg.Hands[s])
		eng.flowers[s] = slices.Clone(cfg.Flowers[s])
	}
	eng.lastDrawnTile = eng.concealed[cfg.Dealer][len(eng.concealed[cfg.Dealer])-1]
	return eng, nil
}

func validateTurnConfig(cfg TurnConfig) error {
	if cfg.Wall == nil {
		return fmt.Errorf("wall is nil: %w", ErrInvalidTurnConfig)
	}
	if !cfg.RoundWind.Valid() || !cfg.Dealer.Valid() || cfg.DealerStreak < 0 {
		return fmt.Errorf("roundWind=%v dealer=%v streak=%d: %w", cfg.RoundWind, cfg.Dealer, cfg.DealerStreak, ErrInvalidTurnConfig)
	}
	if cfg.Base < 0 || cfg.PerFan < 0 {
		return fmt.Errorf("base=%d perFan=%d: %w", cfg.Base, cfg.PerFan, ErrInvalidTurnConfig)
	}
	var seenFlower [TileMax + 1]bool
	for _, s := range AllSeats {
		if err := validateSeatSetup(cfg, s, &seenFlower); err != nil {
			return err
		}
	}
	return nil
}

func validateSeatSetup(cfg TurnConfig, s Seat, seenFlower *[TileMax + 1]bool) error {
	if !cfg.SeatWinds[s].Valid() {
		return fmt.Errorf("seat %v wind invalid: %w", s, ErrInvalidTurnConfig)
	}
	wantLen := WaitingHandSize
	if s == cfg.Dealer {
		wantLen = DrawnHandSize
	}
	hand := cfg.Hands[s]
	if len(hand) != wantLen {
		return fmt.Errorf("seat %v hand len %d, want %d: %w", s, len(hand), wantLen, ErrInvalidTurnConfig)
	}
	for _, t := range hand {
		if !t.Valid() || t.IsFlower() {
			return fmt.Errorf("seat %v invalid hand tile %v: %w", s, t, ErrInvalidTurnConfig)
		}
	}
	for _, fl := range cfg.Flowers[s] {
		if !fl.IsFlower() || seenFlower[fl] {
			return fmt.Errorf("seat %v invalid/duplicate flower %v: %w", s, fl, ErrInvalidTurnConfig)
		}
		seenFlower[fl] = true
	}
	return nil
}

// Start transitions the engine from PhaseNotStarted into initial flower claims (if applicable)
// or directly into dealer's first-hand self-declaration check without drawing.
func (e *TurnEngine) Start() []Event {
	if e.phase != PhaseNotStarted {
		return nil
	}
	if cw := e.buildFlowerClaimWindow(e.dealer, TileInvalid); cw != nil {
		e.claimWindow = cw
		e.flowerResume = flowerResumeDealerFirst
		e.phase = PhaseWaitClaims
		return cw.Prompts()
	}
	return e.startDealerFirstHand()
}

func (e *TurnEngine) startDealerFirstHand() []Event {
	e.currentTurn = e.dealer
	e.afterSupplement = false
	return prependEvent(
		TurnChangedEvent{Seat: e.dealer, WaitingForDraw: false},
		e.enterSelfCheck(e.dealer, e.lastDrawnTile, false),
	)
}

// Apply validates and executes one PlayerAction with an externally measured thinking duration.
// On error, no engine state is mutated.
func (e *TurnEngine) Apply(action PlayerAction, thinkElapsed time.Duration) ([]Event, error) {
	if thinkElapsed < 0 {
		return nil, ErrInvalidThinkTime
	}
	if err := action.Validate(); err != nil {
		return nil, err
	}

	switch e.phase {
	case PhaseWaitDraw:
		return e.applyDraw(action)
	case PhaseWaitSelfDeclare:
		return e.applySelfDeclare(action, thinkElapsed)
	case PhaseWaitDiscard:
		return e.applyDiscard(action, thinkElapsed)
	case PhaseWaitClaims, PhaseWaitChowChoice:
		return e.applyClaim(action)
	case PhaseUnknown, PhaseNotStarted, PhaseHandEnded:
		return nil, fmt.Errorf("phase %d: %w", e.phase, ErrInvalidPhase)
	default:
		return nil, fmt.Errorf("phase %d: %w", e.phase, ErrInvalidPhase)
	}
}

func (e *TurnEngine) applyDraw(action PlayerAction) ([]Event, error) {
	if action.Kind != ActionDraw {
		return nil, fmt.Errorf("expected draw in WaitDraw: %w", ErrInvalidPhase)
	}
	if action.Seat != e.currentTurn {
		return nil, fmt.Errorf("seat %v != active %v: %w", action.Seat, e.currentTurn, ErrNotYourTurn)
	}

	tile, err := e.wall.Draw()
	if err != nil {
		return nil, err
	}
	isSupp := e.pendingSupplement
	e.pendingSupplement = false

	events := []Event{
		TileDrawnPublicEvent{Seat: e.currentTurn, DrawableRemaining: e.wall.DrawableRemaining(), IsSupplement: isSupp},
		TileDrawnEvent{Seat: e.currentTurn, Tile: tile, IsSupplement: isSupp},
	}
	if tile.IsFlower() {
		return append(events, e.handleFlowerTile(e.currentTurn, tile)...), nil
	}
	e.concealed[e.currentTurn] = append(e.concealed[e.currentTurn], tile)
	e.lastDrawnTile = tile
	e.afterSupplement = isSupp
	return append(events, e.enterSelfCheck(e.currentTurn, tile, true)...), nil
}

func (e *TurnEngine) handleFlowerTile(seat Seat, flower Tile) []Event {
	e.flowers[seat] = append(e.flowers[seat], flower)
	events := []Event{
		FlowerExposedEvent{Seat: seat, Flower: flower, DrawableRemaining: e.wall.DrawableRemaining()},
	}
	if cw := e.buildFlowerClaimWindow(seat, flower); cw != nil {
		e.claimWindow = cw
		e.flowerResume = flowerResumeSupplement
		e.phase = PhaseWaitClaims
		return append(events, cw.Prompts()...)
	}
	return append(events, e.continueFlowerSupplement(seat)...)
}

func (e *TurnEngine) continueFlowerSupplement(seat Seat) []Event {
	var events []Event
	for {
		if e.wall.DrawableRemaining() == 0 {
			return append(events, e.finishHandExhausted()...)
		}
		tile, err := e.wall.Draw()
		if err != nil {
			return append(events, e.finishHandExhausted()...)
		}
		events = append(events,
			TileDrawnPublicEvent{Seat: seat, DrawableRemaining: e.wall.DrawableRemaining(), IsSupplement: true},
			TileDrawnEvent{Seat: seat, Tile: tile, IsSupplement: true},
		)
		if !tile.IsFlower() {
			e.concealed[seat] = append(e.concealed[seat], tile)
			e.lastDrawnTile = tile
			e.afterSupplement = true
			return append(events, e.enterSelfCheck(seat, tile, true)...)
		}
		e.flowers[seat] = append(e.flowers[seat], tile)
		events = append(events,
			FlowerExposedEvent{Seat: seat, Flower: tile, DrawableRemaining: e.wall.DrawableRemaining()},
		)
		if cw := e.buildFlowerClaimWindow(seat, tile); cw != nil {
			e.claimWindow = cw
			e.flowerResume = flowerResumeSupplement
			e.phase = PhaseWaitClaims
			return append(events, cw.Prompts()...)
		}
	}
}

func (e *TurnEngine) enterSelfCheck(seat Seat, drawnTile Tile, fromDraw bool) []Event {
	var events []Event
	canWinNow := e.canSeatSelfWin(seat, drawnTile)

	// QKMJ 0.95m strict passed-win: clears only for non-Riichi seats when drawing/supplementing
	// a non-flower tile that cannot win; Riichi passed-win is permanent for the rest of the hand.
	if fromDraw && e.passedWin[seat] && !canWinNow && !e.riichi[seat] {
		e.passedWin[seat] = false
		events = append(events, PassedWinChangedEvent{Seat: seat, Passed: false})
	}

	prompt, hasPrompt := e.buildSelfDeclarePrompt(seat, !e.passedWin[seat] && canWinNow)
	if hasPrompt {
		e.selfPrompt = prompt
		e.phase = PhaseWaitSelfDeclare
		return append(events, cloneSelfDeclarePrompt(prompt))
	}

	e.selfPrompt = SelfDeclarePromptEvent{}
	e.phase = PhaseWaitDiscard
	return events
}

func (e *TurnEngine) buildSelfDeclarePrompt(seat Seat, canSelfWin bool) (SelfDeclarePromptEvent, bool) {
	canNineTerminals := !e.anyMeldDeclared && !e.hasDiscarded[seat] && CountDistinctTerminalOrHonor(e.concealed[seat]) >= 9
	var concealedKongs, addedKongs, riichiDiscards []Tile
	if e.wall.DrawableRemaining() > 0 && !e.riichi[seat] && !e.pendingFourKongsDraw {
		concealedKongs = e.findConcealedKongTiles(seat)
		addedKongs = e.findAddedKongTiles(seat)
		riichiDiscards = FindRiichiDiscardTiles(e.concealed[seat], e.melds[seat])
	}
	hasPrompt := canSelfWin || canNineTerminals || len(concealedKongs) > 0 || len(addedKongs) > 0 || len(riichiDiscards) > 0
	if !hasPrompt {
		return SelfDeclarePromptEvent{}, false
	}
	return SelfDeclarePromptEvent{
		Seat:               seat,
		CanSelfWin:         canSelfWin,
		ConcealedKongTiles: concealedKongs,
		AddedKongTiles:     addedKongs,
		CanNineTerminals:   canNineTerminals,
		CanRiichi:          len(riichiDiscards) > 0,
		RiichiDiscardTiles: riichiDiscards,
	}, true
}

func cloneSelfDeclarePrompt(p SelfDeclarePromptEvent) SelfDeclarePromptEvent {
	p.ConcealedKongTiles = slices.Clone(p.ConcealedKongTiles)
	p.AddedKongTiles = slices.Clone(p.AddedKongTiles)
	p.RiichiDiscardTiles = slices.Clone(p.RiichiDiscardTiles)
	return p
}

func (e *TurnEngine) applySelfDeclare(action PlayerAction, thinkElapsed time.Duration) ([]Event, error) {
	if action.Kind != ActionSelfDeclare {
		return nil, fmt.Errorf("expected self declare in WaitSelfDeclare: %w", ErrInvalidPhase)
	}
	if action.Seat != e.currentTurn {
		return nil, fmt.Errorf("seat %v != active %v: %w", action.Seat, e.currentTurn, ErrNotYourTurn)
	}
	if err := e.validateSelfDeclareChoice(action); err != nil {
		return nil, err
	}

	var events []Event
	if e.selfPrompt.CanSelfWin && action.SelfDeclare != SelfDeclareSelfWin && !e.passedWin[action.Seat] {
		e.passedWin[action.Seat] = true
		events = append(events, PassedWinChangedEvent{Seat: action.Seat, Passed: true})
	}
	e.selfPrompt = SelfDeclarePromptEvent{}

	switch action.SelfDeclare {
	case SelfDeclarePass:
		e.phase = PhaseWaitDiscard
		return events, nil
	case SelfDeclareSelfWin:
		return append(events, e.finishHandByWin(WinSelfDraw, action.Seat, SeatUnknown, e.lastDrawnTile)...), nil
	case SelfDeclareConcealedKong:
		return append(events, e.executeConcealedKong(action.Seat, action.Tile)...), nil
	case SelfDeclareAddedKong:
		return append(events, e.startAddedKong(action.Seat, action.Tile)...), nil
	case SelfDeclareNineTerminals:
		return append(events, e.finishHandDraw(HandEndNineTerminals)...), nil
	case SelfDeclareRiichi:
		riichiEvs, err := e.executeRiichi(action.Seat, action.Tile, thinkElapsed)
		if err != nil {
			return nil, err
		}
		return append(events, riichiEvs...), nil
	case SelfDeclareUnknown:
		return nil, ErrInvalidAction
	default:
		return nil, ErrInvalidAction
	}
}

func (e *TurnEngine) validateSelfDeclareChoice(action PlayerAction) error {
	switch action.SelfDeclare {
	case SelfDeclarePass:
		return nil
	case SelfDeclareSelfWin:
		if !e.selfPrompt.CanSelfWin {
			return fmt.Errorf("self win not available: %w", ErrInvalidClaimOption)
		}
		return nil
	case SelfDeclareConcealedKong:
		if !slices.Contains(e.selfPrompt.ConcealedKongTiles, action.Tile) {
			return fmt.Errorf("concealed kong tile %v not available: %w", action.Tile, ErrInvalidClaimOption)
		}
		return nil
	case SelfDeclareAddedKong:
		if !slices.Contains(e.selfPrompt.AddedKongTiles, action.Tile) {
			return fmt.Errorf("added kong tile %v not available: %w", action.Tile, ErrInvalidClaimOption)
		}
		return nil
	case SelfDeclareNineTerminals:
		if !e.selfPrompt.CanNineTerminals {
			return fmt.Errorf("nine terminals draw not available: %w", ErrInvalidClaimOption)
		}
		return nil
	case SelfDeclareRiichi:
		if !e.selfPrompt.CanRiichi || !slices.Contains(e.selfPrompt.RiichiDiscardTiles, action.Tile) {
			return fmt.Errorf("riichi discard tile %v not available: %w", action.Tile, ErrInvalidClaimOption)
		}
		return nil
	case SelfDeclareUnknown:
		return ErrInvalidAction
	default:
		return ErrInvalidAction
	}
}

func (e *TurnEngine) executeRiichi(seat Seat, discardTile Tile, thinkElapsed time.Duration) ([]Event, error) {
	isTianTing := !e.anyMeldDeclared && !e.hasDiscarded[seat]
	e.riichi[seat] = true
	e.tianTing[seat] = isTianTing

	discardEvs, err := e.executeDiscard(seat, discardTile, thinkElapsed)
	if err != nil {
		e.riichi[seat] = false
		e.tianTing[seat] = false
		return nil, err
	}
	return prependEvent(RiichiDeclaredEvent{Seat: seat, IsTianTing: isTianTing}, discardEvs), nil
}

func (e *TurnEngine) recordKong(seat Seat) {
	e.kongCount++
	e.kongSeats[seat] = true
	distinct := 0
	for _, s := range AllSeats {
		if e.kongSeats[s] {
			distinct++
		}
	}
	if e.kongCount >= 4 && distinct >= 2 {
		e.pendingFourKongsDraw = true
	}
}

func (e *TurnEngine) executeConcealedKong(seat Seat, tile Tile) []Event {
	e.concealed[seat] = removeTiles(e.concealed[seat], tile, 4)
	meld := Meld{Kind: MeldConcealedKong, Tiles: []Tile{tile, tile, tile, tile}}
	e.melds[seat] = append(e.melds[seat], meld)
	e.anyMeldDeclared = true
	e.afterSupplement = false
	e.recordKong(seat)
	return prependEvent(NewMeldDeclaredEvent(seat, meld, false), e.enterWaitDraw(seat, true))
}

func (e *TurnEngine) startAddedKong(seat Seat, tile Tile) []Event {
	cw := e.buildRobKongClaimWindow(seat, tile)
	if cw != nil {
		e.pendingAddedKong = tile
		e.claimWindow = cw
		e.phase = PhaseWaitClaims
		return cw.Prompts()
	}
	return e.finalizeAddedKong(seat, tile)
}

func (e *TurnEngine) finalizeAddedKong(seat Seat, tile Tile) []Event {
	e.concealed[seat] = removeTiles(e.concealed[seat], tile, 1)
	var upgraded Meld
	for i := range e.melds[seat] {
		if e.melds[seat][i].Kind == MeldPong && e.melds[seat][i].BaseTile() == tile {
			e.melds[seat][i].Kind = MeldAddedKong
			e.melds[seat][i].Tiles = []Tile{tile, tile, tile, tile}
			upgraded = e.melds[seat][i]
			break
		}
	}
	e.anyMeldDeclared = true
	e.afterSupplement = false
	e.pendingAddedKong = TileInvalid
	e.recordKong(seat)
	return prependEvent(NewMeldDeclaredEvent(seat, upgraded, false), e.enterWaitDraw(seat, true))
}

func (e *TurnEngine) applyDiscard(action PlayerAction, thinkElapsed time.Duration) ([]Event, error) {
	if action.Kind != ActionDiscard {
		return nil, fmt.Errorf("expected discard in WaitDiscard: %w", ErrInvalidPhase)
	}
	if action.Seat != e.currentTurn {
		return nil, fmt.Errorf("seat %v != active %v: %w", action.Seat, e.currentTurn, ErrNotYourTurn)
	}
	if e.riichi[action.Seat] && action.Tile != e.lastDrawnTile {
		return nil, fmt.Errorf("riichi seat %v must discard %s: %w", action.Seat, e.lastDrawnTile, ErrRiichiLocked)
	}
	return e.executeDiscard(action.Seat, action.Tile, thinkElapsed)
}

func (e *TurnEngine) executeDiscard(seat Seat, tile Tile, thinkElapsed time.Duration) ([]Event, error) {
	idx := slices.Index(e.concealed[seat], tile)
	if idx < 0 {
		return nil, fmt.Errorf("tile %s: %w", tile, ErrTileNotInHand)
	}

	e.concealed[seat] = slices.Delete(e.concealed[seat], idx, idx+1)
	e.hasDiscarded[seat] = true
	e.afterSupplement = false
	e.thinkingTime[seat] += thinkElapsed
	e.discards = append(e.discards, DiscardRecord{Seat: seat, Tile: tile})

	events := []Event{
		DiscardedEvent{Seat: seat, Tile: tile, ThinkingTime: e.thinkingTime[seat]},
	}

	cw := e.buildDiscardClaimWindow(seat, tile)
	if cw != nil {
		e.claimWindow = cw
		e.phase = PhaseWaitClaims
		return append(events, cw.Prompts()...), nil
	}
	return append(events, e.advanceAfterNoClaim(seat)...), nil
}

func (e *TurnEngine) applyClaim(action PlayerAction) ([]Event, error) {
	if action.Kind != ActionClaim {
		return nil, fmt.Errorf("expected claim in claim window: %w", ErrInvalidPhase)
	}
	if e.claimWindow == nil {
		return nil, ErrInvalidPhase
	}

	events, res, err := e.claimWindow.Submit(action.Seat, action.Claim, action.Chow)
	if err != nil {
		return nil, err
	}
	if e.claimWindow.WaitingChowSeat() != SeatUnknown {
		e.phase = PhaseWaitChowChoice
		return events, nil
	}
	if res == nil {
		e.phase = PhaseWaitClaims
		return events, nil
	}

	cw := e.claimWindow
	e.claimWindow = nil
	return append(events, e.resolveClaimWindow(cw, *res)...), nil
}

func (e *TurnEngine) resolveClaimWindow(cw *ClaimWindow, res ClaimResolution) []Event {
	var events []Event
	for _, s := range res.PassedWinSeats {
		if !e.passedWin[s] {
			e.passedWin[s] = true
			events = append(events, PassedWinChangedEvent{Seat: s, Passed: true})
		}
	}

	switch cw.Kind() {
	case ClaimWindowFlower:
		return append(events, e.resolveFlowerClaim(cw, res)...)
	case ClaimWindowRobKong:
		return append(events, e.resolveRobKongClaim(cw, res)...)
	case ClaimWindowDiscard:
		return append(events, e.resolveDiscardClaim(cw, res)...)
	case ClaimWindowUnknown:
		return events
	default:
		return events
	}
}

func (e *TurnEngine) resolveFlowerClaim(cw *ClaimWindow, res ClaimResolution) []Event {
	resume := e.flowerResume
	e.flowerResume = flowerResumeNone

	if res.Kind == ClaimWin {
		return e.finishHandByWin(res.FlowerWinKind, res.Winner, res.FlowerPayer, TileInvalid)
	}
	// Mark this flower distribution as declined so it is never asked again.
	for _, seat := range AllSeats {
		cnt := len(e.flowers[seat])
		if cnt == 7 || cnt == 8 {
			e.declinedFlowers[flowerDeclineKey{owner: seat, count: cnt}] = true
		}
	}
	if resume == flowerResumeDealerFirst {
		return e.startDealerFirstHand()
	}
	return e.continueFlowerSupplement(cw.TriggerSeat())
}

func (e *TurnEngine) resolveRobKongClaim(cw *ClaimWindow, res ClaimResolution) []Event {
	kongTile := e.pendingAddedKong
	e.pendingAddedKong = TileInvalid
	if res.IsTripleRon {
		return e.finishHandDraw(HandEndTripleRon)
	}
	if res.Kind == ClaimWin {
		return e.finishHandByWin(WinRobKong, res.Winner, cw.TriggerSeat(), kongTile)
	}
	return e.finalizeAddedKong(cw.TriggerSeat(), kongTile)
}

func (e *TurnEngine) resolveDiscardClaim(cw *ClaimWindow, res ClaimResolution) []Event {
	triggerSeat := cw.TriggerSeat()
	tile := cw.TriggerTile()

	if res.IsTripleRon {
		return e.finishHandDraw(HandEndTripleRon)
	}

	switch res.Kind {
	case ClaimWin:
		return e.finishHandByWin(WinDiscard, res.Winner, triggerSeat, tile)
	case ClaimKong:
		e.popLastDiscard()
		e.concealed[res.Winner] = removeTiles(e.concealed[res.Winner], tile, 3)
		meld := Meld{Kind: MeldExposedKong, Tiles: []Tile{tile, tile, tile, tile}, FromSeat: triggerSeat}
		e.melds[res.Winner] = append(e.melds[res.Winner], meld)
		e.anyMeldDeclared = true
		e.afterSupplement = false
		e.recordKong(res.Winner)
		return prependEvent(NewMeldDeclaredEvent(res.Winner, meld, true), e.enterWaitDraw(res.Winner, true))
	case ClaimPong:
		e.popLastDiscard()
		e.concealed[res.Winner] = removeTiles(e.concealed[res.Winner], tile, 2)
		meld := Meld{Kind: MeldPong, Tiles: []Tile{tile, tile, tile}, FromSeat: triggerSeat}
		e.melds[res.Winner] = append(e.melds[res.Winner], meld)
		e.anyMeldDeclared = true
		e.afterSupplement = false
		return prependEvent(NewMeldDeclaredEvent(res.Winner, meld, true), e.enterWaitDiscard(res.Winner))
	case ClaimChow:
		e.popLastDiscard()
		for _, ct := range res.Chow.Tiles {
			if ct != tile {
				e.concealed[res.Winner] = removeTiles(e.concealed[res.Winner], ct, 1)
			}
		}
		meld := Meld{Kind: MeldChow, Tiles: slices.Clone(res.Chow.Tiles[:]), FromSeat: triggerSeat}
		e.melds[res.Winner] = append(e.melds[res.Winner], meld)
		e.anyMeldDeclared = true
		e.afterSupplement = false
		return prependEvent(NewMeldDeclaredEvent(res.Winner, meld, true), e.enterWaitDiscard(res.Winner))
	case ClaimPass, ClaimUnknown:
		return e.advanceAfterNoClaim(triggerSeat)
	default:
		return e.advanceAfterNoClaim(triggerSeat)
	}
}

func prependEvent(first Event, rest []Event) []Event {
	out := make([]Event, 0, 1+len(rest))
	out = append(out, first)
	return append(out, rest...)
}

func (e *TurnEngine) advanceAfterNoClaim(discarder Seat) []Event {
	if e.isFourWindsDraw() {
		return e.finishHandDraw(HandEndFourWinds)
	}
	if e.pendingFourKongsDraw {
		return e.finishHandDraw(HandEndFourKongs)
	}
	if e.wall.DrawableRemaining() == 0 {
		return e.finishHandExhausted()
	}
	return e.enterWaitDraw(discarder.Next(), false)
}

func (e *TurnEngine) isFourWindsDraw() bool {
	if e.anyMeldDeclared || len(e.discards) != 4 {
		return false
	}
	first := e.discards[0].Tile
	if !first.IsWind() {
		return false
	}
	return e.discards[1].Tile == first && e.discards[2].Tile == first && e.discards[3].Tile == first
}

func (e *TurnEngine) enterWaitDraw(seat Seat, isSupplement bool) []Event {
	e.currentTurn = seat
	e.pendingSupplement = isSupplement
	e.phase = PhaseWaitDraw
	return []Event{
		TurnChangedEvent{Seat: seat, WaitingForDraw: true},
		DrawPromptEvent{Seat: seat, IsSupplement: isSupplement},
	}
}

func (e *TurnEngine) enterWaitDiscard(seat Seat) []Event {
	e.currentTurn = seat
	e.phase = PhaseWaitDiscard
	return []Event{
		TurnChangedEvent{Seat: seat, WaitingForDraw: false},
	}
}

func (e *TurnEngine) finishHandByWin(kind WinKind, winner, payer Seat, winTile Tile) []Event {
	var concealed []Tile
	if kind != WinSevenRobOne && kind != WinEightImmortals {
		if kind == WinSelfDraw {
			concealed = removeTiles(slices.Clone(e.concealed[winner]), winTile, 1)
		} else {
			concealed = slices.Clone(e.concealed[winner])
		}
	}

	firstRound := !e.anyMeldDeclared && !e.hasDiscarded[winner]
	res, err := Score(ScoreContext{
		Concealed:            concealed,
		Melds:                slices.Clone(e.melds[winner]),
		Flowers:              slices.Clone(e.flowers[winner]),
		WinTile:              winTile,
		WinKind:              kind,
		Winner:               winner,
		Payer:                payer,
		Dealer:               e.dealer,
		DealerStreak:         e.dealerStreak,
		RoundWind:            e.roundWind,
		SeatWind:             e.seatWinds[winner],
		IsAfterSupplement:    kind == WinSelfDraw && e.afterSupplement,
		IsLastDrawable:       e.wall.DrawableRemaining() == 0,
		IsFirstRoundUnbroken: firstRound,
		IsRiichi:             e.riichi[winner],
		IsTianTing:           e.tianTing[winner],
		Base:                 e.base,
		PerFan:               e.perFan,
	})
	if err != nil {
		// Unreachable for validated winning paths; surface deterministically as exhausted fallback.
		return e.finishHandExhausted()
	}
	e.phase = PhaseHandEnded
	e.endReason = HandEndWin
	e.endScore = &res
	return []Event{HandEndedEvent{Reason: HandEndWin, Score: &res}}
}

func (e *TurnEngine) finishHandExhausted() []Event {
	return e.finishHandDraw(HandEndExhausted)
}

func (e *TurnEngine) finishHandDraw(reason HandEndReason) []Event {
	e.phase = PhaseHandEnded
	e.endReason = reason
	e.endScore = nil
	return []Event{HandEndedEvent{Reason: reason}}
}

func (e *TurnEngine) buildDiscardClaimWindow(discarder Seat, tile Tile) *ClaimWindow {
	canNonWin := e.wall.DrawableRemaining() > 0 && !e.pendingFourKongsDraw
	var seatOpts []SeatClaimOptions

	seat := discarder.Next()
	for range 3 {
		so := e.computeDiscardSeatOptions(seat, discarder, tile, canNonWin)
		if len(so.Options) > 0 {
			seatOpts = append(seatOpts, so)
		}
		seat = seat.Next()
	}
	if len(seatOpts) == 0 {
		return nil
	}
	return NewClaimWindow(ClaimWindowDiscard, discarder, tile, seatOpts)
}

func (e *TurnEngine) computeDiscardSeatOptions(seat, discarder Seat, tile Tile, canNonWin bool) SeatClaimOptions {
	so := SeatClaimOptions{Seat: seat}
	if canNonWin && !e.riichi[seat] {
		if seat == discarder.Next() {
			if chows := ComputeChowOptions(e.concealed[seat], tile); len(chows) > 0 {
				so.Options = append(so.Options, ClaimChow)
				so.ChowOptions = chows
			}
		}
		cnt := countTile(e.concealed[seat], tile)
		if cnt >= 2 {
			so.Options = append(so.Options, ClaimPong)
		}
		if cnt >= 3 {
			so.Options = append(so.Options, ClaimKong)
		}
	}
	if !e.passedWin[seat] && CanWin(e.concealed[seat], e.melds[seat], tile) {
		so.Options = append(so.Options, ClaimWin)
	}
	return so
}

func (e *TurnEngine) buildRobKongClaimWindow(kongSeat Seat, tile Tile) *ClaimWindow {
	var seatOpts []SeatClaimOptions
	seat := kongSeat.Next()
	for range 3 {
		if !e.passedWin[seat] && CanWin(e.concealed[seat], e.melds[seat], tile) {
			seatOpts = append(seatOpts, SeatClaimOptions{
				Seat:    seat,
				Options: []ClaimKind{ClaimWin},
			})
		}
		seat = seat.Next()
	}
	if len(seatOpts) == 0 {
		return nil
	}
	return NewClaimWindow(ClaimWindowRobKong, kongSeat, tile, seatOpts)
}

func (e *TurnEngine) buildFlowerClaimWindow(triggerSeat Seat, flower Tile) *ClaimWindow {
	var sevenOwner, oneOwner Seat
	for _, s := range AllSeats {
		cnt := len(e.flowers[s])
		if cnt == 8 {
			if e.declinedFlowers[flowerDeclineKey{owner: s, count: 8}] {
				return nil
			}
			return NewClaimWindow(ClaimWindowFlower, triggerSeat, flower, []SeatClaimOptions{
				{Seat: s, Options: []ClaimKind{ClaimWin}, FlowerWinKind: WinEightImmortals},
			})
		}
		if cnt == 7 {
			sevenOwner = s
		}
		if cnt == 1 {
			oneOwner = s
		}
	}
	if sevenOwner.Valid() && oneOwner.Valid() {
		if e.declinedFlowers[flowerDeclineKey{owner: sevenOwner, count: 7}] {
			return nil
		}
		return NewClaimWindow(ClaimWindowFlower, triggerSeat, flower, []SeatClaimOptions{
			{Seat: sevenOwner, Options: []ClaimKind{ClaimWin}, FlowerWinKind: WinSevenRobOne, FlowerPayer: oneOwner},
		})
	}
	return nil
}

func (e *TurnEngine) canSeatSelfWin(seat Seat, drawnTile Tile) bool {
	if len(e.concealed[seat]) == 0 || !drawnTile.Valid() {
		return false
	}
	without := removeTiles(slices.Clone(e.concealed[seat]), drawnTile, 1)
	return CanWin(without, e.melds[seat], drawnTile)
}

func (e *TurnEngine) findConcealedKongTiles(seat Seat) []Tile {
	var counts [NonFlowerMax + 1]int
	for _, t := range e.concealed[seat] {
		counts[t]++
	}
	var out []Tile
	for t := Wan1; t <= NonFlowerMax; t++ {
		if counts[t] == 4 {
			out = append(out, t)
		}
	}
	return out
}

func (e *TurnEngine) findAddedKongTiles(seat Seat) []Tile {
	var out []Tile
	for _, m := range e.melds[seat] {
		if m.Kind != MeldPong {
			continue
		}
		base := m.BaseTile()
		if slices.Contains(e.concealed[seat], base) {
			out = append(out, base)
		}
	}
	slices.Sort(out)
	return out
}

func (e *TurnEngine) popLastDiscard() {
	if len(e.discards) > 0 {
		e.discards = e.discards[:len(e.discards)-1]
	}
}

func countTile(tiles []Tile, target Tile) int {
	n := 0
	for _, t := range tiles {
		if t == target {
			n++
		}
	}
	return n
}

func removeTiles(tiles []Tile, target Tile, count int) []Tile {
	removed := 0
	out := make([]Tile, 0, len(tiles)-min(count, len(tiles)))
	for _, t := range tiles {
		if t == target && removed < count {
			removed++
			continue
		}
		out = append(out, t)
	}
	return out
}

// Phase returns the current TurnPhase of the engine.
func (e *TurnEngine) Phase() TurnPhase { return e.phase }

// CurrentTurn returns the seat that currently owns the turn.
func (e *TurnEngine) CurrentTurn() Seat { return e.currentTurn }

// DrawableRemaining returns the public count of drawable tiles remaining in the wall.
func (e *TurnEngine) DrawableRemaining() int { return e.wall.DrawableRemaining() }

// Concealed returns a defensive copy of seat's concealed hand.
func (e *TurnEngine) Concealed(seat Seat) []Tile {
	if !seat.Valid() {
		return nil
	}
	return slices.Clone(e.concealed[seat])
}

// Melds returns a defensive copy of seat's declared melds.
func (e *TurnEngine) Melds(seat Seat) []Meld {
	if !seat.Valid() {
		return nil
	}
	out := make([]Meld, len(e.melds[seat]))
	for i, m := range e.melds[seat] {
		out[i] = Meld{Kind: m.Kind, Tiles: slices.Clone(m.Tiles), FromSeat: m.FromSeat}
	}
	return out
}

// Flowers returns a defensive copy of seat's exposed flowers.
func (e *TurnEngine) Flowers(seat Seat) []Tile {
	if !seat.Valid() {
		return nil
	}
	return slices.Clone(e.flowers[seat])
}

// Discards returns a defensive copy of the public discard river.
func (e *TurnEngine) Discards() []DiscardRecord {
	return slices.Clone(e.discards)
}

// ThinkingTime returns seat's cumulative thinking time for this hand.
func (e *TurnEngine) ThinkingTime(seat Seat) time.Duration {
	if !seat.Valid() {
		return 0
	}
	return e.thinkingTime[seat]
}

// PassedWin reports whether seat is currently in 0.95m passed-win (過水) state.
func (e *TurnEngine) PassedWin(seat Seat) bool {
	if !seat.Valid() {
		return false
	}
	return e.passedWin[seat]
}

// HandResult returns the terminal reason and score result once PhaseHandEnded is reached.
func (e *TurnEngine) HandResult() (HandEndReason, *ScoreResult) {
	return e.endReason, e.endScore
}

// Riichi reports whether seat has declared Riichi in this hand.
func (e *TurnEngine) Riichi(seat Seat) bool {
	if !seat.Valid() {
		return false
	}
	return e.riichi[seat]
}

// TianTing reports whether seat declared first-round unbroken TianTing in this hand.
func (e *TurnEngine) TianTing(seat Seat) bool {
	if !seat.Valid() {
		return false
	}
	return e.tianTing[seat]
}
