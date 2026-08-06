package branch

import (
	"encoding/json"
	"net/http"
	"strings"
	"testing"

	"github.com/avitsrimer/bitbucket-cli/internal/testutil"
)

func TestListProcessSuccessPreservesAPIOrderWithoutSortFlag(t *testing.T) {
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
	if len(branches) != 2 || branches[0].Name != "zeta" || branches[1].Name != "alpha" {
		t.Errorf("branches = %+v, want API order preserved (zeta, alpha) since --sort was not set", branches)
	}
}

// TestListProcessSortFlagChangedSorts proves the sort-guard (rule 3): core.Sort only runs when
// cmd's "sort" flag is Changed, never unconditionally against an untouched default.
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
