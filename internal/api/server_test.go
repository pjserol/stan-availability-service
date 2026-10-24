package api

import (
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestWriteError(t *testing.T) {
	t.Parallel()
	tests := map[string]struct {
		status   int
		msg      string
		wantBody string
	}{
		"bad request": {
			status:   http.StatusBadRequest,
			msg:      "missing required field: cdn_url",
			wantBody: `{"error":"missing required field: cdn_url"}` + "\n",
		},
		"not found": {
			status:   http.StatusNotFound,
			msg:      "asset not found",
			wantBody: `{"error":"asset not found"}` + "\n",
		},
	}
	for name, tc := range tests {
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			rec := httptest.NewRecorder()
			writeError(rec, tc.status, tc.msg)

			if rec.Code != tc.status {
				t.Errorf("status = %d, want %d", rec.Code, tc.status)
			}
			if got := rec.Header().Get("Content-Type"); got != "application/json" {
				t.Errorf("Content-Type = %q, want %q", got, "application/json")
			}
			if got := rec.Body.String(); got != tc.wantBody {
				t.Errorf("body = %q, want %q", got, tc.wantBody)
			}
		})
	}
}
