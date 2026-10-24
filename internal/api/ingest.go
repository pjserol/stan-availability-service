package api

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"time"

	"github.com/pjserol/stan-availability-service/internal/store"
)

// maxBodyBytes caps ingest bodies so a huge request can't exhaust memory.
const maxBodyBytes = 1 << 20

const msgDecodeFailed = "Could not decode request: JSON parsing failed"

var errTrailingData = errors.New("unexpected data after JSON object")

// ingestRequest uses pointers so a missing field differs from a zero value (segment_index 0 is valid).
type ingestRequest struct {
	AssetID      *string    `json:"asset_id"`
	SegmentIndex *int       `json:"segment_index"`
	CDNURL       *string    `json:"cdn_url"`
	ReceivedAt   *time.Time `json:"received_at"`
}

func (s *Server) handleIngest(w http.ResponseWriter, r *http.Request) {
	req, err := decodeIngest(w, r)
	var tooLarge *http.MaxBytesError
	switch {
	case errors.As(err, &tooLarge):
		writeError(w, http.StatusRequestEntityTooLarge, "request body too large")
		return
	case err != nil:
		writeError(w, http.StatusBadRequest, msgDecodeFailed)
		return
	}
	if err := req.validate(); err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}

	s.store.Put(*req.AssetID, store.Segment{Index: *req.SegmentIndex, CDNURL: *req.CDNURL}, *req.ReceivedAt)
	writeJSON(w, http.StatusAccepted, map[string]string{"status": "accepted"})
}

// decodeIngest reads exactly one JSON object from a size-limited body.
func decodeIngest(w http.ResponseWriter, r *http.Request) (ingestRequest, error) {
	var req ingestRequest
	dec := json.NewDecoder(http.MaxBytesReader(w, r.Body, maxBodyBytes))
	if err := dec.Decode(&req); err != nil {
		return req, err
	}
	if err := dec.Decode(&struct{}{}); !errors.Is(err, io.EOF) {
		var tooLarge *http.MaxBytesError
		if errors.As(err, &tooLarge) {
			return req, err
		}
		return req, errTrailingData
	}
	return req, nil
}

// validate checks fields in a fixed order so the same bad request always gets
// the same message. Empty strings count as missing.
func (req ingestRequest) validate() error {
	switch {
	case req.AssetID == nil || *req.AssetID == "":
		return missingField("asset_id")
	case req.SegmentIndex == nil:
		return missingField("segment_index")
	case *req.SegmentIndex < 0:
		return invalidField("segment_index")
	case req.CDNURL == nil || *req.CDNURL == "":
		return missingField("cdn_url")
	case !isHTTPURL(*req.CDNURL):
		return invalidField("cdn_url")
	case req.ReceivedAt == nil:
		return missingField("received_at")
	}
	return nil
}

func missingField(name string) error { return fmt.Errorf("missing required field: %s", name) }

func invalidField(name string) error { return fmt.Errorf("invalid field: %s", name) }

// isHTTPURL reports whether s is an absolute http(s) URL the CDN can fetch.
func isHTTPURL(s string) bool {
	u, err := url.Parse(s)
	return err == nil && (u.Scheme == "http" || u.Scheme == "https") && u.Host != ""
}
