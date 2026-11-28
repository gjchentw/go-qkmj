package rules

import (
	"errors"
	"fmt"
	"slices"
)

const (
	// TotalDeckTiles is the total number of tiles in a Taiwan 16-tile Mahjong deck.
	TotalDeckTiles = 144
	// ReserveWallTiles is the number of trailing tiles kept untouched as the reserve wall.
	ReserveWallTiles = 16
	// WaitingHandSize is the equivalent tile count of a player's hand outside their turn.
	WaitingHandSize = 16
	// DrawnHandSize is the equivalent tile count of a player's hand before discarding.
	DrawnHandSize = 17
	// MeldEquivalentSize is the equivalent tile count of every meld (including kongs).
	MeldEquivalentSize = 3
)

// Sentinel errors returned by rule model validation.
var (
	ErrInvalidTile      = errors.New("invalid tile")
	ErrInvalidMeld      = errors.New("invalid meld")
	ErrInvalidHandCount = errors.New("invalid hand tile count")
)

// Suit identifies the category of a Mahjong tile. Zero represents an unknown/invalid suit.
type Suit uint8

const (
	SuitUnknown Suit = iota
	SuitWan
	SuitSuo
	SuitTong
	SuitWind
	SuitDragon
	SuitFlower
)

// Valid reports whether s is a defined tile suit.
func (s Suit) Valid() bool {
	return s >= SuitWan && s <= SuitFlower
}

// String returns the Traditional Chinese name of the suit.
func (s Suit) String() string {
	switch s {
	case SuitWan:
		return "萬"
	case SuitSuo:
		return "索"
	case SuitTong:
		return "筒"
	case SuitWind:
		return "風"
	case SuitDragon:
		return "三元"
	case SuitFlower:
		return "花"
	case SuitUnknown:
		return "未知"
	default:
		return "未知"
	}
}

// Wind represents one of the four compass winds. Zero represents an unknown wind.
type Wind uint8

const (
	WindUnknown Wind = iota
	WindEast
	WindSouth
	WindWest
	WindNorth
)

// Valid reports whether w is one of the four winds.
func (w Wind) Valid() bool {
	return w >= WindEast && w <= WindNorth
}

// Next returns the next wind in East -> South -> West -> North -> East order.
func (w Wind) Next() Wind {
	switch w {
	case WindEast:
		return WindSouth
	case WindSouth:
		return WindWest
	case WindWest:
		return WindNorth
	case WindNorth:
		return WindEast
	case WindUnknown:
		return WindUnknown
	default:
		return WindUnknown
	}
}

// Tile returns the corresponding wind tile, or TileInvalid if w is invalid.
func (w Wind) Tile() Tile {
	switch w {
	case WindEast:
		return EastWind
	case WindSouth:
		return SouthWind
	case WindWest:
		return WestWind
	case WindNorth:
		return NorthWind
	case WindUnknown:
		return TileInvalid
	default:
		return TileInvalid
	}
}

// String returns the Traditional Chinese character of the wind.
func (w Wind) String() string {
	switch w {
	case WindEast:
		return "東"
	case WindSouth:
		return "南"
	case WindWest:
		return "西"
	case WindNorth:
		return "北"
	case WindUnknown:
		return "未知"
	default:
		return "未知"
	}
}

// Seat identifies one of the four fixed table seats (1 through 4). Zero is invalid.
type Seat uint8

const (
	SeatUnknown Seat = iota
	Seat1
	Seat2
	Seat3
	Seat4
)

// AllSeats lists the four valid seats in fixed rotation order.
var AllSeats = [4]Seat{Seat1, Seat2, Seat3, Seat4}

// Valid reports whether s is Seat1 through Seat4.
func (s Seat) Valid() bool {
	return s >= Seat1 && s <= Seat4
}

// Next returns the next seat in 1 -> 2 -> 3 -> 4 -> 1 rotation order.
func (s Seat) Next() Seat {
	if !s.Valid() {
		return SeatUnknown
	}
	return (s-Seat1+1)%4 + Seat1
}

// Prev returns the previous seat in rotation order.
func (s Seat) Prev() Seat {
	if !s.Valid() {
		return SeatUnknown
	}
	return (s-Seat1+3)%4 + Seat1
}

// Opposite returns the seat across the table.
func (s Seat) Opposite() Seat {
	if !s.Valid() {
		return SeatUnknown
	}
	return (s-Seat1+2)%4 + Seat1
}

// String returns the seat label.
func (s Seat) String() string {
	if !s.Valid() {
		return "SeatUnknown"
	}
	return fmt.Sprintf("Seat%d", s)
}

// Tile is a compact identifier (1..42) for a Mahjong tile kind. Zero is TileInvalid.
type Tile uint8

const (
	TileInvalid Tile = iota

	Wan1
	Wan2
	Wan3
	Wan4
	Wan5
	Wan6
	Wan7
	Wan8
	Wan9

	Suo1
	Suo2
	Suo3
	Suo4
	Suo5
	Suo6
	Suo7
	Suo8
	Suo9

	Tong1
	Tong2
	Tong3
	Tong4
	Tong5
	Tong6
	Tong7
	Tong8
	Tong9

	EastWind
	SouthWind
	WestWind
	NorthWind

	RedDragon
	WhiteDragon
	GreenDragon

	FlowerSpring
	FlowerSummer
	FlowerAutumn
	FlowerWinter
	FlowerPlum
	FlowerOrchid
	FlowerChrysanthemum
	FlowerBamboo

	// NonFlowerMax is the highest non-flower tile identifier.
	NonFlowerMax = GreenDragon
	// TileMax is the highest valid tile identifier.
	TileMax = FlowerBamboo
)

var tileNames = [TileMax + 1]string{
	TileInvalid: "無效",

	Wan1: "一萬", Wan2: "二萬", Wan3: "三萬", Wan4: "四萬", Wan5: "五萬",
	Wan6: "六萬", Wan7: "七萬", Wan8: "八萬", Wan9: "九萬",

	Suo1: "一索", Suo2: "二索", Suo3: "三索", Suo4: "四索", Suo5: "五索",
	Suo6: "六索", Suo7: "七索", Suo8: "八索", Suo9: "九索",

	Tong1: "一筒", Tong2: "二筒", Tong3: "三筒", Tong4: "四筒", Tong5: "五筒",
	Tong6: "六筒", Tong7: "七筒", Tong8: "八筒", Tong9: "九筒",

	EastWind: "東風", SouthWind: "南風", WestWind: "西風", NorthWind: "北風",
	RedDragon: "紅中", WhiteDragon: "白板", GreenDragon: "青發",

	FlowerSpring: "春１", FlowerSummer: "夏２", FlowerAutumn: "秋３", FlowerWinter: "冬４",
	FlowerPlum: "梅１", FlowerOrchid: "蘭２", FlowerChrysanthemum: "菊３", FlowerBamboo: "竹４",
}

// NewNumberTile constructs a number tile for SuitWan, SuitSuo, or SuitTong and rank 1..9.
func NewNumberTile(suit Suit, rank int) (Tile, error) {
	if rank < 1 || rank > 9 {
		return TileInvalid, fmt.Errorf("rank %d out of range: %w", rank, ErrInvalidTile)
	}
	switch suit {
	case SuitWan:
		return Tile(int(Wan1) + rank - 1), nil
	case SuitSuo:
		return Tile(int(Suo1) + rank - 1), nil
	case SuitTong:
		return Tile(int(Tong1) + rank - 1), nil
	case SuitUnknown, SuitWind, SuitDragon, SuitFlower:
		return TileInvalid, fmt.Errorf("suit %s is not a number suit: %w", suit, ErrInvalidTile)
	default:
		return TileInvalid, fmt.Errorf("unknown suit %d: %w", suit, ErrInvalidTile)
	}
}

// ParseTile resolves a Traditional Chinese tile face name (such as "一萬" or "春１") to a Tile.
func ParseTile(name string) (Tile, error) {
	for t := Wan1; t <= TileMax; t++ {
		if tileNames[t] == name {
			return t, nil
		}
	}
	return TileInvalid, fmt.Errorf("unknown tile %q: %w", name, ErrInvalidTile)
}

// Valid reports whether t is a valid tile identifier.
func (t Tile) Valid() bool {
	return t >= Wan1 && t <= TileMax
}

// Suit returns the suit category of t.
func (t Tile) Suit() Suit {
	switch {
	case t >= Wan1 && t <= Wan9:
		return SuitWan
	case t >= Suo1 && t <= Suo9:
		return SuitSuo
	case t >= Tong1 && t <= Tong9:
		return SuitTong
	case t >= EastWind && t <= NorthWind:
		return SuitWind
	case t >= RedDragon && t <= GreenDragon:
		return SuitDragon
	case t >= FlowerSpring && t <= FlowerBamboo:
		return SuitFlower
	default:
		return SuitUnknown
	}
}

// Rank returns 1..9 for number suits, 1..4 for winds and flowers, and 0 otherwise.
func (t Tile) Rank() int {
	switch {
	case t >= Wan1 && t <= Wan9:
		return int(t-Wan1) + 1
	case t >= Suo1 && t <= Suo9:
		return int(t-Suo1) + 1
	case t >= Tong1 && t <= Tong9:
		return int(t-Tong1) + 1
	case t >= EastWind && t <= NorthWind:
		return int(t-EastWind) + 1
	case t >= FlowerSpring && t <= FlowerWinter:
		return int(t-FlowerSpring) + 1
	case t >= FlowerPlum && t <= FlowerBamboo:
		return int(t-FlowerPlum) + 1
	default:
		return 0
	}
}

// IsNumber reports whether t belongs to Wan, Suo, or Tong.
func (t Tile) IsNumber() bool {
	return t >= Wan1 && t <= Tong9
}

// IsWind reports whether t is one of the four wind tiles.
func (t Tile) IsWind() bool {
	return t >= EastWind && t <= NorthWind
}

// IsDragon reports whether t is one of the three dragon tiles.
func (t Tile) IsDragon() bool {
	return t >= RedDragon && t <= GreenDragon
}

// IsHonor reports whether t is a wind or dragon tile.
func (t Tile) IsHonor() bool {
	return t.IsWind() || t.IsDragon()
}

// IsTerminal reports whether t is a 1 or 9 number tile.
func (t Tile) IsTerminal() bool {
	if !t.IsNumber() {
		return false
	}
	r := t.Rank()
	return r == 1 || r == 9
}

// IsTerminalOrHonor reports whether t is a terminal (1/9) or honor (wind/dragon) tile.
func (t Tile) IsTerminalOrHonor() bool {
	return t.IsTerminal() || t.IsHonor()
}

// IsSimple reports whether t is a 2..8 number tile.
func (t Tile) IsSimple() bool {
	if !t.IsNumber() {
		return false
	}
	r := t.Rank()
	return r >= 2 && r <= 8
}

// IsFlower reports whether t is one of the eight flower tiles.
func (t Tile) IsFlower() bool {
	return t >= FlowerSpring && t <= FlowerBamboo
}

// IsSeasonFlower reports whether t is Spring, Summer, Autumn, or Winter.
func (t Tile) IsSeasonFlower() bool {
	return t >= FlowerSpring && t <= FlowerWinter
}

// IsPlantFlower reports whether t is Plum, Orchid, Chrysanthemum, or Bamboo.
func (t Tile) IsPlantFlower() bool {
	return t >= FlowerPlum && t <= FlowerBamboo
}

// WindValue returns the Wind value of a wind tile, or WindUnknown otherwise.
func (t Tile) WindValue() Wind {
	if !t.IsWind() {
		return WindUnknown
	}
	return Wind(t-EastWind) + WindEast
}

// FlowerWind returns the seat wind associated with a flower tile, or WindUnknown for non-flowers.
func (t Tile) FlowerWind() Wind {
	if t.IsSeasonFlower() {
		return Wind(t-FlowerSpring) + WindEast
	}
	if t.IsPlantFlower() {
		return Wind(t-FlowerPlum) + WindEast
	}
	return WindUnknown
}

// String returns the full-width Traditional Chinese tile face name.
func (t Tile) String() string {
	if !t.Valid() {
		return tileNames[TileInvalid]
	}
	return tileNames[t]
}

// FullDeck returns a fresh canonical slice of all 144 tiles before shuffling.
func FullDeck() []Tile {
	deck := make([]Tile, 0, TotalDeckTiles)
	for t := Wan1; t <= NonFlowerMax; t++ {
		for range 4 {
			deck = append(deck, t)
		}
	}
	for t := FlowerSpring; t <= FlowerBamboo; t++ {
		deck = append(deck, t)
	}
	return deck
}

// MeldKind identifies the type of a declared tile meld. Zero is invalid.
type MeldKind uint8

const (
	MeldUnknown MeldKind = iota
	MeldChow
	MeldPong
	MeldExposedKong
	MeldAddedKong
	MeldConcealedKong
)

// Valid reports whether k is a defined meld kind.
func (k MeldKind) Valid() bool {
	return k >= MeldChow && k <= MeldConcealedKong
}

// IsKong reports whether k is any kind of kong (exposed, added, or concealed).
func (k MeldKind) IsKong() bool {
	return k == MeldExposedKong || k == MeldAddedKong || k == MeldConcealedKong
}

// IsExposed reports whether the meld's tiles are publicly visible during the hand.
func (k MeldKind) IsExposed() bool {
	return k >= MeldChow && k <= MeldAddedKong
}

// BreaksConcealed reports whether declaring this meld breaks menqing (門清).
// Concealed kongs do not break menqing under QKMJ baseline rules.
func (k MeldKind) BreaksConcealed() bool {
	return k.IsExposed()
}

// Meld represents a declared set of tiles (chow, pong, or kong).
type Meld struct {
	Kind     MeldKind
	Tiles    []Tile
	FromSeat Seat
}

// EquivalentTileCount returns the hand-size equivalent tile count of a meld (always 3).
func (m Meld) EquivalentTileCount() int {
	return MeldEquivalentSize
}

// BaseTile returns the canonical lowest tile in the meld, or TileInvalid if empty.
func (m Meld) BaseTile() Tile {
	if len(m.Tiles) == 0 {
		return TileInvalid
	}
	return slices.Min(m.Tiles)
}

// Validate checks that m has a valid kind, valid non-flower tiles, and matching tile pattern.
func (m Meld) Validate() error {
	if !m.Kind.Valid() {
		return fmt.Errorf("unknown meld kind %d: %w", m.Kind, ErrInvalidMeld)
	}
	for _, t := range m.Tiles {
		if !t.Valid() || t.IsFlower() {
			return fmt.Errorf("tile %v not allowed in meld: %w", t, ErrInvalidMeld)
		}
	}

	switch m.Kind {
	case MeldChow:
		return validateChow(m.Tiles)
	case MeldPong:
		return validateSameTiles(m.Tiles, 3)
	case MeldExposedKong, MeldAddedKong, MeldConcealedKong:
		return validateSameTiles(m.Tiles, 4)
	case MeldUnknown:
		return ErrInvalidMeld
	default:
		return ErrInvalidMeld
	}
}

func validateChow(tiles []Tile) error {
	if len(tiles) != 3 {
		return fmt.Errorf("chow requires 3 tiles, got %d: %w", len(tiles), ErrInvalidMeld)
	}
	sorted := slices.Clone(tiles)
	slices.Sort(sorted)
	if !sorted[0].IsNumber() || !sorted[1].IsNumber() || !sorted[2].IsNumber() {
		return fmt.Errorf("chow requires number tiles: %w", ErrInvalidMeld)
	}
	suit := sorted[0].Suit()
	if sorted[1].Suit() != suit || sorted[2].Suit() != suit {
		return fmt.Errorf("chow requires same suit: %w", ErrInvalidMeld)
	}
	if sorted[1].Rank() != sorted[0].Rank()+1 || sorted[2].Rank() != sorted[1].Rank()+1 {
		return fmt.Errorf("chow requires consecutive ranks: %w", ErrInvalidMeld)
	}
	return nil
}

func validateSameTiles(tiles []Tile, want int) error {
	if len(tiles) != want {
		return fmt.Errorf("expected %d identical tiles, got %d: %w", want, len(tiles), ErrInvalidMeld)
	}
	first := tiles[0]
	for _, t := range tiles[1:] {
		if t != first {
			return fmt.Errorf("tiles must be identical: %w", ErrInvalidMeld)
		}
	}
	return nil
}

// ValidateHandCount verifies that concealedCount plus melds (each counting as 3) equals 16 or 17.
func ValidateHandCount(concealedCount int, melds []Meld, expectDrawn bool) error {
	if concealedCount < 0 {
		return fmt.Errorf("negative concealed count %d: %w", concealedCount, ErrInvalidHandCount)
	}
	for i, m := range melds {
		if err := m.Validate(); err != nil {
			return fmt.Errorf("meld %d: %w", i, err)
		}
	}
	want := WaitingHandSize
	if expectDrawn {
		want = DrawnHandSize
	}
	total := concealedCount + len(melds)*MeldEquivalentSize
	if total != want {
		return fmt.Errorf("equivalent hand count %d, want %d: %w", total, want, ErrInvalidHandCount)
	}
	return nil
}
