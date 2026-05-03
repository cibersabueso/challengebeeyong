// Package testutil provides shared test fixtures for integration tests.
package testutil

import (
	"context"
	"errors"
	"net/http/httptest"
	"os"
	"path/filepath"
	"runtime"
	"testing"
	"time"

	"github.com/go-chi/chi/v5"
	chiware "github.com/go-chi/chi/v5/middleware"
	"github.com/golang-migrate/migrate/v4"
	_ "github.com/golang-migrate/migrate/v4/database/postgres"
	_ "github.com/golang-migrate/migrate/v4/source/file"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/cibersabueso/challengebeeyong/backend/internal/expiry"
	"github.com/cibersabueso/challengebeeyong/backend/internal/handler"
	"github.com/cibersabueso/challengebeeyong/backend/internal/repository"
	"github.com/cibersabueso/challengebeeyong/backend/internal/service"
)

const (
	defaultTestDatabaseURL = "postgres://enrique@localhost:5432/challengebeeyong_test?sslmode=disable"
	testTTLSeconds         = 60
	testExpiryInterval     = 5
)

// TestDB bundles the resources needed by an integration test.
type TestDB struct {
	Pool        *pgxpool.Pool
	ItemRepo    *repository.ItemRepository
	ResRepo     *repository.ReservationRepository
	IdempRepo   *repository.IdempotencyRepository
	ItemService *service.ItemService
	ResService  *service.ReservationService
	ExpirySvc   *expiry.Service
}

// NewTestDB initializes the test database (running migrations once) and returns
// a TestDB plus a cleanup function the caller must defer.
func NewTestDB(t *testing.T) (*TestDB, func()) {
	t.Helper()

	url := os.Getenv("DATABASE_URL_TEST")
	if url == "" {
		url = defaultTestDatabaseURL
	}

	if err := runMigrations(url); err != nil {
		t.Fatalf("test migrations: %v", err)
	}

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	pool, err := pgxpool.New(ctx, url)
	if err != nil {
		t.Fatalf("open test pool: %v", err)
	}

	if err := pool.Ping(ctx); err != nil {
		pool.Close()
		t.Fatalf("ping test pool: %v", err)
	}

	itemRepo := repository.NewItemRepository(pool)
	resRepo := repository.NewReservationRepository(pool)
	idempRepo := repository.NewIdempotencyRepository(pool)

	itemSvc := service.NewItemService(itemRepo)
	resSvc := service.NewReservationService(pool, itemRepo, resRepo, idempRepo, testTTLSeconds)
	expSvc := expiry.NewService(resRepo, idempRepo, testExpiryInterval)

	tdb := &TestDB{
		Pool:        pool,
		ItemRepo:    itemRepo,
		ResRepo:     resRepo,
		IdempRepo:   idempRepo,
		ItemService: itemSvc,
		ResService:  resSvc,
		ExpirySvc:   expSvc,
	}

	cleanup := func() {
		pool.Close()
	}
	return tdb, cleanup
}

// Reset truncates the three core tables so each test starts from a clean slate.
func (tdb *TestDB) Reset(t *testing.T) {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	_, err := tdb.Pool.Exec(ctx, "TRUNCATE items, reservations, idempotency_keys CASCADE")
	if err != nil {
		t.Fatalf("truncate: %v", err)
	}
}

// SeedItem inserts a single item with the given total stock and returns its ID.
func (tdb *TestDB) SeedItem(t *testing.T, name string, total int) uuid.UUID {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	id := uuid.New()
	_, err := tdb.Pool.Exec(ctx, `INSERT INTO items (id, name, total, reserved) VALUES ($1, $2, $3, 0)`, id, name, total)
	if err != nil {
		t.Fatalf("seed item: %v", err)
	}
	return id
}

// CountReservations returns the total number of reservations in the table.
func (tdb *TestDB) CountReservations(t *testing.T) int {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	var n int
	if err := tdb.Pool.QueryRow(ctx, `SELECT COUNT(*) FROM reservations`).Scan(&n); err != nil {
		t.Fatalf("count reservations: %v", err)
	}
	return n
}

// ItemReserved returns the current items.reserved column value for the given item.
func (tdb *TestDB) ItemReserved(t *testing.T, itemID uuid.UUID) int {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	var n int
	if err := tdb.Pool.QueryRow(ctx, `SELECT reserved FROM items WHERE id = $1`, itemID).Scan(&n); err != nil {
		t.Fatalf("item reserved: %v", err)
	}
	return n
}

// NewServer wires the same router that production uses and returns an httptest server.
// The caller must defer srv.Close().
func (tdb *TestDB) NewServer(t *testing.T) *httptest.Server {
	t.Helper()

	itemsHandler := handler.NewItemsHandler(tdb.ItemService)
	resHandler := handler.NewReservationsHandler(tdb.ResService)

	r := chi.NewRouter()
	r.Use(chiware.Recoverer)

	r.Route("/api/v1", func(r chi.Router) {
		r.Get("/items", itemsHandler.List)

		r.Group(func(r chi.Router) {
			r.Use(handler.RequireUserID)
			r.Post("/reservations", resHandler.Create)
			r.Get("/reservations", resHandler.ListMine)
			r.Delete("/reservations/{id}", resHandler.Release)
		})
	})

	return httptest.NewServer(r)
}

func runMigrations(databaseURL string) error {
	migrationsPath, err := findMigrationsDir()
	if err != nil {
		return err
	}
	m, err := migrate.New("file://"+migrationsPath, databaseURL)
	if err != nil {
		return err
	}
	defer m.Close()

	if err := m.Up(); err != nil && !errors.Is(err, migrate.ErrNoChange) {
		return err
	}
	return nil
}

// findMigrationsDir walks up from the current test source file until it finds
// the migrations directory, allowing tests to run from any package.
func findMigrationsDir() (string, error) {
	_, thisFile, _, ok := runtime.Caller(0)
	if !ok {
		return "", errors.New("cannot resolve caller for migrations path")
	}
	dir := filepath.Dir(thisFile)
	for i := 0; i < 6; i++ {
		candidate := filepath.Join(dir, "migrations")
		if info, err := os.Stat(candidate); err == nil && info.IsDir() {
			return candidate, nil
		}
		dir = filepath.Dir(dir)
	}
	return "", errors.New("migrations directory not found walking up from testutil")
}
