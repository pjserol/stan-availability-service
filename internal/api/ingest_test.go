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
