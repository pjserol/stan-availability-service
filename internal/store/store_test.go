package store

import "testing"

func TestStore(t *testing.T) {
	tests := map[string]struct {
		// TODO: inputs (sequence of puts) and expected indexes.
	}{
		"sequential ingest":          {},
		"out of order delivery":      {},
		"duplicate segment delivery": {},
		"independent assets":         {},
		"unknown asset":              {},
	}
	for name := range tests {
		t.Run(name, func(t *testing.T) {
			t.Skip("TODO")
		})
	}
}

func TestStoreConcurrent(t *testing.T) {
	// TODO: many goroutines Put/Get across many assets; run with -race and
	// assert every manifest is complete, sorted and duplicate-free.
	t.Skip("TODO")
}

func BenchmarkStorePut(b *testing.B) {
	b.Skip("TODO: b.RunParallel over many asset IDs")
}

func BenchmarkStoreGet(b *testing.B) {
	b.Skip("TODO: Get on assets of varying size (10, 1k, 10k segments)")
}
