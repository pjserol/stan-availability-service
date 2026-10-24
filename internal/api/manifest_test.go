package api

import "testing"

func TestManifest(t *testing.T) {
	tests := map[string]struct{}{
		"scenario 1 sequential":       {},
		"scenario 2 out of order":     {},
		"scenario 3 duplicates":       {},
		"scenario 4 multiple assets":  {},
		"scenario 6 not found":        {},
		"asset_id with slashes route": {},
	}
	for name := range tests {
		t.Run(name, func(t *testing.T) {
			t.Skip("TODO")
		})
	}
}
