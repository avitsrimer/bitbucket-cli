package task

import (
	"encoding/json"
	"net/http"
	"strings"
	"testing"
)

func withGetOptions(t *testing.T, mutate func()) {
	t.Helper()
	old := getOptions.PullRequestID.Value
	t.Cleanup(func() { getOptions.PullRequestID.Value = old })
	mutate()
}

func TestGetProcessSuccess(t *testing.T) {
	withGetOptions(t, func() { getOptions.PullRequestID.Value = "42" })

	var requests []*http.Request
	cmd := setupTest(t, func(w http.ResponseWriter, r *http.Request) {
		requests = append(requests, r)
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"id":7,"content":{"raw":"please fix"},"state":"RESOLVED"}`))
	}, false)

	stdout := captureStdout(t, func() {
		if err := getProcess(cmd, []string{"7"}); err != nil {
			t.Fatalf("getProcess() error = %v", err)
		}
	})

	if len(requests) != 1 {
		t.Fatalf("expected exactly 1 request, got %d", len(requests))
	}
	wantPath := "/2.0/repositories/" + fixtureRepositoryFlag + "/pullrequests/42/tasks/7"
	if requests[0].URL.Path != wantPath {
		t.Errorf("path = %s, want %s", requests[0].URL.Path, wantPath)
	}
	if requests[0].Method != http.MethodGet {
		t.Errorf("method = %s, want GET", requests[0].Method)
	}

	var got Task
	if err := json.Unmarshal([]byte(stdout), &got); err != nil {
		t.Fatalf("cannot unmarshal printed output %q: %v", stdout, err)
	}
	if got.Content.Raw != "please fix" {
		t.Errorf("printed task content = %q, want %q", got.Content.Raw, "please fix")
	}
}

func TestGetProcessAPIError(t *testing.T) {
	withGetOptions(t, func() { getOptions.PullRequestID.Value = "42" })

	cmd := setupTest(t, func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusNotFound)
		_, _ = w.Write([]byte(`{"type":"error","error":{"message":"task not found"}}`))
	}, false)

	err := getProcess(cmd, []string{"7"})
	if err == nil {
		t.Fatal("getProcess() expected an error, got nil")
	}
	if !strings.Contains(err.Error(), "failed to get pullrequest task 7") {
		t.Errorf("error = %q, want it to mention the failed get", err.Error())
	}
	if !strings.Contains(err.Error(), "task not found") {
		t.Errorf("error = %q, want it to contain the BitBucket error message", err.Error())
	}
}

func TestGetProcessDryRun(t *testing.T) {
	withGetOptions(t, func() { getOptions.PullRequestID.Value = "42" })

	var requestCount int
	cmd := setupTest(t, func(http.ResponseWriter, *http.Request) { requestCount++ }, true)

	if err := getProcess(cmd, []string{"7"}); err != nil {
		t.Fatalf("getProcess() error = %v", err)
	}
	if requestCount != 0 {
		t.Errorf("expected no HTTP request in dry-run mode, got %d", requestCount)
	}
}
