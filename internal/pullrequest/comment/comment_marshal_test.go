package comment_test

import (
	"encoding/json"
	"testing"
	"time"

	"github.com/avitsrimer/bitbucket-cli/internal/pullrequest/comment"
)

// TestCommentMarshalOmitsCreatedOnAndUpdatedOnWhenZero reproduces the FINAL CRITICAL GATE's
// year-1 timestamp finding: a Comment built by hand (rather than decoded from a real API
// response) with a zero CreatedOn/UpdatedOn used to emit time.Time's own zero-value marshaling,
// "0001-01-01T00:00:00Z", into machine-readable JSON/YAML output -- the same defect
// Resolution.MarshalJSON (in the same file) was already guarded against, but Comment's own
// MarshalJSON was not. Both keys must be omitted entirely when zero instead.
func TestCommentMarshalOmitsCreatedOnAndUpdatedOnWhenZero(t *testing.T) {
	target := comment.Comment{ID: 1}

	data, err := json.Marshal(target)
	if err != nil {
		t.Fatalf("cannot marshal comment: %v", err)
	}
	var raw map[string]any
	if err := json.Unmarshal(data, &raw); err != nil {
		t.Fatalf("cannot unmarshal marshaled comment: %v", err)
	}
	if _, present := raw["created_on"]; present {
		t.Errorf("marshaled comment carries a \"created_on\" key %v for a zero CreatedOn, want it absent", raw["created_on"])
	}
	if _, present := raw["updated_on"]; present {
		t.Errorf("marshaled comment carries a \"updated_on\" key %v for a zero UpdatedOn, want it absent", raw["updated_on"])
	}
}

// TestCommentMarshalIncludesCreatedOnAndUpdatedOnWhenSet is the positive counterpart: a real,
// non-zero CreatedOn/UpdatedOn must still round-trip.
func TestCommentMarshalIncludesCreatedOnAndUpdatedOnWhenSet(t *testing.T) {
	createdOn := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	updatedOn := time.Date(2026, 1, 2, 0, 0, 0, 0, time.UTC)
	target := comment.Comment{ID: 1, CreatedOn: createdOn, UpdatedOn: updatedOn}

	data, err := json.Marshal(target)
	if err != nil {
		t.Fatalf("cannot marshal comment: %v", err)
	}
	var raw map[string]any
	if err := json.Unmarshal(data, &raw); err != nil {
		t.Fatalf("cannot unmarshal marshaled comment: %v", err)
	}
	if raw["created_on"] == nil {
		t.Error("marshaled comment is missing \"created_on\" for a non-zero CreatedOn")
	}
	if raw["updated_on"] == nil {
		t.Error("marshaled comment is missing \"updated_on\" for a non-zero UpdatedOn")
	}
}
