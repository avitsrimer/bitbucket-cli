package step

import (
	"encoding/json"
	"net/http"
	"strings"
	"testing"

	"github.com/avitsrimer/bitbucket-cli/internal/testutil"
)

// TestListProcessDefaultPreservesExecutionOrder proves `bb pipeline step list` with --sort not
// passed preserves the API's own execution order (setup, then each script step in the order they
// ran, then teardown) instead of re-sorting by "id": a step's uuid has no inherent order, so a
// prior revision marking it DefaultSorter scrambled every step list into random uuid order with
// no --sort value that could restore it. No column in this package's columns table is marked
// DefaultSorter (see step.go's own comment), so common.SortFlagValue returns "" and listProcess
// skips sorting entirely. The fixture's API order (the larger uuid 22222222... first, then the
// smaller 11111111...) is deliberately the reverse of ascending-uuid order, so a regression back
// to sorting by id would flip these two steps and fail this assertion.
func TestListProcessDefaultPreservesExecutionOrder(t *testing.T) {
	var requests []*http.Request
	cmd := setupTest(t, func(w http.ResponseWriter, r *http.Request) {
		requests = append(requests, r)
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"values":[` +
			`{"type":"pipeline_step","uuid":"{22222222-2222-2222-2222-222222222222}","name":"b-step","run_number":2,"state":{"type":"pipeline_step_state_completed","name":"COMPLETED"},"image":{"name":"golang:1.25"},"started_on":"2026-01-01T00:00:00Z","duration_in_seconds":0},` +
			`{"type":"pipeline_step","uuid":"{11111111-1111-1111-1111-111111111111}","name":"a-step","run_number":1,"state":{"type":"pipeline_step_state_completed","name":"COMPLETED"},"image":{"name":"golang:1.25"},"started_on":"2026-01-01T00:00:00Z","duration_in_seconds":0}` +
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
		t.Errorf("steps = %+v, want the API's own execution order (b-step, a-step) preserved by default, not re-sorted by id", steps)
	}
}

// TestListProcessSortFlagChangedSorts proves --sort actually selects the comparator core.Sort
// runs, not just the column table's DefaultSorter ("id"): the fixture's id order and name order
// deliberately disagree (the alphabetically-later "zulu-step" carries the lexically-smaller uuid,
// and vice versa for "alpha-step"), so sorting by the explicitly requested "name" produces a
// different order than the default "id" sort would.
func TestListProcessSortFlagChangedSorts(t *testing.T) {
	cmd := setupTest(t, func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"values":[` +
			`{"type":"pipeline_step","uuid":"{aaaaaaaa-aaaa-aaaa-aaaa-aaaaaaaaaaaa}","name":"zulu-step","started_on":"2026-01-01T00:00:00Z"},` +
			`{"type":"pipeline_step","uuid":"{bbbbbbbb-bbbb-bbbb-bbbb-bbbbbbbbbbbb}","name":"alpha-step","started_on":"2026-01-01T00:00:00Z"}` +
			`]}`))
	}, false)
	if err := cmd.Flags().Set("pipeline", "42"); err != nil {
		t.Fatalf("cannot set pipeline flag: %v", err)
	}
	if err := cmd.Flags().Set("sort", "name"); err != nil {
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
	if len(steps) != 2 || steps[0].Name != "alpha-step" || steps[1].Name != "zulu-step" {
		t.Errorf("steps = %+v, want sorted by name ascending (alpha-step, zulu-step) -- the reverse of the id-ascending default order this fixture would produce", steps)
	}
}

// TestListProcessNoResults proves an empty list prints "No step found" on stdout.
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

// TestListProcessRendersTableOutput proves the columns -> GetHeaders -> GetRow wiring actually
// reaches profile.Print for --output table, not just the JSON path every other test in this file
// drives.
func TestListProcessRendersTableOutput(t *testing.T) {
	cmd := setupTest(t, func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"values":[{"type":"pipeline_step","uuid":"{11111111-1111-1111-1111-111111111111}","name":"build-step","run_number":1,"state":{"type":"pipeline_step_state_completed","name":"COMPLETED"},"image":{"name":"golang:1.25"},"started_on":"2026-01-01T00:00:00Z","duration_in_seconds":0}]}`))
	}, false)
	if err := cmd.Flags().Set("pipeline", "42"); err != nil {
		t.Fatalf("cannot set pipeline flag: %v", err)
	}
	if err := cmd.Flags().Set("output", "table"); err != nil {
		t.Fatalf("cannot set output flag: %v", err)
	}

	stdout := testutil.CaptureStdout(t, func() {
		if err := listProcess(cmd, nil); err != nil {
			t.Fatalf("listProcess() error = %v", err)
		}
	})

	if !strings.Contains(stdout, "build-step") {
		t.Errorf("table output = %q, want it to contain the step name", stdout)
	}
	if !strings.Contains(stdout, "+--") {
		t.Errorf("table output = %q, want tablewriter's box-drawing border", stdout)
	}
	var probe any
	if err := json.Unmarshal([]byte(stdout), &probe); err == nil {
		t.Errorf("table output = %q, want it not to parse as JSON", stdout)
	}
}
