// Command notification-worker is a small Go worker that drains
// notification_outbox rows whose next_attempt_at is due and hands
// them to the existing notification.DevProvider.
//
// Scope (per spec):
//
//   * DevProvider only. No real SMS / WhatsApp / email vendor.
//   * No new secrets, no API keys, no webhooks.
//   * No new migration; uses the existing notification_outbox /
//     notification_templates / notification_delivery_attempts tables
//     from packages/db/migrations/0006_notifications.sql.
//   * No raw patient contact is logged. The provider never sees
//     raw contact (it is not in the outbox); this command never
//     sees raw contact either.
//
// Runtime options are environment-driven:
//
//	SIGAP_NOTIFICATION_WORKER_ENABLED=true|false  (default true)
//	SIGAP_NOTIFICATION_WORKER_ONCE=true|false    (default false)
//	SIGAP_NOTIFICATION_WORKER_INTERVAL_SECONDS=NN (default 30)
//	SIGAP_NOTIFICATION_WORKER_BATCH_SIZE=NN       (default 25)
//	SIGAP_DATABASE_URL=postgres://...            (required)
//
// Behaviour:
//
//   * ONCE=true: process one batch and exit.
//   * ONCE=false: loop every INTERVAL_SECONDS until SIGINT/SIGTERM.
//   * The in-flight row is allowed to finish before the process exits.
//
// The command is intentionally minimal. There is no web admin, no
// metrics endpoint, no flag parsing — every option is an env var
// and the script is meant to run under a process supervisor (or as a
// one-shot in local dev).
package main

import (
	"context"
	"fmt"
	"log/slog"
	"os"
	"os/signal"
	"strconv"
	"syscall"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/sigap/sigap/apps/api/internal/notification"
)

func main() {
	if err := run(); err != nil {
		fmt.Fprintf(os.Stderr, "notification-worker: %v\n", err)
		os.Exit(1)
	}
}

func run() error {
	cfg, err := loadConfig()
	if err != nil {
		return err
	}
	if !cfg.Enabled {
		slog.Info("notification-worker disabled by env (SIGAP_NOTIFICATION_WORKER_ENABLED=false); exiting")
		return nil
	}

	dbURL := os.Getenv("SIGAP_DATABASE_URL")
	if dbURL == "" {
		return fmt.Errorf("SIGAP_DATABASE_URL is required")
	}

	pool, err := pgxpool.New(context.Background(), dbURL)
	if err != nil {
		return fmt.Errorf("connect db: %w", err)
	}
	defer pool.Close()

	provider := notification.NewDevProvider(pool)
	worker := notification.NewWorker(pool, provider, nil)

	ctx, cancel := signal.NotifyContext(context.Background(),
		syscall.SIGINT, syscall.SIGTERM)
	defer cancel()

	slog.Info("notification-worker started",
		"mode", modeLabel(cfg.Once),
		"interval", cfg.Interval.String(),
		"batch_size", cfg.BatchSize,
	)

	if cfg.Once {
		n, err := worker.RunOnce(ctx, cfg.BatchSize)
		if err != nil {
			slog.Warn("notification-worker RunOnce error", "err", err.Error())
		}
		slog.Info("notification-worker ONCE complete", "processed", n)
		return nil
	}

	// Loop mode. Worker.Run returns ctx.Err() on shutdown; we exit 0
	// on cancellation so the process supervisor sees a clean exit.
	if err := worker.Run(ctx, cfg.Interval); err != nil && err != context.Canceled {
		return err
	}
	slog.Info("notification-worker stopped")
	return nil
}

type config struct {
	Enabled   bool
	Once      bool
	Interval  time.Duration
	BatchSize int
}

func loadConfig() (config, error) {
	cfg := config{
		Enabled:   boolEnv("SIGAP_NOTIFICATION_WORKER_ENABLED", true),
		Once:      boolEnv("SIGAP_NOTIFICATION_WORKER_ONCE", false),
		Interval:  durationSecondsEnv("SIGAP_NOTIFICATION_WORKER_INTERVAL_SECONDS", 30*time.Second),
		BatchSize: intEnv("SIGAP_NOTIFICATION_WORKER_BATCH_SIZE", 25),
	}
	if cfg.Interval < 0 {
		return cfg, fmt.Errorf("SIGAP_NOTIFICATION_WORKER_INTERVAL_SECONDS must be >= 0")
	}
	if cfg.BatchSize <= 0 || cfg.BatchSize > 1000 {
		return cfg, fmt.Errorf("SIGAP_NOTIFICATION_WORKER_BATCH_SIZE must be in 1..1000")
	}
	return cfg, nil
}

func boolEnv(name string, def bool) bool {
	v := os.Getenv(name)
	if v == "" {
		return def
	}
	b, err := strconv.ParseBool(v)
	if err != nil {
		slog.Warn("notification-worker: invalid bool env, using default",
			"name", name, "value", v, "default", def)
		return def
	}
	return b
}

func durationSecondsEnv(name string, def time.Duration) time.Duration {
	v := os.Getenv(name)
	if v == "" {
		return def
	}
	n, err := strconv.Atoi(v)
	if err != nil {
		slog.Warn("notification-worker: invalid int env, using default",
			"name", name, "value", v, "default_sec", int(def.Seconds()))
		return def
	}
	return time.Duration(n) * time.Second
}

func intEnv(name string, def int) int {
	v := os.Getenv(name)
	if v == "" {
		return def
	}
	n, err := strconv.Atoi(v)
	if err != nil {
		slog.Warn("notification-worker: invalid int env, using default",
			"name", name, "value", v, "default", def)
		return def
	}
	return n
}

func modeLabel(once bool) string {
	if once {
		return "once"
	}
	return "loop"
}