package step

import (
	"encoding/json"
	"net/http"
	"strings"
	"testing"

	"github.com/avitsrimer/bitbucket-cli/internal/testutil"
)

func TestGetProcessSuccess(t *testing.T) {
	var requests []*http.Request
	cmd := setupTest(t, func(w http.ResponseWriter, r *http.Request) {
		requests = append(requests, r)
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"type":"pipeline_step","uuid":"{cec5beef-dead-deed-bead-5ae1bedd9ada}","name":"Test and Build","run_number":1,"pipeline":{"type":"pipeline","uuid":"{3edaa916-baad-beef-dead-28846deafec1}"},"state":{"type":"pipeline_step_state_completed","name":"COMPLETED"},"image":{"name":"golang:1.25"},"maxTime":120,"duration_in_seconds":19,"started_on":"2026-01-03T07:36:40.109572944Z","completed_on":"2026-01-03T07:36:59.423783406Z"}`))
	}, false)
	if err := cmd.Flags().Set("pipeline", "42"); err != nil {
		t.Fatalf("cannot set pipeline flag: %v", err)
	}

	stdout := testutil.CaptureStdout(t, func() {
		if err := getProcess(cmd, []string{"{cec5beef-dead-deed-bead-5ae1bedd9ada}"}); err != nil {
			t.Fatalf("getProcess() error = %v", err)
		}
	})

	if len(requests) != 1 {
		t.Fatalf("expected exactly 1 request, got %d", len(requests))
	}
	wantPath := "/2.0/repositories/" + testutil.FixtureRepositoryFlag + "/pipelines/42/steps/{cec5beef-dead-deed-bead-5ae1bedd9ada}"
	if requests[0].URL.Path != wantPath {
		t.Errorf("path = %s, want %s", requests[0].URL.Path, wantPath)
	}

	var got Step
	if err := json.Unmarshal([]byte(stdout), &got); err != nil {
		t.Fatalf("cannot unmarshal printed output %q: %v", stdout, err)
	}
	if got.Name != "Test and Build" {
		t.Errorf("printed step name = %q, want %q", got.Name, "Test and Build")
	}
}

func TestGetProcessAPIError(t *testing.T) {
	cmd := setupTest(t, func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusNotFound)
		_, _ = w.Write([]byte(`{"type":"error","error":{"message":"step not found"}}`))
	}, false)
	if err := cmd.Flags().Set("pipeline", "42"); err != nil {
		t.Fatalf("cannot set pipeline flag: %v", err)
	}

	err := getProcess(cmd, []string{"missing"})
	if err == nil {
		t.Fatal("getProcess() expected an error, got nil")
	}
	if !strings.Contains(err.Error(), "cannot get step missing") {
		t.Errorf("error = %q, want it to mention the step argument", err.Error())
	}
	if !strings.Contains(err.Error(), "step not found") {
		t.Errorf("error = %q, want it to contain the BitBucket error message", err.Error())
	}
}

func TestGetProcessDryRunSkipsFetchAndPrinting(t *testing.T) {
	var requestCount int
	cmd := setupTest(t, func(http.ResponseWriter, *http.Request) { requestCount++ }, true)
	if err := cmd.Flags().Set("pipeline", "42"); err != nil {
		t.Fatalf("cannot set pipeline flag: %v", err)
	}

	stdout := testutil.CaptureStdout(t, func() {
		if err := getProcess(cmd, []string{"{cec5beef-dead-deed-bead-5ae1bedd9ada}"}); err != nil {
			t.Fatalf("getProcess() error = %v", err)
		}
	})
	if requestCount != 0 {
		t.Errorf("expected no HTTP request in dry-run mode, got %d", requestCount)
	}
	if stdout != "" {
		t.Errorf("expected no printed output in dry-run mode, got %q", stdout)
	}
}
