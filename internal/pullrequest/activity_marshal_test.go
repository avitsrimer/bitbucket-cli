package pullrequest_test

import (
	"encoding/json"
	"testing"
	"time"

	"github.com/avitsrimer/bitbucket-cli/internal/pullrequest"
)

// TestActivityUpdateMarshalOmitsCreatedOnAndUpdatedOnWhenZero reproduces the FINAL CRITICAL
// GATE's year-1 timestamp finding: Activity.MarshalJSON passes its embedded *ActivityUpdate
// through unchanged, so before ActivityUpdate had its own MarshalJSON, a zero CreatedOn/
// UpdatedOn (e.g. an update activity built by hand, or one BitBucket sends without them) fell
// back to time.Time's own zero-value marshaling, "0001-01-01T00:00:00Z", in machine-readable
// JSON/YAML output. Both keys must be omitted entirely when zero instead.
func TestActivityUpdateMarshalOmitsCreatedOnAndUpdatedOnWhenZero(t *testing.T) {
	target := pullrequest.Activity{
		Update: &pullrequest.ActivityUpdate{Title: "no timestamps"},
	}

	data, err := json.Marshal(target)
	if err != nil {
		t.Fatalf("cannot marshal activity: %v", err)
	}
	var raw map[string]any
	if err := json.Unmarshal(data, &raw); err != nil {
		t.Fatalf("cannot unmarshal marshaled activity: %v", err)
	}
	update, ok := raw["update"].(map[string]any)
	if !ok {
		t.Fatalf("marshaled activity = %v, want an \"update\" object", raw)
	}
	if _, present := update["created_on"]; present {
		t.Errorf("marshaled activity update carries a \"created_on\" key %v for a zero CreatedOn, want it absent", update["created_on"])
	}
	if _, present := update["updated_on"]; present {
		t.Errorf("marshaled activity update carries a \"updated_on\" key %v for a zero UpdatedOn, want it absent", update["updated_on"])
	}
}

// TestActivityUpdateMarshalIncludesCreatedOnAndUpdatedOnWhenSet is the positive counterpart: a
// real, non-zero CreatedOn/UpdatedOn must still round-trip.
func TestActivityUpdateMarshalIncludesCreatedOnAndUpdatedOnWhenSet(t *testing.T) {
	createdOn := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	updatedOn := time.Date(2026, 1, 2, 0, 0, 0, 0, time.UTC)
	target := pullrequest.Activity{
		Update: &pullrequest.ActivityUpdate{Title: "with timestamps", CreatedOn: createdOn, UpdatedOn: updatedOn},
	}

	data, err := json.Marshal(target)
	if err != nil {
		t.Fatalf("cannot marshal activity: %v", err)
	}
	var raw map[string]any
	if err := json.Unmarshal(data, &raw); err != nil {
		t.Fatalf("cannot unmarshal marshaled activity: %v", err)
	}
	update, ok := raw["update"].(map[string]any)
	if !ok {
		t.Fatalf("marshaled activity = %v, want an \"update\" object", raw)
	}
	if update["created_on"] == nil {
		t.Error("marshaled activity update is missing \"created_on\" for a non-zero CreatedOn")
	}
	if update["updated_on"] == nil {
		t.Error("marshaled activity update is missing \"updated_on\" for a non-zero UpdatedOn")
	}
}
