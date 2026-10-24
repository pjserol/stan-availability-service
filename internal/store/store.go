// Package store holds the in-memory segment availability state.
package store

import (
	"cmp"
	"errors"
	"slices"
	"sync"
	"time"
)

// ErrAssetNotFound is returned when no segment has been ingested for an asset.
var ErrAssetNotFound = errors.New("asset not found")

// Segment is a single playable segment of an asset.
type Segment struct {
	Index  int
	CDNURL string
}

// Manifest is a point-in-time snapshot of an asset's segments, sorted by Index.
type Manifest struct {
	AssetID     string
	Segments    []Segment
	LastUpdated time.Time
}

// storedSegment keeps receivedAt alongside the segment to resolve duplicates.
type storedSegment struct {
	segment    Segment
	receivedAt time.Time
}

// Store is a concurrency-safe in-memory segment store.
//
// TODO: replace the single lock with N shards (FNV-1a hash of the asset ID),
// each with its own lock, so unrelated assets don't contend.
type Store struct {
	mu     sync.RWMutex
	assets map[string]map[int]storedSegment
}

// New returns an empty Store.
func New() *Store {
	return &Store{assets: make(map[string]map[int]storedSegment)}
}

// Put records a segment. A duplicate (same asset and index) replaces the stored
// one only if its receivedAt is newer; equal or older duplicates are ignored.
func (s *Store) Put(assetID string, seg Segment, receivedAt time.Time) {
	s.mu.Lock()
	defer s.mu.Unlock()

	segments := s.assets[assetID]
	if segments == nil {
		segments = make(map[int]storedSegment)
		s.assets[assetID] = segments
	}
	existing, exists := segments[seg.Index]
	if !exists || receivedAt.After(existing.receivedAt) {
		segments[seg.Index] = storedSegment{
			segment:    seg,
			receivedAt: receivedAt,
		}
	}
}

// Get returns a sorted copy of an asset's manifest, or ErrAssetNotFound.
// LastUpdated is the latest receivedAt among the asset's stored segments.
func (s *Store) Get(assetID string) (Manifest, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	segments, ok := s.assets[assetID]
	if !ok {
		return Manifest{}, ErrAssetNotFound
	}

	var lastUpdated time.Time
	out := make([]Segment, 0, len(segments))
	for _, stored := range segments {
		out = append(out, stored.segment)
		if stored.receivedAt.After(lastUpdated) {
			lastUpdated = stored.receivedAt
		}
	}

	slices.SortFunc(out, func(a, b Segment) int {
		return cmp.Compare(a.Index, b.Index)
	})

	return Manifest{
		AssetID:     assetID,
		Segments:    out,
		LastUpdated: lastUpdated,
	}, nil
}
