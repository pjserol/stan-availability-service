package api

import (
	"encoding/json"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/pjserol/stan-availability-service/internal/store"
)

func TestHealth(t *testing.T) {
	t.Parallel()
	srv := NewServer(store.New(), slog.New(slog.DiscardHandler))
	srv.startedAt = time.Now().Add(-142 * time.Second)

	rec := httptest.NewRecorder()
	srv.Routes().ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/health", nil))

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d", rec.Code, http.StatusOK)
	}
	if got := rec.Header().Get("Content-Type"); got != "application/json" {
		t.Errorf("Content-Type = %q, want %q", got, "application/json")
	}
	var got healthResponse
	if err := json.NewDecoder(rec.Body).Decode(&got); err != nil {
		t.Fatalf("decode body: %v", err)
	}
	want := healthResponse{Status: "ok", UptimeSeconds: 142}
	if got != want {
		t.Errorf("body = %+v, want %+v", got, want)
	}
}
