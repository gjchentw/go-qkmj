package store

import (
	"errors"
	"slices"
	"testing"
	"time"
)

func TestStore_RBACModerationAndAuditLogs(t *testing.T) {
	t.Parallel()

	runDualBackend(t, func(t *testing.T, s *Store, clk *fakeClock) {
		t.Helper()
		ctx := t.Context()
		for _, name := range []string{"RootAdmin", "ModOne", "PlayerA", "BadUser"} {
			if _, err := s.CreateAccount(ctx, name, "password123"); err != nil {
				t.Fatalf("CreateAccount(%s): %v", name, err)
			}
		}

		// BootstrapAdmin grants RoleAdmin without requiring a prior admin.
		if err := s.BootstrapAdmin(ctx, "RootAdmin"); err != nil {
			t.Fatalf("BootstrapAdmin: %v", err)
		}
		root, err := s.GetAccount(ctx, "RootAdmin")
		if err != nil || !root.HasRole(RoleAdmin) || !root.HasPermission(PermRoleAssign) {
			t.Fatalf("RootAdmin = %+v, err=%v", root, err)
		}

		// Non-admin (PlayerA) cannot assign roles or ban accounts.
		if err := s.AssignRole(ctx, "PlayerA", "ModOne", RoleModerator, "nope"); !errors.Is(err, ErrPermissionDenied) {
			t.Fatalf("PlayerA AssignRole err = %v, want ErrPermissionDenied", err)
		}
		if err := s.BanAccount(ctx, "PlayerA", "BadUser", "spam", new(clk.Now().Add(time.Hour))); !errors.Is(err, ErrPermissionDenied) {
			t.Fatalf("PlayerA BanAccount err = %v, want ErrPermissionDenied", err)
		}

		// Admin assigns RoleModerator to ModOne -> audited in moderation_logs.
		if err := s.AssignRole(ctx, "RootAdmin", "ModOne", RoleModerator, "trusted community mod"); err != nil {
			t.Fatalf("Admin AssignRole(ModOne): %v", err)
		}
		// Moderator CANNOT assign RoleAdmin.
		if err := s.AssignRole(ctx, "ModOne", "PlayerA", RoleAdmin, "escalation"); !errors.Is(err, ErrPermissionDenied) {
			t.Fatalf("ModOne AssignRole(RoleAdmin) err = %v, want ErrPermissionDenied", err)
		}

		// Moderator bans BadUser for 2 hours -> Authenticate MUST fail with AccountBannedError!
		until := clk.Now().Add(2 * time.Hour)
		if err := s.BanAccount(ctx, "ModOne", "BadUser", "abusive chat", &until); err != nil {
			t.Fatalf("ModOne BanAccount: %v", err)
		}
		_, err = s.Authenticate(ctx, "BadUser", "password123", "calm-otter")
		if !errors.Is(err, ErrAccountBanned) {
			t.Fatalf("banned login err = %v, want ErrAccountBanned", err)
		}
		bannedErr, ok := errors.AsType[*AccountBannedError](err)
		if !ok || bannedErr.Reason != "abusive chat" || bannedErr.Until == nil || !bannedErr.Until.Equal(until) {
			t.Fatalf("AccountBannedError = %+v (ok=%v)", bannedErr, ok)
		}

		// After ban expires, login succeeds!
		clk.Advance(3 * time.Hour)
		if _, err := s.Authenticate(ctx, "BadUser", "password123", "calm-otter"); err != nil {
			t.Fatalf("login after ban expiry failed: %v", err)
		}

		// Re-ban and UnbanAccount restores immediate login.
		until2 := clk.Now().Add(24 * time.Hour)
		if err := s.BanAccount(ctx, "ModOne", "BadUser", "repeat offense", &until2); err != nil {
			t.Fatalf("second BanAccount: %v", err)
		}
		if err := s.UnbanAccount(ctx, "ModOne", "BadUser", "appeal approved"); err != nil {
			t.Fatalf("UnbanAccount: %v", err)
		}
		if _, err := s.Authenticate(ctx, "BadUser", "password123", "calm-otter"); err != nil {
			t.Fatalf("login after UnbanAccount failed: %v", err)
		}

		// Admin revokes ModOne's moderator role -> ModOne loses PermUserBan.
		if err := s.RevokeRole(ctx, "RootAdmin", "ModOne", RoleModerator, "stepped down"); err != nil {
			t.Fatalf("RevokeRole: %v", err)
		}
		if err := s.BanAccount(ctx, "ModOne", "BadUser", "after revoke", new(clk.Now().Add(time.Hour))); !errors.Is(err, ErrPermissionDenied) {
			t.Fatalf("revoked mod BanAccount err = %v, want ErrPermissionDenied", err)
		}

		// Verify moderation_logs recorded all actions for BadUser in reverse chronological order.
		logs, err := s.ListModerationLogs(ctx, "BadUser", 10)
		if err != nil {
			t.Fatalf("ListModerationLogs(BadUser): %v", err)
		}
		if len(logs) != 3 || logs[0].Action != ModActionUnban || logs[1].Action != ModActionBan || logs[2].Action != ModActionBan {
			t.Fatalf("BadUser logs = %+v, want [unban, ban, ban]", logs)
		}
		if logs[0].ActorUsername != "ModOne" || logs[0].Reason != "appeal approved" {
			t.Fatalf("latest log = %+v", logs[0])
		}
	})
}

func TestStore_PreferencesAndUserBlocks(t *testing.T) {
	t.Parallel()

	runDualBackend(t, func(t *testing.T, s *Store, _ *fakeClock) {
		t.Helper()
		ctx := t.Context()
		for _, name := range []string{"Alice", "Bob", "Carol"} {
			if _, err := s.CreateAccount(ctx, name, "password123"); err != nil {
				t.Fatalf("CreateAccount(%s): %v", name, err)
			}
		}

		// 1. Default preferences returned when none saved yet.
		prefs, err := s.GetPreferences(ctx, "Alice")
		if err != nil {
			t.Fatalf("GetPreferences(default): %v", err)
		}
		if prefs.DefaultNote != "Welcome!" || !prefs.Beep.Turn || prefs.Display.ASCIIBorders {
			t.Fatalf("default prefs = %+v", prefs)
		}

		// Save updated preferences and read back.
		prefs.DefaultNote = "Quiet Fast Game"
		prefs.TileColors.Tong = "BLUE"
		prefs.Display.ASCIIBorders = true
		prefs.Beep.Chat = false
		if err := s.SavePreferences(ctx, "alice", prefs); err != nil {
			t.Fatalf("SavePreferences: %v", err)
		}
		loaded, err := s.GetPreferences(ctx, "ALICE")
		if err != nil {
			t.Fatalf("GetPreferences(loaded): %v", err)
		}
		if loaded.DefaultNote != "Quiet Fast Game" || loaded.TileColors.Tong != "BLUE" || !loaded.Display.ASCIIBorders || loaded.Beep.Chat {
			t.Fatalf("loaded prefs = %+v, want updated values", loaded)
		}

		// Unknown JSON fields stored in DB are gracefully ignored without error.
		if _, err := s.exec.ExecBatch(ctx, []stmt{{
			query: "UPDATE user_preferences SET prefs_json = ? WHERE account_id = (SELECT id FROM accounts WHERE username_lower = ?)",
			args:  []any{`{"default_note":"Legacy","future_unknown_key":123}`, "alice"},
		}}); err != nil {
			t.Fatalf("inject unknown prefs json: %v", err)
		}
		tolerant, err := s.GetPreferences(ctx, "Alice")
		if err != nil || tolerant.DefaultNote != "Legacy" {
			t.Fatalf("unknown field tolerance = %+v, err=%v", tolerant, err)
		}

		// 2. Personal blocklist: forbid self-block, enforce limit, add/check/list/remove.
		if err := s.BlockUser(ctx, "Alice", "alice"); !errors.Is(err, ErrSelfBlock) {
			t.Fatalf("self block err = %v, want ErrSelfBlock", err)
		}
		if err := s.BlockUser(ctx, "Alice", "GhostUser"); !errors.Is(err, ErrAccountNotFound) {
			t.Fatalf("block unknown user err = %v, want ErrAccountNotFound", err)
		}
		if err := s.BlockUser(ctx, "Alice", "Bob"); err != nil {
			t.Fatalf("BlockUser(Alice->Bob): %v", err)
		}
		// Idempotent re-block succeeds cleanly.
		if err := s.BlockUser(ctx, "Alice", "bob"); err != nil {
			t.Fatalf("idempotent BlockUser(Alice->bob): %v", err)
		}
		if err := s.BlockUser(ctx, "Alice", "Carol"); err != nil {
			t.Fatalf("BlockUser(Alice->Carol): %v", err)
		}
		blocked, err := s.IsBlocked(ctx, "Alice", "Bob")
		if err != nil || !blocked {
			t.Fatalf("IsBlocked(Alice, Bob) = (%v, %v), want (true, nil)", blocked, err)
		}
		revBlocked, err := s.IsBlocked(ctx, "Bob", "Alice")
		if err != nil || revBlocked {
			t.Fatalf("IsBlocked(Bob, Alice) = (%v, %v), want (false, nil)", revBlocked, err)
		}
		list, err := s.ListBlockedUsers(ctx, "Alice")
		if err != nil || !slices.Equal(list, []string{"Bob", "Carol"}) {
			t.Fatalf("ListBlockedUsers = %v, err=%v; want [Bob Carol]", list, err)
		}
		if err := s.UnblockUser(ctx, "Alice", "Bob"); err != nil {
			t.Fatalf("UnblockUser: %v", err)
		}
		blockedAfter, err := s.IsBlocked(ctx, "Alice", "Bob")
		if err != nil || blockedAfter {
			t.Fatalf("IsBlocked after unblock = (%v, %v), want (false, nil)", blockedAfter, err)
		}
		if err := s.UnblockUser(ctx, "Alice", "Bob"); !errors.Is(err, ErrNotBlocked) {
			t.Fatalf("second UnblockUser err = %v, want ErrNotBlocked", err)
		}
	})
}

func TestStore_BlockLimitEnforced(t *testing.T) {
	t.Parallel()
	ctx := t.Context()

	s, err := OpenSQLite(ctx, ":memory:", WithArgon2Params(testArgon2Params()), WithMaxBlocks(2))
	if err != nil {
		t.Fatalf("OpenSQLite: %v", err)
	}
	defer func() { _ = s.Close() }()

	for _, name := range []string{"Alice", "U1", "U2", "U3"} {
		if _, err := s.CreateAccount(ctx, name, "password123"); err != nil {
			t.Fatalf("CreateAccount(%s): %v", name, err)
		}
	}
	if err := s.BlockUser(ctx, "Alice", "U1"); err != nil {
		t.Fatalf("block U1: %v", err)
	}
	if err := s.BlockUser(ctx, "Alice", "U2"); err != nil {
		t.Fatalf("block U2: %v", err)
	}
	if err := s.BlockUser(ctx, "Alice", "U3"); !errors.Is(err, ErrBlockLimitReached) {
		t.Fatalf("block U3 over limit err = %v, want ErrBlockLimitReached", err)
	}
}
