package api

import (
	"net/http"
	"time"
)

type healthResponse struct {
	Status        string `json:"status"`
	UptimeSeconds int64  `json:"uptime_seconds"`
}

func (s *Server) handleHealth(w http.ResponseWriter, _ *http.Request) {
	writeJSON(w, http.StatusOK, healthResponse{
		Status:        "ok",
		UptimeSeconds: int64(time.Since(s.startedAt).Seconds()),
	})
}
