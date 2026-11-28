package rules_test

import (
	"cmp"
	"slices"
	"strings"
	"testing"

	"github.com/gjchentw/go-qkmj/internal/rules"
)

type fanExpectation struct {
	id     rules.FanID
	points int
}

// TestScore_QKMJGoldenSamples verifies all 59 hands from qkmj scoring-samples.md
// (A01-A58, B01), C01-C03 (rob kong, seven rob one, eight immortals), plus
// ping-hu and concealed-triplet negative edge cases.
func TestScore_QKMJGoldenSamples(t *testing.T) {
	t.Parallel()

	type sampleCase struct {
		name      string
		concealed string
		melds     []rules.Meld
		flowers   string
		winTile   string
		winKind   rules.WinKind
		winner    rules.Seat
		payer     rules.Seat
		dealer    rules.Seat
		streak    int
		roundWind rules.Wind
		seatWind  rules.Wind
		afterSupp bool
		lastDraw  bool
		firstRnd  bool
		wantFans  []fanExpectation
		wantTotal int
	}

	chow := func(base rules.Tile) rules.Meld {
		return rules.Meld{Kind: rules.MeldChow, Tiles: []rules.Tile{base, base + 1, base + 2}}
	}
	pong := func(tile rules.Tile) rules.Meld {
		return rules.Meld{Kind: rules.MeldPong, Tiles: []rules.Tile{tile, tile, tile}}
	}
	exposedKong := func(tile rules.Tile) rules.Meld {
		return rules.Meld{Kind: rules.MeldExposedKong, Tiles: []rules.Tile{tile, tile, tile, tile}}
	}
	addedKong := func(tile rules.Tile) rules.Meld {
		return rules.Meld{Kind: rules.MeldAddedKong, Tiles: []rules.Tile{tile, tile, tile, tile}}
	}
	concealedKong := func(tile rules.Tile) rules.Meld {
		return rules.Meld{Kind: rules.MeldConcealedKong, Tiles: []rules.Tile{tile, tile, tile, tile}}
	}

	samples := []sampleCase{
		{
			name:      "A01_門清_胡別人的牌",
			concealed: "一萬 二萬 三萬 四萬 五萬 六萬 七索 八索 九索 二筒 三筒 四筒 六筒 七筒 白板 白板",
			winTile:   "八筒",
			wantFans:  []fanExpectation{{rules.FanConcealed, 1}},
			wantTotal: 1,
		},
		{
			name:      "A02_門清自摸",
			concealed: "一萬 二萬 三萬 四萬 五萬 六萬 七索 八索 九索 二筒 三筒 四筒 六筒 七筒 白板 白板",
			winTile:   "八筒",
			winKind:   rules.WinSelfDraw,
			wantFans:  []fanExpectation{{rules.FanConcealedSelfDraw, 3}},
			wantTotal: 3,
		},
		{
			name:      "A03_自摸_有碰牌",
			concealed: "一萬 二萬 三萬 四萬 五萬 六萬 九索 九索 二筒 三筒 四筒 六筒 七筒",
			melds:     []rules.Meld{pong(rules.Suo2)},
			winTile:   "八筒",
			winKind:   rules.WinSelfDraw,
			wantFans:  []fanExpectation{{rules.FanSelfDraw, 1}},
			wantTotal: 1,
		},
		{
			name:      "A04_莊家連二莊胡牌",
			concealed: "一萬 二萬 三萬 四萬 五萬 六萬 七索 八索 九索 二筒 三筒 四筒 六筒 七筒 白板 白板",
			winTile:   "八筒",
			winner:    rules.Seat1,
			streak:    2,
			wantFans:  []fanExpectation{{rules.FanDealer, 1}, {rules.FanDealerStreak, 4}, {rules.FanConcealed, 1}},
			wantTotal: 6,
		},
		{
			name:      "A05_斷麼九",
			concealed: "二萬 三萬 四萬 五萬 六萬 七萬 八索 八索 三筒 四筒 五筒 六筒 七筒",
			melds:     []rules.Meld{pong(rules.Suo2)},
			winTile:   "八筒",
			wantFans:  []fanExpectation{{rules.FanAllSimples, 1}},
			wantTotal: 1,
		},
		{
			name:      "A06_平胡",
			concealed: "二萬 三萬 四萬 五萬 六萬 七萬 二索 三索 六索 七索 八索 三筒 四筒 五筒 九筒 九筒",
			winTile:   "四索",
			wantFans:  []fanExpectation{{rules.FanPingHu, 2}, {rules.FanConcealed, 1}},
			wantTotal: 3,
		},
		{
			name:      "A07_一杯口",
			concealed: "二萬 二萬 三萬 三萬 四萬 四萬 四索 五索 七索 八索 九索 五筒 六筒 七筒 白板 白板",
			winTile:   "六索",
			wantFans:  []fanExpectation{{rules.FanPureDoubleChow, 1}, {rules.FanConcealed, 1}},
			wantTotal: 2,
		},
		{
			name:      "A08_二杯口",
			concealed: "二萬 二萬 三萬 三萬 四萬 四萬 七索 八索 五筒 五筒 六筒 六筒 七筒 七筒 白板 白板",
			winTile:   "九索",
			wantFans:  []fanExpectation{{rules.FanTwoPureDoubleChow, 2}, {rules.FanConcealed, 1}},
			wantTotal: 3,
		},
		{
			name:      "A09_三色同順_門清",
			concealed: "二萬 三萬 四萬 六萬 七萬 八萬 二索 三索 四索 五索 六索 二筒 三筒 四筒 白板 白板",
			winTile:   "七索",
			wantFans:  []fanExpectation{{rules.FanThreeSuitedSequences, 2}, {rules.FanConcealed, 1}},
			wantTotal: 3,
		},
		{
			name:      "A10_三色同順_吃過牌",
			concealed: "二萬 三萬 四萬 六萬 七萬 八萬 二索 三索 四索 五索 六索 白板 白板",
			melds:     []rules.Meld{chow(rules.Tong2)},
			winTile:   "七索",
			wantFans:  []fanExpectation{{rules.FanThreeSuitedSequences, 1}},
			wantTotal: 1,
		},
		{
			name:      "A11_一條龍_門清",
			concealed: "一萬 二萬 三萬 四萬 五萬 六萬 七萬 八萬 九萬 五索 六索 二筒 三筒 四筒 白板 白板",
			winTile:   "七索",
			wantFans:  []fanExpectation{{rules.FanPureStraight, 2}, {rules.FanConcealed, 1}},
			wantTotal: 3,
		},
		{
			name:      "A12_一條龍_吃過牌",
			concealed: "一萬 二萬 三萬 七萬 八萬 九萬 五索 六索 二筒 三筒 四筒 白板 白板",
			melds:     []rules.Meld{chow(rules.Wan4)},
			winTile:   "七索",
			wantFans:  []fanExpectation{{rules.FanPureStraight, 1}},
			wantTotal: 1,
		},
		{
			name:      "A13_碰碰胡",
			concealed: "八萬 八萬 八萬 九萬 九萬 七索 七索 三筒 三筒 三筒",
			melds:     []rules.Meld{pong(rules.Suo2), pong(rules.Tong5)},
			winTile:   "七索",
			wantFans:  []fanExpectation{{rules.FanAllTriplets, 4}},
			wantTotal: 4,
		},
		{
			name:      "A14_三暗刻",
			concealed: "一萬 一萬 一萬 二萬 三萬 四萬 七索 七索 七索 九索 九索 四筒 四筒 四筒 五筒 六筒",
			winTile:   "七筒",
			wantFans:  []fanExpectation{{rules.FanThreeConcealedTriplets, 2}, {rules.FanConcealed, 1}},
			wantTotal: 3,
		},
		{
			name:      "A15_四暗刻_胡牌湊順子不扣同牌種暗刻",
			concealed: "一萬 一萬 一萬 九萬 九萬 五索 六索 七索 七索 七索 二筒 二筒 二筒 四筒 四筒 四筒",
			winTile:   "七索",
			wantFans:  []fanExpectation{{rules.FanFourConcealedTriplets, 6}, {rules.FanConcealed, 1}},
			wantTotal: 7,
		},
		{
			name:      "A16_五暗刻_自摸",
			concealed: "一萬 一萬 一萬 五萬 五萬 七索 七索 七索 九索 九索 二筒 二筒 二筒 四筒 四筒 四筒",
			winTile:   "九索",
			winKind:   rules.WinSelfDraw,
			wantFans:  []fanExpectation{{rules.FanFiveConcealedTriplets, 8}, {rules.FanConcealedSelfDraw, 3}},
			wantTotal: 11,
		},
		{
			name:      "A17_三杠子",
			concealed: "二萬 三萬 四萬 九索 九索 五筒 六筒",
			melds:     []rules.Meld{exposedKong(rules.Suo2), concealedKong(rules.Tong8), addedKong(rules.Wan5)},
			winTile:   "七筒",
			wantFans:  []fanExpectation{{rules.FanThreeKongs, 2}},
			wantTotal: 2,
		},
		{
			name:      "A18_四杠子",
			concealed: "九索 九索 五筒 六筒",
			melds:     []rules.Meld{exposedKong(rules.Suo2), exposedKong(rules.Tong3), concealedKong(rules.Tong8), addedKong(rules.Wan5)},
			winTile:   "七筒",
			wantFans:  []fanExpectation{{rules.FanFourKongs, 6}},
			wantTotal: 6,
		},
		{
			name:      "A19_三色同刻",
			concealed: "六萬 七萬 八萬 三索 三索 三索 四索 五索 三筒 三筒 三筒 白板 白板",
			melds:     []rules.Meld{pong(rules.Wan3)},
			winTile:   "六索",
			wantFans:  []fanExpectation{{rules.FanThreeSuitedTriplets, 2}},
			wantTotal: 2,
		},
		{
			name:      "A20_混一色",
			concealed: "二萬 三萬 三萬 四萬 五萬 六萬 七萬 八萬 九萬 九萬 南風 南風 南風",
			melds:     []rules.Meld{pong(rules.Wan2)},
			winTile:   "四萬",
			wantFans:  []fanExpectation{{rules.FanHalfFlush, 4}},
			wantTotal: 4,
		},
		{
			name:      "A21_清一色",
			concealed: "二萬 三萬 三萬 四萬 五萬 六萬 七萬 七萬 八萬 八萬 九萬 九萬 九萬",
			melds:     []rules.Meld{pong(rules.Wan2)},
			winTile:   "四萬",
			wantFans:  []fanExpectation{{rules.FanFullFlush, 8}},
			wantTotal: 8,
		},
		{
			name:      "A22_混全帶麼_門清",
			concealed: "一萬 二萬 三萬 七萬 八萬 九萬 一索 一索 一索 七索 八索 九索 一筒 二筒 南風 南風",
			winTile:   "三筒",
			wantFans:  []fanExpectation{{rules.FanMixedOutsideHand, 2}, {rules.FanConcealed, 1}},
			wantTotal: 3,
		},
		{
			name:      "A23_混全帶麼_吃過牌",
			concealed: "七萬 八萬 九萬 一索 一索 一索 七索 八索 九索 一筒 二筒 南風 南風",
			melds:     []rules.Meld{chow(rules.Wan1)},
			winTile:   "三筒",
			wantFans:  []fanExpectation{{rules.FanMixedOutsideHand, 1}},
			wantTotal: 1,
		},
		{
			name:      "A24_純全帶麼",
			concealed: "一萬 二萬 三萬 七萬 八萬 九萬 一索 一索 一索 七索 八索 九索 一筒 二筒 九筒 九筒",
			winTile:   "三筒",
			wantFans:  []fanExpectation{{rules.FanPureOutsideHand, 4}, {rules.FanConcealed, 1}},
			wantTotal: 5,
		},
		{
			name:      "A25_混老頭",
			concealed: "九索 九索 九索 一筒 一筒 南風 南風 南風 白板 白板",
			melds:     []rules.Meld{pong(rules.Wan1), pong(rules.Tong9)},
			winTile:   "一筒",
			wantFans:  []fanExpectation{{rules.FanMixedTerminals, 4}},
			wantTotal: 4,
		},
		{
			name:      "A26_混老頭_碰出字牌",
			concealed: "九索 九索 九索 一筒 一筒 九筒 九筒 九筒 白板 白板",
			melds:     []rules.Meld{pong(rules.Wan1), pong(rules.SouthWind)},
			winTile:   "一筒",
			wantFans:  []fanExpectation{{rules.FanMixedTerminals, 4}},
			wantTotal: 4,
		},
		{
			name:      "A27_清老頭",
			concealed: "一索 一索 九索 九索 九筒 九筒 九筒",
			melds:     []rules.Meld{pong(rules.Wan1), pong(rules.Wan9), pong(rules.Tong1)},
			winTile:   "一索",
			wantFans:  []fanExpectation{{rules.FanPureTerminals, 8}, {rules.FanThreeSuitedTriplets, 2}},
			wantTotal: 10,
		},
		{
			name:      "A28_紅中",
			concealed: "二萬 三萬 四萬 三索 四索 五索 六索 七索 五筒 六筒 七筒 九筒 九筒",
			melds:     []rules.Meld{pong(rules.RedDragon)},
			winTile:   "八索",
			wantFans:  []fanExpectation{{rules.FanRedDragon, 1}},
			wantTotal: 1,
		},
		{
			name:      "A29_小三元",
			concealed: "二萬 三萬 四萬 七索 八索 九索 五筒 六筒 白板 白板 白板 青發 青發",
			melds:     []rules.Meld{pong(rules.RedDragon)},
			winTile:   "七筒",
			wantFans:  []fanExpectation{{rules.FanSmallThreeDragons, 4}},
			wantTotal: 4,
		},
		{
			name:      "A30_大三元",
			concealed: "二萬 三萬 四萬 九索 九索 五筒 六筒 青發 青發 青發",
			melds:     []rules.Meld{pong(rules.RedDragon), pong(rules.WhiteDragon)},
			winTile:   "七筒",
			wantFans:  []fanExpectation{{rules.FanBigThreeDragons, 8}},
			wantTotal: 8,
		},
		{
			name:      "A31_東風_圈風",
			concealed: "二萬 三萬 四萬 三索 四索 五索 六索 七索 五筒 六筒 七筒 九筒 九筒",
			melds:     []rules.Meld{pong(rules.EastWind)},
			winTile:   "八索",
			wantFans:  []fanExpectation{{rules.FanEastWind, 1}},
			wantTotal: 1,
		},
		{
			name:      "A32_北風_門風",
			concealed: "二萬 三萬 四萬 三索 四索 五索 六索 七索 五筒 六筒 七筒 九筒 九筒",
			melds:     []rules.Meld{pong(rules.NorthWind)},
			winTile:   "八索",
			wantFans:  []fanExpectation{{rules.FanNorthWind, 1}},
			wantTotal: 1,
		},
		{
			name:      "A33_東風東",
			concealed: "二萬 三萬 四萬 三索 四索 五索 六索 七索 五筒 六筒 七筒 九筒 九筒",
			melds:     []rules.Meld{pong(rules.EastWind)},
			winTile:   "八索",
			seatWind:  rules.WindEast,
			wantFans:  []fanExpectation{{rules.FanDoubleEastWind, 2}},
			wantTotal: 2,
		},
		{
			name:      "A34_北風北",
			concealed: "二萬 三萬 四萬 三索 四索 五索 六索 七索 五筒 六筒 七筒 九筒 九筒",
			melds:     []rules.Meld{pong(rules.NorthWind)},
			winTile:   "八索",
			roundWind: rules.WindNorth,
			seatWind:  rules.WindNorth,
			wantFans:  []fanExpectation{{rules.FanDoubleNorthWind, 2}},
			wantTotal: 2,
		},
		{
			name:      "A35_小四喜",
			concealed: "二萬 三萬 四萬 五筒 六筒 北風 北風",
			melds:     []rules.Meld{pong(rules.EastWind), pong(rules.SouthWind), pong(rules.WestWind)},
			winTile:   "七筒",
			wantFans:  []fanExpectation{{rules.FanSmallFourWinds, 8}},
			wantTotal: 8,
		},
		{
			name:      "A36_大四喜",
			concealed: "九索 九索 五筒 六筒",
			melds:     []rules.Meld{pong(rules.EastWind), pong(rules.SouthWind), pong(rules.WestWind), pong(rules.NorthWind)},
			winTile:   "七筒",
			wantFans:  []fanExpectation{{rules.FanBigFourWinds, 16}},
			wantTotal: 16,
		},
		{
			name:      "A37_字一色_門清",
			concealed: "東風 東風 東風 南風 南風 南風 西風 西風 西風 紅中 紅中 紅中 白板 白板 青發 青發",
			winTile:   "白板",
			wantFans: []fanExpectation{
				{rules.FanAllHonors, 8},
				{rules.FanSmallThreeDragons, 4},
				{rules.FanEastWind, 1},
				{rules.FanConcealed, 1},
				{rules.FanFourConcealedTriplets, 6},
			},
			wantTotal: 20,
		},
		{
			name:      "A38_字一色_碰出兩組",
			concealed: "西風 西風 西風 紅中 紅中 紅中 白板 白板 青發 青發",
			melds:     []rules.Meld{pong(rules.EastWind), pong(rules.SouthWind)},
			winTile:   "白板",
			wantFans: []fanExpectation{
				{rules.FanAllHonors, 8},
				{rules.FanSmallThreeDragons, 4},
				{rules.FanEastWind, 1},
			},
			wantTotal: 13,
		},
		{
			name:      "A39_花牌_門風北有冬竹春",
			concealed: "二萬 三萬 四萬 五萬 六萬 七萬 八索 八索 三筒 四筒 五筒 六筒 七筒",
			melds:     []rules.Meld{pong(rules.Suo2)},
			flowers:   "春１ 冬４ 竹４",
			winTile:   "八筒",
			wantFans:  []fanExpectation{{rules.FanAllSimples, 1}, {rules.FanSeatFlower, 2}},
			wantTotal: 3,
		},
		{
			name:      "A40_春夏秋冬",
			concealed: "二萬 三萬 四萬 五萬 六萬 七萬 八索 八索 三筒 四筒 五筒 六筒 七筒",
			melds:     []rules.Meld{pong(rules.Suo2)},
			flowers:   "春１ 夏２ 秋３ 冬４",
			winTile:   "八筒",
			wantFans:  []fanExpectation{{rules.FanAllSimples, 1}, {rules.FanSeasonFlowers, 2}},
			wantTotal: 3,
		},
		{
			name:      "A41_梅蘭菊竹",
			concealed: "二萬 三萬 四萬 五萬 六萬 七萬 八索 八索 三筒 四筒 五筒 六筒 七筒",
			melds:     []rules.Meld{pong(rules.Suo2)},
			flowers:   "梅１ 蘭２ 菊３ 竹４",
			winTile:   "八筒",
			wantFans:  []fanExpectation{{rules.FanAllSimples, 1}, {rules.FanPlantFlowers, 2}},
			wantTotal: 3,
		},
		{
			name:      "A42_全求人",
			concealed: "五萬",
			melds:     []rules.Meld{chow(rules.Wan2), chow(rules.Tong5), pong(rules.Suo2), chow(rules.Suo3), pong(rules.Tong9)},
			winTile:   "五萬",
			wantFans:  []fanExpectation{{rules.FanAllClaimed, 2}},
			wantTotal: 2,
		},
		{
			name:      "A43_杠上開花_暗槓後補牌自摸",
			concealed: "二萬 三萬 四萬 三索 四索 五索 六索 七索 五筒 六筒 七筒 九筒 九筒",
			melds:     []rules.Meld{concealedKong(rules.Suo2)},
			winTile:   "八索",
			winKind:   rules.WinSelfDraw,
			afterSupp: true,
			wantFans:  []fanExpectation{{rules.FanKongFlowerWin, 1}, {rules.FanConcealedSelfDraw, 3}},
			wantTotal: 4,
		},
		{
			name:      "A44_海底摸月",
			concealed: "一萬 二萬 三萬 四萬 五萬 六萬 九索 九索 二筒 三筒 四筒 六筒 七筒",
			melds:     []rules.Meld{pong(rules.Suo2)},
			winTile:   "八筒",
			winKind:   rules.WinSelfDraw,
			lastDraw:  true,
			wantFans:  []fanExpectation{{rules.FanSelfDraw, 1}, {rules.FanLastTileSelfDraw, 1}},
			wantTotal: 2,
		},
		{
			name:      "A45_河底撈魚",
			concealed: "一萬 二萬 三萬 四萬 五萬 六萬 九索 九索 二筒 三筒 四筒 六筒 七筒",
			melds:     []rules.Meld{pong(rules.Suo2)},
			winTile:   "八筒",
			lastDraw:  true,
			wantFans:  []fanExpectation{{rules.FanLastTileDiscard, 1}},
			wantTotal: 1,
		},
		{
			name:      "A46_天胡",
			concealed: "一萬 二萬 三萬 四萬 五萬 六萬 七索 八索 九索 二筒 三筒 四筒 六筒 七筒 白板 白板",
			winTile:   "八筒",
			winKind:   rules.WinSelfDraw,
			winner:    rules.Seat1,
			firstRnd:  true,
			wantFans:  []fanExpectation{{rules.FanHeavenlyWin, 16}, {rules.FanDealer, 1}, {rules.FanConcealedSelfDraw, 3}},
			wantTotal: 20,
		},
		{
			name:      "A47_地胡",
			concealed: "一萬 二萬 三萬 四萬 五萬 六萬 七索 八索 九索 二筒 三筒 四筒 六筒 七筒 白板 白板",
			winTile:   "八筒",
			winKind:   rules.WinSelfDraw,
			firstRnd:  true,
			wantFans:  []fanExpectation{{rules.FanEarthlyWin, 16}, {rules.FanConcealedSelfDraw, 3}},
			wantTotal: 19,
		},
		{
			name:      "A48_人胡",
			concealed: "一萬 二萬 三萬 四萬 五萬 六萬 七索 八索 九索 二筒 三筒 四筒 六筒 七筒 白板 白板",
			winTile:   "八筒",
			firstRnd:  true,
			wantFans:  []fanExpectation{{rules.FanHumanWin, 16}, {rules.FanConcealed, 1}},
			wantTotal: 17,
		},
		{
			name:      "A49_暗槓後胡別人的牌_門清",
			concealed: "二萬 三萬 四萬 三索 四索 五索 六索 七索 五筒 六筒 七筒 九筒 九筒",
			melds:     []rules.Meld{concealedKong(rules.Suo2)},
			winTile:   "八索",
			wantFans:  []fanExpectation{{rules.FanConcealed, 1}},
			wantTotal: 1,
		},
		{
			name:      "A50_補花後自摸_沒有槓",
			concealed: "一萬 二萬 三萬 四萬 五萬 六萬 九索 九索 二筒 三筒 四筒 六筒 七筒",
			melds:     []rules.Meld{pong(rules.Suo2)},
			winTile:   "八筒",
			winKind:   rules.WinSelfDraw,
			afterSupp: true,
			wantFans:  []fanExpectation{{rules.FanSelfDraw, 1}, {rules.FanKongFlowerWin, 1}},
			wantTotal: 2,
		},
		{
			name:      "A51_四暗刻_胡的牌與暗刻無關",
			concealed: "一萬 一萬 一萬 九萬 九萬 五索 六索 七索 七索 七索 二筒 二筒 二筒 四筒 四筒 四筒",
			winTile:   "四索",
			wantFans:  []fanExpectation{{rules.FanFourConcealedTriplets, 6}, {rules.FanConcealed, 1}},
			wantTotal: 7,
		},
		{
			name:      "A52_南風_圈風",
			concealed: "二萬 三萬 四萬 三索 四索 五索 六索 七索 五筒 六筒 七筒 九筒 九筒",
			melds:     []rules.Meld{pong(rules.SouthWind)},
			winTile:   "八索",
			roundWind: rules.WindSouth,
			wantFans:  []fanExpectation{{rules.FanSouthWind, 1}},
			wantTotal: 1,
		},
		{
			name:      "A53_西風_門風",
			concealed: "二萬 三萬 四萬 三索 四索 五索 六索 七索 五筒 六筒 七筒 九筒 九筒",
			melds:     []rules.Meld{pong(rules.WestWind)},
			winTile:   "八索",
			seatWind:  rules.WindWest,
			wantFans:  []fanExpectation{{rules.FanWestWind, 1}},
			wantTotal: 1,
		},
		{
			name:      "A54_南風南",
			concealed: "二萬 三萬 四萬 三索 四索 五索 六索 七索 五筒 六筒 七筒 九筒 九筒",
			melds:     []rules.Meld{pong(rules.SouthWind)},
			winTile:   "八索",
			roundWind: rules.WindSouth,
			seatWind:  rules.WindSouth,
			wantFans:  []fanExpectation{{rules.FanDoubleSouthWind, 2}},
			wantTotal: 2,
		},
		{
			name:      "A55_西風西",
			concealed: "二萬 三萬 四萬 三索 四索 五索 六索 七索 五筒 六筒 七筒 九筒 九筒",
			melds:     []rules.Meld{pong(rules.WestWind)},
			winTile:   "八索",
			roundWind: rules.WindWest,
			seatWind:  rules.WindWest,
			wantFans:  []fanExpectation{{rules.FanDoubleWestWind, 2}},
			wantTotal: 2,
		},
		{
			name:      "A56_白板",
			concealed: "二萬 三萬 四萬 三索 四索 五索 六索 七索 五筒 六筒 七筒 九筒 九筒",
			melds:     []rules.Meld{pong(rules.WhiteDragon)},
			winTile:   "八索",
			wantFans:  []fanExpectation{{rules.FanWhiteDragon, 1}},
			wantTotal: 1,
		},
		{
			name:      "A57_青發",
			concealed: "二萬 三萬 四萬 三索 四索 五索 六索 七索 五筒 六筒 七筒 九筒 九筒",
			melds:     []rules.Meld{pong(rules.GreenDragon)},
			winTile:   "八索",
			wantFans:  []fanExpectation{{rules.FanGreenDragon, 1}},
			wantTotal: 1,
		},
		{
			name:      "A58_八張花一般胡牌",
			concealed: "二萬 三萬 四萬 五萬 六萬 七萬 八索 八索 三筒 四筒 五筒 六筒 七筒",
			melds:     []rules.Meld{pong(rules.Suo2)},
			flowers:   "春１ 夏２ 秋３ 冬４ 梅１ 蘭２ 菊３ 竹４",
			winTile:   "八筒",
			wantFans:  []fanExpectation{{rules.FanAllSimples, 1}, {rules.FanSeasonFlowers, 2}, {rules.FanPlantFlowers, 2}},
			wantTotal: 5,
		},
		{
			name:      "B01_平胡條件",
			concealed: "二萬 三萬 四萬 五萬 六萬 七萬 二索 三索 六索 七索 八索 三筒 四筒 五筒 九筒 九筒",
			winTile:   "四索",
			wantFans:  []fanExpectation{{rules.FanPingHu, 2}, {rules.FanConcealed, 1}},
			wantTotal: 3,
		},
		{
			name:      "C01_搶杠",
			concealed: "二萬 三萬 四萬 五筒 六筒 七筒 三索 四索 五索 六索 七索 九筒 九筒",
			melds:     []rules.Meld{pong(rules.Suo2)},
			winTile:   "八索",
			winKind:   rules.WinRobKong,
			wantFans:  []fanExpectation{{rules.FanRobKong, 1}},
			wantTotal: 1,
		},
		{
			name:      "C02_七搶一",
			winKind:   rules.WinSevenRobOne,
			wantFans:  []fanExpectation{{rules.FanSevenRobOne, 8}},
			wantTotal: 8,
		},
		{
			name:      "C03_八仙過海_莊家連一",
			winKind:   rules.WinEightImmortals,
			winner:    rules.Seat1,
			streak:    1,
			wantFans:  []fanExpectation{{rules.FanEightImmortals, 16}, {rules.FanDealer, 1}, {rules.FanDealerStreak, 2}},
			wantTotal: 19,
		},
		{
			name:      "Edge_平胡排除_邊張12聽3",
			concealed: "二萬 三萬 四萬 五萬 六萬 七萬 一索 二索 六索 七索 八索 三筒 四筒 五筒 九筒 九筒",
			winTile:   "三索",
			wantFans:  []fanExpectation{{rules.FanConcealed, 1}},
			wantTotal: 1,
		},
		{
			name:      "Edge_平胡排除_中洞24聽3",
			concealed: "二萬 三萬 四萬 五萬 六萬 七萬 二索 四索 六索 七索 八索 三筒 四筒 五筒 九筒 九筒",
			winTile:   "三索",
			wantFans:  []fanExpectation{{rules.FanConcealed, 1}},
			wantTotal: 1,
		},
		{
			name:      "Edge_平胡排除_單釣眼",
			concealed: "二萬 三萬 四萬 五萬 六萬 七萬 二索 三索 四索 六索 七索 八索 三筒 四筒 五筒 九筒",
			winTile:   "九筒",
			wantFans:  []fanExpectation{{rules.FanConcealed, 1}},
			wantTotal: 1,
		},
		{
			name:      "Edge_平胡排除_字牌眼",
			concealed: "二萬 三萬 四萬 五萬 六萬 七萬 二索 三索 六索 七索 八索 三筒 四筒 五筒 白板 白板",
			winTile:   "四索",
			wantFans:  []fanExpectation{{rules.FanConcealed, 1}},
			wantTotal: 1,
		},
		{
			name:      "Edge_平胡排除_有花牌",
			concealed: "二萬 三萬 四萬 五萬 六萬 七萬 二索 三索 六索 七索 八索 三筒 四筒 五筒 九筒 九筒",
			flowers:   "春１",
			winTile:   "四索",
			wantFans:  []fanExpectation{{rules.FanConcealed, 1}},
			wantTotal: 1,
		},
		{
			name:      "Edge_平胡排除_自摸",
			concealed: "二萬 三萬 四萬 五萬 六萬 七萬 二索 三索 六索 七索 八索 三筒 四筒 五筒 九筒 九筒",
			winTile:   "四索",
			winKind:   rules.WinSelfDraw,
			wantFans:  []fanExpectation{{rules.FanConcealedSelfDraw, 3}},
			wantTotal: 3,
		},
		{
			name:      "Edge_胡別人牌湊成第三刻不算三暗刻",
			concealed: "一萬 一萬 一萬 二萬 三萬 四萬 七索 七索 七索 九筒 九筒 二筒 三筒 四筒 五筒 五筒",
			winTile:   "九筒",
			wantFans:  []fanExpectation{{rules.FanConcealed, 1}},
			wantTotal: 1,
		},
	}

	for _, tc := range samples {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			// Default context matches qkmj scoring-samples.md baseline:
			// RoundWind=East, Dealer=Seat1, Winner=Seat2, SeatWind=North, Payer=Seat3, WinKind=WinDiscard.
			ctx := rules.ScoreContext{
				Concealed:            mustTiles(t, tc.concealed),
				Melds:                tc.melds,
				Flowers:              mustTiles(t, tc.flowers),
				WinTile:              mustOptionalTile(t, tc.winTile),
				WinKind:              orDefaultWinKind(tc.winKind, rules.WinDiscard),
				Winner:               orDefaultSeat(tc.winner, rules.Seat2),
				Payer:                orDefaultSeat(tc.payer, rules.Seat3),
				Dealer:               orDefaultSeat(tc.dealer, rules.Seat1),
				DealerStreak:         tc.streak,
				RoundWind:            orDefaultWind(tc.roundWind, rules.WindEast),
				SeatWind:             orDefaultWind(tc.seatWind, rules.WindNorth),
				IsAfterSupplement:    tc.afterSupp,
				IsLastDrawable:       tc.lastDraw,
				IsFirstRoundUnbroken: tc.firstRnd,
			}

			res, err := rules.Score(ctx)
			if err != nil {
				t.Fatalf("Score() err = %v", err)
			}
			if res.TotalFan != tc.wantTotal {
				t.Errorf("TotalFan = %d, want %d (got fans: %+v)", res.TotalFan, tc.wantTotal, res.Fans)
			}
			assertExactFans(t, res.Fans, tc.wantFans)
		})
	}
}

func mustTiles(t *testing.T, s string) []rules.Tile {
	t.Helper()
	if strings.TrimSpace(s) == "" {
		return nil
	}
	var out []rules.Tile
	for name := range strings.FieldsSeq(s) {
		tile, err := rules.ParseTile(name)
		if err != nil {
			t.Fatalf("ParseTile(%q) err = %v", name, err)
		}
		out = append(out, tile)
	}
	return out
}

func mustOptionalTile(t *testing.T, name string) rules.Tile {
	t.Helper()
	if strings.TrimSpace(name) == "" {
		return rules.TileInvalid
	}
	tile, err := rules.ParseTile(name)
	if err != nil {
		t.Fatalf("ParseTile(%q) err = %v", name, err)
	}
	return tile
}

func orDefaultWinKind(v, fallback rules.WinKind) rules.WinKind {
	if v == rules.WinUnknown {
		return fallback
	}
	return v
}

func orDefaultSeat(v, fallback rules.Seat) rules.Seat {
	if v == rules.SeatUnknown {
		return fallback
	}
	return v
}

func orDefaultWind(v, fallback rules.Wind) rules.Wind {
	if v == rules.WindUnknown {
		return fallback
	}
	return v
}

func assertExactFans(t *testing.T, got []rules.Fan, want []fanExpectation) {
	t.Helper()
	gotPairs := make([]fanExpectation, 0, len(got))
	for _, f := range got {
		gotPairs = append(gotPairs, fanExpectation{id: f.ID, points: f.Points})
	}
	wantCopy := slices.Clone(want)
	sortExpect := func(a, b fanExpectation) int {
		return cmp.Or(cmp.Compare(a.id, b.id), cmp.Compare(a.points, b.points))
	}
	slices.SortFunc(gotPairs, sortExpect)
	slices.SortFunc(wantCopy, sortExpect)

	if !slices.Equal(gotPairs, wantCopy) {
		t.Errorf("fans mismatch:\n got: %+v (%+v)\nwant: %+v", gotPairs, got, wantCopy)
	}
}
