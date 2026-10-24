package store

import (
	"errors"
	"fmt"
	"slices"
	"testing"
	"time"
)

var base = time.Date(2024, 5, 1, 10, 0, 0, 0, time.UTC)

// at returns base plus n seconds.
func at(n int) time.Time {
	return base.Add(time.Duration(n) * time.Second)
}

type put struct {
	assetID    string
	index      int
	cdnURL     string
	receivedAt time.Time
}

// reverseOrderPuts ingests indexes n-1 down to 0, so an unsorted Get can't pass by chance.
func reverseOrderPuts(assetID string, n int) []put {
	puts := make([]put, 0, n)
	for i := n - 1; i >= 0; i-- {
		puts = append(puts, put{assetID, i, fmt.Sprintf("seg%d.ts", i), at(n - 1 - i)})
	}
	return puts
}

func sequentialSegments(n int) []Segment {
	segments := make([]Segment, 0, n)
	for i := range n {
		segments = append(segments, Segment{i, fmt.Sprintf("seg%d.ts", i)})
	}
	return segments
}

func TestStore(t *testing.T) {
	const (
		ep1   = "show/thunderbirds/season/1/ep1"
		ep4   = "show/scooby/season/1/ep4"
		other = "show/theoriginals/season/1/ep1"
	)
	tests := map[string]struct {
		puts    []put
		get     string
		want    Manifest
		wantErr error
	}{
		"sequential ingest": {
			puts: []put{
				{ep1, 0, "seg0.ts", at(0)},
				{ep1, 1, "seg1.ts", at(1)},
				{ep1, 2, "seg2.ts", at(2)},
			},
			get: ep1,
			want: Manifest{
				AssetID:     ep1,
				Segments:    []Segment{{0, "seg0.ts"}, {1, "seg1.ts"}, {2, "seg2.ts"}},
				LastUpdated: at(2),
			},
		},
		"out of order delivery": {
			puts: []put{
				{ep4, 5, "seg5.ts", at(0)},
				{ep4, 2, "seg2.ts", at(1)},
				{ep4, 0, "seg0.ts", at(2)},
				{ep4, 3, "seg3.ts", at(3)},
				{ep4, 1, "seg1.ts", at(4)},
				{ep4, 4, "seg4.ts", at(5)},
			},
			get: ep4,
			want: Manifest{
				AssetID: ep4,
				Segments: []Segment{
					{0, "seg0.ts"}, {1, "seg1.ts"}, {2, "seg2.ts"},
					{3, "seg3.ts"}, {4, "seg4.ts"}, {5, "seg5.ts"},
				},
				LastUpdated: at(5),
			},
		},
		"large reverse order delivery": {
			puts: reverseOrderPuts(ep1, 50),
			get:  ep1,
			want: Manifest{
				AssetID:     ep1,
				Segments:    sequentialSegments(50),
				LastUpdated: at(49),
			},
		},
		"duplicate delivery keeps one entry per index": {
			puts: []put{
				{ep1, 0, "seg0.ts", at(0)},
				{ep1, 1, "seg1.ts", at(1)},
				{ep1, 0, "seg0.ts", at(2)},
				{ep1, 1, "seg1.ts", at(3)},
				{ep1, 2, "seg2.ts", at(4)},
			},
			get: ep1,
			want: Manifest{
				AssetID:     ep1,
				Segments:    []Segment{{0, "seg0.ts"}, {1, "seg1.ts"}, {2, "seg2.ts"}},
				LastUpdated: at(4),
			},
		},
		"newer duplicate replaces segment": {
			puts: []put{
				{ep1, 0, "wrong.ts", at(0)},
				{ep1, 0, "fixed.ts", at(1)},
			},
			get: ep1,
			want: Manifest{
				AssetID:     ep1,
				Segments:    []Segment{{0, "fixed.ts"}},
				LastUpdated: at(1),
			},
		},
		"stale duplicate is ignored": {
			puts: []put{
				{ep1, 0, "fixed.ts", at(1)},
				{ep1, 0, "wrong.ts", at(0)},
			},
			get: ep1,
			want: Manifest{
				AssetID:     ep1,
				Segments:    []Segment{{0, "fixed.ts"}},
				LastUpdated: at(1),
			},
		},
		"duplicate with same receivedAt keeps first": {
			puts: []put{
				{ep1, 0, "first.ts", at(0)},
				{ep1, 0, "second.ts", at(0)},
			},
			get: ep1,
			want: Manifest{
				AssetID:     ep1,
				Segments:    []Segment{{0, "first.ts"}},
				LastUpdated: at(0),
			},
		},
		"last updated is latest receivedAt not last arrival": {
			puts: []put{
				{ep1, 1, "seg1.ts", at(5)},
				{ep1, 0, "seg0.ts", at(2)},
			},
			get: ep1,
			want: Manifest{
				AssetID:     ep1,
				Segments:    []Segment{{0, "seg0.ts"}, {1, "seg1.ts"}},
				LastUpdated: at(5),
			},
		},
		"independent assets": {
			puts: []put{
				{ep1, 0, "tb0.ts", at(0)},
				{ep4, 0, "sc0.ts", at(0)},
				{other, 0, "or0.ts", at(0)},
				{ep1, 1, "tb1.ts", at(1)},
				{ep4, 1, "sc1.ts", at(1)},
				{other, 1, "or1.ts", at(1)},
			},
			get: ep4,
			want: Manifest{
				AssetID:     ep4,
				Segments:    []Segment{{0, "sc0.ts"}, {1, "sc1.ts"}},
				LastUpdated: at(1),
			},
		},
		"asset id prefix is not a match": {
			puts:    []put{{ep1, 0, "seg0.ts", at(0)}},
			get:     "show/thunderbirds/season/1",
			wantErr: ErrAssetNotFound,
		},
		"unknown asset": {
			get:     "show/nonexistent/season/1/ep99",
			wantErr: ErrAssetNotFound,
		},
	}
	for name, tc := range tests {
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			s := New()
			for _, p := range tc.puts {
				s.Put(p.assetID, Segment{Index: p.index, CDNURL: p.cdnURL}, p.receivedAt)
			}

			got, err := s.Get(tc.get)
			if !errors.Is(err, tc.wantErr) {
				t.Fatalf("Get(%q) error = %v, want %v", tc.get, err, tc.wantErr)
			}
			assertManifest(t, got, tc.want)
		})
	}
}

func TestStoreGetReturnsCopy(t *testing.T) {
	t.Parallel()
	const asset = "show/thunderbirds/season/1/ep1"
	s := New()
	s.Put(asset, Segment{Index: 0, CDNURL: "seg0.ts"}, at(0))

	first, err := s.Get(asset)
	if err != nil {
		t.Fatalf("Get() unexpected error: %v", err)
	}
	first.Segments[0].CDNURL = "mutated.ts"

	second, err := s.Get(asset)
	if err != nil {
		t.Fatalf("Get() unexpected error: %v", err)
	}
	if got := second.Segments[0].CDNURL; got != "seg0.ts" {
		t.Errorf("store changed through returned manifest: CDNURL = %q, want %q", got, "seg0.ts")
	}
}

func assertManifest(t *testing.T, got, want Manifest) {
	t.Helper()
	if got.AssetID != want.AssetID {
		t.Errorf("AssetID = %q, want %q", got.AssetID, want.AssetID)
	}
	if !slices.Equal(got.Segments, want.Segments) {
		t.Errorf("Segments = %+v, want %+v", got.Segments, want.Segments)
	}
	if !got.LastUpdated.Equal(want.LastUpdated) {
		t.Errorf("LastUpdated = %v, want %v", got.LastUpdated, want.LastUpdated)
	}
}

func TestStoreConcurrent(t *testing.T) {
	// TODO: many goroutines Put/Get across many assets; run with -race and
	// assert every manifest is complete, sorted and duplicate-free.
	t.Skip("TODO: PR 3")
}

func BenchmarkStorePut(b *testing.B) {
	b.Skip("TODO: PR 3 — b.RunParallel over many asset IDs")
}

func BenchmarkStoreGet(b *testing.B) {
	b.Skip("TODO: PR 3 — Get on assets of varying size (10, 1k, 10k segments)")
}
