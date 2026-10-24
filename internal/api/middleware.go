package api

import (
	"log/slog"
	"net/http"
	"runtime/debug"
	"time"
)

// logRequests logs one structured line per request and turns a handler panic
// into a logged 500 instead of a dropped connection.
func (s *Server) logRequests(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		start := time.Now()
		rec := &statusRecorder{ResponseWriter: w, status: http.StatusOK}
		defer func() {
			if p := recover(); p != nil {
				if p == http.ErrAbortHandler {
					panic(p) // deliberate abort: let net/http handle it silently
				}
				s.logger.LogAttrs(r.Context(), slog.LevelError, "handler panic",
					slog.Any("panic", p),
					slog.String("stack", string(debug.Stack())),
				)
				if !rec.wroteHeader {
					writeError(rec, http.StatusInternalServerError, "internal server error")
				}
			}

			level := slog.LevelInfo
			if rec.status >= http.StatusInternalServerError {
				level = slog.LevelError
			}
			s.logger.LogAttrs(r.Context(), level, "request",
				slog.String("method", r.Method),
				slog.String("path", r.URL.Path),
				slog.Int("status", rec.status),
				slog.Float64("duration_ms", float64(time.Since(start).Microseconds())/1000),
			)
		}()
		next.ServeHTTP(rec, r)
	})
}

// statusRecorder captures the status code, which http.ResponseWriter can't report.
// It defaults to 200 because handlers that never call WriteHeader send 200.
type statusRecorder struct {
	http.ResponseWriter
	status      int
	wroteHeader bool
}

// WriteHeader records only the first final status, matching what net/http sends.
func (r *statusRecorder) WriteHeader(code int) {
	if !r.wroteHeader && code >= http.StatusOK {
		r.status = code
		r.wroteHeader = true
	}
	r.ResponseWriter.WriteHeader(code)
}

func (r *statusRecorder) Write(b []byte) (int, error) {
	if !r.wroteHeader {
		r.WriteHeader(http.StatusOK)
	}
	return r.ResponseWriter.Write(b)
}

// Unwrap lets http.ResponseController reach the underlying writer (e.g. Flush).
func (r *statusRecorder) Unwrap() http.ResponseWriter {
	return r.ResponseWriter
}
