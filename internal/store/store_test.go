package store

import (
	"errors"
	"fmt"
	"math/rand/v2"
	"slices"
	"sync"
	"sync/atomic"
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

func TestShardFor(t *testing.T) {
	t.Parallel()
	s := New()

	t.Run("same id maps to same shard", func(t *testing.T) {
		const id = "show/the-walking-dead/season/1/ep1"
		if s.shardFor(id) != s.shardFor(id) {
			t.Errorf("shardFor(%q) returned different shards", id)
		}
	})

	t.Run("ids spread across shards", func(t *testing.T) {
		used := make(map[*shard]bool)
		for i := range 100 {
			used[s.shardFor(fmt.Sprintf("show/show%d/season/1/ep1", i))] = true
		}
		if len(used) < shardCount/2 {
			t.Errorf("100 ids used %d of %d shards, want at least %d", len(used), shardCount, shardCount/2)
		}
	})
}

func TestStoreConcurrent(t *testing.T) {
	t.Parallel()
	const (
		writers  = 8
		readers  = 4
		assets   = 50
		segments = 100
	)
	assetID := func(a int) string { return fmt.Sprintf("show/show%d/season/1/ep1", a) }
	s := New()

	// Readers poll while writers run: a manifest may be incomplete, but must
	// always be sorted with no duplicate indexes.
	done := make(chan struct{})
	var readersWG sync.WaitGroup
	for r := range readers {
		readersWG.Go(func() {
			rng := rand.New(rand.NewPCG(uint64(writers+r), 0))
			for {
				select {
				case <-done:
					return
				default:
				}
				id := assetID(rng.IntN(assets))
				m, err := s.Get(id)
				if errors.Is(err, ErrAssetNotFound) {
					continue
				}
				if err != nil {
					t.Errorf("Get(%q) unexpected error: %v", id, err)
					return
				}
				for i := 1; i < len(m.Segments); i++ {
					if m.Segments[i].Index <= m.Segments[i-1].Index {
						t.Errorf("Get(%q) not strictly ascending at %d: %+v", id, i, m.Segments)
						return
					}
				}
			}
		})
	}

	// Every writer sends every segment in its own shuffled order, so each one
	// arrives out of order and is duplicated across writers.
	var writersWG sync.WaitGroup
	for w := range writers {
		writersWG.Go(func() {
			rng := rand.New(rand.NewPCG(uint64(w), 0))
			for _, i := range rng.Perm(assets * segments) {
				a, index := i/segments, i%segments
				s.Put(assetID(a), Segment{index, fmt.Sprintf("seg%d.ts", index)}, at(index))
			}
		})
	}
	writersWG.Wait()
	close(done)
	readersWG.Wait()

	want := Manifest{Segments: sequentialSegments(segments), LastUpdated: at(segments - 1)}
	for a := range assets {
		want.AssetID = assetID(a)
		got, err := s.Get(want.AssetID)
		if err != nil {
			t.Fatalf("Get(%q) unexpected error: %v", want.AssetID, err)
		}
		assertManifest(t, got, want)
	}
}

func BenchmarkStorePut(b *testing.B) {
	const (
		assets   = 1_000
		segments = 1_000
	)
	ids := make([]string, assets)
	for a := range ids {
		ids[a] = fmt.Sprintf("show/show%d/season/1/ep1", a)
	}
	s := New()

	// Each goroutine keeps its own counter, so a shared one doesn't become the bottleneck.
	var workers atomic.Int64
	b.RunParallel(func(pb *testing.PB) {
		offset := int(workers.Add(1))
		for i := 0; pb.Next(); i++ {
			index := i % segments
			s.Put(ids[(offset+i)%assets], Segment{Index: index, CDNURL: "seg.ts"}, base.Add(time.Duration(i)))
		}
	})
}

func BenchmarkStoreGet(b *testing.B) {
	const asset = "show/the-walking-dead/season/1/ep1"
	for _, n := range []int{10, 1_000, 10_000} {
		b.Run(fmt.Sprintf("segments=%d", n), func(b *testing.B) {
			s := New()
			for _, p := range reverseOrderPuts(asset, n) {
				s.Put(p.assetID, Segment{Index: p.index, CDNURL: p.cdnURL}, p.receivedAt)
			}
			for b.Loop() {
				if _, err := s.Get(asset); err != nil {
					b.Fatal(err)
				}
			}
		})
	}
}
