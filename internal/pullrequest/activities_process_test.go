package pullrequest

import (
	"encoding/json"
	"net/http"
	"os"
	"strings"
	"testing"

	"github.com/avitsrimer/bitbucket-cli/internal/profile"
	"github.com/avitsrimer/bitbucket-cli/internal/testutil"
)

// TestFilterUnknownActivityKindsWarnsOncePerDistinctKind drives filterUnknownActivityKinds
// directly against a mix of known and unknown-kind activities decoded from a real fixture feed
// (FR-5's pinned design), proving: known entries pass through untouched, unknown-kind entries are
// dropped, and exactly one [WARN] is emitted per DISTINCT unknown kind no matter how many entries
// carry it.
func TestFilterUnknownActivityKindsWarnsOncePerDistinctKind(t *testing.T) {
	fixture, err := os.ReadFile("../../testdata/activity-feed-mixed.json")
	if err != nil {
		t.Fatalf("cannot read testdata: %v", err)
	}
	var page struct {
		Values []Activity `json:"values"`
	}
	if err := json.Unmarshal(fixture, &page); err != nil {
		t.Fatalf("cannot unmarshal fixture feed: %v", err)
	}
	if len(page.Values) != 7 {
		t.Fatalf("fixture feed decoded %d activities, want 7", len(page.Values))
	}

	logBuf := testutil.CaptureLog(t)
	known := filterUnknownActivityKinds("1650", page.Values)

	if len(known) != 4 {
		t.Fatalf("filterUnknownActivityKinds returned %d known activities, want 4 (approval, changes_requested, comment, update)", len(known))
	}
	for _, activity := range known {
		if activity.Approval == nil && activity.ChangesRequested == nil && activity.Comment == nil && activity.Update == nil {
			t.Errorf("filterUnknownActivityKinds kept an activity with no known variant: %+v", activity)
		}
	}

	logOutput := logBuf.String()
	for _, wantKind := range []string{"some_future_activity_kind", "another_new_activity_kind"} {
		count := strings.Count(logOutput, wantKind)
		if count != 1 {
			t.Errorf("log output mentions unknown kind %q %d times, want exactly 1 (deduped): %s", wantKind, count, logOutput)
		}
	}
	if warnCount := strings.Count(logOutput, "WARN"); warnCount != 2 {
		t.Errorf("log output has %d WARN lines, want exactly 2 (one per distinct unknown kind): %s", warnCount, logOutput)
	}
}

// TestActivitiesProcessRendersKnownEntriesFromAMixedFeed is the RunE-level regression test for
// FR-5: before the fix, an activity feed containing even one activity Activity.Validate did not
// recognize aborted json.Unmarshal for the WHOLE page (profile.GetAll decodes one page in a single
// json.Unmarshal call), so activitiesProcess returned an error and rendered nothing at all. After
// the fix, it must succeed, print every known entry, and warn once per distinct unknown kind.
func TestActivitiesProcessRendersKnownEntriesFromAMixedFeed(t *testing.T) {
	fixture, err := os.ReadFile("../../testdata/activity-feed-mixed.json")
	if err != nil {
		t.Fatalf("cannot read testdata: %v", err)
	}

	cmd := setupTest(t, func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write(fixture)
	}, false)

	// profile.GetProfileFromCommand's first call in the whole test binary resolves the local
	// config via common.Initialize, which unconditionally resets the global lgr logger to
	// os.Stderr. Warming that resolution up here, before CaptureLog installs its buffer, keeps
	// activitiesProcess's own (later) call to it a no-op that leaves the buffer redirect intact
	// for the [WARN] lines this test asserts on.
	if _, err := profile.GetProfileFromCommand(cmd.Context(), cmd); err != nil {
		t.Fatalf("cannot warm up profile resolution: %v", err)
	}

	logBuf := testutil.CaptureLog(t)
	var runErr error
	stdout := testutil.CaptureStdout(t, func() {
		runErr = activitiesProcess(cmd, []string{"1650"})
	})
	if runErr != nil {
		t.Fatalf("activitiesProcess() error = %v, want nil (a mixed feed with unknown kinds must not fail the command)", runErr)
	}

	var activities []Activity
	if err := json.Unmarshal([]byte(stdout), &activities); err != nil {
		t.Fatalf("cannot unmarshal printed output %q: %v", stdout, err)
	}
	if len(activities) != 4 {
		t.Fatalf("printed %d activities, want 4 (the unknown-kind entries must be skipped, not printed)", len(activities))
	}

	logOutput := logBuf.String()
	if warnCount := strings.Count(logOutput, "WARN"); warnCount != 2 {
		t.Errorf("log output has %d WARN lines, want exactly 2 (one per distinct unknown kind): %s", warnCount, logOutput)
	}
}

// TestActivitiesProcessLimitAppliesAfterFilteringUnknownKinds is a regression test: --limit must
// count only KNOWN activities, not the raw, unfiltered feed. Before the fix, profile.GetAll
// applied --limit to the raw page (known and unknown kinds together), so a feed containing
// unrecognized entries could silently return fewer than --limit known activities. The fixture
// here has 3 unknown-kind entries interleaved before 2 known ones; --limit 2 must still return
// both known entries.
func TestActivitiesProcessLimitAppliesAfterFilteringUnknownKinds(t *testing.T) {
	const fixture = `{"values":[
		{"pull_request":{"id":1650},"some_kind":{"note":"unknown 1"}},
		{"pull_request":{"id":1650},"some_kind":{"note":"unknown 2"}},
		{"pull_request":{"id":1650},"some_kind":{"note":"unknown 3"}},
		{"pull_request":{"id":1650},"approval":{"date":"2026-08-01T00:00:00+00:00","user":{"display_name":"A"}}},
		{"pull_request":{"id":1650},"approval":{"date":"2026-08-02T00:00:00+00:00","user":{"display_name":"B"}}}
	],"next":""}`

	cmd := setupTest(t, func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(fixture))
	}, false)
	if err := cmd.Flags().Set("limit", "2"); err != nil {
		t.Fatalf("cannot set limit flag: %v", err)
	}

	if _, err := profile.GetProfileFromCommand(cmd.Context(), cmd); err != nil {
		t.Fatalf("cannot warm up profile resolution: %v", err)
	}

	var runErr error
	stdout := testutil.CaptureStdout(t, func() {
		runErr = activitiesProcess(cmd, []string{"1650"})
	})
	if runErr != nil {
		t.Fatalf("activitiesProcess() error = %v", runErr)
	}

	var activities []Activity
	if err := json.Unmarshal([]byte(stdout), &activities); err != nil {
		t.Fatalf("cannot unmarshal printed output %q: %v", stdout, err)
	}
	if len(activities) != 2 {
		t.Fatalf("printed %d activities, want 2 known activities despite --limit=2 also matching 3 unknown-kind entries in the raw feed", len(activities))
	}
}

// TestUnrecognizedActivityVariantIgnoresNonObjectKeys proves an incidental non-object top-level
// key (e.g. a scalar "id" BitBucket might add alongside a genuine variant) is never mistaken for
// the unknown variant itself: only a key whose value is a JSON object qualifies.
func TestUnrecognizedActivityVariantIgnoresNonObjectKeys(t *testing.T) {
	data := []byte(`{"pull_request":{"id":1650},"id":999,"some_future_activity_kind":{"date":"2026-08-01T00:00:00+00:00"}}`)

	variant, found := unrecognizedActivityVariant(data)
	if !found {
		t.Fatal("unrecognizedActivityVariant() found = false, want true")
	}
	if variant != "some_future_activity_kind" {
		t.Errorf("unrecognizedActivityVariant() = %q, want %q (not the scalar \"id\" key)", variant, "some_future_activity_kind")
	}
}

// TestUnrecognizedActivityVariantNoObjectShapedKeys proves an entry whose only non-known keys are
// all non-object (so none of them can be a genuine variant payload) reports not found.
func TestUnrecognizedActivityVariantNoObjectShapedKeys(t *testing.T) {
	data := []byte(`{"pull_request":{"id":1650},"id":999,"note":"just some scalar metadata"}`)

	_, found := unrecognizedActivityVariant(data)
	if found {
		t.Error("unrecognizedActivityVariant() found = true, want false (no object-shaped unrecognized key)")
	}
}
