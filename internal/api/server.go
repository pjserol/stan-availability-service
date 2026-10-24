// Package api exposes the HTTP interface of the availability service.
package api

import (
	"encoding/json"
	"log/slog"
	"net/http"
	"time"

	"github.com/pjserol/stan-availability-service/internal/store"
)

// Server wires HTTP handlers to the store.
type Server struct {
	store     *store.Store
	logger    *slog.Logger
	startedAt time.Time
}

// NewServer returns a Server.
func NewServer(st *store.Store, logger *slog.Logger) *Server {
	return &Server{store: st, logger: logger, startedAt: time.Now()}
}

// Routes returns the service mux.
func (s *Server) Routes() http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("POST /ingest", s.handleIngest)
	// asset_id contains slashes, so it needs the trailing "..." wildcard.
	mux.HandleFunc("GET /manifest/{asset_id...}", s.handleManifest)
	mux.HandleFunc("GET /health", s.handleHealth)
	return s.logRequests(mux)
}

func writeJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(v)
}

func writeError(w http.ResponseWriter, status int, msg string) {
	writeJSON(w, status, map[string]string{"error": msg})
}
