package common

import (
	"os"
	"time"
)

// GetEnvAsString returns the value of the environment variable name, or fallback when it is
// unset or empty.
func GetEnvAsString(name, fallback string) string {
	if value := os.Getenv(name); value != "" {
		return value
	}
	return fallback
}

// GetEnvAsDuration returns the environment variable name parsed as a time.Duration (e.g.
// "5m", "30s"), or fallback when it is unset, empty, or not a valid Go duration string.
func GetEnvAsDuration(name string, fallback time.Duration) time.Duration {
	if value := os.Getenv(name); value != "" {
		if duration, err := time.ParseDuration(value); err == nil {
			return duration
		}
	}
	return fallback
}
