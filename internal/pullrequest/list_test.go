package pullrequest

import (
	"encoding/json"
	"net/http"
	"os"
	"strings"
	"testing"
)

func withListOptions(t *testing.T, mutate func()) {
	t.Helper()
	old := listOptions
	t.Cleanup(func() { listOptions = old })
	mutate()
}

func TestListProcessSuccess(t *testing.T) {
	withListOptions(t, func() {
		listOptions.Commit = ""
		listOptions.Query = ""
	})

	fixture, err := os.ReadFile("../../testdata/pullrequests.json")
	if err != nil {
		t.Fatalf("cannot read testdata: %v", err)
	}

	var requests []*http.Request
	cmd := setupTest(t, func(w http.ResponseWriter, r *http.Request) {
		requests = append(requests, r)
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write(fixture)
	}, false)

	stdout := captureStdout(t, func() {
		if err := listProcess(cmd, nil); err != nil {
			t.Fatalf("listProcess() error = %v", err)
		}
	})

	if len(requests) != 1 {
		t.Fatalf("expected exactly 1 request, got %d", len(requests))
	}
	wantPath := "/2.0/repositories/" + fixtureRepositoryFlag + "/pullrequests"
	if requests[0].URL.Path != wantPath {
		t.Errorf("path = %s, want %s", requests[0].URL.Path, wantPath)
	}
	if requests[0].URL.Query().Get("state") != "OPEN" {
		t.Errorf("state query = %s, want OPEN", requests[0].URL.Query().Get("state"))
	}

	var pullrequests []PullRequest
	if err := json.Unmarshal([]byte(stdout), &pullrequests); err != nil {
		t.Fatalf("cannot unmarshal printed output %q: %v", stdout, err)
	}
	if len(pullrequests) != 2 {
		t.Fatalf("expected 2 pullrequests, got %d", len(pullrequests))
	}
	if pullrequests[0].ID != 1 || pullrequests[1].ID != 2 {
		t.Errorf("pullrequests = %+v, want sorted by id ascending (1, 2)", pullrequests)
	}
}

func TestListProcessNoResults(t *testing.T) {
	withListOptions(t, func() {
		listOptions.Commit = ""
		listOptions.Query = ""
	})

	var requestCount int
	cmd := setupTest(t, func(w http.ResponseWriter, r *http.Request) {
		requestCount++
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"values":[]}`))
	}, false)

	stdout := captureStdout(t, func() {
		if err := listProcess(cmd, nil); err != nil {
			t.Fatalf("listProcess() error = %v", err)
		}
	})

	if requestCount != 1 {
		t.Fatalf("expected exactly 1 request, got %d", requestCount)
	}
	if strings.TrimSpace(stdout) != "" {
		t.Errorf("expected no output when there are no pullrequests, got %q", stdout)
	}
}

func TestListProcessAPIError(t *testing.T) {
	withListOptions(t, func() {
		listOptions.Commit = ""
		listOptions.Query = ""
	})

	cmd := setupTest(t, func(w http.ResponseWriter, r *http.Request) {
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
	withListOptions(t, func() {
		listOptions.Commit = ""
		listOptions.Query = ""
	})

	var requestCount int
	cmd := setupTest(t, func(http.ResponseWriter, *http.Request) { requestCount++ }, true)

	if err := listProcess(cmd, nil); err != nil {
		t.Fatalf("listProcess() error = %v", err)
	}
	if requestCount != 0 {
		t.Errorf("expected no HTTP request in dry-run mode, got %d", requestCount)
	}
}
