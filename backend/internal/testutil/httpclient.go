package testutil

import (
	"bytes"
	"encoding/json"
	"io"
	"net/http"
	"testing"
)

// PostReservation sends a POST /api/v1/reservations with the given headers and body.
// Returns the status code and the response body bytes.
func PostReservation(t *testing.T, baseURL, userID, idempKey string, body any) (int, []byte) {
	t.Helper()

	raw, err := json.Marshal(body)
	if err != nil {
		t.Fatalf("marshal body: %v", err)
	}

	req, err := http.NewRequest(http.MethodPost, baseURL+"/api/v1/reservations", bytes.NewReader(raw))
	if err != nil {
		t.Fatalf("build request: %v", err)
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("X-User-Id", userID)
	if idempKey != "" {
		req.Header.Set("Idempotency-Key", idempKey)
	}

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("do request: %v", err)
	}
	defer resp.Body.Close()

	respBody, _ := io.ReadAll(resp.Body)
	return resp.StatusCode, respBody
}

// DeleteReservation sends a DELETE /api/v1/reservations/{id} with the given user header.
func DeleteReservation(t *testing.T, baseURL, userID, reservationID string) (int, []byte) {
	t.Helper()

	req, err := http.NewRequest(http.MethodDelete, baseURL+"/api/v1/reservations/"+reservationID, nil)
	if err != nil {
		t.Fatalf("build request: %v", err)
	}
	req.Header.Set("X-User-Id", userID)

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("do request: %v", err)
	}
	defer resp.Body.Close()

	respBody, _ := io.ReadAll(resp.Body)
	return resp.StatusCode, respBody
}
