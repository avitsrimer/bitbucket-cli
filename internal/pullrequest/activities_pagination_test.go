package pullrequest

import (
	"encoding/json"
	"fmt"
	"net/http"
	"testing"

	"github.com/avitsrimer/bitbucket-cli/internal/profile"
	"github.com/avitsrimer/bitbucket-cli/internal/testutil"
)

// TestActivitiesProcessLimitBoundsRequestsToOnePage is a regression test: before the fix,
// activitiesProcess fetched the ENTIRE activity feed (profile.GetAllUnbounded) regardless of
// --limit, discarding everything past the requested count only after every page was already
// fetched. Here the first (and only) page already contains --limit known activities, but "next"
// still points at a second page the server would fail this test for serving -- proving --limit
// stops pagination once enough known-kind entries are collected, instead of draining the feed.
func TestActivitiesProcessLimitBoundsRequestsToOnePage(t *testing.T) {
	requestCount := 0
	cmd := setupTest(t, func(w http.ResponseWriter, r *http.Request) {
		requestCount++
		if requestCount > 1 {
			t.Errorf("unexpected second request to %s: --limit should have stopped pagination after the first page satisfied it", r.URL.String())
			w.WriteHeader(http.StatusInternalServerError)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		fmt.Fprintf(w, `{"values":[
			{"pull_request":{"id":1650},"approval":{"date":"2026-08-01T00:00:00+00:00","user":{"display_name":"A"}}},
			{"pull_request":{"id":1650},"approval":{"date":"2026-08-02T00:00:00+00:00","user":{"display_name":"B"}}},
			{"pull_request":{"id":1650},"approval":{"date":"2026-08-03T00:00:00+00:00","user":{"display_name":"C"}}}
		],"next":"%s/2.0/repositories/acme/widgets/pullrequests/1650/activity?page=2"}`, "http://"+r.Host)
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
	if requestCount != 1 {
		t.Errorf("server received %d requests, want exactly 1 (limit already satisfied by the first page)", requestCount)
	}

	var activities []Activity
	if err := json.Unmarshal([]byte(stdout), &activities); err != nil {
		t.Fatalf("cannot unmarshal printed output %q: %v", stdout, err)
	}
	if len(activities) != 2 {
		t.Fatalf("printed %d activities, want 2 (trimmed to --limit)", len(activities))
	}
}

// TestActivitiesProcessLimitFollowsNextPageWhenUnknownKindsPushBelowLimit proves the inverse: when
// the first page's known-kind count falls short of --limit because it also carries unknown-kind
// entries, fetchActivityPages still follows "next" to collect enough known activities instead of
// stopping early on the raw (unfiltered) page size.
func TestActivitiesProcessLimitFollowsNextPageWhenUnknownKindsPushBelowLimit(t *testing.T) {
	requestCount := 0
	cmd := setupTest(t, func(w http.ResponseWriter, r *http.Request) {
		requestCount++
		w.Header().Set("Content-Type", "application/json")
		switch requestCount {
		case 1:
			fmt.Fprintf(w, `{"values":[
				{"pull_request":{"id":1650},"some_kind":{"note":"unknown 1"}},
				{"pull_request":{"id":1650},"approval":{"date":"2026-08-01T00:00:00+00:00","user":{"display_name":"A"}}}
			],"next":"%s/2.0/repositories/acme/widgets/pullrequests/1650/activity?page=2"}`, "http://"+r.Host)
		case 2:
			fmt.Fprint(w, `{"values":[
				{"pull_request":{"id":1650},"approval":{"date":"2026-08-02T00:00:00+00:00","user":{"display_name":"B"}}}
			],"next":""}`)
		default:
			t.Errorf("unexpected request #%d to %s", requestCount, r.URL.String())
			w.WriteHeader(http.StatusInternalServerError)
		}
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
	if requestCount != 2 {
		t.Errorf("server received %d requests, want exactly 2 (first page fell short of --limit after filtering unknown kinds)", requestCount)
	}

	var activities []Activity
	if err := json.Unmarshal([]byte(stdout), &activities); err != nil {
		t.Fatalf("cannot unmarshal printed output %q: %v", stdout, err)
	}
	if len(activities) != 2 {
		t.Fatalf("printed %d activities, want 2 known activities collected across both pages", len(activities))
	}
}
