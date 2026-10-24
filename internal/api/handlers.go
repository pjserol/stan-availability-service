// Package api exposes the HTTP interface of the availability service.
package api

import (
	"encoding/json"
	"log/slog"
	"net/http"
	"time"

	"github.com/pjserol/stan-availability-service/internal/store"
)

const maxBodyBytes = 1 << 20

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
	// TODO: wrap with request logging middleware (method, path, status, duration).
	return mux
}

// ingestRequest uses pointers so a missing field differs from a zero value (segment_index 0 is valid).
type ingestRequest struct {
	AssetID      *string    `json:"asset_id"`
	SegmentIndex *int       `json:"segment_index"`
	CDNURL       *string    `json:"cdn_url"`
	ReceivedAt   *time.Time `json:"received_at"`
}

func (s *Server) handleIngest(w http.ResponseWriter, r *http.Request) {
	// TODO: http.MaxBytesReader(w, r.Body, maxBodyBytes)
	// TODO: decode; any decode error (invalid JSON, wrong type, empty body) ->
	//       400 "Could not decode request: JSON parsing failed"
	// TODO: validate required fields -> 400 "missing required field: <name>"
	//       (consider: negative index, empty strings, cdn_url scheme)
	// TODO: s.store.Put(...) then 202 {"status":"accepted"}
	writeError(w, http.StatusNotImplemented, "not implemented")
}

func (s *Server) handleManifest(w http.ResponseWriter, r *http.Request) {
	// TODO: r.PathValue("asset_id"); store.Get; errors.Is(err, store.ErrAssetNotFound) -> 404
	writeError(w, http.StatusNotImplemented, "not implemented")
}

func (s *Server) handleHealth(w http.ResponseWriter, _ *http.Request) {
	// TODO: {"status":"ok","uptime_seconds":<int>}
	writeError(w, http.StatusNotImplemented, "not implemented")
}

func writeJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(v)
}

func writeError(w http.ResponseWriter, status int, msg string) {
	writeJSON(w, status, map[string]string{"error": msg})
}
