// Package config loads service configuration from the environment.
package config

import "time"

// Config holds runtime settings.
type Config struct {
	Port            string
	ShutdownTimeout time.Duration
}

// Load reads configuration from environment variables with defaults.
func Load() (Config, error) {
	// TODO: read PORT (Render sets it; default "8080") and SHUTDOWN_TIMEOUT
	// (time.ParseDuration; default 15s). Wrap parse errors with %w.
	return Config{Port: "8080", ShutdownTimeout: 15 * time.Second}, nil
}
