package api

import (
	"bufio"
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

// decodeLogs parses every JSON log line written to buf.
func decodeLogs(t *testing.T, buf *bytes.Buffer) []logLine {
	t.Helper()
	var lines []logLine
	scanner := bufio.NewScanner(buf)
	scanner.Buffer(nil, 1<<20) // panic lines carry a stack trace
	for scanner.Scan() {
		var line logLine
		if err := json.Unmarshal(scanner.Bytes(), &line); err != nil {
			t.Fatalf("log line is not JSON: %v\n%s", err, scanner.Text())
		}
		lines = append(lines, line)
	}
	return lines
}

// requestLog returns the single "request" line, failing if there isn't exactly one.
func requestLog(t *testing.T, lines []logLine) logLine {
	t.Helper()
	var found []logLine
	for _, l := range lines {
		if l.Msg == "request" {
			found = append(found, l)
		}
	}
	if len(found) != 1 {
		t.Fatalf("got %d request log lines, want 1: %+v", len(found), lines)
	}
	return found[0]
}

func TestLogRequests(t *testing.T) {
	t.Parallel()
	tests := map[string]struct {
		handler   func(w http.ResponseWriter)
		wantLevel string
		want      int
		wantPanic bool
	}{
		"implicit 200": {
			handler:   func(w http.ResponseWriter) { _, _ = w.Write([]byte("ok")) },
			wantLevel: "INFO",
			want:      http.StatusOK,
		},
		"explicit 202": {
			handler:   func(w http.ResponseWriter) { w.WriteHeader(http.StatusAccepted) },
			wantLevel: "INFO",
			want:      http.StatusAccepted,
		},
		"client error": {
			handler:   func(w http.ResponseWriter) { w.WriteHeader(http.StatusBadRequest) },
			wantLevel: "INFO",
			want:      http.StatusBadRequest,
		},
		"server error": {
			handler:   func(w http.ResponseWriter) { w.WriteHeader(http.StatusInternalServerError) },
			wantLevel: "ERROR",
			want:      http.StatusInternalServerError,
		},
		"first WriteHeader wins": {
			handler: func(w http.ResponseWriter) {
				w.WriteHeader(http.StatusAccepted)
				w.WriteHeader(http.StatusInternalServerError)
			},
			wantLevel: "INFO",
			want:      http.StatusAccepted,
		},
		"panic becomes logged 500": {
			handler:   func(http.ResponseWriter) { panic("boom") },
			wantLevel: "ERROR",
			want:      http.StatusInternalServerError,
			wantPanic: true,
		},
	}
	for name, tc := range tests {
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			var buf bytes.Buffer
			srv := NewServer(store.New(), slog.New(slog.NewJSONHandler(&buf, nil)))
			handler := srv.logRequests(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
				tc.handler(w)
			}))

			rec := httptest.NewRecorder()
			handler.ServeHTTP(rec, httptest.NewRequest(http.MethodPost, "/ingest", nil))

			if rec.Code != tc.want {
				t.Errorf("response status = %d, want %d", rec.Code, tc.want)
			}
			lines := decodeLogs(t, &buf)
			got := requestLog(t, lines)
			if got.DurationMS < 0 {
				t.Errorf("duration_ms = %v, want >= 0", got.DurationMS)
			}
			got.DurationMS = 0
			want := logLine{Level: tc.wantLevel, Msg: "request", Method: http.MethodPost, Path: "/ingest", Status: tc.want}
			if got != want {
				t.Errorf("request log = %+v, want %+v", got, want)
			}

			panicLogged := false
			for _, l := range lines {
				panicLogged = panicLogged || (l.Msg == "handler panic" && l.Level == "ERROR")
			}
			if panicLogged != tc.wantPanic {
				t.Errorf("panic logged = %v, want %v", panicLogged, tc.wantPanic)
			}
		})
	}
}

func TestRoutesAreLogged(t *testing.T) {
	t.Parallel()
	var buf bytes.Buffer
	srv := NewServer(store.New(), slog.New(slog.NewJSONHandler(&buf, nil)))

	srv.Routes().ServeHTTP(httptest.NewRecorder(), httptest.NewRequest(http.MethodGet, "/health", nil))

	got := requestLog(t, decodeLogs(t, &buf))
	if got.Path != "/health" || got.Status != http.StatusOK {
		t.Errorf("log = %+v, want path /health with status 200", got)
	}
}
