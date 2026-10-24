package api

import "net/http"

func (s *Server) handleManifest(w http.ResponseWriter, r *http.Request) {
	// TODO: r.PathValue("asset_id"); store.Get; errors.Is(err, store.ErrAssetNotFound) -> 404
	writeError(w, http.StatusNotImplemented, "not implemented")
}
