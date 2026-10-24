package api

import (
	"net/http"
	"net/url"
	"time"

	"github.com/pjserol/stan-availability-service/internal/store"
)

// ingestRequest uses pointers so a missing field differs from a zero value (segment_index 0 is valid).
type ingestRequest struct {
	AssetID      *string    `json:"asset_id"`
	SegmentIndex *int       `json:"segment_index"`
	CDNURL       *string    `json:"cdn_url"`
	ReceivedAt   *time.Time `json:"received_at"`
}

type ingestResponse struct {
	Status string `json:"status"`
}

func (s *Server) handleIngest(w http.ResponseWriter, r *http.Request) {
	var req ingestRequest
	if err := decodeJSON(w, r, &req); err != nil {
		writeDecodeError(w, err)
		return
	}
	if err := req.validate(); err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}

	s.store.Put(*req.AssetID, store.Segment{Index: *req.SegmentIndex, CDNURL: *req.CDNURL}, *req.ReceivedAt)
	writeJSON(w, http.StatusAccepted, ingestResponse{Status: "accepted"})
}

// fieldError reports a request field that is missing or present but invalid.
type fieldError struct {
	Field   string
	Missing bool
}

func (e *fieldError) Error() string {
	if e.Missing {
		return "missing required field: " + e.Field
	}
	return "invalid field: " + e.Field
}

// validate checks fields in a fixed order so the same bad request always gets
// the same message. Empty strings count as missing.
func (req ingestRequest) validate() error {
	switch {
	case req.AssetID == nil || *req.AssetID == "":
		return &fieldError{Field: "asset_id", Missing: true}
	case req.SegmentIndex == nil:
		return &fieldError{Field: "segment_index", Missing: true}
	case *req.SegmentIndex < 0:
		return &fieldError{Field: "segment_index"}
	case req.CDNURL == nil || *req.CDNURL == "":
		return &fieldError{Field: "cdn_url", Missing: true}
	case !isHTTPURL(*req.CDNURL):
		return &fieldError{Field: "cdn_url"}
	case req.ReceivedAt == nil:
		return &fieldError{Field: "received_at", Missing: true}
	}
	return nil
}

// isHTTPURL reports whether s is an absolute http(s) URL the CDN can fetch.
func isHTTPURL(s string) bool {
	u, err := url.Parse(s)
	return err == nil && (u.Scheme == "http" || u.Scheme == "https") && u.Host != ""
}
