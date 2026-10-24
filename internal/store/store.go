// Package store holds the in-memory segment availability state.
package store

import (
	"cmp"
	"errors"
	"hash/fnv"
	"slices"
	"sync"
	"time"
)

// ErrAssetNotFound is returned when no segment has been ingested for an asset.
var ErrAssetNotFound = errors.New("asset not found")

// shardCount is a power of two, so the shard index is a cheap bitmask of the hash.
const shardCount = 64

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

// shard owns a subset of assets, chosen by hash, under its own lock.
type shard struct {
	mu     sync.RWMutex
	assets map[string]map[int]storedSegment
}

// Store is a concurrency-safe in-memory segment store. Assets are spread across
// shards by hash, each with its own lock, so unrelated assets don't contend.
type Store struct {
	shards [shardCount]shard
}

// New returns an empty Store.
func New() *Store {
	s := &Store{}
	for i := range s.shards {
		s.shards[i].assets = make(map[string]map[int]storedSegment)
	}

	return s
}

// shardFor returns the shard that owns assetID; the same ID always maps to the same shard.
func (s *Store) shardFor(assetID string) *shard {
	h := fnv.New32a()
	h.Write([]byte(assetID))
	return &s.shards[h.Sum32()%shardCount]
}

// Put records a segment. A duplicate (same asset and index) replaces the stored
// one only if its receivedAt is newer; equal or older duplicates are ignored.
func (s *Store) Put(assetID string, seg Segment, receivedAt time.Time) {
	sh := s.shardFor(assetID)
	sh.mu.Lock()
	defer sh.mu.Unlock()

	segments := sh.assets[assetID]
	if segments == nil {
		segments = make(map[int]storedSegment)
		sh.assets[assetID] = segments
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
	m, ok := s.shardFor(assetID).snapshot(assetID)
	if !ok {
		return Manifest{}, ErrAssetNotFound
	}

	slices.SortFunc(m.Segments, func(a, b Segment) int {
		return cmp.Compare(a.Index, b.Index)
	})
	return m, nil
}

// snapshot copies an asset's manifest under the read lock. Segments are
// unsorted, so callers can sort the copy without holding the lock.
func (sh *shard) snapshot(assetID string) (Manifest, bool) {
	sh.mu.RLock()
	defer sh.mu.RUnlock()

	segments, ok := sh.assets[assetID]
	if !ok {
		return Manifest{}, false
	}

	m := Manifest{AssetID: assetID, Segments: make([]Segment, 0, len(segments))}
	for _, stored := range segments {
		m.Segments = append(m.Segments, stored.segment)
		if stored.receivedAt.After(m.LastUpdated) {
			m.LastUpdated = stored.receivedAt
		}
	}
	return m, true
}
