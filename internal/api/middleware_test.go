package api

import (
	"bytes"
	"encoding/json"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/pjserol/stan-availability-service/internal/store"
)

// logLine is the JSON shape logRequests writes.
type logLine struct {
	Level      string  `json:"level"`
	Msg        string  `json:"msg"`
	Method     string  `json:"method"`
	Path       string  `json:"path"`
	Status     int     `json:"status"`
	DurationMS float64 `json:"duration_ms"`
}

// decodeLog parses the single log line written to buf.
func decodeLog(t *testing.T, buf *bytes.Buffer) logLine {
	t.Helper()
	var line logLine
	if err := json.Unmarshal(buf.Bytes(), &line); err != nil {
		t.Fatalf("log is not a single JSON line: %v\n%s", err, buf.String())
	}
	return line
}

func TestLogRequests(t *testing.T) {
	t.Parallel()
	tests := map[string]struct {
		status    int // 0 means the handler never calls WriteHeader
		wantLevel string
		want      int
	}{
		"implicit 200": {status: 0, wantLevel: "INFO", want: http.StatusOK},
		"explicit 202": {status: http.StatusAccepted, wantLevel: "INFO", want: http.StatusAccepted},
		"client error": {status: http.StatusBadRequest, wantLevel: "INFO", want: http.StatusBadRequest},
		"server error": {status: http.StatusInternalServerError, wantLevel: "ERROR", want: http.StatusInternalServerError},
	}
	for name, tc := range tests {
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			var buf bytes.Buffer
			srv := NewServer(store.New(), slog.New(slog.NewJSONHandler(&buf, nil)))
			handler := srv.logRequests(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
				if tc.status != 0 {
					w.WriteHeader(tc.status)
				}
			}))

			rec := httptest.NewRecorder()
			handler.ServeHTTP(rec, httptest.NewRequest(http.MethodPost, "/ingest", nil))

			if rec.Code != tc.want {
				t.Errorf("response status = %d, want %d", rec.Code, tc.want)
			}
			got := decodeLog(t, &buf)
			want := logLine{Level: tc.wantLevel, Msg: "request", Method: http.MethodPost, Path: "/ingest", Status: tc.want}
			if got.DurationMS < 0 {
				t.Errorf("duration_ms = %v, want >= 0", got.DurationMS)
			}
			got.DurationMS = 0
			if got != want {
				t.Errorf("log = %+v, want %+v", got, want)
			}
		})
	}
}

func TestRoutesAreLogged(t *testing.T) {
	t.Parallel()
	var buf bytes.Buffer
	srv := NewServer(store.New(), slog.New(slog.NewJSONHandler(&buf, nil)))

	srv.Routes().ServeHTTP(httptest.NewRecorder(), httptest.NewRequest(http.MethodGet, "/health", nil))

	got := decodeLog(t, &buf)
	if got.Path != "/health" || got.Status != http.StatusOK {
		t.Errorf("log = %+v, want path /health with status 200", got)
	}
}
