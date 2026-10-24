// Package api exposes the HTTP interface of the availability service.
package api

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"time"

	"github.com/pjserol/stan-availability-service/internal/store"
)

// maxBodyBytes caps request bodies so a huge request can't exhaust memory.
const maxBodyBytes = 1 << 20

// Client-facing decode messages; msgDecodeFailed is the exact text the brief requires.
const (
	msgDecodeFailed = "Could not decode request: JSON parsing failed"
	msgBodyTooLarge = "request body too large"
)

var (
	errBodyTooLarge  = errors.New("request body too large")
	errMalformedJSON = errors.New("malformed JSON")
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

type errorResponse struct {
	Error string `json:"error"`
}

// decodeJSON reads exactly one JSON value from a size-limited body into v.
// It returns errBodyTooLarge or errMalformedJSON, wrapping the cause.
func decodeJSON(w http.ResponseWriter, r *http.Request, v any) error {
	dec := json.NewDecoder(http.MaxBytesReader(w, r.Body, maxBodyBytes))
	if err := dec.Decode(v); err != nil {
		return classifyDecodeError(err)
	}
	if err := dec.Decode(&struct{}{}); !errors.Is(err, io.EOF) {
		if err == nil {
			return fmt.Errorf("%w: unexpected data after JSON object", errMalformedJSON)
		}
		return classifyDecodeError(err)
	}
	return nil
}

// writeDecodeError answers a decodeJSON failure: 413 if too large, else 400.
func writeDecodeError(w http.ResponseWriter, err error) {
	if errors.Is(err, errBodyTooLarge) {
		writeError(w, http.StatusRequestEntityTooLarge, msgBodyTooLarge)
		return
	}
	writeError(w, http.StatusBadRequest, msgDecodeFailed)
}

func classifyDecodeError(err error) error {
	var tooLarge *http.MaxBytesError
	if errors.As(err, &tooLarge) {
		return fmt.Errorf("%w: %w", errBodyTooLarge, err)
	}
	return fmt.Errorf("%w: %w", errMalformedJSON, err)
}

func writeJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	// Headers are already sent, so an encode error can't change the response.
	_ = json.NewEncoder(w).Encode(v)
}

func writeError(w http.ResponseWriter, status int, msg string) {
	writeJSON(w, status, errorResponse{Error: msg})
}
