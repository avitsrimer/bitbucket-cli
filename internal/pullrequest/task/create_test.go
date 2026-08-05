package task

import (
	"encoding/json"
	"net/http"
	"strings"
	"testing"
)

func withCreateOptions(t *testing.T, mutate func()) {
	t.Helper()
	oldPullRequestIDValue := createOptions.PullRequestID.Value
	oldContent := createOptions.Content
	oldCommentIDValue := createOptions.CommentID.Value
	oldPending := createOptions.Pending
	t.Cleanup(func() {
		createOptions.PullRequestID.Value = oldPullRequestIDValue
		createOptions.Content = oldContent
		createOptions.CommentID.Value = oldCommentIDValue
		createOptions.Pending = oldPending
	})
	mutate()
}

func TestCreateProcessSuccess(t *testing.T) {
	withCreateOptions(t, func() {
		createOptions.PullRequestID.Value = "42"
		createOptions.Content = "please fix"
		createOptions.CommentID.Value = ""
		createOptions.Pending = false
	})

	var requests []*http.Request
	var gotBody TaskCreator
	cmd := setupTest(t, func(w http.ResponseWriter, r *http.Request) {
		requests = append(requests, r)
		_ = json.NewDecoder(r.Body).Decode(&gotBody)
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"id":99,"content":{"raw":"please fix"}}`))
	}, false)

	stdout := captureStdout(t, func() {
		if err := createProcess(cmd, nil); err != nil {
			t.Fatalf("createProcess() error = %v", err)
		}
	})

	if len(requests) != 1 {
		t.Fatalf("expected exactly 1 request, got %d", len(requests))
	}
	wantPath := "/2.0/repositories/" + fixtureRepositoryFlag + "/pullrequests/42/tasks"
	if requests[0].URL.Path != wantPath {
		t.Errorf("path = %s, want %s", requests[0].URL.Path, wantPath)
	}
	if requests[0].Method != http.MethodPost {
		t.Errorf("method = %s, want POST", requests[0].Method)
	}
	if gotBody.Content.Raw != "please fix" {
		t.Errorf("posted content = %q, want %q", gotBody.Content.Raw, "please fix")
	}
	if gotBody.Comment != nil {
		t.Errorf("posted comment = %+v, want nil", gotBody.Comment)
	}

	var created Task
	if err := json.Unmarshal([]byte(stdout), &created); err != nil {
		t.Fatalf("cannot unmarshal printed output %q: %v", stdout, err)
	}
	if created.ID != 99 {
		t.Errorf("printed task id = %d, want 99", created.ID)
	}
}

func TestCreateProcessWithComment(t *testing.T) {
	withCreateOptions(t, func() {
		createOptions.PullRequestID.Value = "42"
		createOptions.Content = "please fix"
		createOptions.CommentID.Value = "123"
		createOptions.Pending = true
	})

	var gotBody TaskCreator
	cmd := setupTest(t, func(w http.ResponseWriter, r *http.Request) {
		_ = json.NewDecoder(r.Body).Decode(&gotBody)
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"id":100,"content":{"raw":"please fix"},"pending":true}`))
	}, false)

	captureStdout(t, func() {
		if err := createProcess(cmd, nil); err != nil {
			t.Fatalf("createProcess() error = %v", err)
		}
	})
	if gotBody.Comment == nil || gotBody.Comment.ID != 123 {
		t.Errorf("posted comment = %+v, want ID 123", gotBody.Comment)
	}
	if !gotBody.IsPending {
		t.Error("posted pending = false, want true")
	}
}

func TestCreateProcessInvalidCommentID(t *testing.T) {
	withCreateOptions(t, func() {
		createOptions.PullRequestID.Value = "42"
		createOptions.Content = "please fix"
		createOptions.CommentID.Value = "not-a-number"
	})

	var requestCount int
	cmd := setupTest(t, func(http.ResponseWriter, *http.Request) { requestCount++ }, false)

	err := createProcess(cmd, nil)
	if err == nil {
		t.Fatal("createProcess() expected an error, got nil")
	}
	if !strings.Contains(err.Error(), "failed to parse comment ID") {
		t.Errorf("error = %q, want it to mention the failed comment ID parse", err.Error())
	}
	if requestCount != 0 {
		t.Errorf("expected no HTTP request for an invalid comment ID, got %d", requestCount)
	}
}

func TestCreateProcessAPIError(t *testing.T) {
	withCreateOptions(t, func() {
		createOptions.PullRequestID.Value = "42"
		createOptions.Content = "please fix"
		createOptions.CommentID.Value = ""
	})

	cmd := setupTest(t, func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusBadRequest)
		_, _ = w.Write([]byte(`{"type":"error","error":{"message":"pull request is not open"}}`))
	}, false)

	err := createProcess(cmd, nil)
	if err == nil {
		t.Fatal("createProcess() expected an error, got nil")
	}
	if !strings.Contains(err.Error(), "failed to create pull request task on pull request 42") {
		t.Errorf("error = %q, want it to mention the failed create", err.Error())
	}
	if !strings.Contains(err.Error(), "pull request is not open") {
		t.Errorf("error = %q, want it to contain the BitBucket error message", err.Error())
	}
}

func TestCreateProcessDryRun(t *testing.T) {
	withCreateOptions(t, func() {
		createOptions.PullRequestID.Value = "42"
		createOptions.Content = "please fix"
		createOptions.CommentID.Value = ""
	})

	var requestCount int
	cmd := setupTest(t, func(http.ResponseWriter, *http.Request) { requestCount++ }, true)

	if err := createProcess(cmd, nil); err != nil {
		t.Fatalf("createProcess() error = %v", err)
	}
	if requestCount != 0 {
		t.Errorf("expected no HTTP request in dry-run mode, got %d", requestCount)
	}
}
