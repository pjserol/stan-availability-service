package api

import (
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/pjserol/stan-availability-service/internal/store"
)

const (
	decodeError = `{"error":"Could not decode request: JSON parsing failed"}` + "\n"
	accepted    = `{"status":"accepted"}` + "\n"
	testAsset   = "show/thunderbirds/season/1/ep1"
)

func TestIngest(t *testing.T) {
	t.Parallel()
	tests := map[string]struct {
		body       string
		wantStatus int
		wantBody   string
		wantStored *store.Segment // checked via the store when non-nil
	}{
		"valid segment": {
			body:       `{"asset_id":"show/thunderbirds/season/1/ep1","segment_index":4,"cdn_url":"https://cdn.example.com/seg4.ts","received_at":"2024-05-01T10:00:00Z"}`,
			wantStatus: http.StatusAccepted,
			wantBody:   accepted,
			wantStored: &store.Segment{Index: 4, CDNURL: "https://cdn.example.com/seg4.ts"},
		},
		"segment_index zero is valid": {
			body:       `{"asset_id":"show/thunderbirds/season/1/ep1","segment_index":0,"cdn_url":"https://cdn.example.com/seg0.ts","received_at":"2024-05-01T10:00:00Z"}`,
			wantStatus: http.StatusAccepted,
			wantBody:   accepted,
			wantStored: &store.Segment{Index: 0, CDNURL: "https://cdn.example.com/seg0.ts"},
		},
		"unknown fields are ignored": {
			body:       `{"asset_id":"show/thunderbirds/season/1/ep1","segment_index":1,"cdn_url":"https://cdn.example.com/seg1.ts","received_at":"2024-05-01T10:00:00Z","codec":"h264"}`,
			wantStatus: http.StatusAccepted,
			wantBody:   accepted,
			wantStored: &store.Segment{Index: 1, CDNURL: "https://cdn.example.com/seg1.ts"},
		},
		"5a invalid json": {
			body:       `{ "asset_id": "show/thunderbirds", "segment_index": ]]]`,
			wantStatus: http.StatusBadRequest,
			wantBody:   decodeError,
		},
		"5b missing cdn_url": {
			body:       `{"asset_id":"show/thunderbirds/season/1/ep1","segment_index":0,"received_at":"2024-05-01T10:00:00Z"}`,
			wantStatus: http.StatusBadRequest,
			wantBody:   `{"error":"missing required field: cdn_url"}` + "\n",
		},
		"5c segment_index as string": {
			body:       `{"asset_id":"show/thunderbirds/season/1/ep1","segment_index":"four","cdn_url":"https://cdn.example.com/seg4.ts","received_at":"2024-05-01T10:00:00Z"}`,
			wantStatus: http.StatusBadRequest,
			wantBody:   decodeError,
		},
		"5d empty body": {
			body:       "",
			wantStatus: http.StatusBadRequest,
			wantBody:   decodeError,
		},
		"trailing data after the object": {
			body:       `{"asset_id":"show/thunderbirds/season/1/ep1","segment_index":0,"cdn_url":"https://cdn.example.com/seg0.ts","received_at":"2024-05-01T10:00:00Z"} xyz`,
			wantStatus: http.StatusBadRequest,
			wantBody:   decodeError,
		},
		"received_at not a timestamp": {
			body:       `{"asset_id":"show/thunderbirds/season/1/ep1","segment_index":0,"cdn_url":"https://cdn.example.com/seg0.ts","received_at":"yesterday"}`,
			wantStatus: http.StatusBadRequest,
			wantBody:   decodeError,
		},
		"missing asset_id": {
			body:       `{"segment_index":0,"cdn_url":"https://cdn.example.com/seg0.ts","received_at":"2024-05-01T10:00:00Z"}`,
			wantStatus: http.StatusBadRequest,
			wantBody:   `{"error":"missing required field: asset_id"}` + "\n",
		},
		"missing segment_index": {
			body:       `{"asset_id":"show/thunderbirds/season/1/ep1","cdn_url":"https://cdn.example.com/seg0.ts","received_at":"2024-05-01T10:00:00Z"}`,
			wantStatus: http.StatusBadRequest,
			wantBody:   `{"error":"missing required field: segment_index"}` + "\n",
		},
		"missing received_at": {
			body:       `{"asset_id":"show/thunderbirds/season/1/ep1","segment_index":0,"cdn_url":"https://cdn.example.com/seg0.ts"}`,
			wantStatus: http.StatusBadRequest,
			wantBody:   `{"error":"missing required field: received_at"}` + "\n",
		},
		"empty object reports first missing field": {
			body:       `{}`,
			wantStatus: http.StatusBadRequest,
			wantBody:   `{"error":"missing required field: asset_id"}` + "\n",
		},
		"null field is missing": {
			body:       `{"asset_id":"show/thunderbirds/season/1/ep1","segment_index":0,"cdn_url":null,"received_at":"2024-05-01T10:00:00Z"}`,
			wantStatus: http.StatusBadRequest,
			wantBody:   `{"error":"missing required field: cdn_url"}` + "\n",
		},
		"empty asset_id is missing": {
			body:       `{"asset_id":"","segment_index":0,"cdn_url":"https://cdn.example.com/seg0.ts","received_at":"2024-05-01T10:00:00Z"}`,
			wantStatus: http.StatusBadRequest,
			wantBody:   `{"error":"missing required field: asset_id"}` + "\n",
		},
		"negative segment_index": {
			body:       `{"asset_id":"show/thunderbirds/season/1/ep1","segment_index":-1,"cdn_url":"https://cdn.example.com/seg0.ts","received_at":"2024-05-01T10:00:00Z"}`,
			wantStatus: http.StatusBadRequest,
			wantBody:   `{"error":"invalid field: segment_index"}` + "\n",
		},
		"relative cdn_url": {
			body:       `{"asset_id":"show/thunderbirds/season/1/ep1","segment_index":0,"cdn_url":"seg0.ts","received_at":"2024-05-01T10:00:00Z"}`,
			wantStatus: http.StatusBadRequest,
			wantBody:   `{"error":"invalid field: cdn_url"}` + "\n",
		},
		"non-http cdn_url": {
			body:       `{"asset_id":"show/thunderbirds/season/1/ep1","segment_index":0,"cdn_url":"ftp://cdn.example.com/seg0.ts","received_at":"2024-05-01T10:00:00Z"}`,
			wantStatus: http.StatusBadRequest,
			wantBody:   `{"error":"invalid field: cdn_url"}` + "\n",
		},
		"body over the size limit": {
			body:       `{"asset_id":"show/thunderbirds/season/1/ep1","segment_index":0,"cdn_url":"https://cdn.example.com/` + strings.Repeat("a", maxBodyBytes) + `","received_at":"2024-05-01T10:00:00Z"}`,
			wantStatus: http.StatusRequestEntityTooLarge,
			wantBody:   `{"error":"request body too large"}` + "\n",
		},
	}
	for name, tc := range tests {
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			st := store.New()
			srv := NewServer(st, slog.New(slog.DiscardHandler))

			rec := httptest.NewRecorder()
			req := httptest.NewRequest(http.MethodPost, "/ingest", strings.NewReader(tc.body))
			srv.Routes().ServeHTTP(rec, req)

			if rec.Code != tc.wantStatus {
				t.Errorf("status = %d, want %d", rec.Code, tc.wantStatus)
			}
			if got := rec.Body.String(); got != tc.wantBody {
				t.Errorf("body = %q, want %q", got, tc.wantBody)
			}

			m, err := st.Get(testAsset)
			if tc.wantStored == nil {
				if err == nil {
					t.Errorf("store has %+v, want nothing stored", m.Segments)
				}
				return
			}
			if err != nil {
				t.Fatalf("store.Get(%q) error = %v, want stored segment", testAsset, err)
			}
			if len(m.Segments) != 1 || m.Segments[0] != *tc.wantStored {
				t.Errorf("stored = %+v, want [%+v]", m.Segments, *tc.wantStored)
			}
		})
	}
}
