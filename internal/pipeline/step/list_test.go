package step

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
			`{"type":"pipeline_step","uuid":"{11111111-1111-1111-1111-111111111111}","name":"b-step","run_number":2,"state":{"type":"pipeline_step_state_completed","name":"COMPLETED"},"image":{"name":"golang:1.25"},"started_on":"2026-01-01T00:00:00Z","duration_in_seconds":0},` +
			`{"type":"pipeline_step","uuid":"{22222222-2222-2222-2222-222222222222}","name":"a-step","run_number":1,"state":{"type":"pipeline_step_state_completed","name":"COMPLETED"},"image":{"name":"golang:1.25"},"started_on":"2026-01-01T00:00:00Z","duration_in_seconds":0}` +
			`]}`))
	}, false)
	if err := cmd.Flags().Set("pipeline", "42"); err != nil {
		t.Fatalf("cannot set pipeline flag: %v", err)
	}

	stdout := testutil.CaptureStdout(t, func() {
		if err := listProcess(cmd, nil); err != nil {
			t.Fatalf("listProcess() error = %v", err)
		}
	})

	if len(requests) != 1 {
		t.Fatalf("expected exactly 1 request, got %d", len(requests))
	}
	wantPath := "/2.0/repositories/" + testutil.FixtureRepositoryFlag + "/pipelines/42/steps"
	if requests[0].URL.Path != wantPath {
		t.Errorf("path = %s, want %s", requests[0].URL.Path, wantPath)
	}

	var steps []Step
	if err := json.Unmarshal([]byte(stdout), &steps); err != nil {
		t.Fatalf("cannot unmarshal printed output %q: %v", stdout, err)
	}
	if len(steps) != 2 || steps[0].Name != "b-step" || steps[1].Name != "a-step" {
		t.Errorf("steps = %+v, want API order preserved (b-step, a-step) since --sort was not set", steps)
	}
}

// TestListProcessSortFlagChangedSorts proves the sort-guard (rule 3): core.Sort only runs when
// cmd's "sort" flag is Changed, never unconditionally against an untouched default. The sort value
// exercised is "id" -- the column table's DefaultSorter -- mirroring internal/pipeline/list_test.go's
// own TestListProcessSortFlagChangedSorts: listOptions.SortBy is a package-level *common.EnumFlag
// bound to the real listCmd's own "sort" pflag.Value, so a standalone test cmd's separate "sort"
// string flag (registered by setupTest) only ever flips cmd.Flag("sort").Changed to trigger the
// guard -- the actual sort key used is always listOptions.SortBy's own value, which is why this
// works precisely because "id" already is that default.
func TestListProcessSortFlagChangedSorts(t *testing.T) {
	cmd := setupTest(t, func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"values":[` +
			`{"type":"pipeline_step","uuid":"{bbbbbbbb-bbbb-bbbb-bbbb-bbbbbbbbbbbb}","name":"b-step","started_on":"2026-01-01T00:00:00Z"},` +
			`{"type":"pipeline_step","uuid":"{aaaaaaaa-aaaa-aaaa-aaaa-aaaaaaaaaaaa}","name":"a-step","started_on":"2026-01-01T00:00:00Z"}` +
			`]}`))
	}, false)
	if err := cmd.Flags().Set("pipeline", "42"); err != nil {
		t.Fatalf("cannot set pipeline flag: %v", err)
	}
	if err := cmd.Flags().Set("sort", "id"); err != nil {
		t.Fatalf("cannot set sort flag: %v", err)
	}

	stdout := testutil.CaptureStdout(t, func() {
		if err := listProcess(cmd, nil); err != nil {
			t.Fatalf("listProcess() error = %v", err)
		}
	})

	var steps []Step
	if err := json.Unmarshal([]byte(stdout), &steps); err != nil {
		t.Fatalf("cannot unmarshal printed output %q: %v", stdout, err)
	}
	if len(steps) != 2 || steps[0].Name != "a-step" || steps[1].Name != "b-step" {
		t.Errorf("steps = %+v, want sorted by id ascending (a-step, b-step) once --sort is Changed", steps)
	}
}

// TestListProcessNoResults proves rule 4: an empty list prints "No step found" on stdout, fixing
// upstream's copy-paste "No comment found".
func TestListProcessNoResults(t *testing.T) {
	var requests []*http.Request
	cmd := setupTest(t, func(w http.ResponseWriter, r *http.Request) {
		requests = append(requests, r)
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"values":[]}`))
	}, false)
	if err := cmd.Flags().Set("pipeline", "42"); err != nil {
		t.Fatalf("cannot set pipeline flag: %v", err)
	}

	stdout := testutil.CaptureStdout(t, func() {
		if err := listProcess(cmd, nil); err != nil {
			t.Fatalf("listProcess() error = %v", err)
		}
	})

	if len(requests) != 1 {
		t.Fatalf("expected exactly 1 request, got %d", len(requests))
	}
	if strings.TrimSpace(stdout) != "No step found" {
		t.Errorf("stdout = %q, want %q printed on stdout", stdout, "No step found")
	}
}

func TestListProcessAPIError(t *testing.T) {
	cmd := setupTest(t, func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusInternalServerError)
		_, _ = w.Write([]byte(`{"type":"error","error":{"message":"server exploded"}}`))
	}, false)
	if err := cmd.Flags().Set("pipeline", "42"); err != nil {
		t.Fatalf("cannot set pipeline flag: %v", err)
	}

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
	if err := cmd.Flags().Set("pipeline", "42"); err != nil {
		t.Fatalf("cannot set pipeline flag: %v", err)
	}

	if err := listProcess(cmd, nil); err != nil {
		t.Fatalf("listProcess() error = %v", err)
	}
	if requestCount != 0 {
		t.Errorf("expected no HTTP request in dry-run mode, got %d", requestCount)
	}
}
