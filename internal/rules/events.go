package rules

import "time"

// EventAudience specifies whether an event is broadcast publicly or delivered privately to one seat.
type EventAudience uint8

const (
	AudienceUnknown EventAudience = iota
	AudiencePublic
	AudiencePrivate
)

// ClaimWindowKind identifies which trigger opened a claim arbitration window.
type ClaimWindowKind uint8

const (
	ClaimWindowUnknown ClaimWindowKind = iota
	ClaimWindowDiscard
	ClaimWindowRobKong
	ClaimWindowFlower
)

// HandEndReason classifies why a hand terminated.
type HandEndReason uint8

const (
	HandEndUnknown HandEndReason = iota
	HandEndWin
	HandEndExhausted
	HandEndAborted
)

// Event is a sealed strongly-typed output emitted by pure rules state transitions.
// Downstream table actors route events using Audience() and TargetSeat() without leaking hidden data.
type Event interface {
	Audience() EventAudience
	TargetSeat() Seat
	rulesEvent()
}

// TurnChangedEvent publicly announces whose turn it is and whether they must draw.
type TurnChangedEvent struct {
	Seat           Seat
	WaitingForDraw bool
}

// Audience returns AudiencePublic.
func (TurnChangedEvent) Audience() EventAudience { return AudiencePublic }

// TargetSeat returns SeatUnknown for public events.
func (TurnChangedEvent) TargetSeat() Seat { return SeatUnknown }
func (TurnChangedEvent) rulesEvent()      {}

// DrawPromptEvent privately prompts the active seat to send ActionDraw.
type DrawPromptEvent struct {
	Seat         Seat
	IsSupplement bool
}

// Audience returns AudiencePrivate.
func (DrawPromptEvent) Audience() EventAudience { return AudiencePrivate }

// TargetSeat returns the prompted seat.
func (e DrawPromptEvent) TargetSeat() Seat { return e.Seat }
func (DrawPromptEvent) rulesEvent()        {}

// TileDrawnPublicEvent publicly announces that a seat drew a tile and updates DrawableRemaining.
type TileDrawnPublicEvent struct {
	Seat              Seat
	DrawableRemaining int
	IsSupplement      bool
}

// Audience returns AudiencePublic.
func (TileDrawnPublicEvent) Audience() EventAudience { return AudiencePublic }

// TargetSeat returns SeatUnknown for public events.
func (TileDrawnPublicEvent) TargetSeat() Seat { return SeatUnknown }
func (TileDrawnPublicEvent) rulesEvent()      {}

// TileDrawnEvent privately reveals the drawn or supplemented tile face to the drawing seat.
type TileDrawnEvent struct {
	Seat         Seat
	Tile         Tile
	IsSupplement bool
}

// Audience returns AudiencePrivate.
func (TileDrawnEvent) Audience() EventAudience { return AudiencePrivate }

// TargetSeat returns the drawing seat.
func (e TileDrawnEvent) TargetSeat() Seat { return e.Seat }
func (TileDrawnEvent) rulesEvent()        {}

// FlowerExposedEvent publicly announces that a seat exposed a flower tile.
type FlowerExposedEvent struct {
	Seat              Seat
	Flower            Tile
	DrawableRemaining int
}

// Audience returns AudiencePublic.
func (FlowerExposedEvent) Audience() EventAudience { return AudiencePublic }

// TargetSeat returns SeatUnknown for public events.
func (FlowerExposedEvent) TargetSeat() Seat { return SeatUnknown }
func (FlowerExposedEvent) rulesEvent()      {}

// SelfDeclarePromptEvent privately lists available self-declarations for the active seat.
type SelfDeclarePromptEvent struct {
	Seat               Seat
	CanSelfWin         bool
	ConcealedKongTiles []Tile
	AddedKongTiles     []Tile
}

// Audience returns AudiencePrivate.
func (SelfDeclarePromptEvent) Audience() EventAudience { return AudiencePrivate }

// TargetSeat returns the prompted seat.
func (e SelfDeclarePromptEvent) TargetSeat() Seat { return e.Seat }
func (SelfDeclarePromptEvent) rulesEvent()        {}

// ClaimPromptEvent privately notifies a seat of its legal options inside a claim window.
type ClaimPromptEvent struct {
	Seat          Seat
	WindowKind    ClaimWindowKind
	TriggerSeat   Seat
	TriggerTile   Tile
	Options       []ClaimKind
	ChowOptions   []ChowOption
	FlowerWinKind WinKind
}

// Audience returns AudiencePrivate.
func (ClaimPromptEvent) Audience() EventAudience { return AudiencePrivate }

// TargetSeat returns the prompted seat.
func (e ClaimPromptEvent) TargetSeat() Seat { return e.Seat }
func (ClaimPromptEvent) rulesEvent()        {}

// ChowChoicePromptEvent privately prompts a seat to choose among two or more legal chow combinations.
type ChowChoicePromptEvent struct {
	Seat        Seat
	TriggerSeat Seat
	TriggerTile Tile
	Choices     []ChowOption
}

// Audience returns AudiencePrivate.
func (ChowChoicePromptEvent) Audience() EventAudience { return AudiencePrivate }

// TargetSeat returns the prompted seat.
func (e ChowChoicePromptEvent) TargetSeat() Seat { return e.Seat }
func (ChowChoicePromptEvent) rulesEvent()        {}

// MeldDeclaredEvent publicly announces a resolved meld (with concealed kong faces masked).
type MeldDeclaredEvent struct {
	Seat             Seat
	Meld             PublicMeld
	RemovedFromRiver bool
}

// NewMeldDeclaredEvent constructs a visibility-safe MeldDeclaredEvent that masks concealed kongs.
func NewMeldDeclaredEvent(seat Seat, meld Meld, removedFromRiver bool) MeldDeclaredEvent {
	return MeldDeclaredEvent{
		Seat:             seat,
		Meld:             ProjectMeld(meld, false),
		RemovedFromRiver: removedFromRiver,
	}
}

// Audience returns AudiencePublic.
func (MeldDeclaredEvent) Audience() EventAudience { return AudiencePublic }

// TargetSeat returns SeatUnknown for public events.
func (MeldDeclaredEvent) TargetSeat() Seat { return SeatUnknown }
func (MeldDeclaredEvent) rulesEvent()      {}

// DiscardedEvent publicly announces a discarded tile and the seat's updated cumulative thinking time.
type DiscardedEvent struct {
	Seat         Seat
	Tile         Tile
	ThinkingTime time.Duration
}

// Audience returns AudiencePublic.
func (DiscardedEvent) Audience() EventAudience { return AudiencePublic }

// TargetSeat returns SeatUnknown for public events.
func (DiscardedEvent) TargetSeat() Seat { return SeatUnknown }
func (DiscardedEvent) rulesEvent()      {}

// PassedWinChangedEvent privately informs a seat when it enters or clears 0.95m passed-win (過水) state.
type PassedWinChangedEvent struct {
	Seat   Seat
	Passed bool
}

// Audience returns AudiencePrivate.
func (PassedWinChangedEvent) Audience() EventAudience { return AudiencePrivate }

// TargetSeat returns the affected seat.
func (e PassedWinChangedEvent) TargetSeat() Seat { return e.Seat }
func (PassedWinChangedEvent) rulesEvent()        {}

// HandEndedEvent publicly announces the termination of the hand and optional winning score result.
type HandEndedEvent struct {
	Reason HandEndReason
	Score  *ScoreResult
}

// Audience returns AudiencePublic.
func (HandEndedEvent) Audience() EventAudience { return AudiencePublic }

// TargetSeat returns SeatUnknown for public events.
func (HandEndedEvent) TargetSeat() Seat { return SeatUnknown }
func (HandEndedEvent) rulesEvent()      {}
