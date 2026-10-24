// Package store holds the in-memory segment availability state.
package store

import (
	"errors"
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

// Store is a concurrency-safe in-memory segment store.
//
// TODO: no single global mutex. Suggested shape: N shards (FNV-1a hash of
// asset_id), each a sync.RWMutex + map[string]*asset, and each asset with its
// own sync.RWMutex + map[int]Segment (idempotent by index).
type Store struct{}

// New returns an empty Store.
func New() *Store {
	return &Store{}
}

// Put records a segment. Re-putting the same asset_id + index must not duplicate it.
func (s *Store) Put(assetID string, seg Segment, receivedAt time.Time) {
	// TODO: decide update vs no-op on duplicates and document it in the README.
	// TODO: decide what LastUpdated means (server clock vs max received_at).
}

// Get returns a sorted copy of an asset's manifest, or ErrAssetNotFound.
func (s *Store) Get(assetID string) (Manifest, error) {
	// TODO: return a copy sorted ascending by Index; never leak internal slices/maps.
	return Manifest{}, ErrAssetNotFound
}
