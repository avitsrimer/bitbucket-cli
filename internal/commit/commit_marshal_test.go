package commit_test

import (
	"encoding/json"
	"testing"
	"time"

	"github.com/avitsrimer/bitbucket-cli/internal/commit"
)

// TestCommitMarshalOmitsDateWhenZero reproduces the FINAL CRITICAL GATE's year-1 timestamp
// finding: a Commit built by hand (rather than decoded from a real API response) with a zero
// Date used to emit time.Time's own zero-value marshaling, "0001-01-01T00:00:00Z", into
// machine-readable JSON/YAML output. The key must be omitted entirely when zero instead.
func TestCommitMarshalOmitsDateWhenZero(t *testing.T) {
	target := commit.Commit{Hash: "abc123"}

	data, err := json.Marshal(target)
	if err != nil {
		t.Fatalf("cannot marshal commit: %v", err)
	}
	var raw map[string]any
	if err := json.Unmarshal(data, &raw); err != nil {
		t.Fatalf("cannot unmarshal marshaled commit: %v", err)
	}
	if _, present := raw["date"]; present {
		t.Errorf("marshaled commit carries a \"date\" key %v for a zero Date, want it absent", raw["date"])
	}
}

// TestCommitMarshalIncludesDateWhenSet is the positive counterpart: a real, non-zero Date must
// still round-trip.
func TestCommitMarshalIncludesDateWhenSet(t *testing.T) {
	date := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	target := commit.Commit{Hash: "abc123", Date: date}

	data, err := json.Marshal(target)
	if err != nil {
		t.Fatalf("cannot marshal commit: %v", err)
	}
	var raw map[string]any
	if err := json.Unmarshal(data, &raw); err != nil {
		t.Fatalf("cannot unmarshal marshaled commit: %v", err)
	}
	if raw["date"] == nil {
		t.Error("marshaled commit is missing \"date\" for a non-zero Date")
	}
}
