package main

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/go-chi/chi/v5"
	chiware "github.com/go-chi/chi/v5/middleware"
	"github.com/golang-migrate/migrate/v4"
	_ "github.com/golang-migrate/migrate/v4/database/postgres"
	_ "github.com/golang-migrate/migrate/v4/source/file"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/cibersabueso/challengebeeyong/backend/internal/config"
	"github.com/cibersabueso/challengebeeyong/backend/internal/handler"
	"github.com/cibersabueso/challengebeeyong/backend/internal/platform"
	"github.com/cibersabueso/challengebeeyong/backend/internal/repository"
	"github.com/cibersabueso/challengebeeyong/backend/internal/service"
)

const reservationTTLSeconds = 60

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

	itemRepo := repository.NewItemRepository(pool)
	reservationRepo := repository.NewReservationRepository(pool)
	idempotencyRepo := repository.NewIdempotencyRepository(pool)

	itemSvc := service.NewItemService(itemRepo)
	reservationSvc := service.NewReservationService(pool, itemRepo, reservationRepo, idempotencyRepo, reservationTTLSeconds)

	itemsHandler := handler.NewItemsHandler(itemSvc)
	reservationsHandler := handler.NewReservationsHandler(reservationSvc)

	r := chi.NewRouter()
	r.Use(chiware.RequestID)
	r.Use(chiware.RealIP)
	r.Use(chiware.Logger)
	r.Use(chiware.Recoverer)
	r.Use(chiware.Timeout(15 * time.Second))

	r.Get("/healthz", func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
	})

	r.Route("/api/v1", func(r chi.Router) {
		r.Get("/items", itemsHandler.List)

		r.Group(func(r chi.Router) {
			r.Use(handler.RequireUserID)
			r.Post("/reservations", reservationsHandler.Create)
			r.Get("/reservations", reservationsHandler.ListMine)
			r.Delete("/reservations/{id}", reservationsHandler.Release)
		})
	})

	srv := &http.Server{
		Addr:              ":" + cfg.Port,
		Handler:           r,
		ReadHeaderTimeout: 5 * time.Second,
	}

	go func() {
		slog.InfoContext(ctx, "http server listening", "port", cfg.Port)
		if err := srv.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
			slog.Error("http server error", "err", err)
			cancel()
		}
	}()

	<-ctx.Done()
	slog.InfoContext(context.Background(), "shutting down")

	shutdownCtx, shutdownCancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer shutdownCancel()
	if err := srv.Shutdown(shutdownCtx); err != nil {
		slog.Error("http shutdown error", "err", err)
	}

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
