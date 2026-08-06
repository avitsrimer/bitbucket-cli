package pullrequest_test

import (
	"encoding/json"
	"testing"
	"time"

	"github.com/avitsrimer/bitbucket-cli/internal/pullrequest"
)

// TestPullRequestMarshalOmitsCreatedOnAndUpdatedOnWhenZero reproduces the FINAL CRITICAL GATE's
// year-1 timestamp finding: a PullRequest built by hand (rather than decoded from a real API
// response, e.g. in a test or a payload this codebase constructs itself) with a zero CreatedOn/
// UpdatedOn used to emit time.Time's own zero-value marshaling, "0001-01-01T00:00:00Z", into
// machine-readable JSON/YAML output. Both keys must be omitted entirely when zero instead.
func TestPullRequestMarshalOmitsCreatedOnAndUpdatedOnWhenZero(t *testing.T) {
	target := pullrequest.PullRequest{ID: 1, Title: "no timestamps"}

	data, err := json.Marshal(target)
	if err != nil {
		t.Fatalf("cannot marshal pullrequest: %v", err)
	}
	var raw map[string]any
	if err := json.Unmarshal(data, &raw); err != nil {
		t.Fatalf("cannot unmarshal marshaled pullrequest: %v", err)
	}
	if _, present := raw["created_on"]; present {
		t.Errorf("marshaled pullrequest carries a \"created_on\" key %v for a zero CreatedOn, want it absent", raw["created_on"])
	}
	if _, present := raw["updated_on"]; present {
		t.Errorf("marshaled pullrequest carries a \"updated_on\" key %v for a zero UpdatedOn, want it absent", raw["updated_on"])
	}
}

// TestPullRequestMarshalIncludesCreatedOnAndUpdatedOnWhenSet is the positive counterpart: a real,
// non-zero CreatedOn/UpdatedOn must still round-trip.
func TestPullRequestMarshalIncludesCreatedOnAndUpdatedOnWhenSet(t *testing.T) {
	createdOn := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	updatedOn := time.Date(2026, 1, 2, 0, 0, 0, 0, time.UTC)
	target := pullrequest.PullRequest{ID: 1, Title: "with timestamps", CreatedOn: createdOn, UpdatedOn: updatedOn}

	data, err := json.Marshal(target)
	if err != nil {
		t.Fatalf("cannot marshal pullrequest: %v", err)
	}
	var raw map[string]any
	if err := json.Unmarshal(data, &raw); err != nil {
		t.Fatalf("cannot unmarshal marshaled pullrequest: %v", err)
	}
	if raw["created_on"] == nil {
		t.Error("marshaled pullrequest is missing \"created_on\" for a non-zero CreatedOn")
	}
	if raw["updated_on"] == nil {
		t.Error("marshaled pullrequest is missing \"updated_on\" for a non-zero UpdatedOn")
	}
}
