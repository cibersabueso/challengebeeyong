package service_test

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/google/uuid"

	"github.com/cibersabueso/challengebeeyong/backend/internal/domain"
	"github.com/cibersabueso/challengebeeyong/backend/internal/testutil"
)

// TestConcurrency_50_LastUnit covers AC-008 variant 50.
// Implements requirement: "50+ simultaneous reservation requests for the last remaining item".
func TestConcurrency_50_LastUnit(t *testing.T) {
	tdb, cleanup := testutil.NewTestDB(t)
	defer cleanup()
	tdb.Reset(t)

	itemID := tdb.SeedItem(t, "Single Unit Item", 1)
	srv := tdb.NewServer(t)
	defer srv.Close()

	const goroutines = 50
	var successes, conflicts atomic.Int32
	var wg sync.WaitGroup

	wg.Add(goroutines)
	for i := 0; i < goroutines; i++ {
		go func(idx int) {
			defer wg.Done()
			userID := uuid.New().String()
			key := fmt.Sprintf("last-unit-%d", idx)
			status, _ := testutil.PostReservation(t, srv.URL, userID, key, map[string]any{
				"item_id":  itemID.String(),
				"quantity": 1,
			})
			switch status {
			case http.StatusCreated:
				successes.Add(1)
			case http.StatusConflict:
				conflicts.Add(1)
			default:
				t.Errorf("goroutine %d: unexpected status %d", idx, status)
			}
		}(i)
	}
	wg.Wait()

	if successes.Load() != 1 {
		t.Errorf("expected exactly 1 success, got %d", successes.Load())
	}
	if conflicts.Load() != goroutines-1 {
		t.Errorf("expected %d conflicts, got %d", goroutines-1, conflicts.Load())
	}
	if reserved := tdb.ItemReserved(t, itemID); reserved != 1 {
		t.Errorf("expected items.reserved=1, got %d", reserved)
	}
	if count := tdb.CountReservations(t); count != 1 {
		t.Errorf("expected 1 reservation row, got %d", count)
	}
}

// TestConcurrency_100_over_10 covers AC-008.
// Implements requirement: "100 concurrent reserve requests for 10 units must result in
// exactly 10 successful reservations and 90 rejections, with no negative stock".
func TestConcurrency_100_over_10(t *testing.T) {
	tdb, cleanup := testutil.NewTestDB(t)
	defer cleanup()
	tdb.Reset(t)

	itemID := tdb.SeedItem(t, "Ten Unit Item", 10)
	srv := tdb.NewServer(t)
	defer srv.Close()

	const goroutines = 100
	var successes, conflicts atomic.Int32
	var wg sync.WaitGroup

	wg.Add(goroutines)
	for i := 0; i < goroutines; i++ {
		go func(idx int) {
			defer wg.Done()
			userID := uuid.New().String()
			key := fmt.Sprintf("burst-%d", idx)
			status, _ := testutil.PostReservation(t, srv.URL, userID, key, map[string]any{
				"item_id":  itemID.String(),
				"quantity": 1,
			})
			switch status {
			case http.StatusCreated:
				successes.Add(1)
			case http.StatusConflict:
				conflicts.Add(1)
			default:
				t.Errorf("goroutine %d: unexpected status %d", idx, status)
			}
		}(i)
	}
	wg.Wait()

	if successes.Load() != 10 {
		t.Errorf("expected exactly 10 successes, got %d", successes.Load())
	}
	if conflicts.Load() != 90 {
		t.Errorf("expected exactly 90 conflicts, got %d", conflicts.Load())
	}
	if reserved := tdb.ItemReserved(t, itemID); reserved != 10 {
		t.Errorf("expected items.reserved=10, got %d (overselling or underselling)", reserved)
	}
	if count := tdb.CountReservations(t); count != 10 {
		t.Errorf("expected 10 reservation rows, got %d", count)
	}
}

// TestIdempotency_PostConcurrent covers AC-006.
// Implements requirement: "the same Idempotency-Key sent twice in parallel results in
// one reservation and one stock decrement".
func TestIdempotency_PostConcurrent(t *testing.T) {
	tdb, cleanup := testutil.NewTestDB(t)
	defer cleanup()
	tdb.Reset(t)

	itemID := tdb.SeedItem(t, "Idemp Item", 5)
	srv := tdb.NewServer(t)
	defer srv.Close()

	userID := uuid.New().String()
	idempKey := "duplicate-key-001"
	body := map[string]any{
		"item_id":  itemID.String(),
		"quantity": 2,
	}

	const goroutines = 2
	var statusCodes [goroutines]int
	var bodies [goroutines][]byte
	var wg sync.WaitGroup
	wg.Add(goroutines)

	start := make(chan struct{})
	for i := 0; i < goroutines; i++ {
		go func(idx int) {
			defer wg.Done()
			<-start
			statusCodes[idx], bodies[idx] = testutil.PostReservation(t, srv.URL, userID, idempKey, body)
		}(i)
	}
	close(start)
	wg.Wait()

	created := 0
	replays := 0
	for i, s := range statusCodes {
		switch s {
		case http.StatusCreated:
			created++
		case http.StatusOK:
			replays++
		default:
			t.Errorf("goroutine %d: unexpected status %d body=%s", i, s, string(bodies[i]))
		}
	}

	if created+replays != goroutines {
		t.Errorf("expected created+replays = %d, got created=%d replays=%d", goroutines, created, replays)
	}
	if created < 1 {
		t.Errorf("expected at least 1 created (201), got %d", created)
	}

	var firstID, secondID string
	if err := extractReservationID(bodies[0], &firstID); err != nil {
		t.Fatalf("extract id 0: %v", err)
	}
	if err := extractReservationID(bodies[1], &secondID); err != nil {
		t.Fatalf("extract id 1: %v", err)
	}
	if firstID == "" || firstID != secondID {
		t.Errorf("expected same reservation id, got %q vs %q", firstID, secondID)
	}

	if reserved := tdb.ItemReserved(t, itemID); reserved != 2 {
		t.Errorf("expected items.reserved=2 (one decrement), got %d", reserved)
	}
	if count := tdb.CountReservations(t); count != 1 {
		t.Errorf("expected exactly 1 reservation row, got %d", count)
	}
}

// TestIdempotency_DeleteConcurrent covers AC-011 and AC-012.
// Implements requirement: "calling release twice on the same reservation results in
// stock returned exactly once".
func TestIdempotency_DeleteConcurrent(t *testing.T) {
	tdb, cleanup := testutil.NewTestDB(t)
	defer cleanup()
	tdb.Reset(t)

	itemID := tdb.SeedItem(t, "Release Item", 5)
	srv := tdb.NewServer(t)
	defer srv.Close()

	userID := uuid.New().String()

	status, body := testutil.PostReservation(t, srv.URL, userID, "release-key-001", map[string]any{
		"item_id":  itemID.String(),
		"quantity": 3,
	})
	if status != http.StatusCreated {
		t.Fatalf("setup: create reservation failed status=%d body=%s", status, string(body))
	}

	var reservationID string
	if err := extractReservationID(body, &reservationID); err != nil {
		t.Fatalf("extract reservation id: %v", err)
	}

	if reserved := tdb.ItemReserved(t, itemID); reserved != 3 {
		t.Fatalf("setup: expected items.reserved=3, got %d", reserved)
	}

	const goroutines = 50
	var releasedCount, alreadyReleasedCount atomic.Int32
	var wg sync.WaitGroup

	start := make(chan struct{})
	wg.Add(goroutines)
	for i := 0; i < goroutines; i++ {
		go func(idx int) {
			defer wg.Done()
			<-start
			status, respBody := testutil.DeleteReservation(t, srv.URL, userID, reservationID)
			if status != http.StatusOK {
				t.Errorf("goroutine %d: expected 200, got %d body=%s", idx, status, string(respBody))
				return
			}
			var env map[string]any
			if err := json.Unmarshal(respBody, &env); err != nil {
				t.Errorf("goroutine %d: unmarshal: %v body=%s", idx, err, string(respBody))
				return
			}
			switch env["status"] {
			case "released":
				releasedCount.Add(1)
			case "already_released":
				alreadyReleasedCount.Add(1)
			default:
				t.Errorf("goroutine %d: unexpected status %v", idx, env["status"])
			}
		}(i)
	}
	close(start)
	wg.Wait()

	if releasedCount.Load() != 1 {
		t.Errorf("expected exactly 1 released, got %d", releasedCount.Load())
	}
	if alreadyReleasedCount.Load() != goroutines-1 {
		t.Errorf("expected %d already_released, got %d", goroutines-1, alreadyReleasedCount.Load())
	}

	if reserved := tdb.ItemReserved(t, itemID); reserved != 0 {
		t.Errorf("expected items.reserved=0 (stock returned once), got %d", reserved)
	}
}

// TestBootstrapCleanup covers EC-06: the expiry service must process overdue reservations
// during the synchronous bootstrap pass before any traffic is served.
func TestBootstrapCleanup(t *testing.T) {
	tdb, cleanup := testutil.NewTestDB(t)
	defer cleanup()
	tdb.Reset(t)

	itemID := tdb.SeedItem(t, "Cleanup Item", 5)
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	_, err := tdb.Pool.Exec(ctx,
		`UPDATE items SET reserved = reserved + 2 WHERE id = $1`, itemID)
	if err != nil {
		t.Fatalf("bump reserved: %v", err)
	}
	_, err = tdb.Pool.Exec(ctx, `
        INSERT INTO reservations (id, item_id, user_id, quantity, status, expires_at, created_at)
        VALUES (gen_random_uuid(), $1, $2, 2, 'active', NOW() - INTERVAL '5 seconds', NOW() - INTERVAL '70 seconds')`,
		itemID, uuid.New())
	if err != nil {
		t.Fatalf("insert overdue: %v", err)
	}

	if reserved := tdb.ItemReserved(t, itemID); reserved != 2 {
		t.Fatalf("setup expectation: reserved=2 before bootstrap, got %d", reserved)
	}

	if err := tdb.ExpirySvc.Bootstrap(ctx); err != nil {
		t.Fatalf("bootstrap: %v", err)
	}

	if reserved := tdb.ItemReserved(t, itemID); reserved != 0 {
		t.Errorf("expected reserved=0 after bootstrap, got %d", reserved)
	}
}

// TestCrossUserDelete covers AC-021: a DELETE from a non-owner user must return 404,
// not 403, to avoid leaking ownership information.
func TestCrossUserDelete(t *testing.T) {
	tdb, cleanup := testutil.NewTestDB(t)
	defer cleanup()
	tdb.Reset(t)

	itemID := tdb.SeedItem(t, "Owner Test", 5)
	srv := tdb.NewServer(t)
	defer srv.Close()

	owner := uuid.New().String()
	intruder := uuid.New().String()

	status, body := testutil.PostReservation(t, srv.URL, owner, "owner-key-001", map[string]any{
		"item_id":  itemID.String(),
		"quantity": 1,
	})
	if status != http.StatusCreated {
		t.Fatalf("setup: status=%d body=%s", status, string(body))
	}

	var reservationID string
	if err := extractReservationID(body, &reservationID); err != nil {
		t.Fatalf("extract id: %v", err)
	}

	delStatus, delBody := testutil.DeleteReservation(t, srv.URL, intruder, reservationID)
	if delStatus != http.StatusNotFound {
		t.Errorf("expected 404 from intruder, got %d body=%s", delStatus, string(delBody))
	}

	var env map[string]any
	_ = json.Unmarshal(delBody, &env)
	if code, _ := env["code"].(string); code != domain.CodeReservationNotFound {
		t.Errorf("expected code RESERVATION_NOT_FOUND, got %q", code)
	}
	if reserved := tdb.ItemReserved(t, itemID); reserved != 1 {
		t.Errorf("expected items.reserved=1 (untouched), got %d", reserved)
	}
}

func extractReservationID(body []byte, out *string) error {
	var env map[string]any
	if err := json.Unmarshal(body, &env); err != nil {
		return err
	}
	if id, ok := env["id"].(string); ok && id != "" {
		*out = id
		return nil
	}
	if id, ok := env["reservation_id"].(string); ok && id != "" {
		*out = id
		return nil
	}
	return fmt.Errorf("body has no id or reservation_id field: %s", string(body))
}
