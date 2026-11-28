package store_test

import (
	"errors"
	"strings"
	"testing"

	"github.com/gjchentw/go-qkmj/internal/store"
)

func TestPassword_Argon2id_PHC_AndValidation(t *testing.T) {
	t.Parallel()

	defaults := store.DefaultArgon2Params()
	if err := defaults.Validate(); err != nil {
		t.Fatalf("DefaultArgon2Params().Validate() err = %v", err)
	}
	if defaults.MemoryKiB < 19456 || defaults.Iterations < 2 || defaults.Parallelism < 1 || defaults.SaltLen < 16 || defaults.KeyLen < 32 {
		t.Fatalf("DefaultArgon2Params() = %+v, does not meet OWASP minimum", defaults)
	}

	t.Run("default OWASP params hash and verify roundtrip", func(t *testing.T) {
		t.Parallel()
		pw := "correct horse battery staple! 99"
		hash1, err := store.HashPassword(pw, defaults)
		if err != nil {
			t.Fatalf("HashPassword err = %v", err)
		}
		hash2, err := store.HashPassword(pw, defaults)
		if err != nil {
			t.Fatalf("second HashPassword err = %v", err)
		}
		if !strings.HasPrefix(hash1, "$argon2id$v=19$") {
			t.Fatalf("hash1 = %q, want PHC $argon2id$v=19$ prefix", hash1)
		}
		if hash1 == hash2 {
			t.Fatal("two hashes of identical password must use unique random salts")
		}
		ok, err := store.VerifyPassword(pw, hash1)
		if err != nil || !ok {
			t.Fatalf("VerifyPassword(correct) = (%v, %v), want (true, nil)", ok, err)
		}
		ok, err = store.VerifyPassword("wrong password 123", hash1)
		if err != nil || ok {
			t.Fatalf("VerifyPassword(wrong) = (%v, %v), want (false, nil)", ok, err)
		}
	})

	t.Run("over 72 bytes password is NOT truncated unlike bcrypt", func(t *testing.T) {
		t.Parallel()
		light := store.Argon2Params{MemoryKiB: 1024, Iterations: 1, Parallelism: 1, KeyLen: 32, SaltLen: 16}
		prefix := strings.Repeat("a", 80)
		pwA := prefix + "1"
		pwB := prefix + "2"
		hashA, err := store.HashPassword(pwA, light)
		if err != nil {
			t.Fatalf("HashPassword(81 bytes) err = %v", err)
		}
		ok, err := store.VerifyPassword(pwB, hashA)
		if err != nil || ok {
			t.Fatalf("VerifyPassword(different suffix after byte 72) = (%v, %v), want (false, nil)", ok, err)
		}
	})

	t.Run("validation boundaries 8..128 bytes and control chars", func(t *testing.T) {
		t.Parallel()
		light := store.Argon2Params{MemoryKiB: 1024, Iterations: 1, Parallelism: 1, KeyLen: 32, SaltLen: 16}
		bad := []string{
			"",
			"1234567",                // 7 bytes (< 8)
			strings.Repeat("x", 129), // 129 bytes (> 128)
			"validpass\x00withnull",  // control char
			"validpass\nwithnewline", // control char
		}
		for _, pw := range bad {
			if _, err := store.HashPassword(pw, light); !errors.Is(err, store.ErrInvalidPassword) {
				t.Errorf("HashPassword(len=%d) err = %v, want ErrInvalidPassword", len(pw), err)
			}
		}
		if _, err := store.HashPassword(strings.Repeat("z", 128), light); err != nil {
			t.Errorf("HashPassword(128 bytes) unexpected err = %v", err)
		}
	})

	t.Run("bot sentinel hash never matches", func(t *testing.T) {
		t.Parallel()
		ok, err := store.VerifyPassword("anything123", store.BotUnmatchableHash)
		if err != nil || ok {
			t.Fatalf("VerifyPassword(bot sentinel) = (%v, %v), want (false, nil)", ok, err)
		}
	})
}
