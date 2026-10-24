package api

import (
	"errors"
	"net/http"
	"time"

	"github.com/pjserol/stan-availability-service/internal/store"
)

type manifestResponse struct {
	AssetID     string            `json:"asset_id"`
	Segments    []segmentResponse `json:"segments"`
	LastUpdated time.Time         `json:"last_updated"`
}

type segmentResponse struct {
	Index  int    `json:"index"`
	CDNURL string `json:"cdn_url"`
}

func (s *Server) handleManifest(w http.ResponseWriter, r *http.Request) {
	m, err := s.store.Get(r.PathValue("asset_id"))
	switch {
	case errors.Is(err, store.ErrAssetNotFound):
		writeError(w, http.StatusNotFound, "asset not found")
		return
	case err != nil:
		writeError(w, http.StatusInternalServerError, "internal server error")
		return
	}
	writeJSON(w, http.StatusOK, newManifestResponse(m))
}

// newManifestResponse maps the store's manifest to the API shape, so the store
// stays free of JSON concerns. Timestamps are reported in UTC.
func newManifestResponse(m store.Manifest) manifestResponse {
	segments := make([]segmentResponse, len(m.Segments))
	for i, seg := range m.Segments {
		segments[i] = segmentResponse{Index: seg.Index, CDNURL: seg.CDNURL}
	}
	return manifestResponse{AssetID: m.AssetID, Segments: segments, LastUpdated: m.LastUpdated.UTC()}
}
