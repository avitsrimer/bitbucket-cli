package artifact

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
			`{"name":"zeta.log","size":200,"downloads":1},` +
			`{"name":"alpha.log","size":100,"downloads":2}` +
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
	wantPath := "/2.0/repositories/" + testutil.FixtureRepositoryFlag + "/downloads"
	if requests[0].URL.Path != wantPath {
		t.Errorf("path = %s, want %s", requests[0].URL.Path, wantPath)
	}

	var artifacts []Artifact
	if err := json.Unmarshal([]byte(stdout), &artifacts); err != nil {
		t.Fatalf("cannot unmarshal printed output %q: %v", stdout, err)
	}
	if len(artifacts) != 2 || artifacts[0].Name != "zeta.log" || artifacts[1].Name != "alpha.log" {
		t.Errorf("artifacts = %+v, want API order preserved (zeta.log, alpha.log) since --sort was not set", artifacts)
	}
}

// TestListProcessSortFlagChangedSorts proves the sort-guard (rule 3): core.Sort only runs when
// cmd's "sort" flag is Changed, never unconditionally against an untouched default.
func TestListProcessSortFlagChangedSorts(t *testing.T) {
	cmd := setupTest(t, func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"values":[` +
			`{"name":"zeta.log"},` +
			`{"name":"alpha.log"}` +
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

	var artifacts []Artifact
	if err := json.Unmarshal([]byte(stdout), &artifacts); err != nil {
		t.Fatalf("cannot unmarshal printed output %q: %v", stdout, err)
	}
	if len(artifacts) != 2 || artifacts[0].Name != "alpha.log" || artifacts[1].Name != "zeta.log" {
		t.Errorf("artifacts = %+v, want sorted by name ascending (alpha.log, zeta.log) once --sort is Changed", artifacts)
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
	if strings.TrimSpace(stdout) != "No artifact found" {
		t.Errorf("stdout = %q, want %q printed on stdout", stdout, "No artifact found")
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
	if err := cmd.Flags().Set("query", `name~"build"`); err != nil {
		t.Fatalf("cannot set query flag: %v", err)
	}

	if err := listProcess(cmd, nil); err != nil {
		t.Fatalf("listProcess() error = %v", err)
	}

	if len(requests) != 1 {
		t.Fatalf("expected exactly 1 request, got %d", len(requests))
	}
	if got := requests[0].URL.Query().Get("q"); got != `name~"build"` {
		t.Errorf("q query = %q, want %q", got, `name~"build"`)
	}
}
