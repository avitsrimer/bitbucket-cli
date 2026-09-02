package pullrequest_test

import (
	"encoding/json"
	"os"
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

// TestPullRequestDraftDecodesFromFixture proves the API's "draft" key is mapped onto
// PullRequest.Draft: the fixture's open draft entry (the only state Bitbucket allows a draft in)
// decodes to true.
func TestPullRequestDraftDecodesFromFixture(t *testing.T) {
	fixture, err := os.ReadFile("../../testdata/pullrequests.json")
	if err != nil {
		t.Fatalf("cannot read testdata: %v", err)
	}
	var page struct {
		Values []pullrequest.PullRequest `json:"values"`
	}
	if err := json.Unmarshal(fixture, &page); err != nil {
		t.Fatalf("cannot unmarshal fixture: %v", err)
	}
	byID := make(map[uint64]pullrequest.PullRequest, len(page.Values))
	for _, pr := range page.Values {
		byID[pr.ID] = pr
	}

	draft, ok := byID[3]
	if !ok {
		t.Fatal("fixture has no pull request 3 (the open entry carrying \"draft\": true)")
	}
	if !draft.Draft {
		t.Error("pull request 3 decoded with Draft = false, want true")
	}
	if draft.State != "OPEN" {
		t.Errorf("pull request 3 state = %q, want OPEN: Bitbucket only allows drafts on open pull requests", draft.State)
	}
}

// TestPullRequestDraftDecodesAbsentKeyAsFalse pins the decode of a payload with no "draft" key at
// all (the shape an older API version, or a hand-built body, has): Draft is a plain bool, so the
// absent key decodes to false rather than failing. The literal is inline, and its lack of a
// "draft" key asserted directly, so the premise of the test cannot silently evaporate the way it
// would if the case rode on a shared fixture entry someone later adds the key to.
func TestPullRequestDraftDecodesAbsentKeyAsFalse(t *testing.T) {
	const payload = `{"id":42,"title":"No draft key","state":"DECLINED"}`

	var raw map[string]any
	if err := json.Unmarshal([]byte(payload), &raw); err != nil {
		t.Fatalf("cannot unmarshal payload: %v", err)
	}
	if _, present := raw["draft"]; present {
		t.Fatalf("payload %s carries a \"draft\" key, but this test is about an absent one", payload)
	}

	var target pullrequest.PullRequest
	if err := json.Unmarshal([]byte(payload), &target); err != nil {
		t.Fatalf("cannot unmarshal pullrequest: %v", err)
	}
	if target.Draft {
		t.Error("Draft = true, want false for an absent \"draft\" key")
	}
}

// TestPullRequestMarshalAlwaysEmitsDraft proves the "draft" key is present in marshaled output for
// both states: -o json must always show it, and an update PUT clearing draft status must serialize
// "draft": false rather than omitting the key (which the server would read as "leave unchanged").
func TestPullRequestMarshalAlwaysEmitsDraft(t *testing.T) {
	tests := []struct {
		name  string
		draft bool
	}{
		{name: "draft", draft: true},
		{name: "ready", draft: false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			data, err := json.Marshal(pullrequest.PullRequest{ID: 42, Draft: tt.draft})
			if err != nil {
				t.Fatalf("cannot marshal pullrequest: %v", err)
			}
			var raw map[string]any
			if err := json.Unmarshal(data, &raw); err != nil {
				t.Fatalf("cannot unmarshal marshaled pullrequest: %v", err)
			}
			got, present := raw["draft"]
			if !present {
				t.Fatalf("marshaled pullrequest has no \"draft\" key, want %t", tt.draft)
			}
			if got != tt.draft {
				t.Errorf("marshaled \"draft\" = %v, want %t", got, tt.draft)
			}
		})
	}
}
