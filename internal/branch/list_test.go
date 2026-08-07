package branch

import (
	"encoding/json"
	"net/http"
	"strings"
	"testing"

	"github.com/avitsrimer/bitbucket-cli/internal/testutil"
)

// TestListProcessDefaultSortsByName proves the real command's documented default ("--sort string
// Column to sort by (default \"name\")") actually applies when --sort is not passed: columns
// marks "name" as its DefaultSorter, and common.SortFlagValue resolves that default from the flag
// itself, so this must always sort ascending by name -- not merely preserve whatever order the
// API happened to return. The fixture's API order (zeta, then alpha) is deliberately reversed
// from the expected sorted order, so the two orders can never be confused.
func TestListProcessDefaultSortsByName(t *testing.T) {
	var requests []*http.Request
	cmd := setupTest(t, func(w http.ResponseWriter, r *http.Request) {
		requests = append(requests, r)
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"values":[` +
			`{"type":"branch","name":"zeta"},` +
			`{"type":"branch","name":"alpha"}` +
			`]}`))
	}, false)

	stdout := testutil.CaptureStdout(t, func() {
		if err := listProcess(cmd, nil); err != nil {
			t.Fatalf("listProcess() error = %v", err)
		}
	})

	if len(requests) != 1 {
		t.Fatalf("expected exactly 1 request, got %d", len(requests))
	}
	wantPath := "/2.0/repositories/" + testutil.FixtureRepositoryFlag + "/refs/branches"
	if requests[0].URL.Path != wantPath {
		t.Errorf("path = %s, want %s", requests[0].URL.Path, wantPath)
	}

	// Unmarshal into a minimal local struct rather than []Branch: Branch embeds a full
	// commit.Commit as its Target (no omitempty), and Commit's own Repository field carries
	// Repository.UnmarshalJSON's Validate call, which rejects round-tripping the fixture's empty
	// embedded repository back through the full type.
	var branches []struct {
		Name string `json:"name"`
	}
	if err := json.Unmarshal([]byte(stdout), &branches); err != nil {
		t.Fatalf("cannot unmarshal printed output %q: %v", stdout, err)
	}
	if len(branches) != 2 || branches[0].Name != "alpha" || branches[1].Name != "zeta" {
		t.Errorf("branches = %+v, want sorted by name ascending (alpha, zeta) by default, not the API's raw order (zeta, alpha)", branches)
	}
}

// TestListProcessSortFlagChangedSorts proves --sort actually selects the comparator
// common.Sort runs: reading it via common.SortFlagValue(cmd) (cmd's own --sort flag, not a
// package-level SortBy.Value binding that is only ever populated on the real command) means
// the process sorts identically whether cmd is the real command or, as here, a standalone
// test command carrying its own --sort flag.
func TestListProcessSortFlagChangedSorts(t *testing.T) {
	cmd := setupTest(t, func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"values":[` +
			`{"type":"branch","name":"zeta"},` +
			`{"type":"branch","name":"alpha"}` +
			`]}`))
	}, false)
	if err := cmd.Flags().Set("sort", "name"); err != nil {
		t.Fatalf("cannot set sort flag: %v", err)
	}

	stdout := testutil.CaptureStdout(t, func() {
		if err := listProcess(cmd, nil); err != nil {
			t.Fatalf("listProcess() error = %v", err)
		}
	})

	// Unmarshal into a minimal local struct rather than []Branch: Branch embeds a full
	// commit.Commit as its Target (no omitempty), and Commit's own Repository field carries
	// Repository.UnmarshalJSON's Validate call, which rejects round-tripping the fixture's empty
	// embedded repository back through the full type.
	var branches []struct {
		Name string `json:"name"`
	}
	if err := json.Unmarshal([]byte(stdout), &branches); err != nil {
		t.Fatalf("cannot unmarshal printed output %q: %v", stdout, err)
	}
	if len(branches) != 2 || branches[0].Name != "alpha" || branches[1].Name != "zeta" {
		t.Errorf("branches = %+v, want sorted by name ascending once --sort is Changed", branches)
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
	if strings.TrimSpace(stdout) != "No branch found" {
		t.Errorf("stdout = %q, want %q printed on stdout", stdout, "No branch found")
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

func TestListProcessQueryFlag(t *testing.T) {
	var requests []*http.Request
	cmd := setupTest(t, func(w http.ResponseWriter, r *http.Request) {
		requests = append(requests, r)
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"values":[]}`))
	}, false)
	if err := cmd.Flags().Set("query", `name~"feature"`); err != nil {
		t.Fatalf("cannot set query flag: %v", err)
	}

	if err := listProcess(cmd, nil); err != nil {
		t.Fatalf("listProcess() error = %v", err)
	}

	if len(requests) != 1 {
		t.Fatalf("expected exactly 1 request, got %d", len(requests))
	}
	if got := requests[0].URL.Query().Get("q"); got != `name~"feature"` {
		t.Errorf("q query = %q, want %q", got, `name~"feature"`)
	}
}

// TestListProcessRendersTableOutput proves the columns -> GetHeaders -> GetRow wiring
// actually reaches profile.Print for --output table, not just the JSON path every other
// test in this file drives.
func TestListProcessRendersTableOutput(t *testing.T) {
	cmd := setupTest(t, func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"values":[{"type":"branch","name":"main"}]}`))
	}, false)
	if err := cmd.Flags().Set("output", "table"); err != nil {
		t.Fatalf("cannot set output flag: %v", err)
	}

	stdout := testutil.CaptureStdout(t, func() {
		if err := listProcess(cmd, nil); err != nil {
			t.Fatalf("listProcess() error = %v", err)
		}
	})

	if !strings.Contains(stdout, "main") {
		t.Errorf("table output = %q, want it to contain the branch name", stdout)
	}
	if !strings.Contains(stdout, "+--") {
		t.Errorf("table output = %q, want the table renderer's box-drawing border", stdout)
	}
	var probe any
	if err := json.Unmarshal([]byte(stdout), &probe); err == nil {
		t.Errorf("table output = %q, want it not to parse as JSON", stdout)
	}
}
