package api

import (
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
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

func TestDecodeJSON(t *testing.T) {
	t.Parallel()
	type payload struct {
		N int `json:"n"`
	}
	tests := map[string]struct {
		body    string
		want    payload
		wantErr error
	}{
		"valid object":        {body: `{"n":4}`, want: payload{N: 4}},
		"surrounding spaces":  {body: " {\"n\":4}\n ", want: payload{N: 4}},
		"empty body":          {body: "", wantErr: errMalformedJSON},
		"invalid json":        {body: `{"n":]]]`, wantErr: errMalformedJSON},
		"wrong type":          {body: `{"n":"four"}`, wantErr: errMalformedJSON},
		"trailing garbage":    {body: `{"n":4} xyz`, wantErr: errMalformedJSON},
		"second object":       {body: `{"n":4}{"n":5}`, wantErr: errMalformedJSON},
		"over the size limit": {body: `{"n":4,"pad":"` + strings.Repeat("a", maxBodyBytes) + `"}`, wantErr: errBodyTooLarge},
	}
	for name, tc := range tests {
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			req := httptest.NewRequest(http.MethodPost, "/", strings.NewReader(tc.body))
			var got payload
			err := decodeJSON(httptest.NewRecorder(), req, &got)
			if !errors.Is(err, tc.wantErr) {
				t.Fatalf("decodeJSON() error = %v, want %v", err, tc.wantErr)
			}
			if tc.wantErr == nil && got != tc.want {
				t.Errorf("decoded = %+v, want %+v", got, tc.want)
			}
		})
	}
}

func TestWriteDecodeError(t *testing.T) {
	t.Parallel()
	tests := map[string]struct {
		err        error
		wantStatus int
		wantBody   string
	}{
		"too large": {
			err:        fmt.Errorf("%w: cause", errBodyTooLarge),
			wantStatus: http.StatusRequestEntityTooLarge,
			wantBody:   `{"error":"request body too large"}` + "\n",
		},
		"malformed": {
			err:        fmt.Errorf("%w: cause", errMalformedJSON),
			wantStatus: http.StatusBadRequest,
			wantBody:   `{"error":"Could not decode request: JSON parsing failed"}` + "\n",
		},
	}
	for name, tc := range tests {
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			rec := httptest.NewRecorder()
			writeDecodeError(rec, tc.err)
			if rec.Code != tc.wantStatus {
				t.Errorf("status = %d, want %d", rec.Code, tc.wantStatus)
			}
			if got := rec.Body.String(); got != tc.wantBody {
				t.Errorf("body = %q, want %q", got, tc.wantBody)
			}
		})
	}
}
