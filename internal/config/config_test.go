package config

import (
	"errors"
	"testing"
	"time"
)

func TestLoad(t *testing.T) {
	tests := map[string]struct {
		env     map[string]string
		want    Config
		wantErr error
	}{
		"defaults when unset": {
			want: Config{Port: "8080", ShutdownTimeout: 15 * time.Second},
		},
		"values from env": {
			env:  map[string]string{envPort: "10000", envShutdownTimeout: "30s"},
			want: Config{Port: "10000", ShutdownTimeout: 30 * time.Second},
		},
		"invalid port": {
			env:     map[string]string{envPort: "abc"},
			wantErr: ErrInvalidPort,
		},
		"invalid shutdown timeout": {
			env:     map[string]string{envShutdownTimeout: "abc"},
			wantErr: ErrInvalidShutdownTimeout,
		},
	}
	for name, tc := range tests {
		t.Run(name, func(t *testing.T) {
			// Clear any values inherited from the developer's shell.
			t.Setenv(envPort, "")
			t.Setenv(envShutdownTimeout, "")
			for k, v := range tc.env {
				t.Setenv(k, v)
			}

			got, err := Load()
			if !errors.Is(err, tc.wantErr) {
				t.Fatalf("Load() error = %v, want %v", err, tc.wantErr)
			}
			if got != tc.want {
				t.Errorf("Load() = %+v, want %+v", got, tc.want)
			}
		})
	}
}

func TestParsePort(t *testing.T) {
	t.Parallel()
	tests := map[string]struct {
		in      string
		want    string
		wantErr error
	}{
		"empty uses default": {in: "", want: "8080"},
		"valid":              {in: "10000", want: "10000"},
		"lowest valid":       {in: "1", want: "1"},
		"highest valid":      {in: "65535", want: "65535"},
		"zero":               {in: "0", wantErr: ErrInvalidPort},
		"above range":        {in: "65536", wantErr: ErrInvalidPort},
		"negative":           {in: "-1", wantErr: ErrInvalidPort},
		"non-numeric":        {in: "abc", wantErr: ErrInvalidPort},
		"surrounding spaces": {in: " 8080 ", wantErr: ErrInvalidPort},
	}
	for name, tc := range tests {
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			got, err := parsePort(tc.in)
			if !errors.Is(err, tc.wantErr) {
				t.Fatalf("parsePort(%q) error = %v, want %v", tc.in, err, tc.wantErr)
			}
			if got != tc.want {
				t.Errorf("parsePort(%q) = %q, want %q", tc.in, got, tc.want)
			}
		})
	}
}

func TestParseShutdownTimeout(t *testing.T) {
	t.Parallel()
	tests := map[string]struct {
		in      string
		want    time.Duration
		wantErr error
	}{
		"empty uses default": {in: "", want: 15 * time.Second},
		"seconds":            {in: "30s", want: 30 * time.Second},
		"sub-second":         {in: "500ms", want: 500 * time.Millisecond},
		"compound":           {in: "1m30s", want: 90 * time.Second},
		"no unit":            {in: "15", wantErr: ErrInvalidShutdownTimeout},
		"unparseable":        {in: "abc", wantErr: ErrInvalidShutdownTimeout},
		"zero":               {in: "0s", wantErr: ErrInvalidShutdownTimeout},
		"negative":           {in: "-5s", wantErr: ErrInvalidShutdownTimeout},
	}
	for name, tc := range tests {
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			got, err := parseShutdownTimeout(tc.in)
			if !errors.Is(err, tc.wantErr) {
				t.Fatalf("parseShutdownTimeout(%q) error = %v, want %v", tc.in, err, tc.wantErr)
			}
			if got != tc.want {
				t.Errorf("parseShutdownTimeout(%q) = %v, want %v", tc.in, got, tc.want)
			}
		})
	}
}
