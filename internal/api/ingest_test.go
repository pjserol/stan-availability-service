package api

import (
	"errors"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/pjserol/stan-availability-service/internal/store"
)

const testAsset = "show/thunderbirds/season/1/ep1"

// TestIngest covers the HTTP wiring: brief scenarios, status codes and that
// accepted segments reach the store. Validation rules are in TestValidate.
func TestIngest(t *testing.T) {
	t.Parallel()
	const (
		decodeError = `{"error":"Could not decode request: JSON parsing failed"}` + "\n"
		accepted    = `{"status":"accepted"}` + "\n"
	)
	tests := map[string]struct {
		body       string
		wantStatus int
		wantBody   string
		wantStored *store.Segment // checked via the store when non-nil
		wantAt     time.Time      // received_at the stored segment must carry
	}{
		"valid segment": {
			body:       `{"asset_id":"show/thunderbirds/season/1/ep1","segment_index":4,"cdn_url":"https://cdn.example.com/seg4.ts","received_at":"2024-05-01T10:00:04Z"}`,
			wantStatus: http.StatusAccepted,
			wantBody:   accepted,
			wantStored: &store.Segment{Index: 4, CDNURL: "https://cdn.example.com/seg4.ts"},
			wantAt:     time.Date(2024, 5, 1, 10, 0, 4, 0, time.UTC),
		},
		"segment_index zero is valid": {
			body:       `{"asset_id":"show/thunderbirds/season/1/ep1","segment_index":0,"cdn_url":"https://cdn.example.com/seg0.ts","received_at":"2024-05-01T10:00:00Z"}`,
			wantStatus: http.StatusAccepted,
			wantBody:   accepted,
			wantStored: &store.Segment{Index: 0, CDNURL: "https://cdn.example.com/seg0.ts"},
			wantAt:     time.Date(2024, 5, 1, 10, 0, 0, 0, time.UTC),
		},
		"unknown fields are ignored": {
			body:       `{"asset_id":"show/thunderbirds/season/1/ep1","segment_index":1,"cdn_url":"https://cdn.example.com/seg1.ts","received_at":"2024-05-01T10:00:01Z","codec":"h264"}`,
			wantStatus: http.StatusAccepted,
			wantBody:   accepted,
			wantStored: &store.Segment{Index: 1, CDNURL: "https://cdn.example.com/seg1.ts"},
			wantAt:     time.Date(2024, 5, 1, 10, 0, 1, 0, time.UTC),
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
		"received_at not a timestamp": {
			body:       `{"asset_id":"show/thunderbirds/season/1/ep1","segment_index":0,"cdn_url":"https://cdn.example.com/seg0.ts","received_at":"yesterday"}`,
			wantStatus: http.StatusBadRequest,
			wantBody:   decodeError,
		},
		"invalid field": {
			body:       `{"asset_id":"show/thunderbirds/season/1/ep1","segment_index":-1,"cdn_url":"https://cdn.example.com/seg0.ts","received_at":"2024-05-01T10:00:00Z"}`,
			wantStatus: http.StatusBadRequest,
			wantBody:   `{"error":"invalid field: segment_index"}` + "\n",
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
			if !m.LastUpdated.Equal(tc.wantAt) {
				t.Errorf("stored received_at = %v, want %v", m.LastUpdated, tc.wantAt)
			}
		})
	}
}

func ptr[T any](v T) *T { return &v }

// validRequest returns a request that passes validation, for cases to modify.
func validRequest() ingestRequest {
	return ingestRequest{
		AssetID:      ptr(testAsset),
		SegmentIndex: ptr(4),
		CDNURL:       ptr("https://cdn.example.com/seg4.ts"),
		ReceivedAt:   ptr(time.Date(2024, 5, 1, 10, 0, 0, 0, time.UTC)),
	}
}

func TestValidate(t *testing.T) {
	t.Parallel()
	tests := map[string]struct {
		modify func(*ingestRequest)
		want   *fieldError // nil means valid
	}{
		"valid":                  {modify: func(*ingestRequest) {}},
		"segment_index zero":     {modify: func(r *ingestRequest) { r.SegmentIndex = ptr(0) }},
		"missing asset_id":       {modify: func(r *ingestRequest) { r.AssetID = nil }, want: &fieldError{Field: "asset_id", Missing: true}},
		"empty asset_id":         {modify: func(r *ingestRequest) { r.AssetID = ptr("") }, want: &fieldError{Field: "asset_id", Missing: true}},
		"missing segment_index":  {modify: func(r *ingestRequest) { r.SegmentIndex = nil }, want: &fieldError{Field: "segment_index", Missing: true}},
		"negative segment_index": {modify: func(r *ingestRequest) { r.SegmentIndex = ptr(-1) }, want: &fieldError{Field: "segment_index"}},
		"missing cdn_url":        {modify: func(r *ingestRequest) { r.CDNURL = nil }, want: &fieldError{Field: "cdn_url", Missing: true}},
		"empty cdn_url":          {modify: func(r *ingestRequest) { r.CDNURL = ptr("") }, want: &fieldError{Field: "cdn_url", Missing: true}},
		"invalid cdn_url":        {modify: func(r *ingestRequest) { r.CDNURL = ptr("seg4.ts") }, want: &fieldError{Field: "cdn_url"}},
		"missing received_at":    {modify: func(r *ingestRequest) { r.ReceivedAt = nil }, want: &fieldError{Field: "received_at", Missing: true}},
		"all missing reports asset_id first": {
			modify: func(r *ingestRequest) { *r = ingestRequest{} },
			want:   &fieldError{Field: "asset_id", Missing: true},
		},
		"cdn_url reported before received_at": {
			modify: func(r *ingestRequest) { r.CDNURL, r.ReceivedAt = nil, nil },
			want:   &fieldError{Field: "cdn_url", Missing: true},
		},
	}
	for name, tc := range tests {
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			req := validRequest()
			tc.modify(&req)

			err := req.validate()
			if tc.want == nil {
				if err != nil {
					t.Fatalf("validate() = %v, want nil", err)
				}
				return
			}
			var got *fieldError
			if !errors.As(err, &got) {
				t.Fatalf("validate() = %v, want %+v", err, tc.want)
			}
			if *got != *tc.want {
				t.Errorf("validate() = %+v, want %+v", got, tc.want)
			}
		})
	}
}

func TestFieldErrorMessage(t *testing.T) {
	t.Parallel()
	tests := map[string]struct {
		err  *fieldError
		want string
	}{
		"missing": {err: &fieldError{Field: "cdn_url", Missing: true}, want: "missing required field: cdn_url"},
		"invalid": {err: &fieldError{Field: "cdn_url"}, want: "invalid field: cdn_url"},
	}
	for name, tc := range tests {
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			if got := tc.err.Error(); got != tc.want {
				t.Errorf("Error() = %q, want %q", got, tc.want)
			}
		})
	}
}

func TestIsHTTPURL(t *testing.T) {
	t.Parallel()
	tests := map[string]struct {
		in   string
		want bool
	}{
		"https":      {in: "https://cdn.example.com/seg0.ts", want: true},
		"http":       {in: "http://cdn.example.com/seg0.ts", want: true},
		"with query": {in: "https://cdn.example.com/seg0.ts?token=abc", want: true},
		"relative":   {in: "seg0.ts", want: false},
		"ftp":        {in: "ftp://cdn.example.com/seg0.ts", want: false},
		"no host":    {in: "https://", want: false},
		"empty":      {in: "", want: false},
		"garbage":    {in: "://", want: false},
	}
	for name, tc := range tests {
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			if got := isHTTPURL(tc.in); got != tc.want {
				t.Errorf("isHTTPURL(%q) = %v, want %v", tc.in, got, tc.want)
			}
		})
	}
}
