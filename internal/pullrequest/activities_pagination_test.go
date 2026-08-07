package pullrequest

import (
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
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

// TestActivitiesProcessQueryPersistsOnFollowupPages is a regression test: fetchActivityPages' loop
// used to assign uripath = paginated.Next verbatim on every page after the first, dropping --query's
// q= parameter whenever BitBucket's own "next" link omitted it (silently merging unfiltered page-2+
// entries into the result). Routing through profile.NextPageURL -- the same, already unit-tested
// invariant profile.GetAll itself uses -- re-adds it.
func TestActivitiesProcessQueryPersistsOnFollowupPages(t *testing.T) {
	const wantQuery = `state="OPEN"`
	oldQuery := activitiesOptions.Query
	activitiesOptions.Query = wantQuery
	t.Cleanup(func() { activitiesOptions.Query = oldQuery })

	var secondRequestQuery string
	requestCount := 0
	cmd := setupTest(t, func(w http.ResponseWriter, r *http.Request) {
		requestCount++
		w.Header().Set("Content-Type", "application/json")
		switch requestCount {
		case 1:
			// BitBucket's own "next" link carries only pagination params, never q=.
			fmt.Fprintf(w, `{"values":[
				{"pull_request":{"id":1650},"approval":{"date":"2026-08-01T00:00:00+00:00","user":{"display_name":"A"}}}
			],"next":"%s/2.0/repositories/acme/widgets/pullrequests/1650/activity?page=2"}`, "http://"+r.Host)
		case 2:
			secondRequestQuery = r.URL.Query().Get("q")
			fmt.Fprint(w, `{"values":[
				{"pull_request":{"id":1650},"approval":{"date":"2026-08-02T00:00:00+00:00","user":{"display_name":"B"}}}
			],"next":""}`)
		default:
			t.Errorf("unexpected request #%d to %s", requestCount, r.URL.String())
			w.WriteHeader(http.StatusInternalServerError)
		}
	}, false)

	if _, err := profile.GetProfileFromCommand(cmd.Context(), cmd); err != nil {
		t.Fatalf("cannot warm up profile resolution: %v", err)
	}

	var runErr error
	testutil.CaptureStdout(t, func() {
		runErr = activitiesProcess(cmd, []string{"1650"})
	})
	if runErr != nil {
		t.Fatalf("activitiesProcess() error = %v", runErr)
	}
	if requestCount != 2 {
		t.Fatalf("server received %d requests, want 2", requestCount)
	}
	if secondRequestQuery != wantQuery {
		t.Errorf("second request q = %q, want %q (--query preserved across pages)", secondRequestQuery, wantQuery)
	}
}

// TestActivitiesProcessPageLengthShrinksOnFinalPage proves the other half of the same invariant:
// profile.NextPageURL's pagelen shrink (only fetch what --limit still needs) must still apply now
// that fetchActivityPages routes page-2+ requests through it.
func TestActivitiesProcessPageLengthShrinksOnFinalPage(t *testing.T) {
	var secondRequestPagelen string
	requestCount := 0
	cmd := setupTest(t, func(w http.ResponseWriter, r *http.Request) {
		requestCount++
		w.Header().Set("Content-Type", "application/json")
		switch requestCount {
		case 1:
			fmt.Fprintf(w, `{"values":[
				{"pull_request":{"id":1650},"approval":{"date":"2026-08-01T00:00:00+00:00","user":{"display_name":"A"}}},
				{"pull_request":{"id":1650},"approval":{"date":"2026-08-02T00:00:00+00:00","user":{"display_name":"B"}}}
			],"next":"%s/2.0/repositories/acme/widgets/pullrequests/1650/activity?page=2"}`, "http://"+r.Host)
		case 2:
			secondRequestPagelen = r.URL.Query().Get("pagelen")
			fmt.Fprint(w, `{"values":[
				{"pull_request":{"id":1650},"approval":{"date":"2026-08-03T00:00:00+00:00","user":{"display_name":"C"}}}
			],"next":""}`)
		default:
			t.Errorf("unexpected request #%d to %s", requestCount, r.URL.String())
			w.WriteHeader(http.StatusInternalServerError)
		}
	}, false)
	if err := cmd.Flags().Set("page-length", "2"); err != nil {
		t.Fatalf("cannot set page-length flag: %v", err)
	}
	if err := cmd.Flags().Set("limit", "3"); err != nil {
		t.Fatalf("cannot set limit flag: %v", err)
	}

	if _, err := profile.GetProfileFromCommand(cmd.Context(), cmd); err != nil {
		t.Fatalf("cannot warm up profile resolution: %v", err)
	}

	var runErr error
	testutil.CaptureStdout(t, func() {
		runErr = activitiesProcess(cmd, []string{"1650"})
	})
	if runErr != nil {
		t.Fatalf("activitiesProcess() error = %v", runErr)
	}
	if requestCount != 2 {
		t.Fatalf("server received %d requests, want 2", requestCount)
	}
	if secondRequestPagelen != "1" {
		t.Errorf("second request pagelen = %q, want %q (limit 3 - 2 known activities already collected)", secondRequestPagelen, "1")
	}
}

// TestActivitiesProcessPageLengthNotDroppedWhenQueryTextContainsPagelenSubstring pins that
// fetchActivityPages' pagelen= guard checks the actual "pagelen" query KEY, not a substring test
// over the WHOLE request path: a --query text that happens to contain "pagelen" (e.g.
// `--query 'title~"pagelen"'`) must not prevent --page-length from being appended, even though no
// pagelen= query parameter is actually present.
func TestActivitiesProcessPageLengthNotDroppedWhenQueryTextContainsPagelenSubstring(t *testing.T) {
	const wantQuery = `title~"pagelen"`
	oldQuery := activitiesOptions.Query
	activitiesOptions.Query = wantQuery
	t.Cleanup(func() { activitiesOptions.Query = oldQuery })

	var requestedPagelen string
	cmd := setupTest(t, func(w http.ResponseWriter, r *http.Request) {
		requestedPagelen = r.URL.Query().Get("pagelen")
		w.Header().Set("Content-Type", "application/json")
		fmt.Fprint(w, `{"values":[],"next":""}`)
	}, false)
	if err := cmd.Flags().Set("page-length", "25"); err != nil {
		t.Fatalf("cannot set page-length flag: %v", err)
	}

	if _, err := profile.GetProfileFromCommand(cmd.Context(), cmd); err != nil {
		t.Fatalf("cannot warm up profile resolution: %v", err)
	}

	var runErr error
	testutil.CaptureStdout(t, func() {
		runErr = activitiesProcess(cmd, []string{"1650"})
	})
	if runErr != nil {
		t.Fatalf("activitiesProcess() error = %v", runErr)
	}
	if requestedPagelen != "25" {
		t.Errorf("request pagelen = %q, want %q (--page-length must not be dropped just because --query's TEXT contains \"pagelen\")", requestedPagelen, "25")
	}
}

// TestFetchActivityPagesMalformedNextReturnsParseError proves fetchActivityPages surfaces
// profile.NextPageURL's own parse error, rather than silently requesting a broken next URL or
// returning a different error, when BitBucket's response body carries an unparsable "next" link.
func TestFetchActivityPagesMalformedNextReturnsParseError(t *testing.T) {
	cmd := setupTest(t, func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		fmt.Fprint(w, `{"values":[
			{"pull_request":{"id":1650},"approval":{"date":"2026-08-01T00:00:00+00:00","user":{"display_name":"A"}}}
		],"next":"://not-a-url"}`)
	}, false)

	currentProfile, err := profile.GetProfileFromCommand(cmd.Context(), cmd)
	if err != nil {
		t.Fatalf("cannot warm up profile resolution: %v", err)
	}

	uripath := "/2.0/repositories/acme/widgets/pullrequests/1650/activity"
	if _, err := fetchActivityPages(cmd, currentProfile, uripath); err == nil {
		t.Fatal("fetchActivityPages() expected an error for a malformed next page url, got nil")
	} else if !strings.Contains(err.Error(), "cannot parse next page url") {
		t.Errorf("fetchActivityPages() error = %q, want it to mention %q", err.Error(), "cannot parse next page url")
	}
}
