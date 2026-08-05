package pullrequest

import (
	"encoding/json"
	"net/http"
	"strings"
	"testing"
)

func TestGetProcessSuccess(t *testing.T) {
	var requests []*http.Request
	cmd := setupTest(t, func(w http.ResponseWriter, r *http.Request) {
		requests = append(requests, r)
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"id":42,"title":"Add feature","state":"OPEN"}`))
	}, false)

	stdout := captureStdout(t, func() {
		if err := getProcess(cmd, []string{"42"}); err != nil {
			t.Fatalf("getProcess() error = %v", err)
		}
	})

	if len(requests) != 1 {
		t.Fatalf("expected exactly 1 request, got %d", len(requests))
	}
	wantPath := "/2.0/repositories/" + fixtureRepositoryFlag + "/pullrequests/42"
	if requests[0].URL.Path != wantPath {
		t.Errorf("path = %s, want %s", requests[0].URL.Path, wantPath)
	}
	if requests[0].Method != http.MethodGet {
		t.Errorf("method = %s, want GET", requests[0].Method)
	}

	var pr PullRequest
	if err := json.Unmarshal([]byte(stdout), &pr); err != nil {
		t.Fatalf("cannot unmarshal printed output %q: %v", stdout, err)
	}
	if pr.Title != "Add feature" {
		t.Errorf("printed pullrequest title = %q, want %q", pr.Title, "Add feature")
	}
}

func TestGetProcessAPIError(t *testing.T) {
	cmd := setupTest(t, func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusNotFound)
		_, _ = w.Write([]byte(`{"type":"error","error":{"message":"pull request not found"}}`))
	}, false)

	err := getProcess(cmd, []string{"42"})
	if err == nil {
		t.Fatal("getProcess() expected an error, got nil")
	}
	if !strings.Contains(err.Error(), "failed to get pullrequest 42") {
		t.Errorf("error = %q, want it to mention the failed get", err.Error())
	}
	if !strings.Contains(err.Error(), "pull request not found") {
		t.Errorf("error = %q, want it to contain the BitBucket error message", err.Error())
	}
}

func TestGetProcessDryRun(t *testing.T) {
	var requestCount int
	cmd := setupTest(t, func(http.ResponseWriter, *http.Request) { requestCount++ }, true)

	if err := getProcess(cmd, []string{"42"}); err != nil {
		t.Fatalf("getProcess() error = %v", err)
	}
	if requestCount != 0 {
		t.Errorf("expected no HTTP request in dry-run mode, got %d", requestCount)
	}
}
