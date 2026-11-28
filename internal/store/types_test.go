package store_test

import (
	"errors"
	"testing"
	"time"

	"github.com/gjchentw/go-qkmj/internal/store"
)

func TestValidateUsernameAndReservedPrefix(t *testing.T) {
	t.Parallel()

	validCases := []struct {
		name      string
		input     string
		wantCanon string
		wantLower string
	}{
		{name: "single char", input: "A", wantCanon: "A", wantLower: "a"},
		{name: "mixed alphanumeric", input: "Alice99", wantCanon: "Alice99", wantLower: "alice99"},
		{name: "max 20 chars", input: "AbCdEfGhIj1234567890", wantCanon: "AbCdEfGhIj1234567890", wantLower: "abcdefghij1234567890"},
	}

	for _, tc := range validCases {
		t.Run("valid/"+tc.name, func(t *testing.T) {
			t.Parallel()
			canon, lower, err := store.ValidateRegistrationUsername(tc.input)
			if err != nil {
				t.Fatalf("ValidateRegistrationUsername(%q) unexpected err: %v", tc.input, err)
			}
			if canon != tc.wantCanon || lower != tc.wantLower {
				t.Fatalf("got (%q, %q), want (%q, %q)", canon, lower, tc.wantCanon, tc.wantLower)
			}
		})
	}

	invalidCases := []struct {
		name    string
		input   string
		wantErr error
	}{
		{name: "empty", input: "", wantErr: store.ErrInvalidUsername},
		{name: "too long 21 chars", input: "AbCdEfGhIj12345678901", wantErr: store.ErrInvalidUsername},
		{name: "space", input: "Alice 1", wantErr: store.ErrInvalidUsername},
		{name: "underscore", input: "alice_1", wantErr: store.ErrInvalidUsername},
		{name: "dash", input: "alice-1", wantErr: store.ErrInvalidUsername},
		{name: "unicode chinese", input: "小明", wantErr: store.ErrInvalidUsername},
		{name: "control char", input: "alice\n", wantErr: store.ErrInvalidUsername},
		{name: "reserved exact lower", input: "qkmj", wantErr: store.ErrReservedUsername},
		{name: "reserved exact upper", input: "QKMJ0", wantErr: store.ErrReservedUsername},
		{name: "reserved mixed case", input: "QkMjHero", wantErr: store.ErrReservedUsername},
	}

	for _, tc := range invalidCases {
		t.Run("invalid/"+tc.name, func(t *testing.T) {
			t.Parallel()
			_, _, err := store.ValidateRegistrationUsername(tc.input)
			if !errors.Is(err, tc.wantErr) {
				t.Fatalf("ValidateRegistrationUsername(%q) err = %v, want %v", tc.input, err, tc.wantErr)
			}
		})
	}

	// ValidateUsername (for lookup, e.g. /stat QKMJ0) MUST allow reserved prefix while rejecting bad syntax.
	canon, lower, err := store.ValidateUsername("QKMJ0")
	if err != nil || canon != "QKMJ0" || lower != "qkmj0" {
		t.Fatalf("ValidateUsername(QKMJ0) = (%q, %q, %v), want (QKMJ0, qkmj0, nil)", canon, lower, err)
	}
	if !store.IsReservedUsername("qKmJ9") || store.IsReservedUsername("player1") {
		t.Fatalf("IsReservedUsername mismatch")
	}
}

func TestRBAC_RolesAndPermissions(t *testing.T) {
	t.Parallel()

	if store.RoleUnknown.Valid() || store.PermUnknown.Valid() || store.ModActionUnknown.Valid() || store.ReadConsistencyUnknown.Valid() {
		t.Fatal("zero-value sentinels must report Valid() == false")
	}

	tests := []struct {
		name      string
		roles     []store.Role
		wantPerms map[store.Permission]bool
		wantBot   bool
	}{
		{
			name:  "player",
			roles: []store.Role{store.RolePlayer},
			wantPerms: map[store.Permission]bool{
				store.PermTablePlay:      true,
				store.PermUserKick:       false,
				store.PermUserBan:        false,
				store.PermServerAnnounce: false,
				store.PermRoleAssign:     false,
			},
		},
		{
			name:  "moderator",
			roles: []store.Role{store.RolePlayer, store.RoleModerator},
			wantPerms: map[store.Permission]bool{
				store.PermTablePlay:      true,
				store.PermUserKick:       true,
				store.PermUserBan:        true,
				store.PermServerAnnounce: true,
				store.PermRoleAssign:     false,
			},
		},
		{
			name:  "admin",
			roles: []store.Role{store.RoleAdmin},
			wantPerms: map[store.Permission]bool{
				store.PermTablePlay:      true,
				store.PermUserKick:       true,
				store.PermUserBan:        true,
				store.PermServerAnnounce: true,
				store.PermRoleAssign:     true,
			},
		},
		{
			name:    "bot",
			roles:   []store.Role{store.RoleBot},
			wantBot: true,
			wantPerms: map[store.Permission]bool{
				store.PermTablePlay:  true,
				store.PermUserKick:   false,
				store.PermUserBan:    false,
				store.PermRoleAssign: false,
			},
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			acc := store.Account{Username: "TestUser", Roles: tc.roles}
			if acc.IsBot() != tc.wantBot {
				t.Errorf("IsBot() = %v, want %v", acc.IsBot(), tc.wantBot)
			}
			for perm, want := range tc.wantPerms {
				if got := acc.HasPermission(perm); got != want {
					t.Errorf("HasPermission(%v) = %v, want %v", perm, got, want)
				}
			}
		})
	}

	for _, roleStr := range []string{"player", "moderator", "admin", "bot"} {
		r, err := store.ParseRole(roleStr)
		if err != nil || !r.Valid() || r.String() != roleStr {
			t.Errorf("ParseRole(%q) = (%v, %v)", roleStr, r, err)
		}
	}
	if _, err := store.ParseRole("superuser"); !errors.Is(err, store.ErrInvalidRole) {
		t.Errorf("ParseRole(superuser) err = %v, want ErrInvalidRole", err)
	}

	now := time.Date(2026, 11, 28, 10, 0, 0, 0, time.UTC)
	if (store.Account{}).IsBannedAt(now) {
		t.Error("unbanned account should not be banned")
	}
	if !(store.Account{BannedUntil: new(now.Add(time.Hour))}).IsBannedAt(now) {
		t.Error("account with future BannedUntil should be banned")
	}
	if (store.Account{BannedUntil: new(now.Add(-time.Minute))}).IsBannedAt(now) {
		t.Error("account with expired BannedUntil should not be banned")
	}
}
