package rules

import (
	"errors"
	"fmt"
	"strconv"
	"strings"
)

const (
	// DefaultBase is the QKMJ baseline base payment per winning hand.
	DefaultBase = 500
	// DefaultPerFan is the QKMJ baseline payment per fan (台).
	DefaultPerFan = 200
)

// Sentinel errors returned by Score.
var (
	ErrInvalidScoreContext = errors.New("invalid score context")
	ErrNotWinningHand      = errors.New("hand is not a valid winning hand")
)

// WinKind classifies how a winning hand was achieved. Zero is invalid.
type WinKind uint8

const (
	WinUnknown WinKind = iota
	WinDiscard
	WinSelfDraw
	WinRobKong
	WinSevenRobOne
	WinEightImmortals
)

// Valid reports whether k is a defined WinKind.
func (k WinKind) Valid() bool {
	return k >= WinDiscard && k <= WinEightImmortals
}

// IsAllPay reports whether all three other seats pay the winner.
func (k WinKind) IsAllPay() bool {
	return k == WinSelfDraw || k == WinEightImmortals
}

// FanID uniquely identifies each QKMJ baseline scoring fan (台型). Zero is invalid.
type FanID uint8

const (
	FanUnknown FanID = iota

	// Basic fans
	FanDealer
	FanDealerStreak
	FanConcealed
	FanSelfDraw
	FanConcealedSelfDraw

	// Situational fans
	FanRiichi
	FanKongFlowerWin
	FanLastTileSelfDraw
	FanLastTileDiscard
	FanRobKong
	FanAllClaimed
	FanTianTing
	FanHeavenlyWin
	FanEarthlyWin
	FanHumanWin

	// Wind fans
	FanEastWind
	FanSouthWind
	FanWestWind
	FanNorthWind
	FanDoubleEastWind
	FanDoubleSouthWind
	FanDoubleWestWind
	FanDoubleNorthWind
	FanSmallFourWinds
	FanBigFourWinds

	// Dragon & honor fans
	FanRedDragon
	FanWhiteDragon
	FanGreenDragon
	FanSmallThreeDragons
	FanBigThreeDragons
	FanAllHonors

	// Flower fans
	FanSeatFlower
	FanSeasonFlowers
	FanPlantFlowers
	FanSevenRobOne
	FanEightImmortals

	// Sequence fans
	FanPingHu
	FanPureDoubleChow
	FanTwoPureDoubleChow
	FanThreeSuitedSequences
	FanPureStraight
	FanAllSimples

	// Triplet & kong fans
	FanAllTriplets
	FanThreeConcealedTriplets
	FanFourConcealedTriplets
	FanFiveConcealedTriplets
	FanThreeKongs
	FanFourKongs
	FanThreeSuitedTriplets

	// Suit & terminal fans
	FanHalfFlush
	FanFullFlush
	FanMixedOutsideHand
	FanPureOutsideHand
	FanMixedTerminals
	FanPureTerminals
)

// Fan represents one credited scoring item and its fan value.
type Fan struct {
	ID     FanID
	Name   string
	Points int
}

// ScoreContext supplies all stateless inputs required to evaluate a winning hand and payments.
type ScoreContext struct {
	Concealed            []Tile
	Melds                []Meld
	Flowers              []Tile
	WinTile              Tile
	WinKind              WinKind
	Winner               Seat
	Payer                Seat
	Dealer               Seat
	DealerStreak         int
	RoundWind            Wind
	SeatWind             Wind
	IsAfterSupplement    bool
	IsLastDrawable       bool
	IsFirstRoundUnbroken bool
	IsRiichi             bool
	IsTianTing           bool
	Base                 int
	PerFan               int
}

// ScoreResult is the complete outcome of evaluating a winning hand.
type ScoreResult struct {
	Winner         Seat
	WinKind        WinKind
	Payer          Seat
	Fans           []Fan
	TotalFan       int
	DealerExtraFan int
	DealerStreak   int
	Decomposition  *Decomposition
	Deltas         map[Seat]int
}

// Score evaluates a winning hand under QKMJ baseline rules, selecting the highest-scoring
// decomposition and computing per-seat payments.
func Score(ctx ScoreContext) (ScoreResult, error) {
	if err := validateScoreContext(ctx); err != nil {
		return ScoreResult{}, err
	}

	if ctx.WinKind == WinSevenRobOne || ctx.WinKind == WinEightImmortals {
		fans := scoreSpecialFlowerWin(ctx)
		return buildScoreResult(ctx, fans, nil), nil
	}

	decomps := Decompose(ctx.Concealed, ctx.Melds, ctx.WinTile)
	if len(decomps) == 0 {
		return ScoreResult{}, ErrNotWinningHand
	}

	var bestFans []Fan
	bestTotal := -1
	bestIndex := 0
	for i := range decomps {
		fans := evaluateDecomposition(ctx, decomps[i])
		total := sumFans(fans)
		if total > bestTotal {
			bestTotal = total
			bestFans = fans
			bestIndex = i
		}
	}

	chosen := decomps[bestIndex]
	return buildScoreResult(ctx, bestFans, &chosen), nil
}

func validateScoreContext(ctx ScoreContext) error {
	if !ctx.WinKind.Valid() {
		return fmt.Errorf("win kind %d: %w", ctx.WinKind, ErrInvalidScoreContext)
	}
	if !ctx.Winner.Valid() || !ctx.Dealer.Valid() {
		return fmt.Errorf("winner=%v dealer=%v: %w", ctx.Winner, ctx.Dealer, ErrInvalidScoreContext)
	}
	if !ctx.RoundWind.Valid() || !ctx.SeatWind.Valid() {
		return fmt.Errorf("roundWind=%v seatWind=%v: %w", ctx.RoundWind, ctx.SeatWind, ErrInvalidScoreContext)
	}
	if ctx.DealerStreak < 0 {
		return fmt.Errorf("dealer streak %d: %w", ctx.DealerStreak, ErrInvalidScoreContext)
	}
	if ctx.Base < 0 || ctx.PerFan < 0 {
		return fmt.Errorf("base=%d perFan=%d: %w", ctx.Base, ctx.PerFan, ErrInvalidScoreContext)
	}
	if !ctx.WinKind.IsAllPay() && (!ctx.Payer.Valid() || ctx.Payer == ctx.Winner) {
		return fmt.Errorf("payer=%v winner=%v: %w", ctx.Payer, ctx.Winner, ErrInvalidScoreContext)
	}
	var seenFlower [TileMax + 1]bool
	for _, f := range ctx.Flowers {
		if !f.IsFlower() {
			return fmt.Errorf("non-flower tile %v in flowers: %w", f, ErrInvalidScoreContext)
		}
		if seenFlower[f] {
			return fmt.Errorf("duplicate flower tile %s: %w", f, ErrInvalidScoreContext)
		}
		seenFlower[f] = true
	}
	return nil
}

func scoreSpecialFlowerWin(ctx ScoreContext) []Fan {
	fans := make([]Fan, 0, 3)
	if ctx.WinKind == WinSevenRobOne {
		fans = append(fans, Fan{ID: FanSevenRobOne, Name: "七搶一", Points: 8})
	} else {
		fans = append(fans, Fan{ID: FanEightImmortals, Name: "八仙過海", Points: 16})
	}
	return appendDealerFans(fans, ctx)
}

func appendDealerFans(fans []Fan, ctx ScoreContext) []Fan {
	if ctx.Winner != ctx.Dealer {
		return fans
	}
	fans = append(fans, Fan{ID: FanDealer, Name: "莊家", Points: 1})
	if ctx.DealerStreak > 0 {
		fans = append(fans, Fan{
			ID:     FanDealerStreak,
			Name:   formatDealerStreakName(ctx.DealerStreak),
			Points: 2 * ctx.DealerStreak,
		})
	}
	return fans
}

func formatDealerStreakName(streak int) string {
	n := toFullWidthDigits(streak)
	return "連" + n + "拉" + n
}

func toFullWidthDigits(n int) string {
	raw := strconv.Itoa(n)
	var b strings.Builder
	b.Grow(len(raw) * 3)
	for i := range len(raw) {
		b.WriteRune('０' + rune(raw[i]-'0'))
	}
	return b.String()
}

type handFeatures struct {
	isConcealed                bool
	allSequences               bool
	allTriplets                bool
	hasWan                     bool
	hasSuo                     bool
	hasTong                    bool
	hasHonor                   bool
	hasNumber                  bool
	allHonors                  bool
	allTerminals               bool
	allTerminalOrHonor         bool
	allSimples                 bool
	everySetHasTerminal        bool
	everySetHasTerminalOrHonor bool
	concealedTriplets          int
	kongCount                  int
	windTriplets               [5]bool
	windTripletCount           int
	dragonTriplets             [3]bool
	dragonTripletCount         int
	seqCount                   [NonFlowerMax + 1]int
	tripletOrKong              [NonFlowerMax + 1]bool
}

func analyzeHand(ctx ScoreContext, d Decomposition) handFeatures {
	f := handFeatures{
		isConcealed:                isHandConcealed(ctx.Melds),
		allSequences:               true,
		allTriplets:                true,
		allHonors:                  true,
		allTerminals:               true,
		allTerminalOrHonor:         true,
		allSimples:                 true,
		everySetHasTerminal:        true,
		everySetHasTerminalOrHonor: true,
	}

	inspectTileSet(&f, []Tile{d.Pair})
	for i, g := range d.Groups {
		analyzeGroup(&f, ctx, d, i, g)
	}
	return f
}

func isHandConcealed(melds []Meld) bool {
	for _, m := range melds {
		if m.Kind.BreaksConcealed() {
			return false
		}
	}
	return true
}

func analyzeGroup(f *handFeatures, ctx ScoreContext, d Decomposition, idx int, g Group) {
	if g.Kind == GroupSequence {
		f.allTriplets = false
		f.seqCount[g.Base]++
		inspectTileSet(f, []Tile{g.Base, g.Base + 1, g.Base + 2})
		return
	}

	f.allSequences = false
	f.tripletOrKong[g.Base] = true
	if g.IsKong() {
		f.kongCount++
	}
	if isConcealedTriplet(ctx, d, idx, g) {
		f.concealedTriplets++
	}
	if g.Base.IsWind() {
		w := g.Base.WindValue()
		f.windTriplets[w] = true
		f.windTripletCount++
	}
	if g.Base.IsDragon() {
		f.dragonTriplets[g.Base-RedDragon] = true
		f.dragonTripletCount++
	}
	inspectTileSet(f, []Tile{g.Base})
}

func isConcealedTriplet(ctx ScoreContext, d Decomposition, idx int, g Group) bool {
	if g.FromMeld == MeldConcealedKong {
		return true
	}
	if g.FromMeld != MeldUnknown {
		return false
	}
	if ctx.WinKind != WinSelfDraw && d.WinRole == WinRoleGroup && d.WinGroupIndex == idx {
		return false
	}
	return true
}

func inspectTileSet(f *handFeatures, tiles []Tile) {
	setHasTerminal := false
	setHasTerminalOrHonor := false
	for _, t := range tiles {
		switch t.Suit() {
		case SuitWan:
			f.hasWan = true
			f.hasNumber = true
		case SuitSuo:
			f.hasSuo = true
			f.hasNumber = true
		case SuitTong:
			f.hasTong = true
			f.hasNumber = true
		case SuitWind, SuitDragon:
			f.hasHonor = true
		case SuitUnknown, SuitFlower:
		}
		if !t.IsHonor() {
			f.allHonors = false
		}
		if !t.IsTerminal() {
			f.allTerminals = false
		} else {
			setHasTerminal = true
			setHasTerminalOrHonor = true
		}
		if !t.IsTerminalOrHonor() {
			f.allTerminalOrHonor = false
		} else {
			setHasTerminalOrHonor = true
		}
		if !t.IsSimple() {
			f.allSimples = false
		}
	}
	if !setHasTerminal {
		f.everySetHasTerminal = false
	}
	if !setHasTerminalOrHonor {
		f.everySetHasTerminalOrHonor = false
	}
}

func evaluateDecomposition(ctx ScoreContext, d Decomposition) []Fan {
	f := analyzeHand(ctx, d)
	fans := make([]Fan, 0, 8)
	fans = appendBasicFans(fans, ctx, f)
	fans = appendSituationalFans(fans, ctx, f)
	fans = appendWindFans(fans, ctx, d, f)
	fans = appendDragonAndHonorFans(fans, d, f)
	fans = appendFlowerFans(fans, ctx)
	fans = appendSequenceFans(fans, ctx, d, f)
	fans = appendTripletAndKongFans(fans, f)
	fans = appendSuitAndTerminalFans(fans, f)
	return fans
}

func appendBasicFans(fans []Fan, ctx ScoreContext, f handFeatures) []Fan {
	fans = appendDealerFans(fans, ctx)
	isSelfDraw := ctx.WinKind == WinSelfDraw
	switch {
	case f.isConcealed && isSelfDraw:
		fans = append(fans, Fan{ID: FanConcealedSelfDraw, Name: "門清自摸", Points: 3})
	case f.isConcealed:
		fans = append(fans, Fan{ID: FanConcealed, Name: "門清", Points: 1})
	case isSelfDraw:
		fans = append(fans, Fan{ID: FanSelfDraw, Name: "自摸", Points: 1})
	}
	return fans
}

func appendSituationalFans(fans []Fan, ctx ScoreContext, f handFeatures) []Fan {
	isSelfDraw := ctx.WinKind == WinSelfDraw
	if f.isConcealed {
		switch {
		case ctx.IsTianTing:
			fans = append(fans, Fan{ID: FanTianTing, Name: "天聽", Points: 8})
		case ctx.IsRiichi:
			fans = append(fans, Fan{ID: FanRiichi, Name: "立直", Points: 1})
		}
	}
	if isSelfDraw && ctx.IsAfterSupplement {
		fans = append(fans, Fan{ID: FanKongFlowerWin, Name: "杠上開花", Points: 1})
	}
	if isSelfDraw && ctx.IsLastDrawable {
		fans = append(fans, Fan{ID: FanLastTileSelfDraw, Name: "海底摸月", Points: 1})
	}
	if ctx.WinKind == WinDiscard && ctx.IsLastDrawable {
		fans = append(fans, Fan{ID: FanLastTileDiscard, Name: "河底撈魚", Points: 1})
	}
	if ctx.WinKind == WinRobKong {
		fans = append(fans, Fan{ID: FanRobKong, Name: "搶杠", Points: 1})
	}
	if !isSelfDraw && len(ctx.Concealed) == 1 {
		fans = append(fans, Fan{ID: FanAllClaimed, Name: "全求人", Points: 2})
	}
	return appendFirstRoundWinFan(fans, ctx, isSelfDraw)
}

func appendFirstRoundWinFan(fans []Fan, ctx ScoreContext, isSelfDraw bool) []Fan {
	if !ctx.IsFirstRoundUnbroken {
		return fans
	}
	switch {
	case isSelfDraw && ctx.Winner == ctx.Dealer:
		return append(fans, Fan{ID: FanHeavenlyWin, Name: "天胡", Points: 16})
	case isSelfDraw && ctx.Winner != ctx.Dealer:
		return append(fans, Fan{ID: FanEarthlyWin, Name: "地胡", Points: 16})
	case !isSelfDraw && ctx.Winner != ctx.Dealer:
		return append(fans, Fan{ID: FanHumanWin, Name: "人胡", Points: 16})
	default:
		return fans
	}
}

func appendWindFans(fans []Fan, ctx ScoreContext, d Decomposition, f handFeatures) []Fan {
	if f.windTripletCount == 4 {
		return append(fans, Fan{ID: FanBigFourWinds, Name: "大四喜", Points: 16})
	}
	if f.windTripletCount == 3 && d.Pair.IsWind() {
		return append(fans, Fan{ID: FanSmallFourWinds, Name: "小四喜", Points: 8})
	}

	type windSpec struct {
		wind       Wind
		singleID   FanID
		singleName string
		doubleID   FanID
		doubleName string
	}
	specs := [4]windSpec{
		{WindEast, FanEastWind, "東風", FanDoubleEastWind, "東風東"},
		{WindSouth, FanSouthWind, "南風", FanDoubleSouthWind, "南風南"},
		{WindWest, FanWestWind, "西風", FanDoubleWestWind, "西風西"},
		{WindNorth, FanNorthWind, "北風", FanDoubleNorthWind, "北風北"},
	}
	for _, s := range specs {
		if !f.windTriplets[s.wind] {
			continue
		}
		isRound := ctx.RoundWind == s.wind
		isSeat := ctx.SeatWind == s.wind
		if isRound && isSeat {
			fans = append(fans, Fan{ID: s.doubleID, Name: s.doubleName, Points: 2})
		} else if isRound || isSeat {
			fans = append(fans, Fan{ID: s.singleID, Name: s.singleName, Points: 1})
		}
	}
	return fans
}

func appendDragonAndHonorFans(fans []Fan, d Decomposition, f handFeatures) []Fan {
	switch {
	case f.dragonTripletCount == 3:
		fans = append(fans, Fan{ID: FanBigThreeDragons, Name: "大三元", Points: 8})
	case f.dragonTripletCount == 2 && d.Pair.IsDragon():
		fans = append(fans, Fan{ID: FanSmallThreeDragons, Name: "小三元", Points: 4})
	default:
		if f.dragonTriplets[0] {
			fans = append(fans, Fan{ID: FanRedDragon, Name: "紅中", Points: 1})
		}
		if f.dragonTriplets[1] {
			fans = append(fans, Fan{ID: FanWhiteDragon, Name: "白板", Points: 1})
		}
		if f.dragonTriplets[2] {
			fans = append(fans, Fan{ID: FanGreenDragon, Name: "青發", Points: 1})
		}
	}

	if f.allHonors {
		fans = append(fans, Fan{ID: FanAllHonors, Name: "字一色", Points: 8})
	}
	return fans
}

func appendFlowerFans(fans []Fan, ctx ScoreContext) []Fan {
	if len(ctx.Flowers) == 0 {
		return fans
	}
	var has [TileMax + 1]bool
	for _, fl := range ctx.Flowers {
		has[fl] = true
	}
	allSeasons := has[FlowerSpring] && has[FlowerSummer] && has[FlowerAutumn] && has[FlowerWinter]
	allPlants := has[FlowerPlum] && has[FlowerOrchid] && has[FlowerChrysanthemum] && has[FlowerBamboo]

	if allSeasons {
		fans = append(fans, Fan{ID: FanSeasonFlowers, Name: "春夏秋冬", Points: 2})
	}
	if allPlants {
		fans = append(fans, Fan{ID: FanPlantFlowers, Name: "梅蘭菊竹", Points: 2})
	}

	if pts := countSeatFlowerPoints(ctx.Flowers, ctx.SeatWind, allSeasons, allPlants); pts > 0 {
		fans = append(fans, Fan{ID: FanSeatFlower, Name: "花牌", Points: pts})
	}
	return fans
}

func countSeatFlowerPoints(flowers []Tile, seatWind Wind, allSeasons, allPlants bool) int {
	pts := 0
	for _, fl := range flowers {
		if fl.FlowerWind() != seatWind {
			continue
		}
		if (fl.IsSeasonFlower() && allSeasons) || (fl.IsPlantFlower() && allPlants) {
			continue
		}
		pts++
	}
	return pts
}

func appendSequenceFans(fans []Fan, ctx ScoreContext, d Decomposition, f handFeatures) []Fan {
	if isPingHu(ctx, d, f) {
		fans = append(fans, Fan{ID: FanPingHu, Name: "平胡", Points: 2})
	}
	if f.isConcealed {
		doubleSeqPairs := 0
		for t := Wan1; t <= Tong9; t++ {
			doubleSeqPairs += f.seqCount[t] / 2
		}
		if doubleSeqPairs >= 2 {
			fans = append(fans, Fan{ID: FanTwoPureDoubleChow, Name: "二杯口", Points: 2})
		} else if doubleSeqPairs == 1 {
			fans = append(fans, Fan{ID: FanPureDoubleChow, Name: "一杯口", Points: 1})
		}
	}
	if hasThreeSuitedSequences(f) {
		pts := 1
		if f.isConcealed {
			pts = 2
		}
		fans = append(fans, Fan{ID: FanThreeSuitedSequences, Name: "三色同順", Points: pts})
	}
	if hasPureStraight(f) {
		pts := 1
		if f.isConcealed {
			pts = 2
		}
		fans = append(fans, Fan{ID: FanPureStraight, Name: "一條龍", Points: pts})
	}
	if f.allSimples {
		fans = append(fans, Fan{ID: FanAllSimples, Name: "斷麼九", Points: 1})
	}
	return fans
}

func isPingHu(ctx ScoreContext, d Decomposition, f handFeatures) bool {
	return len(ctx.Flowers) == 0 &&
		f.allSequences &&
		!d.Pair.IsHonor() &&
		ctx.WinKind != WinSelfDraw &&
		d.WinRole != WinRolePair &&
		d.IsTwoSidedWait(ctx.WinTile)
}

func hasThreeSuitedSequences(f handFeatures) bool {
	for offset := range 7 {
		if f.seqCount[Wan1+Tile(offset)] > 0 &&
			f.seqCount[Suo1+Tile(offset)] > 0 &&
			f.seqCount[Tong1+Tile(offset)] > 0 {
			return true
		}
	}
	return false
}

func hasPureStraight(f handFeatures) bool {
	bases := [3]Tile{Wan1, Suo1, Tong1}
	for _, b := range bases {
		if f.seqCount[b] > 0 && f.seqCount[b+3] > 0 && f.seqCount[b+6] > 0 {
			return true
		}
	}
	return false
}

func appendTripletAndKongFans(fans []Fan, f handFeatures) []Fan {
	// All-triplets (碰碰胡) is suppressed by 五暗刻, 字一色, 混老頭, and 清老頭.
	suppressedByHigher := f.concealedTriplets == 5 || f.allHonors || f.allTerminalOrHonor
	if f.allTriplets && !suppressedByHigher {
		fans = append(fans, Fan{ID: FanAllTriplets, Name: "碰碰胡", Points: 4})
	}

	switch f.concealedTriplets {
	case 5:
		fans = append(fans, Fan{ID: FanFiveConcealedTriplets, Name: "五暗刻", Points: 8})
	case 4:
		fans = append(fans, Fan{ID: FanFourConcealedTriplets, Name: "四暗刻", Points: 6})
	case 3:
		fans = append(fans, Fan{ID: FanThreeConcealedTriplets, Name: "三暗刻", Points: 2})
	}

	switch f.kongCount {
	case 4:
		fans = append(fans, Fan{ID: FanFourKongs, Name: "四杠子", Points: 6})
	case 3:
		fans = append(fans, Fan{ID: FanThreeKongs, Name: "三杠子", Points: 2})
	}

	if hasThreeSuitedTriplets(f) {
		pts := 1
		if f.isConcealed {
			pts = 2
		}
		fans = append(fans, Fan{ID: FanThreeSuitedTriplets, Name: "三色同刻", Points: pts})
	}
	return fans
}

func hasThreeSuitedTriplets(f handFeatures) bool {
	for offset := range 9 {
		if f.tripletOrKong[Wan1+Tile(offset)] &&
			f.tripletOrKong[Suo1+Tile(offset)] &&
			f.tripletOrKong[Tong1+Tile(offset)] {
			return true
		}
	}
	return false
}

func appendSuitAndTerminalFans(fans []Fan, f handFeatures) []Fan {
	suitCount := boolToInt(f.hasWan) + boolToInt(f.hasSuo) + boolToInt(f.hasTong)
	if suitCount == 1 {
		if !f.hasHonor {
			fans = append(fans, Fan{ID: FanFullFlush, Name: "清一色", Points: 8})
		} else {
			fans = append(fans, Fan{ID: FanHalfFlush, Name: "混一色", Points: 4})
		}
	}

	switch {
	case f.allTerminals:
		fans = append(fans, Fan{ID: FanPureTerminals, Name: "清老頭", Points: 8})
	case f.allTerminalOrHonor && f.hasNumber:
		fans = append(fans, Fan{ID: FanMixedTerminals, Name: "混老頭", Points: 4})
	case f.everySetHasTerminal && !f.hasHonor:
		fans = append(fans, Fan{ID: FanPureOutsideHand, Name: "純全帶麼", Points: 4})
	case f.everySetHasTerminalOrHonor && f.hasNumber:
		pts := 1
		if f.isConcealed {
			pts = 2
		}
		fans = append(fans, Fan{ID: FanMixedOutsideHand, Name: "混全帶麼", Points: pts})
	}
	return fans
}

func boolToInt(b bool) int {
	if b {
		return 1
	}
	return 0
}

func sumFans(fans []Fan) int {
	total := 0
	for _, f := range fans {
		total += f.Points
	}
	return total
}

func buildScoreResult(ctx ScoreContext, fans []Fan, decomp *Decomposition) ScoreResult {
	base := ctx.Base
	perFan := ctx.PerFan
	if base == 0 && perFan == 0 {
		base = DefaultBase
		perFan = DefaultPerFan
	}

	totalFan := sumFans(fans)
	deltas := map[Seat]int{
		Seat1: 0,
		Seat2: 0,
		Seat3: 0,
		Seat4: 0,
	}

	dealerExtra := 0
	if ctx.Winner != ctx.Dealer && (ctx.WinKind.IsAllPay() || ctx.Payer == ctx.Dealer) {
		dealerExtra = 1 + 2*ctx.DealerStreak
	}

	if ctx.WinKind.IsAllPay() {
		for _, s := range AllSeats {
			if s == ctx.Winner {
				continue
			}
			fan := totalFan
			if s == ctx.Dealer {
				fan += dealerExtra
			}
			pay := base + fan*perFan
			deltas[s] = -pay
			deltas[ctx.Winner] += pay
		}
	} else {
		fan := totalFan
		if ctx.Payer == ctx.Dealer {
			fan += dealerExtra
		}
		pay := base + fan*perFan
		deltas[ctx.Payer] = -pay
		deltas[ctx.Winner] = pay
	}

	payer := ctx.Payer
	if ctx.WinKind.IsAllPay() {
		payer = SeatUnknown
	}

	return ScoreResult{
		Winner:         ctx.Winner,
		WinKind:        ctx.WinKind,
		Payer:          payer,
		Fans:           fans,
		TotalFan:       totalFan,
		DealerExtraFan: dealerExtra,
		DealerStreak:   ctx.DealerStreak,
		Decomposition:  decomp,
		Deltas:         deltas,
	}
}
