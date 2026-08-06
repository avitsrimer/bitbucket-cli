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
	if len(page.Values) != 6 {
		t.Fatalf("fixture feed decoded %d activities, want 6", len(page.Values))
	}

	logBuf := testutil.CaptureLog(t)
	known := filterUnknownActivityKinds("1650", page.Values)

	if len(known) != 3 {
		t.Fatalf("filterUnknownActivityKinds returned %d known activities, want 3 (approval, changes_requested, update)", len(known))
	}
	for _, activity := range known {
		if activity.Approval == nil && activity.ChangesRequested == nil && activity.Update == nil {
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
	if len(activities) != 3 {
		t.Fatalf("printed %d activities, want 3 (the unknown-kind entries must be skipped, not printed)", len(activities))
	}

	logOutput := logBuf.String()
	if warnCount := strings.Count(logOutput, "WARN"); warnCount != 2 {
		t.Errorf("log output has %d WARN lines, want exactly 2 (one per distinct unknown kind): %s", warnCount, logOutput)
	}
}
