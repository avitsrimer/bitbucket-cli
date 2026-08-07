package commit

import (
	"encoding/json"
	"net/http"
	"slices"
	"strings"
	"testing"

	"github.com/avitsrimer/bitbucket-cli/internal/testutil"
)

// commitOrderDisagreementFixture is shared by TestListProcessSortFlagChangedSorts and
// TestListProcessDefaultPreservesFetchOrder. With only two commits, hash-ascending order and
// date-ascending order cannot both differ from the raw fetch order AND from each other -- three
// items in a 2-element space forces at least two of those three orderings to coincide (pigeonhole)
// -- so this fixture uses three commits, with hash values and dates chosen as two independent
// cyclic rotations of the fetch order: fetch order is C1,C2,C3; hash-ascending order is C2,C3,C1;
// date-ascending order is C3,C1,C2. All three orderings are pairwise distinct, so no test built on
// this fixture can pass by coincidentally agreeing with the wrong ordering.
const commitOrderDisagreementFixture = `{"values":[` +
	`{"type":"commit","hash":"ccc3333","message":"c1","date":"2026-01-02T00:00:00+00:00"},` +
	`{"type":"commit","hash":"aaa1111","message":"c2","date":"2026-01-03T00:00:00+00:00"},` +
	`{"type":"commit","hash":"bbb2222","message":"c3","date":"2026-01-01T00:00:00+00:00"}` +
	`]}`

// TestListProcessDefaultPreservesFetchOrder proves `bb commit list` with --sort not passed
// preserves BitBucket's own commits-endpoint order (newest first, like `git log`) instead of
// re-sorting ascending by date: no column in this package's columns table is marked DefaultSorter
// (see commit.go's own comment), so common.SortFlagValue returns "" and listProcess skips sorting
// entirely. Using commitOrderDisagreementFixture (rather than a fixture where fetch order and
// date-ascending order happen to coincide) means a regression that reintroduced date as
// DefaultSorter would flip this fixture's two commits and fail this assertion, instead of passing
// by coincidence.
func TestListProcessDefaultPreservesFetchOrder(t *testing.T) {
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
	wantOrder := []string{"ccc3333", "aaa1111", "bbb2222"}
	gotOrder := []string{commits[0].Hash, commits[1].Hash, commits[2].Hash}
	if len(commits) != 3 || !slices.Equal(gotOrder, wantOrder) {
		t.Errorf("commits = %v, want the API's own fetch order %v preserved by default, not re-sorted by date", gotOrder, wantOrder)
	}
}

// TestListProcessSortFlagChangedSorts proves --sort actually selects the comparator common.Sort
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
	wantOrder := []string{"aaa1111", "bbb2222", "ccc3333"}
	gotOrder := []string{commits[0].Hash, commits[1].Hash, commits[2].Hash}
	if len(commits) != 3 || !slices.Equal(gotOrder, wantOrder) {
		t.Errorf("commits = %v, want sorted by hash ascending %v", gotOrder, wantOrder)
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
