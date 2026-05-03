package main

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"os"
	"os/signal"
	"syscall"

	"github.com/golang-migrate/migrate/v4"
	_ "github.com/golang-migrate/migrate/v4/database/postgres"
	_ "github.com/golang-migrate/migrate/v4/source/file"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/cibersabueso/challengebeeyong/backend/internal/config"
	"github.com/cibersabueso/challengebeeyong/backend/internal/platform"
)

func main() {
	if err := run(); err != nil {
		slog.Error("fatal startup error", "err", err)
		os.Exit(1)
	}
}

func run() error {
	cfg, err := config.Load()
	if err != nil {
		return fmt.Errorf("load config: %w", err)
	}

	logger := slog.New(slog.NewJSONHandler(os.Stdout, &slog.HandlerOptions{
		Level: parseLogLevel(cfg.LogLevel),
	}))
	slog.SetDefault(logger)

	ctx, cancel := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer cancel()

	if err := runMigrations(cfg.DatabaseURL); err != nil {
		return fmt.Errorf("migrations: %w", err)
	}
	slog.InfoContext(ctx, "migrations applied")

	pool, err := platform.NewPool(ctx, cfg.DatabaseURL)
	if err != nil {
		return fmt.Errorf("connect database: %w", err)
	}
	defer pool.Close()
	slog.InfoContext(ctx, "database pool ready")

	if err := applySeed(ctx, pool); err != nil {
		return fmt.Errorf("seed: %w", err)
	}

	slog.InfoContext(ctx, "server scaffold ready (handlers will be wired in next blocks)", "port", cfg.Port)

	<-ctx.Done()
	slog.InfoContext(context.Background(), "shutting down")
	return nil
}

func runMigrations(databaseURL string) error {
	m, err := migrate.New("file://migrations", databaseURL)
	if err != nil {
		return fmt.Errorf("init migrate: %w", err)
	}
	defer m.Close()

	if err := m.Up(); err != nil && !errors.Is(err, migrate.ErrNoChange) {
		return fmt.Errorf("apply migrations: %w", err)
	}
	return nil
}

func applySeed(ctx context.Context, pool *pgxpool.Pool) error {
	data, err := os.ReadFile("seed/seed.sql")
	if err != nil {
		return fmt.Errorf("read seed file: %w", err)
	}
	if _, err := pool.Exec(ctx, string(data)); err != nil {
		return fmt.Errorf("execute seed: %w", err)
	}
	slog.InfoContext(ctx, "seed applied")
	return nil
}

func parseLogLevel(s string) slog.Level {
	switch s {
	case "debug":
		return slog.LevelDebug
	case "warn":
		return slog.LevelWarn
	case "error":
		return slog.LevelError
	default:
		return slog.LevelInfo
	}
}
