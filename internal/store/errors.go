package store

import (
	"errors"
	"fmt"
	"time"
)

// Sentinel errors returned by Store operations.
var (
	ErrInvalidConfig       = errors.New("invalid store configuration")
	ErrInvalidUsername     = errors.New("invalid username")
	ErrReservedUsername    = errors.New("username prefix is reserved")
	ErrInvalidPassword     = errors.New("invalid password")
	ErrUsernameTaken       = errors.New("username already taken")
	ErrAccountNotFound     = errors.New("account not found")
	ErrInvalidCredentials  = errors.New("invalid credentials")
	ErrBotLoginForbidden   = errors.New("bot account login forbidden")
	ErrAccountBanned       = errors.New("account is banned")
	ErrPermissionDenied    = errors.New("permission denied")
	ErrInvalidRole         = errors.New("invalid role")
	ErrInvalidAction       = errors.New("invalid moderation action")
	ErrInvalidSettlement   = errors.New("invalid hand settlement")
	ErrDuplicateSettlement = errors.New("duplicate hand settlement")
	ErrSelfBlock           = errors.New("cannot block self")
	ErrBlockLimitReached   = errors.New("block list limit reached")
	ErrNotBlocked          = errors.New("target user is not blocked")
	ErrBackendFailure      = errors.New("storage backend failure")
)

// AccountBannedError provides ban reason and optional expiry while matching ErrAccountBanned via errors.Is.
type AccountBannedError struct {
	Reason string
	Until  *time.Time
}

// Error implements the error interface.
func (e *AccountBannedError) Error() string {
	if e.Until != nil {
		return fmt.Sprintf("account banned until %s: %s", e.Until.UTC().Format(time.RFC3339), e.Reason)
	}
	return "account permanently banned: " + e.Reason
}

// Unwrap allows errors.Is(err, ErrAccountBanned) to match.
func (*AccountBannedError) Unwrap() error {
	return ErrAccountBanned
}
