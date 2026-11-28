package store

import (
	"crypto/rand"
	"crypto/subtle"
	"encoding/base64"
	"fmt"
	"strconv"
	"strings"

	"golang.org/x/crypto/argon2"
)

// Password policy boundaries (NIST SP 800-63B aligned, no bcrypt 72-byte truncation).
const (
	MinPasswordBytes = 8
	MaxPasswordBytes = 128

	// BotUnmatchableHash is stored for built-in AI bot accounts so password verification always fails.
	BotUnmatchableHash = "!"
)

// Argon2Params configures Argon2id memory-hard key derivation parameters.
type Argon2Params struct {
	MemoryKiB   uint32
	Iterations  uint32
	Parallelism uint8
	KeyLen      uint32
	SaltLen     uint32
}

// DefaultArgon2Params returns OWASP-recommended Argon2id parameters (19 MiB, t=2, p=1).
func DefaultArgon2Params() Argon2Params {
	return Argon2Params{
		MemoryKiB:   19456,
		Iterations:  2,
		Parallelism: 1,
		KeyLen:      32,
		SaltLen:     16,
	}
}

// Validate checks that params are non-zero and safe for key derivation.
func (p Argon2Params) Validate() error {
	if p.MemoryKiB < 64 || p.Iterations == 0 || p.Parallelism == 0 || p.KeyLen < 16 || p.SaltLen < 8 {
		return fmt.Errorf("invalid argon2 params %+v: %w", p, ErrInvalidConfig)
	}
	return nil
}

// ValidatePassword checks password length (8..128 bytes) and rejects ASCII control characters.
func ValidatePassword(password string) error {
	n := len(password)
	if n < MinPasswordBytes || n > MaxPasswordBytes {
		return fmt.Errorf("password length %d outside [%d,%d]: %w", n, MinPasswordBytes, MaxPasswordBytes, ErrInvalidPassword)
	}
	for i := range n {
		b := password[i]
		if b < 0x20 || b == 0x7f {
			return fmt.Errorf("password contains control character: %w", ErrInvalidPassword)
		}
	}
	return nil
}

// HashPassword validates password and returns a PHC-formatted Argon2id hash string using crypto/rand salt.
func HashPassword(password string, params Argon2Params) (string, error) {
	if err := ValidatePassword(password); err != nil {
		return "", err
	}
	if err := params.Validate(); err != nil {
		return "", err
	}

	salt := make([]byte, params.SaltLen)
	if _, err := rand.Read(salt); err != nil {
		return "", fmt.Errorf("generate password salt: %w", err)
	}

	sum := argon2.IDKey([]byte(password), salt, params.Iterations, params.MemoryKiB, params.Parallelism, params.KeyLen)
	b64Salt := base64.RawStdEncoding.EncodeToString(salt)
	b64Sum := base64.RawStdEncoding.EncodeToString(sum)

	return fmt.Sprintf("$argon2id$v=%d$m=%d,t=%d,p=%d$%s$%s",
		argon2.Version, params.MemoryKiB, params.Iterations, params.Parallelism, b64Salt, b64Sum), nil
}

// VerifyPassword compares password against a PHC-formatted Argon2id hash using constant-time comparison.
// It returns (false, nil) for BotUnmatchableHash.
func VerifyPassword(password, phcHash string) (bool, error) {
	if phcHash == BotUnmatchableHash {
		return false, nil
	}
	params, salt, expected, err := parsePHCArgon2id(phcHash)
	if err != nil {
		return false, err
	}
	actual := argon2.IDKey([]byte(password), salt, params.Iterations, params.MemoryKiB, params.Parallelism, params.KeyLen)
	return subtle.ConstantTimeCompare(actual, expected) == 1, nil
}

func parsePHCArgon2id(phc string) (params Argon2Params, salt, expected []byte, err error) {
	parts := strings.Split(phc, "$")
	if len(parts) != 6 || parts[0] != "" || parts[1] != "argon2id" {
		return Argon2Params{}, nil, nil, fmt.Errorf("unsupported password hash format: %w", ErrInvalidCredentials)
	}
	var version int
	if _, scanErr := fmt.Sscanf(parts[2], "v=%d", &version); scanErr != nil || version != argon2.Version {
		return Argon2Params{}, nil, nil, fmt.Errorf("unsupported argon2 version: %w", ErrInvalidCredentials)
	}

	params, err = parseArgon2ParamSection(parts[3])
	if err != nil {
		return Argon2Params{}, nil, nil, err
	}

	salt, err = base64.RawStdEncoding.DecodeString(parts[4])
	if err != nil || len(salt) == 0 {
		return Argon2Params{}, nil, nil, fmt.Errorf("decode argon2 salt: %w", ErrInvalidCredentials)
	}
	expected, err = base64.RawStdEncoding.DecodeString(parts[5])
	if err != nil || len(expected) == 0 {
		return Argon2Params{}, nil, nil, fmt.Errorf("decode argon2 digest: %w", ErrInvalidCredentials)
	}

	params.SaltLen = uint32(len(salt))    //nolint:gosec // salt length validated small PHC token
	params.KeyLen = uint32(len(expected)) //nolint:gosec // digest length validated small PHC token
	if valErr := params.Validate(); valErr != nil {
		return Argon2Params{}, nil, nil, fmt.Errorf("invalid stored argon2 params: %w", ErrInvalidCredentials)
	}
	return params, salt, expected, nil
}

func parseArgon2ParamSection(section string) (Argon2Params, error) {
	var p Argon2Params
	for kv := range strings.SplitSeq(section, ",") {
		k, v, ok := strings.Cut(kv, "=")
		if !ok {
			return Argon2Params{}, fmt.Errorf("malformed argon2 param %q: %w", kv, ErrInvalidCredentials)
		}
		n, err := strconv.ParseUint(v, 10, 32)
		if err != nil {
			return Argon2Params{}, fmt.Errorf("parse argon2 param %q: %w", kv, ErrInvalidCredentials)
		}
		switch k {
		case "m":
			p.MemoryKiB = uint32(n)
		case "t":
			p.Iterations = uint32(n)
		case "p":
			if n > 255 {
				return Argon2Params{}, fmt.Errorf("argon2 parallelism overflow: %w", ErrInvalidCredentials)
			}
			p.Parallelism = uint8(n)
		default:
			return Argon2Params{}, fmt.Errorf("unknown argon2 param %q: %w", k, ErrInvalidCredentials)
		}
	}
	return p, nil
}
