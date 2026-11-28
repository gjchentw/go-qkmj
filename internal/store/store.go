package store

import (
	"context"
	"fmt"
	"net/http"
	"time"
)

type storeConfig struct {
	now             func() time.Time
	argon2          Argon2Params
	maxBlocks       int
	requestTimeout  time.Duration
	readConsistency ReadConsistency
	httpClient      *http.Client
}

func defaultStoreConfig() storeConfig {
	return storeConfig{
		now:             func() time.Time { return time.Now().UTC() },
		argon2:          DefaultArgon2Params(),
		maxBlocks:       DefaultMaxBlocks,
		requestTimeout:  defaultRqliteTimeout,
		readConsistency: ReadConsistencyWeak,
	}
}

// Option configures Store construction with validation performed at open time.
type Option func(*storeConfig) error

// WithClock overrides the UTC clock source used for timestamps and ban checks.
func WithClock(fn func() time.Time) Option {
	return func(c *storeConfig) error {
		if fn == nil {
			return fmt.Errorf("clock func must not be nil: %w", ErrInvalidConfig)
		}
		c.now = fn
		return nil
	}
}

// WithArgon2Params overrides Argon2id parameters (primarily for fast unit tests).
func WithArgon2Params(p Argon2Params) Option {
	return func(c *storeConfig) error {
		if err := p.Validate(); err != nil {
			return err
		}
		c.argon2 = p
		return nil
	}
}

// WithMaxBlocks overrides the maximum entries allowed per user in user_blocks.
func WithMaxBlocks(limit int) Option {
	return func(c *storeConfig) error {
		if limit <= 0 {
			return fmt.Errorf("max blocks %d must be positive: %w", limit, ErrInvalidConfig)
		}
		c.maxBlocks = limit
		return nil
	}
}

// WithRequestTimeout overrides the per-request timeout applied to external backend calls.
func WithRequestTimeout(d time.Duration) Option {
	return func(c *storeConfig) error {
		if d <= 0 {
			return fmt.Errorf("request timeout %v must be positive: %w", d, ErrInvalidConfig)
		}
		c.requestTimeout = d
		return nil
	}
}

// WithReadConsistency overrides the rqlite read consistency level (none, weak, strong).
func WithReadConsistency(level ReadConsistency) Option {
	return func(c *storeConfig) error {
		if !level.Valid() {
			return fmt.Errorf("invalid read consistency %v: %w", level, ErrInvalidConfig)
		}
		c.readConsistency = level
		return nil
	}
}

// WithHTTPClient injects a custom *http.Client transport for the rqlite backend.
func WithHTTPClient(client *http.Client) Option {
	return func(c *storeConfig) error {
		if client == nil {
			return fmt.Errorf("http client must not be nil: %w", ErrInvalidConfig)
		}
		c.httpClient = client
		return nil
	}
}

// Store exposes semantic atomic persistence operations over either SQLite or rqlite
// without leaking *sql.Tx or driver internals.
type Store struct {
	exec executor
	cfg  storeConfig
}

func applyStoreOptions(opts []Option) (storeConfig, error) {
	cfg := defaultStoreConfig()
	for _, opt := range opts {
		if opt == nil {
			return storeConfig{}, fmt.Errorf("nil store option: %w", ErrInvalidConfig)
		}
		if err := opt(&cfg); err != nil {
			return storeConfig{}, err
		}
	}
	return cfg, nil
}

// OpenSQLite opens a pure-Go modernc.org/sqlite Store, applies embedded migrations, and seeds QKMJ0..QKMJ9.
func OpenSQLite(ctx context.Context, dsn string, opts ...Option) (*Store, error) {
	cfg, err := applyStoreOptions(opts)
	if err != nil {
		return nil, err
	}
	exec, err := newSQLiteExecutor(ctx, dsn)
	if err != nil {
		return nil, err
	}
	return openWithExecutor(ctx, exec, cfg)
}

// OpenRqlite opens an HTTP atomic-batch rqlite Store, applies embedded migrations, and seeds QKMJ0..QKMJ9.
func OpenRqlite(ctx context.Context, endpoints []string, opts ...Option) (*Store, error) {
	cfg, err := applyStoreOptions(opts)
	if err != nil {
		return nil, err
	}
	exec, err := newRqliteExecutor(endpoints, cfg.readConsistency, cfg.requestTimeout, cfg.httpClient)
	if err != nil {
		return nil, err
	}
	return openWithExecutor(ctx, exec, cfg)
}

func openWithExecutor(ctx context.Context, exec executor, cfg storeConfig) (*Store, error) {
	if err := migrateAndSeed(ctx, exec, cfg.now); err != nil {
		_ = exec.Close()
		return nil, err
	}
	return &Store{exec: exec, cfg: cfg}, nil
}

// Close releases underlying database or HTTP resources.
func (s *Store) Close() error {
	return s.exec.Close()
}

func (s *Store) nowUTC() time.Time {
	return s.cfg.now().UTC()
}
