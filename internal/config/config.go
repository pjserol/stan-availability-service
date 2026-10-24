// Package config loads service configuration from the environment.
package config

import (
	"errors"
	"fmt"
	"os"
	"strconv"
	"time"
)

const (
	envPort            = "PORT"
	envShutdownTimeout = "SHUTDOWN_TIMEOUT"

	defaultPort            = "8080"
	defaultShutdownTimeout = 15 * time.Second
)

var (
	ErrInvalidPort            = errors.New("invalid " + envPort)
	ErrInvalidShutdownTimeout = errors.New("invalid " + envShutdownTimeout)
)

// Config holds runtime settings.
type Config struct {
	Port            string
	ShutdownTimeout time.Duration
}

// Load reads configuration from environment variables, falling back to
// defaults for unset or empty values and failing fast on invalid ones.
func Load() (Config, error) {
	port, err := parsePort(os.Getenv(envPort))
	if err != nil {
		return Config{}, err
	}

	shutdownTimeout, err := parseShutdownTimeout(os.Getenv(envShutdownTimeout))
	if err != nil {
		return Config{}, err
	}

	return Config{Port: port, ShutdownTimeout: shutdownTimeout}, nil
}

func parsePort(v string) (string, error) {
	if v == "" {
		return defaultPort, nil
	}
	port, err := strconv.Atoi(v)
	if err != nil || port < 1 || port > 65535 {
		return "", fmt.Errorf("%w %q: must be 1-65535", ErrInvalidPort, v)
	}
	return v, nil
}

func parseShutdownTimeout(v string) (time.Duration, error) {
	if v == "" {
		return defaultShutdownTimeout, nil
	}
	d, err := time.ParseDuration(v)
	if err != nil {
		return 0, fmt.Errorf("%w %q: %w", ErrInvalidShutdownTimeout, v, err)
	}
	if d <= 0 {
		return 0, fmt.Errorf("%w %q: must be positive", ErrInvalidShutdownTimeout, v)
	}
	return d, nil
}
