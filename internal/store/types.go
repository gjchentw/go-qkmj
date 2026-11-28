package store

import (
	"fmt"
	"regexp"
	"slices"
	"strings"
	"time"
)

// QKMJ 95p3+ economy, bot pool, and governance defaults.
const (
	InitialBalance    = 20000
	ReliefThreshold   = 20000
	ReliefMinGames    = 16
	RankMinGames      = 16
	DefaultMaxBlocks  = 100
	BotAccountCount   = 10
	BotUsernamePrefix = "QKMJ"
	ReservedPrefix    = "qkmj"
)

var usernameRegex = regexp.MustCompile(`^[0-9A-Za-z]{1,20}$`)

// IsReservedUsername reports whether name starts with the system-reserved prefix ("qkmj", case-insensitive).
func IsReservedUsername(name string) bool {
	return strings.HasPrefix(strings.ToLower(name), ReservedPrefix)
}

// ValidateUsername checks ASCII alphanumeric syntax (1..20 chars) and returns canonical and lowercase forms.
// It allows reserved prefixes so callers can query built-in bot accounts (e.g., QKMJ0).
func ValidateUsername(name string) (canonical, lower string, err error) {
	if !usernameRegex.MatchString(name) {
		return "", "", fmt.Errorf("username %q must match ^[0-9A-Za-z]{1,20}$: %w", name, ErrInvalidUsername)
	}
	return name, strings.ToLower(name), nil
}

// ValidateRegistrationUsername checks username syntax and rejects system-reserved prefixes ("qkmj*").
func ValidateRegistrationUsername(name string) (canonical, lower string, err error) {
	canonical, lower, err = ValidateUsername(name)
	if err != nil {
		return "", "", err
	}
	if strings.HasPrefix(lower, ReservedPrefix) {
		return "", "", fmt.Errorf("username %q uses reserved prefix %q: %w", name, ReservedPrefix, ErrReservedUsername)
	}
	return canonical, lower, nil
}

// Role identifies a strongly typed RBAC role assigned to an account.
type Role uint8

const (
	RoleUnknown Role = iota
	RolePlayer
	RoleModerator
	RoleAdmin
	RoleBot
)

// Valid reports whether r is a known role.
func (r Role) Valid() bool {
	return r >= RolePlayer && r <= RoleBot
}

// String returns the canonical lowercase role identifier stored in the database.
func (r Role) String() string {
	switch r {
	case RolePlayer:
		return "player"
	case RoleModerator:
		return "moderator"
	case RoleAdmin:
		return "admin"
	case RoleBot:
		return "bot"
	case RoleUnknown:
		return "unknown"
	default:
		return "unknown"
	}
}

// ParseRole parses a database or caller role name into a Role.
func ParseRole(s string) (Role, error) {
	switch strings.ToLower(strings.TrimSpace(s)) {
	case "player":
		return RolePlayer, nil
	case "moderator":
		return RoleModerator, nil
	case "admin":
		return RoleAdmin, nil
	case "bot":
		return RoleBot, nil
	default:
		return RoleUnknown, fmt.Errorf("role %q: %w", s, ErrInvalidRole)
	}
}

// Permission identifies a strongly typed RBAC capability checked via Account.HasPermission.
type Permission uint8

const (
	PermUnknown Permission = iota
	PermTablePlay
	PermUserKick
	PermUserBan
	PermServerAnnounce
	PermRoleAssign
)

// Valid reports whether p is a known permission.
func (p Permission) Valid() bool {
	return p >= PermTablePlay && p <= PermRoleAssign
}

// String returns the canonical permission identifier.
func (p Permission) String() string {
	switch p {
	case PermTablePlay:
		return "table:play"
	case PermUserKick:
		return "user:kick"
	case PermUserBan:
		return "user:ban"
	case PermServerAnnounce:
		return "server:announce"
	case PermRoleAssign:
		return "role:assign"
	case PermUnknown:
		return "unknown"
	default:
		return "unknown"
	}
}

func roleHasPermission(r Role, p Permission) bool {
	switch r {
	case RolePlayer, RoleBot:
		return p == PermTablePlay
	case RoleModerator:
		switch p {
		case PermTablePlay, PermUserKick, PermUserBan, PermServerAnnounce:
			return true
		case PermUnknown, PermRoleAssign:
			return false
		default:
			return false
		}
	case RoleAdmin:
		return p.Valid()
	case RoleUnknown:
		return false
	default:
		return false
	}
}

// ModerationAction identifies an audited administrative action recorded in moderation_logs.
type ModerationAction uint8

const (
	ModActionUnknown ModerationAction = iota
	ModActionBan
	ModActionUnban
	ModActionAssignRole
	ModActionRevokeRole
)

// Valid reports whether a is a known moderation action.
func (a ModerationAction) Valid() bool {
	return a >= ModActionBan && a <= ModActionRevokeRole
}

// String returns the canonical database action name.
func (a ModerationAction) String() string {
	switch a {
	case ModActionBan:
		return "ban"
	case ModActionUnban:
		return "unban"
	case ModActionAssignRole:
		return "assign_role"
	case ModActionRevokeRole:
		return "revoke_role"
	case ModActionUnknown:
		return "unknown"
	default:
		return "unknown"
	}
}

// ParseModerationAction parses a stored action name into a ModerationAction.
func ParseModerationAction(s string) (ModerationAction, error) {
	switch strings.ToLower(strings.TrimSpace(s)) {
	case "ban":
		return ModActionBan, nil
	case "unban":
		return ModActionUnban, nil
	case "assign_role":
		return ModActionAssignRole, nil
	case "revoke_role":
		return ModActionRevokeRole, nil
	default:
		return ModActionUnknown, fmt.Errorf("moderation action %q: %w", s, ErrInvalidAction)
	}
}

// ReadConsistency controls rqlite query read consistency level.
type ReadConsistency uint8

const (
	ReadConsistencyUnknown ReadConsistency = iota
	ReadConsistencyNone
	ReadConsistencyWeak
	ReadConsistencyStrong
)

// Valid reports whether c is a valid read consistency level.
func (c ReadConsistency) Valid() bool {
	return c >= ReadConsistencyNone && c <= ReadConsistencyStrong
}

// String returns the rqlite query parameter value for c.
func (c ReadConsistency) String() string {
	switch c {
	case ReadConsistencyNone:
		return "none"
	case ReadConsistencyWeak:
		return "weak"
	case ReadConsistencyStrong:
		return "strong"
	case ReadConsistencyUnknown:
		return "unknown"
	default:
		return "unknown"
	}
}

// Account is the public-safe representation of a player or built-in bot account.
// It intentionally never carries PasswordHash.
type Account struct {
	ID             int64
	Username       string
	Balance        int64
	GamesPlayed    int64
	SelfDrawWins   int64
	DiscardWins    int64
	DealIns        int64
	Draws          int64
	AbandonedHands int64
	LoginCount     int64
	LastLoginAt    *time.Time
	LastNodeName   string
	BannedUntil    *time.Time
	BannedReason   string
	CreatedAt      time.Time
	Roles          []Role
}

// HasRole reports whether the account holds role r.
func (a Account) HasRole(r Role) bool {
	return slices.Contains(a.Roles, r)
}

// HasPermission reports whether any role on the account grants permission p.
func (a Account) HasPermission(p Permission) bool {
	if !p.Valid() {
		return false
	}
	for _, r := range a.Roles {
		if roleHasPermission(r, p) {
			return true
		}
	}
	return false
}

// IsBot reports whether the account is a built-in AI bot account.
func (a Account) IsBot() bool {
	return a.HasRole(RoleBot)
}

// IsBannedAt reports whether the account is actively banned at timestamp now.
func (a Account) IsBannedAt(now time.Time) bool {
	if a.BannedUntil == nil {
		return false
	}
	return a.BannedUntil.After(now)
}

// AuthResult contains the authenticated account and whether QKMJ 95p3 login relief was granted.
type AuthResult struct {
	Account       Account
	ReliefGranted bool
}

// AccountRank captures a player's ranking among all accounts with at least RankMinGames played.
type AccountRank struct {
	Qualified bool
	Rank      int64
	Total     int64
}

// SettlementEntry describes one seat's balance and lifetime stat delta for a settled hand.
type SettlementEntry struct {
	Username       string `json:"username"`
	Delta          int64  `json:"delta"`
	IncrementGames bool   `json:"increment_games,omitzero"`
	SelfDrawWin    bool   `json:"self_draw_win,omitzero"`
	DiscardWin     bool   `json:"discard_win,omitzero"`
	DealIn         bool   `json:"deal_in,omitzero"`
	Draw           bool   `json:"draw,omitzero"`
}

// HandSettlement is the semantic atomic payload passed to Store.SettleHand.
type HandSettlement struct {
	HandID    string
	NodeName  string
	EndReason string
	IsDraw    bool
	Entries   []SettlementEntry
}

// ModerationLog is an immutable audit record stored in moderation_logs.
type ModerationLog struct {
	ID              int64
	ActorAccountID  int64
	ActorUsername   string
	TargetAccountID int64
	TargetUsername  string
	Action          ModerationAction
	Reason          string
	ExpiresAt       *time.Time
	CreatedAt       time.Time
}

// BeepPrefs stores per-event sound alert preferences matching QKMJ 0.95d.
type BeepPrefs struct {
	Claim  bool `json:"claim"`
	Turn   bool `json:"turn"`
	Chat   bool `json:"chat"`
	Invite bool `json:"invite"`
	Prompt bool `json:"prompt"`
}

// TileColorPrefs stores per-suit/category color overrides for accessibility.
type TileColorPrefs struct {
	Back   string `json:"back,omitempty"`
	Wan    string `json:"wan,omitempty"`
	Suo    string `json:"suo,omitempty"`
	Tong   string `json:"tong,omitempty"`
	Wind   string `json:"wind,omitempty"`
	Dragon string `json:"dragon,omitempty"`
}

// DisplayPrefs stores terminal rendering preferences.
type DisplayPrefs struct {
	ColorLevel     string `json:"color_level,omitempty"`
	AmbiguousWidth string `json:"ambiguous_width,omitempty"`
	ASCIIBorders   bool   `json:"ascii_borders,omitzero"`
}

// UserPreferences stores server-persisted personal preferences replacing legacy .qkmjrc.
type UserPreferences struct {
	DefaultNote  string            `json:"default_note,omitempty"`
	Beep         BeepPrefs         `json:"beep"`
	TileColors   TileColorPrefs    `json:"tile_colors"`
	Display      DisplayPrefs      `json:"display"`
	FunctionKeys map[string]string `json:"function_keys,omitempty"`
}

// DefaultUserPreferences returns the default preference set for new accounts.
func DefaultUserPreferences() UserPreferences {
	return UserPreferences{
		DefaultNote: "Welcome!",
		Beep: BeepPrefs{
			Claim:  false,
			Turn:   true,
			Chat:   true,
			Invite: true,
			Prompt: true,
		},
		Display: DisplayPrefs{
			ColorLevel:     "truecolor",
			AmbiguousWidth: "auto",
		},
		FunctionKeys: map[string]string{
			"F1": "/free",
			"F2": "/player",
			"F3": "/table",
			"F4": "/stat",
		},
	}
}
