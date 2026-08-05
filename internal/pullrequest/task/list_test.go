package task

import (
	"encoding/json"
	"net/http"
	"strings"
	"testing"
)

func withListOptions(t *testing.T, mutate func()) {
	t.Helper()
	oldPullRequestIDValue := listOptions.PullRequestID.Value
	oldQuery := listOptions.Query
	t.Cleanup(func() {
		listOptions.PullRequestID.Value = oldPullRequestIDValue
		listOptions.Query = oldQuery
	})
	mutate()
}

func TestListProcessSuccess(t *testing.T) {
	withListOptions(t, func() {
		listOptions.PullRequestID.Value = "42"
		listOptions.Query = ""
	})

	var requests []*http.Request
	cmd := setupTest(t, func(w http.ResponseWriter, r *http.Request) {
		requests = append(requests, r)
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"values":[{"id":1,"content":{"raw":"do X"}},{"id":2,"content":{"raw":"do Y"}}]}`))
	}, false)

	stdout := captureStdout(t, func() {
		if err := listProcess(cmd, nil); err != nil {
			t.Fatalf("listProcess() error = %v", err)
		}
	})

	if len(requests) != 1 {
		t.Fatalf("expected exactly 1 request, got %d", len(requests))
	}
	wantPath := "/2.0/repositories/" + fixtureRepositoryFlag + "/pullrequests/42/tasks"
	if requests[0].URL.Path != wantPath {
		t.Errorf("path = %s, want %s", requests[0].URL.Path, wantPath)
	}

	var tasks []Task
	if err := json.Unmarshal([]byte(stdout), &tasks); err != nil {
		t.Fatalf("cannot unmarshal printed output %q: %v", stdout, err)
	}
	if len(tasks) != 2 {
		t.Fatalf("expected 2 tasks, got %d", len(tasks))
	}
}

func TestListProcessNoResults(t *testing.T) {
	withListOptions(t, func() {
		listOptions.PullRequestID.Value = "42"
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
		t.Errorf("expected no output when there are no tasks, got %q", stdout)
	}
}

func TestListProcessWithQuery(t *testing.T) {
	withListOptions(t, func() {
		listOptions.PullRequestID.Value = "42"
		listOptions.Query = `content.raw~"fix"`
	})

	var requests []*http.Request
	cmd := setupTest(t, func(w http.ResponseWriter, r *http.Request) {
		requests = append(requests, r)
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"values":[]}`))
	}, false)

	if err := listProcess(cmd, nil); err != nil {
		t.Fatalf("listProcess() error = %v", err)
	}
	if len(requests) != 1 {
		t.Fatalf("expected exactly 1 request, got %d", len(requests))
	}
	want := `content.raw~"fix"`
	if got := requests[0].URL.Query().Get("q"); got != want {
		t.Errorf("q query parameter = %q, want %q", got, want)
	}
}

func TestListProcessAPIError(t *testing.T) {
	withListOptions(t, func() {
		listOptions.PullRequestID.Value = "42"
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

// TestListCmdRegistersLimitFlag proves --limit is registered on the real "pr task list" command.
func TestListCmdRegistersLimitFlag(t *testing.T) {
	if listCmd.Flags().Lookup("limit") == nil {
		t.Fatal(`"pr task list" has no --limit flag registered`)
	}
}

// TestListProcessRespectsLimitFlag is a regression test for --limit being wired onto a real
// command: it drives listProcess with a "limit" flag on its cmd, proving the value actually
// reaches GetAll and truncates the result instead of being permanently unreachable dead plumbing.
func TestListProcessRespectsLimitFlag(t *testing.T) {
	withListOptions(t, func() {
		listOptions.PullRequestID.Value = "42"
		listOptions.Query = ""
	})

	cmd := setupTest(t, func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"values":[{"id":1,"content":{"raw":"do X"}},{"id":2,"content":{"raw":"do Y"}}]}`))
	}, false)
	cmd.Flags().Int("limit", 0, "")
	if err := cmd.Flags().Set("limit", "1"); err != nil {
		t.Fatalf("cannot set limit flag: %v", err)
	}

	stdout := captureStdout(t, func() {
		if err := listProcess(cmd, nil); err != nil {
			t.Fatalf("listProcess() error = %v", err)
		}
	})

	var tasks []Task
	if err := json.Unmarshal([]byte(stdout), &tasks); err != nil {
		t.Fatalf("cannot unmarshal printed output %q: %v", stdout, err)
	}
	if len(tasks) != 1 {
		t.Fatalf("expected exactly 1 task with --limit 1, got %d", len(tasks))
	}
}

func TestListProcessDryRun(t *testing.T) {
	withListOptions(t, func() {
		listOptions.PullRequestID.Value = "42"
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
