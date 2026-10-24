package api

import "testing"

func TestIngest(t *testing.T) {
	tests := map[string]struct {
		body       string
		wantStatus int
		wantError  string
	}{
		"valid segment":           {},
		"5a invalid json":         {},
		"5b missing cdn_url":      {},
		"5c segment_index string": {},
		"5d empty body":           {},
		"segment_index zero ok":   {},
	}
	for name := range tests {
		t.Run(name, func(t *testing.T) {
			t.Skip("TODO: httptest.NewRecorder + NewServer(...).Routes().ServeHTTP")
		})
	}
}

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

func TestHealth(t *testing.T) {
	t.Skip("TODO")
}
