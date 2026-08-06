package pipeline

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
		_, _ = w.Write([]byte(`{"type":"pipeline","uuid":"{a1b2c3d4-e5f6-7890-abcd-ef1234567890}","build_number":42,"state":{"type":"pipeline_state_completed","name":"COMPLETED"},"target":{"type":"pipeline_ref_target","ref_type":"branch","ref_name":"main"},"created_on":"2026-01-01T00:00:00+00:00","duration_in_seconds":0}`))
	}, false)

	stdout := testutil.CaptureStdout(t, func() {
		if err := getProcess(cmd, []string{"42"}); err != nil {
			t.Fatalf("getProcess() error = %v", err)
		}
	})

	if len(requests) != 1 {
		t.Fatalf("expected exactly 1 request, got %d", len(requests))
	}
	wantPath := "/2.0/repositories/" + testutil.FixtureRepositoryFlag + "/pipelines/42"
	if requests[0].URL.Path != wantPath {
		t.Errorf("path = %s, want %s", requests[0].URL.Path, wantPath)
	}

	var got Pipeline
	if err := json.Unmarshal([]byte(stdout), &got); err != nil {
		t.Fatalf("cannot unmarshal printed output %q: %v", stdout, err)
	}
	if got.BuildNumber != 42 {
		t.Errorf("printed pipeline build number = %d, want 42", got.BuildNumber)
	}
}

func TestGetProcessAPIError(t *testing.T) {
	cmd := setupTest(t, func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusNotFound)
		_, _ = w.Write([]byte(`{"type":"error","error":{"message":"pipeline not found"}}`))
	}, false)

	err := getProcess(cmd, []string{"99"})
	if err == nil {
		t.Fatal("getProcess() expected an error, got nil")
	}
	if !strings.Contains(err.Error(), "cannot get pipeline 99") {
		t.Errorf("error = %q, want it to mention the pipeline argument", err.Error())
	}
	if !strings.Contains(err.Error(), "pipeline not found") {
		t.Errorf("error = %q, want it to contain the BitBucket error message", err.Error())
	}
}

func TestGetProcessDryRunSkipsFetchAndPrinting(t *testing.T) {
	var requestCount int
	cmd := setupTest(t, func(http.ResponseWriter, *http.Request) { requestCount++ }, true)

	stdout := testutil.CaptureStdout(t, func() {
		if err := getProcess(cmd, []string{"42"}); err != nil {
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
