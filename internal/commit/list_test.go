package commit

import (
	"encoding/json"
	"net/http"
	"strings"
	"testing"

	"github.com/avitsrimer/bitbucket-cli/internal/testutil"
)

// commitOrderDisagreementFixture is shared by TestListProcessSortFlagChangedSorts and
// TestListProcessDefaultSortsByDate: hash order and date order deliberately disagree (zzzzzzz is
// EARLIER, aaaaaaa is LATER), so the two tests' expected orderings are each other's reverse --
// proving --sort actually changes which comparator runs, not just that both tests happen to agree
// with commit's DefaultSorter (date). A fixture where hash and date order coincide cannot tell
// "sorted by hash" apart from "sorted by date", so a test built on it would pass even if --sort
// were silently ignored.
const commitOrderDisagreementFixture = `{"values":[` +
	`{"type":"commit","hash":"zzzzzzz","message":"zzz","date":"2026-01-01T00:00:00+00:00"},` +
	`{"type":"commit","hash":"aaaaaaa","message":"aaa","date":"2026-01-02T00:00:00+00:00"}` +
	`]}`

// TestListProcessDefaultSortsByDate proves the unadorned default (no --sort passed) actually
// sorts by date ascending -- the column commit.go marks DefaultSorter -- using the same
// order-disagreement fixture as TestListProcessSortFlagChangedSorts, so the two tests' opposite
// expected orders demonstrate the comparator genuinely changes with --sort rather than both
// tests accidentally agreeing with whatever the code happens to do.
func TestListProcessDefaultSortsByDate(t *testing.T) {
	var requests []*http.Request
	cmd := setupTest(t, func(w http.ResponseWriter, r *http.Request) {
		requests = append(requests, r)
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(commitOrderDisagreementFixture))
	}, false)

	stdout := testutil.CaptureStdout(t, func() {
		if err := listProcess(cmd, nil); err != nil {
			t.Fatalf("listProcess() error = %v", err)
		}
	})

	if len(requests) != 1 {
		t.Fatalf("expected exactly 1 request, got %d", len(requests))
	}
	wantPath := "/2.0/repositories/" + testutil.FixtureRepositoryFlag + "/commits"
	if requests[0].URL.Path != wantPath {
		t.Errorf("path = %s, want %s", requests[0].URL.Path, wantPath)
	}

	var commits []struct {
		Hash string `json:"hash"`
	}
	if err := json.Unmarshal([]byte(stdout), &commits); err != nil {
		t.Fatalf("cannot unmarshal printed output %q: %v", stdout, err)
	}
	if len(commits) != 2 || commits[0].Hash != "zzzzzzz" || commits[1].Hash != "aaaaaaa" {
		t.Errorf("commits = %+v, want sorted by date ascending by default (zzzzzzz, aaaaaaa)", commits)
	}
}

// TestListProcessSortFlagChangedSorts proves --sort actually selects the comparator core.Sort
// runs: reading it via common.SortFlagValue(cmd) (cmd's own --sort flag, not a package-level
// listOptions.SortBy.Value binding that is only ever populated on the real listCmd) means
// listProcess sorts identically whether cmd is the real command or, as here, a standalone test
// command carrying its own --sort flag.
func TestListProcessSortFlagChangedSorts(t *testing.T) {
	cmd := setupTest(t, func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(commitOrderDisagreementFixture))
	}, false)
	if err := cmd.Flags().Set("sort", "hash"); err != nil {
		t.Fatalf("cannot set sort flag: %v", err)
	}

	stdout := testutil.CaptureStdout(t, func() {
		if err := listProcess(cmd, nil); err != nil {
			t.Fatalf("listProcess() error = %v", err)
		}
	})

	// Unmarshal into a minimal local struct rather than []Commit: the fixture responses carry no
	// "repository" object, so the printed output's embedded repository is empty, and
	// Repository.UnmarshalJSON's Validate call would reject round-tripping that back through the
	// full Commit type.
	var commits []struct {
		Hash string `json:"hash"`
	}
	if err := json.Unmarshal([]byte(stdout), &commits); err != nil {
		t.Fatalf("cannot unmarshal printed output %q: %v", stdout, err)
	}
	if len(commits) != 2 || commits[0].Hash != "aaaaaaa" || commits[1].Hash != "zzzzzzz" {
		t.Errorf("commits = %+v, want sorted by hash ascending (aaaaaaa, zzzzzzz) -- the reverse of the default date order TestListProcessDefaultSortsByDate expects from this same fixture", commits)
	}
}

func TestListProcessNoResults(t *testing.T) {
	var requests []*http.Request
	cmd := setupTest(t, func(w http.ResponseWriter, r *http.Request) {
		requests = append(requests, r)
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"values":[]}`))
	}, false)

	stdout := testutil.CaptureStdout(t, func() {
		if err := listProcess(cmd, nil); err != nil {
			t.Fatalf("listProcess() error = %v", err)
		}
	})

	if len(requests) != 1 {
		t.Fatalf("expected exactly 1 request, got %d", len(requests))
	}
	if strings.TrimSpace(stdout) != "No commit found" {
		t.Errorf("stdout = %q, want %q printed on stdout", stdout, "No commit found")
	}
}

func TestListProcessAPIError(t *testing.T) {
	cmd := setupTest(t, func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusInternalServerError)
		_, _ = w.Write([]byte(`{"type":"error","error":{"message":"server exploded"}}`))
	}, false)

	err := listProcess(cmd, nil)
	if err == nil {
		t.Fatal("listProcess() expected an error, got nil")
	}
	if !strings.Contains(err.Error(), "server exploded") {
		t.Errorf("error = %q, want it to contain the BitBucket error message", err.Error())
	}
}

func TestListProcessDryRun(t *testing.T) {
	var requestCount int
	cmd := setupTest(t, func(http.ResponseWriter, *http.Request) { requestCount++ }, true)

	if err := listProcess(cmd, nil); err != nil {
		t.Fatalf("listProcess() error = %v", err)
	}
	if requestCount != 0 {
		t.Errorf("expected no HTTP request in dry-run mode, got %d", requestCount)
	}
}

func TestListProcessQueryIncludeExcludeFlags(t *testing.T) {
	var requests []*http.Request
	cmd := setupTest(t, func(w http.ResponseWriter, r *http.Request) {
		requests = append(requests, r)
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"values":[]}`))
	}, false)
	if err := cmd.Flags().Set("query", "message~\"fix\""); err != nil {
		t.Fatalf("cannot set query flag: %v", err)
	}
	if err := cmd.Flags().Set("include", "feature-branch"); err != nil {
		t.Fatalf("cannot set include flag: %v", err)
	}
	if err := cmd.Flags().Set("exclude", "main"); err != nil {
		t.Fatalf("cannot set exclude flag: %v", err)
	}

	if err := listProcess(cmd, nil); err != nil {
		t.Fatalf("listProcess() error = %v", err)
	}

	if len(requests) != 1 {
		t.Fatalf("expected exactly 1 request, got %d", len(requests))
	}
	query := requests[0].URL.Query()
	if got := query.Get("q"); got != `message~"fix"` {
		t.Errorf("q query = %q, want %q", got, `message~"fix"`)
	}
	if got := query.Get("include"); got != "feature-branch" {
		t.Errorf("include query = %q, want %q", got, "feature-branch")
	}
	if got := query.Get("exclude"); got != "main" {
		t.Errorf("exclude query = %q, want %q", got, "main")
	}
}

// TestListProcessRendersTableOutput proves the columns -> GetHeaders -> GetRow wiring actually
// reaches profile.Print for --output table, not just the JSON path every other test in this file
// drives.
func TestListProcessRendersTableOutput(t *testing.T) {
	cmd := setupTest(t, func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"values":[{"type":"commit","hash":"cafebeef","message":"Add feature","date":"2026-01-01T00:00:00+00:00"}]}`))
	}, false)
	if err := cmd.Flags().Set("output", "table"); err != nil {
		t.Fatalf("cannot set output flag: %v", err)
	}

	stdout := testutil.CaptureStdout(t, func() {
		if err := listProcess(cmd, nil); err != nil {
			t.Fatalf("listProcess() error = %v", err)
		}
	})

	if !strings.Contains(stdout, "Add feature") {
		t.Errorf("table output = %q, want it to contain the commit message", stdout)
	}
	if !strings.Contains(stdout, "+--") {
		t.Errorf("table output = %q, want tablewriter's box-drawing border", stdout)
	}
	var probe any
	if err := json.Unmarshal([]byte(stdout), &probe); err == nil {
		t.Errorf("table output = %q, want it not to parse as JSON", stdout)
	}
}
