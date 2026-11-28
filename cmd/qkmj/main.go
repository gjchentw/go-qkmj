package main

import (
	"cmp"
	"context"
	"crypto/rand"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io"
	"log/slog"
	"math/big"
	"net"
	"net/http"
	"os"
	"os/signal"
	"strings"
	"syscall"
	"time"

	"github.com/gjchentw/go-qkmj/internal/store"
)

type appConfig struct {
	storeDriver       string
	storeDSN          string
	rqliteConsistency string
	nodeName          string
	bootstrapAdmin    string
	healthAddr        string
}

func main() {
	os.Exit(mainExit())
}

func mainExit() int {
	logger := slog.New(slog.NewTextHandler(os.Stdout, &slog.HandlerOptions{Level: slog.LevelInfo}))
	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer stop()

	if err := run(ctx, os.Args[1:], os.Getenv, logger); err != nil {
		logger.Error("qkmj exited with error", "err", err)
		return 1
	}
	return 0
}

func parseConfig(args []string, getenv func(string) string) (appConfig, error) {
	fs := flag.NewFlagSet("qkmj", flag.ContinueOnError)
	fs.SetOutput(io.Discard)

	defaultDriver := cmp.Or(strings.TrimSpace(getenv("QKMJ_STORE_DRIVER")), "sqlite")
	defaultDSN := cmp.Or(strings.TrimSpace(getenv("QKMJ_STORE_DSN")), "qkmj.db")
	defaultConsistency := cmp.Or(strings.TrimSpace(getenv("QKMJ_RQLITE_CONSISTENCY")), "weak")
	defaultNodeName := strings.TrimSpace(getenv("QKMJ_NODE_NAME"))
	defaultAdmin := strings.TrimSpace(getenv("QKMJ_BOOTSTRAP_ADMIN"))
	defaultHealthAddr := cmp.Or(strings.TrimSpace(getenv("QKMJ_HEALTH_ADDR")), ":8081")

	var cfg appConfig
	fs.StringVar(&cfg.storeDriver, "store-driver", defaultDriver, "storage backend driver: sqlite or rqlite")
	fs.StringVar(&cfg.storeDSN, "store-dsn", defaultDSN, "sqlite path/:memory: or comma-separated rqlite URLs")
	fs.StringVar(&cfg.rqliteConsistency, "rqlite-consistency", defaultConsistency, "rqlite read consistency: none, weak, or strong")
	fs.StringVar(&cfg.nodeName, "node-name", defaultNodeName, "display node name (defaults to random Docker-style name)")
	fs.StringVar(&cfg.bootstrapAdmin, "bootstrap-admin", defaultAdmin, "optional existing username to grant admin role on startup")
	fs.StringVar(&cfg.healthAddr, "health-addr", defaultHealthAddr, "optional HTTP healthcheck listen address (empty to disable)")

	if err := fs.Parse(args); err != nil {
		return appConfig{}, fmt.Errorf("parse flags: %w", err)
	}
	cfg.storeDriver = strings.ToLower(strings.TrimSpace(cfg.storeDriver))
	cfg.storeDSN = strings.TrimSpace(cfg.storeDSN)
	cfg.rqliteConsistency = strings.ToLower(strings.TrimSpace(cfg.rqliteConsistency))
	cfg.nodeName = strings.TrimSpace(cfg.nodeName)
	if cfg.nodeName == "" {
		cfg.nodeName = generateNodeName()
	}
	cfg.bootstrapAdmin = strings.TrimSpace(cfg.bootstrapAdmin)
	cfg.healthAddr = strings.TrimSpace(cfg.healthAddr)
	return cfg, nil
}

func run(ctx context.Context, args []string, getenv func(string) string, logger *slog.Logger) error {
	return runWithReady(ctx, args, getenv, logger, nil)
}

func runWithReady(ctx context.Context, args []string, getenv func(string) string, logger *slog.Logger, readyCh chan<- string) error {
	cfg, err := parseConfig(args, getenv)
	if err != nil {
		return err
	}

	st, err := openStore(ctx, cfg)
	if err != nil {
		return err
	}
	defer func() {
		if closeErr := st.Close(); closeErr != nil {
			logger.Error("close store failed", "err", closeErr)
		}
	}()

	if cfg.bootstrapAdmin != "" {
		if bootErr := st.BootstrapAdmin(ctx, cfg.bootstrapAdmin); bootErr != nil {
			return fmt.Errorf("bootstrap admin %q: %w", cfg.bootstrapAdmin, bootErr)
		}
		logger.Info("bootstrapped admin role", "username", cfg.bootstrapAdmin)
	}

	bots, err := st.ListBotAccounts(ctx)
	if err != nil {
		return fmt.Errorf("verify built-in bot accounts: %w", err)
	}
	logger.Info("qkmj store ready",
		"node", cfg.nodeName,
		"driver", cfg.storeDriver,
		"bots", len(bots),
	)

	srv, boundAddr, serveErrCh, err := startHealthServer(ctx, cfg, logger)
	if err != nil {
		return err
	}
	if readyCh != nil {
		readyCh <- boundAddr
	}

	select {
	case <-ctx.Done():
		logger.Info("shutting down qkmj", "node", cfg.nodeName)
		if srv != nil {
			shutdownCtx, cancel := context.WithTimeout(context.WithoutCancel(ctx), 5*time.Second)
			defer cancel()
			if shutErr := srv.Shutdown(shutdownCtx); shutErr != nil {
				return fmt.Errorf("shutdown health server: %w", shutErr)
			}
		}
		return nil
	case srvErr := <-serveErrCh:
		return srvErr
	}
}

func openStore(ctx context.Context, cfg appConfig) (*store.Store, error) {
	switch cfg.storeDriver {
	case "sqlite":
		return store.OpenSQLite(ctx, cfg.storeDSN)
	case "rqlite":
		consistency, err := parseReadConsistency(cfg.rqliteConsistency)
		if err != nil {
			return nil, err
		}
		endpoints := splitEndpoints(cfg.storeDSN)
		return store.OpenRqlite(ctx, endpoints, store.WithReadConsistency(consistency))
	default:
		return nil, fmt.Errorf("unsupported store driver %q: %w", cfg.storeDriver, store.ErrInvalidConfig)
	}
}

func parseReadConsistency(raw string) (store.ReadConsistency, error) {
	switch raw {
	case "none":
		return store.ReadConsistencyNone, nil
	case "weak", "":
		return store.ReadConsistencyWeak, nil
	case "strong":
		return store.ReadConsistencyStrong, nil
	default:
		return store.ReadConsistencyUnknown, fmt.Errorf("invalid rqlite consistency %q: %w", raw, store.ErrInvalidConfig)
	}
}

func splitEndpoints(raw string) []string {
	var out []string
	for part := range strings.SplitSeq(raw, ",") {
		trimmed := strings.TrimSpace(part)
		if trimmed != "" {
			out = append(out, trimmed)
		}
	}
	return out
}

func startHealthServer(ctx context.Context, cfg appConfig, logger *slog.Logger) (srv *http.Server, boundAddr string, errCh <-chan error, err error) {
	ch := make(chan error, 1)
	if cfg.healthAddr == "" {
		return nil, "", ch, nil
	}
	var lc net.ListenConfig
	ln, err := lc.Listen(ctx, "tcp", cfg.healthAddr)
	if err != nil {
		return nil, "", nil, fmt.Errorf("listen health addr %q: %w", cfg.healthAddr, err)
	}

	mux := http.NewServeMux()
	mux.HandleFunc("GET /healthz", func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		if encErr := json.NewEncoder(w).Encode(map[string]string{
			"status": "ok",
			"node":   cfg.nodeName,
			"driver": cfg.storeDriver,
		}); encErr != nil {
			logger.Warn("write healthz response failed", "err", encErr)
		}
	})

	srv = &http.Server{
		Handler:           mux,
		ReadHeaderTimeout: 5 * time.Second,
	}
	boundAddr = ln.Addr().String()
	logger.Info("health endpoint listening", "addr", boundAddr)
	go func() {
		if serveErr := srv.Serve(ln); serveErr != nil && !errors.Is(serveErr, http.ErrServerClosed) {
			ch <- fmt.Errorf("health server: %w", serveErr)
		}
	}()
	return srv, boundAddr, ch, nil
}

var (
	nodeAdjectives = []string{"calm", "brisk", "keen", "swift", "lucid", "bold", "warm", "bright"}
	nodeNouns      = []string{"otter", "falcon", "badger", "crane", "fox", "heron", "lynx", "sparrow"}
)

func generateNodeName() string {
	adj := nodeAdjectives[cryptoRandInt(len(nodeAdjectives))]
	noun := nodeNouns[cryptoRandInt(len(nodeNouns))]
	return adj + "-" + noun
}

func cryptoRandInt(n int) int {
	v, err := rand.Int(rand.Reader, big.NewInt(int64(n)))
	if err != nil {
		return 0
	}
	return int(v.Int64())
}
