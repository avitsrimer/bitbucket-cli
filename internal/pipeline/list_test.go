package pipeline

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
			`{"type":"pipeline","uuid":"{11111111-1111-1111-1111-111111111111}","build_number":9,"state":{"type":"pipeline_state_completed","name":"COMPLETED"},"target":{"type":"pipeline_ref_target"},"created_on":"2026-01-02T00:00:00+00:00","duration_in_seconds":0},` +
			`{"type":"pipeline","uuid":"{22222222-2222-2222-2222-222222222222}","build_number":3,"state":{"type":"pipeline_state_completed","name":"COMPLETED"},"target":{"type":"pipeline_ref_target"},"created_on":"2026-01-01T00:00:00+00:00","duration_in_seconds":0}` +
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
	wantPath := "/2.0/repositories/" + testutil.FixtureRepositoryFlag + "/pipelines"
	if requests[0].URL.Path != wantPath {
		t.Errorf("path = %s, want %s", requests[0].URL.Path, wantPath)
	}
	if got := requests[0].URL.Query().Get("sort"); got != "-created_on" {
		t.Errorf("sort query = %q, want %q", got, "-created_on")
	}

	var pipelines []Pipeline
	if err := json.Unmarshal([]byte(stdout), &pipelines); err != nil {
		t.Fatalf("cannot unmarshal printed output %q: %v", stdout, err)
	}
	if len(pipelines) != 2 || pipelines[0].BuildNumber != 9 || pipelines[1].BuildNumber != 3 {
		t.Errorf("pipelines = %+v, want API order preserved (9, 3) since --sort was not set", pipelines)
	}
}

// TestListProcessSortFlagChangedSorts proves the sort-guard (rule 3): core.Sort only runs when
// cmd's "sort" flag is Changed, never unconditionally against an untouched default.
func TestListProcessSortFlagChangedSorts(t *testing.T) {
	cmd := setupTest(t, func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"values":[` +
			`{"type":"pipeline","uuid":"{11111111-1111-1111-1111-111111111111}","build_number":9,"state":{"type":"pipeline_state_completed","name":"COMPLETED"},"target":{"type":"pipeline_ref_target"},"created_on":"2026-01-02T00:00:00+00:00","duration_in_seconds":0},` +
			`{"type":"pipeline","uuid":"{22222222-2222-2222-2222-222222222222}","build_number":3,"state":{"type":"pipeline_state_completed","name":"COMPLETED"},"target":{"type":"pipeline_ref_target"},"created_on":"2026-01-01T00:00:00+00:00","duration_in_seconds":0}` +
			`]}`))
	}, false)
	if err := cmd.Flags().Set("sort", "build_number"); err != nil {
		t.Fatalf("cannot set sort flag: %v", err)
	}

	stdout := testutil.CaptureStdout(t, func() {
		if err := listProcess(cmd, nil); err != nil {
			t.Fatalf("listProcess() error = %v", err)
		}
	})

	var pipelines []Pipeline
	if err := json.Unmarshal([]byte(stdout), &pipelines); err != nil {
		t.Fatalf("cannot unmarshal printed output %q: %v", stdout, err)
	}
	if len(pipelines) != 2 || pipelines[0].BuildNumber != 3 || pipelines[1].BuildNumber != 9 {
		t.Errorf("pipelines = %+v, want sorted by build_number ascending (3, 9) once --sort is Changed", pipelines)
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
	if strings.TrimSpace(stdout) != "No pipeline found" {
		t.Errorf("stdout = %q, want %q printed on stdout", stdout, "No pipeline found")
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
	if err := cmd.Flags().Set("query", `target.branch.name="main"`); err != nil {
		t.Fatalf("cannot set query flag: %v", err)
	}

	if err := listProcess(cmd, nil); err != nil {
		t.Fatalf("listProcess() error = %v", err)
	}

	if len(requests) != 1 {
		t.Fatalf("expected exactly 1 request, got %d", len(requests))
	}
	if got := requests[0].URL.Query().Get("q"); got != `target.branch.name="main"` {
		t.Errorf("q query = %q, want %q", got, `target.branch.name="main"`)
	}
}
