package api

import (
	"fmt"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/pjserol/stan-availability-service/internal/store"
)

func ingestJSON(assetID string, index int, cdnURL, receivedAt string) string {
	return fmt.Sprintf(`{"asset_id":%q,"segment_index":%d,"cdn_url":%q,"received_at":%q}`, assetID, index, cdnURL, receivedAt)
}

// manifestJSON builds the expected body; urls are listed in index order from 0.
func manifestJSON(assetID, lastUpdated string, urls ...string) string {
	segments := make([]string, len(urls))
	for i, u := range urls {
		segments[i] = fmt.Sprintf(`{"index":%d,"cdn_url":%q}`, i, u)
	}
	return fmt.Sprintf(`{"asset_id":%q,"segments":[%s],"last_updated":%q}`, assetID, strings.Join(segments, ","), lastUpdated) + "\n"
}

// TestManifest replays the brief's scenarios through POST /ingest, then reads
// the manifest back, so it covers the whole ingest → store → manifest flow.
func TestManifest(t *testing.T) {
	t.Parallel()
	const (
		tb1   = "show/thunderbirds/season/1/ep1"
		sc4   = "show/scooby/season/1/ep4"
		ts2   = "show/thetaste/season/1/ep2"
		or1   = "show/theoriginals/season/1/ep1"
		tbURL = "https://cdn.example.com/thunderbirds/s1e1/seg%d.ts"
		scURL = "https://cdn.example.com/scooby/s1e4/seg%d.ts"
		tsURL = "https://cdn.example.com/thetaste/s1e2/seg%d.ts"
		orURL = "https://cdn.example.com/theoriginals/s1e1/seg%d.ts"
	)
	u := fmt.Sprintf
	scenario4 := []string{
		ingestJSON(tb1, 0, u(tbURL, 0), "2024-05-01T10:00:00Z"),
		ingestJSON(sc4, 0, u(scURL, 0), "2024-05-01T10:00:00Z"),
		ingestJSON(or1, 0, u(orURL, 0), "2024-05-01T10:00:00Z"),
		ingestJSON(tb1, 1, u(tbURL, 1), "2024-05-01T10:00:01Z"),
		ingestJSON(sc4, 1, u(scURL, 1), "2024-05-01T10:00:01Z"),
		ingestJSON(or1, 1, u(orURL, 1), "2024-05-01T10:00:01Z"),
	}
	tests := map[string]struct {
		ingest     []string
		get        string
		wantStatus int
		wantBody   string
	}{
		"scenario 1 sequential": {
			ingest: []string{
				ingestJSON(tb1, 0, u(tbURL, 0), "2024-05-01T10:00:00Z"),
				ingestJSON(tb1, 1, u(tbURL, 1), "2024-05-01T10:00:01Z"),
				ingestJSON(tb1, 2, u(tbURL, 2), "2024-05-01T10:00:02Z"),
			},
			get:        tb1,
			wantStatus: http.StatusOK,
			wantBody:   manifestJSON(tb1, "2024-05-01T10:00:02Z", u(tbURL, 0), u(tbURL, 1), u(tbURL, 2)),
		},
		"scenario 2 out of order": {
			ingest: []string{
				ingestJSON(sc4, 5, u(scURL, 5), "2024-05-01T10:00:00Z"),
				ingestJSON(sc4, 2, u(scURL, 2), "2024-05-01T10:00:01Z"),
				ingestJSON(sc4, 0, u(scURL, 0), "2024-05-01T10:00:02Z"),
				ingestJSON(sc4, 3, u(scURL, 3), "2024-05-01T10:00:03Z"),
				ingestJSON(sc4, 1, u(scURL, 1), "2024-05-01T10:00:04Z"),
				ingestJSON(sc4, 4, u(scURL, 4), "2024-05-01T10:00:05Z"),
			},
			get:        sc4,
			wantStatus: http.StatusOK,
			wantBody: manifestJSON(sc4, "2024-05-01T10:00:05Z",
				u(scURL, 0), u(scURL, 1), u(scURL, 2), u(scURL, 3), u(scURL, 4), u(scURL, 5)),
		},
		"scenario 3 duplicates": {
			ingest: []string{
				ingestJSON(ts2, 0, u(tsURL, 0), "2024-05-01T10:00:00Z"),
				ingestJSON(ts2, 1, u(tsURL, 1), "2024-05-01T10:00:01Z"),
				ingestJSON(ts2, 0, u(tsURL, 0), "2024-05-01T10:00:02Z"),
				ingestJSON(ts2, 1, u(tsURL, 1), "2024-05-01T10:00:03Z"),
				ingestJSON(ts2, 2, u(tsURL, 2), "2024-05-01T10:00:04Z"),
			},
			get:        ts2,
			wantStatus: http.StatusOK,
			wantBody:   manifestJSON(ts2, "2024-05-01T10:00:04Z", u(tsURL, 0), u(tsURL, 1), u(tsURL, 2)),
		},
		"scenario 4 thunderbirds": {
			ingest:     scenario4,
			get:        tb1,
			wantStatus: http.StatusOK,
			wantBody:   manifestJSON(tb1, "2024-05-01T10:00:01Z", u(tbURL, 0), u(tbURL, 1)),
		},
		"scenario 4 scooby": {
			ingest:     scenario4,
			get:        sc4,
			wantStatus: http.StatusOK,
			wantBody:   manifestJSON(sc4, "2024-05-01T10:00:01Z", u(scURL, 0), u(scURL, 1)),
		},
		"scenario 4 theoriginals": {
			ingest:     scenario4,
			get:        or1,
			wantStatus: http.StatusOK,
			wantBody:   manifestJSON(or1, "2024-05-01T10:00:01Z", u(orURL, 0), u(orURL, 1)),
		},
		"scenario 6 not found": {
			get:        "show/nonexistent/season/1/ep99",
			wantStatus: http.StatusNotFound,
			wantBody:   `{"error":"asset not found"}` + "\n",
		},
		"prefix of an existing asset is not found": {
			ingest:     []string{ingestJSON(tb1, 0, u(tbURL, 0), "2024-05-01T10:00:00Z")},
			get:        "show/thunderbirds/season/1",
			wantStatus: http.StatusNotFound,
			wantBody:   `{"error":"asset not found"}` + "\n",
		},
		"empty asset id": {
			get:        "",
			wantStatus: http.StatusNotFound,
			wantBody:   `{"error":"asset not found"}` + "\n",
		},
		"trailing slash is a different asset": {
			ingest:     []string{ingestJSON(tb1, 0, u(tbURL, 0), "2024-05-01T10:00:00Z")},
			get:        tb1 + "/",
			wantStatus: http.StatusNotFound,
			wantBody:   `{"error":"asset not found"}` + "\n",
		},
		"encoded slashes resolve to the asset": {
			ingest:     []string{ingestJSON(tb1, 0, u(tbURL, 0), "2024-05-01T10:00:00Z")},
			get:        "show%2Fthunderbirds%2Fseason%2F1%2Fep1",
			wantStatus: http.StatusOK,
			wantBody:   manifestJSON(tb1, "2024-05-01T10:00:00Z", u(tbURL, 0)),
		},
	}
	for name, tc := range tests {
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			routes := NewServer(store.New(), slog.New(slog.DiscardHandler)).Routes()
			for _, body := range tc.ingest {
				rec := httptest.NewRecorder()
				routes.ServeHTTP(rec, httptest.NewRequest(http.MethodPost, "/ingest", strings.NewReader(body)))
				if rec.Code != http.StatusAccepted {
					t.Fatalf("ingest %s: status = %d, want %d", body, rec.Code, http.StatusAccepted)
				}
			}

			rec := httptest.NewRecorder()
			routes.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/manifest/"+tc.get, nil))

			if rec.Code != tc.wantStatus {
				t.Errorf("status = %d, want %d", rec.Code, tc.wantStatus)
			}
			if got := rec.Body.String(); got != tc.wantBody {
				t.Errorf("body =\n%s\nwant\n%s", got, tc.wantBody)
			}
		})
	}
}

func TestNewManifestResponse(t *testing.T) {
	t.Parallel()
	sydney := time.FixedZone("AEST", 10*60*60)
	tests := map[string]struct {
		in   store.Manifest
		want manifestResponse
	}{
		"maps fields keeping segment order": {
			in: store.Manifest{
				AssetID:     testAsset,
				Segments:    []store.Segment{{Index: 0, CDNURL: "seg0.ts"}, {Index: 1, CDNURL: "seg1.ts"}},
				LastUpdated: time.Date(2024, 5, 1, 10, 0, 1, 0, time.UTC),
			},
			want: manifestResponse{
				AssetID:     testAsset,
				Segments:    []segmentResponse{{Index: 0, CDNURL: "seg0.ts"}, {Index: 1, CDNURL: "seg1.ts"}},
				LastUpdated: time.Date(2024, 5, 1, 10, 0, 1, 0, time.UTC),
			},
		},
		"last_updated converted to UTC": {
			in: store.Manifest{
				AssetID:     testAsset,
				Segments:    []store.Segment{{Index: 0, CDNURL: "seg0.ts"}},
				LastUpdated: time.Date(2024, 5, 1, 20, 0, 0, 0, sydney),
			},
			want: manifestResponse{
				AssetID:     testAsset,
				Segments:    []segmentResponse{{Index: 0, CDNURL: "seg0.ts"}},
				LastUpdated: time.Date(2024, 5, 1, 10, 0, 0, 0, time.UTC),
			},
		},
	}
	for name, tc := range tests {
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			got := newManifestResponse(tc.in)
			if got.AssetID != tc.want.AssetID {
				t.Errorf("AssetID = %q, want %q", got.AssetID, tc.want.AssetID)
			}
			if !slices.Equal(got.Segments, tc.want.Segments) {
				t.Errorf("Segments = %+v, want %+v", got.Segments, tc.want.Segments)
			}
			if !got.LastUpdated.Equal(tc.want.LastUpdated) || got.LastUpdated.Location() != time.UTC {
				t.Errorf("LastUpdated = %v, want %v in UTC", got.LastUpdated, tc.want.LastUpdated)
			}
		})
	}
}
