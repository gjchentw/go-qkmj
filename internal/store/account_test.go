package store

import (
	"errors"
	"sync"
	"testing"
	"time"
)

type fakeClock struct {
	mu  sync.Mutex
	now time.Time
}

func newFakeClock(start time.Time) *fakeClock {
	return &fakeClock{now: start.UTC()}
}

func (c *fakeClock) Now() time.Time {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.now
}

func (c *fakeClock) Advance(d time.Duration) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.now = c.now.Add(d)
}

func testArgon2Params() Argon2Params {
	return Argon2Params{MemoryKiB: 1024, Iterations: 1, Parallelism: 1, KeyLen: 32, SaltLen: 16}
}

func runDualBackend(t *testing.T, fn func(t *testing.T, s *Store, clk *fakeClock)) {
	t.Helper()
	for _, backend := range []string{"sqlite", "rqlite"} {
		t.Run(backend, func(t *testing.T) {
			t.Parallel()
			clk := newFakeClock(time.Date(2026, 11, 28, 12, 0, 0, 0, time.UTC))
			opts := []Option{
				WithClock(clk.Now),
				WithArgon2Params(testArgon2Params()),
			}
			var (
				s   *Store
				err error
			)
			if backend == "sqlite" {
				s, err = OpenSQLite(t.Context(), ":memory:", opts...)
			} else {
				fake := newFakeRqliteServer(t.Context(), t)
				opts = append(opts, WithHTTPClient(fake.server.Client()), WithReadConsistency(ReadConsistencyWeak))
				s, err = OpenRqlite(t.Context(), []string{fake.server.URL}, opts...)
			}
			if err != nil {
				t.Fatalf("open %s store: %v", backend, err)
			}
			t.Cleanup(func() { _ = s.Close() })
			fn(t, s, clk)
		})
	}
}

func TestStore_OptionsValidation(t *testing.T) {
	t.Parallel()
	ctx := t.Context()

	cases := []struct {
		name string
		opt  Option
	}{
		{name: "nil option", opt: nil},
		{name: "nil clock", opt: WithClock(nil)},
		{name: "invalid argon2", opt: WithArgon2Params(Argon2Params{})},
		{name: "non-positive max blocks", opt: WithMaxBlocks(0)},
		{name: "non-positive timeout", opt: WithRequestTimeout(0)},
		{name: "unknown read consistency", opt: WithReadConsistency(ReadConsistencyUnknown)},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			if _, err := OpenSQLite(ctx, ":memory:", tc.opt); !errors.Is(err, ErrInvalidConfig) {
				t.Fatalf("OpenSQLite(%s) err = %v, want ErrInvalidConfig", tc.name, err)
			}
		})
	}
}

func TestStore_AccountsAuthReliefRankAndBots(t *testing.T) {
	t.Parallel()

	runDualBackend(t, func(t *testing.T, s *Store, clk *fakeClock) {
		t.Helper()
		ctx := t.Context()

		// 1. Built-in bots QKMJ0..QKMJ9 exist and cannot authenticate.
		bots, err := s.ListBotAccounts(ctx)
		if err != nil {
			t.Fatalf("ListBotAccounts: %v", err)
		}
		if len(bots) != BotAccountCount || bots[0].Username != "QKMJ0" || bots[9].Username != "QKMJ9" {
			t.Fatalf("bots = %+v, want 10 bots QKMJ0..QKMJ9", bots)
		}
		for _, b := range bots {
			if !b.IsBot() || b.Balance != InitialBalance {
				t.Fatalf("bot %s invalid: roles=%v balance=%d", b.Username, b.Roles, b.Balance)
			}
		}
		if _, err := s.Authenticate(ctx, "qkmj0", "anypassword123", "calm-otter"); !errors.Is(err, ErrBotLoginForbidden) {
			t.Fatalf("Authenticate(qkmj0) err = %v, want ErrBotLoginForbidden", err)
		}

		// 2. Reserved prefix & duplicate case-insensitive registration rejected.
		if _, err := s.CreateAccount(ctx, "qkmjHero", "secret1234"); !errors.Is(err, ErrReservedUsername) {
			t.Fatalf("CreateAccount(qkmjHero) err = %v, want ErrReservedUsername", err)
		}
		acc, err := s.CreateAccount(ctx, "Alice99", "secret1234")
		if err != nil {
			t.Fatalf("CreateAccount(Alice99): %v", err)
		}
		if acc.Username != "Alice99" || acc.Balance != InitialBalance || !acc.HasRole(RolePlayer) {
			t.Fatalf("created account = %+v", acc)
		}
		if _, err := s.CreateAccount(ctx, "ALICE99", "anotherpass12"); !errors.Is(err, ErrUsernameTaken) {
			t.Fatalf("duplicate case-insensitive err = %v, want ErrUsernameTaken", err)
		}

		// 3. Authenticate case-insensitive updates login_count, last_login_at, last_node_name; wrong password rejected.
		if _, err := s.Authenticate(ctx, "alice99", "wrongpass123", "calm-otter"); !errors.Is(err, ErrInvalidCredentials) {
			t.Fatalf("wrong password err = %v, want ErrInvalidCredentials", err)
		}
		clk.Advance(5 * time.Minute)
		auth1, err := s.Authenticate(ctx, "alice99", "secret1234", "calm-otter")
		if err != nil {
			t.Fatalf("Authenticate(alice99): %v", err)
		}
		if auth1.ReliefGranted || auth1.Account.Username != "Alice99" || auth1.Account.LoginCount != 1 || auth1.Account.LastNodeName != "calm-otter" || auth1.Account.LastLoginAt == nil || !auth1.Account.LastLoginAt.Equal(clk.Now()) {
			t.Fatalf("auth1 = %+v", auth1)
		}

		// 4. ChangePassword verifies old password and rotates hash.
		if err := s.ChangePassword(ctx, "alice99", "wrongold123", "newsecret123"); !errors.Is(err, ErrInvalidCredentials) {
			t.Fatalf("ChangePassword(wrong old) err = %v, want ErrInvalidCredentials", err)
		}
		if err := s.ChangePassword(ctx, "alice99", "secret1234", "newsecret123"); err != nil {
			t.Fatalf("ChangePassword: %v", err)
		}
		if _, err := s.Authenticate(ctx, "alice99", "secret1234", "calm-otter"); !errors.Is(err, ErrInvalidCredentials) {
			t.Fatalf("old password still accepted: err=%v", err)
		}
		if _, err := s.Authenticate(ctx, "alice99", "newsecret123", "brisk-falcon"); err != nil {
			t.Fatalf("new password login failed: %v", err)
		}

		// 5. QKMJ 95p3 login relief: < 20000 && games_played >= 16 triggers relief; 15 games does NOT.
		if _, err := s.exec.ExecBatch(ctx, []stmt{{
			query: "UPDATE accounts SET balance = ?, games_played = ? WHERE username_lower = ?",
			args:  []any{int64(5000), int64(15), "alice99"},
		}}); err != nil {
			t.Fatalf("setup 15 games: %v", err)
		}
		noRelief, err := s.Authenticate(ctx, "alice99", "newsecret123", "calm-otter")
		if err != nil || noRelief.ReliefGranted || noRelief.Account.Balance != 5000 {
			t.Fatalf("15-game auth = %+v, err=%v; want balance=5000 no relief", noRelief, err)
		}
		rank15, err := s.GetAccountRank(ctx, "alice99")
		if err != nil || rank15.Qualified {
			t.Fatalf("15-game rank = %+v, err=%v; want Qualified=false", rank15, err)
		}

		if _, err := s.exec.ExecBatch(ctx, []stmt{{
			query: "UPDATE accounts SET balance = ?, games_played = ? WHERE username_lower = ?",
			args:  []any{int64(-3500), int64(16), "alice99"},
		}}); err != nil {
			t.Fatalf("setup 16 games negative balance: %v", err)
		}
		withRelief, err := s.Authenticate(ctx, "alice99", "newsecret123", "keen-badger")
		if err != nil || !withRelief.ReliefGranted || withRelief.Account.Balance != ReliefThreshold || withRelief.Account.LastNodeName != "keen-badger" {
			t.Fatalf("16-game relief auth = %+v, err=%v; want ReliefGranted=true balance=%d", withRelief, err, ReliefThreshold)
		}

		// 6. Ranking ties among accounts with games_played >= 16: 50000, 30000, 30000, 20000 (Alice99).
		for _, u := range []struct {
			name    string
			balance int64
		}{
			{"Bob", 50000},
			{"Carol", 30000},
			{"Dave", 30000},
		} {
			if _, err := s.CreateAccount(ctx, u.name, "password123"); err != nil {
				t.Fatalf("CreateAccount(%s): %v", u.name, err)
			}
			if _, err := s.exec.ExecBatch(ctx, []stmt{{
				query: "UPDATE accounts SET balance = ?, games_played = 16 WHERE username_lower = ?",
				args:  []any{u.balance, lowerOf(t, u.name)},
			}}); err != nil {
				t.Fatalf("update %s balance: %v", u.name, err)
			}
		}

		for _, tc := range []struct {
			name      string
			wantRank  int64
			wantTotal int64
		}{
			{"Bob", 1, 4},
			{"Carol", 2, 4},
			{"Dave", 2, 4},
			{"Alice99", 4, 4},
		} {
			rk, err := s.GetAccountRank(ctx, tc.name)
			if err != nil {
				t.Fatalf("GetAccountRank(%s): %v", tc.name, err)
			}
			if !rk.Qualified || rk.Rank != tc.wantRank || rk.Total != tc.wantTotal {
				t.Errorf("GetAccountRank(%s) = %+v, want Qualified=true %d/%d", tc.name, rk, tc.wantRank, tc.wantTotal)
			}
		}
	})
}

func lowerOf(t *testing.T, s string) string {
	t.Helper()
	_, lower, err := ValidateUsername(s)
	if err != nil {
		t.Fatalf("ValidateUsername(%q): %v", s, err)
	}
	return lower
}
