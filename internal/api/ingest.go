package api

import (
	"net/http"
	"time"
)

// maxBodyBytes caps ingest bodies so a huge request can't exhaust memory.
const maxBodyBytes = 1 << 20

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
