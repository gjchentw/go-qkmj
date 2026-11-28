package rules

// GroupKind classifies one of the five groups in a standard winning decomposition.
type GroupKind uint8

const (
	GroupUnknown GroupKind = iota
	GroupSequence
	GroupTriplet
	GroupKong
)

// Group represents a single sequence, triplet, or kong inside a winning hand decomposition.
// FromMeld is MeldUnknown when the group was formed in the concealed hand.
type Group struct {
	Kind     GroupKind
	Base     Tile
	FromMeld MeldKind
}

// IsConcealed reports whether the group was formed privately (concealed hand or concealed kong).
func (g Group) IsConcealed() bool {
	return g.FromMeld == MeldUnknown || g.FromMeld == MeldConcealedKong
}

// IsKong reports whether the group is a kong.
func (g Group) IsKong() bool {
	return g.Kind == GroupKong
}

// IsTripletOrKong reports whether the group is a triplet or kong.
func (g Group) IsTripletOrKong() bool {
	return g.Kind == GroupTriplet || g.Kind == GroupKong
}

// Contains reports whether tile is part of this group.
func (g Group) Contains(tile Tile) bool {
	if g.Kind == GroupSequence {
		return tile == g.Base || tile == g.Base+1 || tile == g.Base+2
	}
	return tile == g.Base
}

// WinRoleKind records whether WinTile completed the pair or one of the concealed groups.
type WinRoleKind uint8

const (
	WinRoleUnknown WinRoleKind = iota
	WinRolePair
	WinRoleGroup
)

// Decomposition represents one valid 5-group + 1-pair interpretation of a winning hand,
// together with which concealed component absorbed WinTile.
type Decomposition struct {
	Pair          Tile
	Groups        [5]Group
	WinRole       WinRoleKind
	WinGroupIndex int
}

// IsTwoSidedWait reports whether WinTile completed a true two-sided wait (兩面聽)
// on a concealed sequence (excluding edge waits 12->3 and 89->7, middle waits, and pair waits).
func (d Decomposition) IsTwoSidedWait(winTile Tile) bool {
	if d.WinRole != WinRoleGroup || d.WinGroupIndex < 0 || d.WinGroupIndex >= len(d.Groups) {
		return false
	}
	g := d.Groups[d.WinGroupIndex]
	if g.Kind != GroupSequence || !g.IsConcealed() || !g.Contains(winTile) {
		return false
	}
	baseRank := g.Base.Rank()
	winRank := winTile.Rank()
	if winRank == baseRank && baseRank <= 6 {
		return true
	}
	if winRank == baseRank+2 && baseRank >= 2 {
		return true
	}
	return false
}

// CanWin reports whether concealed tiles + melds + winTile form a valid 5-group + 1-pair winning hand.
func CanWin(concealed []Tile, melds []Meld, winTile Tile) bool {
	counts, ok := prepareHandCounts(concealed, melds, winTile)
	if !ok {
		return false
	}
	return canWinCounts(&counts, 5-len(melds))
}

func canWinCounts(counts *[NonFlowerMax + 1]int, needGroups int) bool {
	for pair := Wan1; pair <= NonFlowerMax; pair++ {
		if counts[pair] < 2 {
			continue
		}
		counts[pair] -= 2
		if canPartitionGroups(counts, needGroups) {
			counts[pair] += 2
			return true
		}
		counts[pair] += 2
	}
	return false
}

// FindRiichiDiscardTiles returns the sorted, deduplicated non-flower tiles in concealed whose
// discard leaves a concealed (門清) tenpai hand waiting on at least one valid tile.
// Any exposed meld (BreaksConcealed() == true) immediately disqualifies Riichi and returns nil.
func FindRiichiDiscardTiles(concealed []Tile, melds []Meld) []Tile {
	if len(melds) > 5 || len(concealed)+len(melds)*MeldEquivalentSize != DrawnHandSize {
		return nil
	}
	var counts, total [NonFlowerMax + 1]int
	for _, m := range melds {
		if m.Kind.BreaksConcealed() || m.Validate() != nil {
			return nil
		}
		for _, t := range m.Tiles {
			total[t]++
			if total[t] > 4 {
				return nil
			}
		}
	}
	for _, t := range concealed {
		if !t.Valid() || t.IsFlower() {
			return nil
		}
		counts[t]++
		total[t]++
		if total[t] > 4 {
			return nil
		}
	}

	needGroups := 5 - len(melds)
	var out []Tile
	for discard := Wan1; discard <= NonFlowerMax; discard++ {
		if counts[discard] == 0 {
			continue
		}
		counts[discard]--
		total[discard]--
		if hasAnyWinningTile(&counts, &total, needGroups) {
			out = append(out, discard)
		}
		counts[discard]++
		total[discard]++
	}
	return out
}

func hasAnyWinningTile(counts, total *[NonFlowerMax + 1]int, needGroups int) bool {
	for wait := Wan1; wait <= NonFlowerMax; wait++ {
		if total[wait] >= 4 {
			continue
		}
		counts[wait]++
		win := canWinCounts(counts, needGroups)
		counts[wait]--
		if win {
			return true
		}
	}
	return false
}

// CountDistinctTerminalOrHonor counts how many distinct terminal (1/9) or honor (wind/dragon)
// tile types appear in concealed, used for Nine Terminals (九么九和) eligibility.
func CountDistinctTerminalOrHonor(concealed []Tile) int {
	var seen [NonFlowerMax + 1]bool
	distinct := 0
	for _, t := range concealed {
		if t.Valid() && !t.IsFlower() && t.IsTerminalOrHonor() && !seen[t] {
			seen[t] = true
			distinct++
		}
	}
	return distinct
}

// Decompose enumerates all distinct canonical 5-group + 1-pair decompositions
// along with each distinct role WinTile can play inside the concealed hand.
func Decompose(concealed []Tile, melds []Meld, winTile Tile) []Decomposition {
	counts, ok := prepareHandCounts(concealed, melds, winTile)
	if !ok {
		return nil
	}

	var baseGroups [5]Group
	for i, m := range melds {
		baseGroups[i] = meldToGroup(m)
	}
	fixedCount := len(melds)
	needGroups := 5 - fixedCount

	var results []Decomposition
	for pair := Wan1; pair <= NonFlowerMax; pair++ {
		if counts[pair] < 2 {
			continue
		}
		counts[pair] -= 2
		var concealedGroups []Group
		collectGroupPartitions(&counts, needGroups, TileInvalid, true, &concealedGroups, func(chosen []Group) {
			groups := baseGroups
			copy(groups[fixedCount:], chosen)
			results = appendWinRoles(results, pair, groups, fixedCount, winTile)
		})
		counts[pair] += 2
	}
	return results
}

func prepareHandCounts(concealed []Tile, melds []Meld, winTile Tile) ([NonFlowerMax + 1]int, bool) {
	var counts [NonFlowerMax + 1]int
	if !winTile.Valid() || winTile.IsFlower() {
		return counts, false
	}
	if len(melds) > 5 {
		return counts, false
	}
	if len(concealed)+1+len(melds)*MeldEquivalentSize != DrawnHandSize {
		return counts, false
	}

	var total [NonFlowerMax + 1]int
	for _, m := range melds {
		if err := m.Validate(); err != nil {
			return counts, false
		}
		for _, t := range m.Tiles {
			total[t]++
			if total[t] > 4 {
				return counts, false
			}
		}
	}

	for _, t := range concealed {
		if !t.Valid() || t.IsFlower() {
			return counts, false
		}
		counts[t]++
		total[t]++
		if total[t] > 4 {
			return counts, false
		}
	}

	counts[winTile]++
	total[winTile]++
	if total[winTile] > 4 {
		return counts, false
	}
	return counts, true
}

func meldToGroup(m Meld) Group {
	kind := GroupTriplet
	switch m.Kind {
	case MeldChow:
		kind = GroupSequence
	case MeldPong:
		kind = GroupTriplet
	case MeldExposedKong, MeldAddedKong, MeldConcealedKong:
		kind = GroupKong
	case MeldUnknown:
		kind = GroupUnknown
	}
	return Group{
		Kind:     kind,
		Base:     m.BaseTile(),
		FromMeld: m.Kind,
	}
}

func firstNonZeroTile(counts *[NonFlowerMax + 1]int) Tile {
	for t := Wan1; t <= NonFlowerMax; t++ {
		if counts[t] > 0 {
			return t
		}
	}
	return TileInvalid
}

func canFormSequenceAt(counts *[NonFlowerMax + 1]int, t Tile) bool {
	return t.IsNumber() && t.Rank() <= 7 && counts[t+1] > 0 && counts[t+2] > 0
}

func canPartitionGroups(counts *[NonFlowerMax + 1]int, remaining int) bool {
	if remaining == 0 {
		return true
	}
	t := firstNonZeroTile(counts)
	if t == TileInvalid {
		return false
	}

	if counts[t] >= 3 {
		counts[t] -= 3
		if canPartitionGroups(counts, remaining-1) {
			counts[t] += 3
			return true
		}
		counts[t] += 3
	}

	if canFormSequenceAt(counts, t) {
		counts[t]--
		counts[t+1]--
		counts[t+2]--
		if canPartitionGroups(counts, remaining-1) {
			counts[t]++
			counts[t+1]++
			counts[t+2]++
			return true
		}
		counts[t]++
		counts[t+1]++
		counts[t+2]++
	}
	return false
}

// collectGroupPartitions enumerates each canonical multiset of concealed groups exactly once
// by always removing from the smallest remaining tile and ordering triplets before sequences
// when the same tile is chosen consecutively.
func collectGroupPartitions(
	counts *[NonFlowerMax + 1]int,
	remaining int,
	prevTile Tile,
	allowTriplet bool,
	acc *[]Group,
	emit func([]Group),
) {
	if remaining == 0 {
		emit(*acc)
		return
	}
	t := firstNonZeroTile(counts)
	if t == TileInvalid {
		return
	}
	canTriplet := allowTriplet || t != prevTile

	if canTriplet && counts[t] >= 3 {
		counts[t] -= 3
		*acc = append(*acc, Group{Kind: GroupTriplet, Base: t})
		collectGroupPartitions(counts, remaining-1, t, true, acc, emit)
		*acc = (*acc)[:len(*acc)-1]
		counts[t] += 3
	}

	if canFormSequenceAt(counts, t) {
		counts[t]--
		counts[t+1]--
		counts[t+2]--
		*acc = append(*acc, Group{Kind: GroupSequence, Base: t})
		collectGroupPartitions(counts, remaining-1, t, false, acc, emit)
		*acc = (*acc)[:len(*acc)-1]
		counts[t]++
		counts[t+1]++
		counts[t+2]++
	}
}

func appendWinRoles(results []Decomposition, pair Tile, groups [5]Group, fixedCount int, winTile Tile) []Decomposition {
	if pair == winTile {
		results = append(results, Decomposition{
			Pair:          pair,
			Groups:        groups,
			WinRole:       WinRolePair,
			WinGroupIndex: -1,
		})
	}
	for i := fixedCount; i < len(groups); i++ {
		g := groups[i]
		if !g.Contains(winTile) {
			continue
		}
		// Skip duplicate identical concealed groups so the same role is emitted once.
		if i > fixedCount && groups[i-1] == g {
			continue
		}
		results = append(results, Decomposition{
			Pair:          pair,
			Groups:        groups,
			WinRole:       WinRoleGroup,
			WinGroupIndex: i,
		})
	}
	return results
}
