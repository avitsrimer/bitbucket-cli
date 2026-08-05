package comment

import (
	"encoding/json"
	"net/http"
	"strings"
	"testing"
)

func TestUpdateProcessSuccess(t *testing.T) {
	withCommentEditOptions(t, &updateOptions, func() {
		updateOptions.PullRequestID.Value = "42"
		updateOptions.Comment = "updated comment"
	})

	var requests []*http.Request
	var gotBody CommentUpdator
	cmd := setupTest(t, func(w http.ResponseWriter, r *http.Request) {
		requests = append(requests, r)
		if err := json.NewDecoder(r.Body).Decode(&gotBody); err != nil {
			t.Errorf("cannot decode request body: %v", err)
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"id":7,"content":{"raw":"updated comment"}}`))
	}, false)

	stdout := captureStdout(t, func() {
		if err := updateProcess(cmd, []string{"7"}); err != nil {
			t.Fatalf("updateProcess() error = %v", err)
		}
	})

	if len(requests) != 1 {
		t.Fatalf("expected exactly 1 request, got %d", len(requests))
	}
	wantPath := "/2.0/repositories/" + fixtureRepositoryFlag + "/pullrequests/42/comments/7"
	if requests[0].URL.Path != wantPath {
		t.Errorf("path = %s, want %s", requests[0].URL.Path, wantPath)
	}
	if requests[0].Method != http.MethodPut {
		t.Errorf("method = %s, want PUT", requests[0].Method)
	}
	if gotBody.Content.Raw != "updated comment" {
		t.Errorf("posted content.raw = %q, want %q", gotBody.Content.Raw, "updated comment")
	}
	var printed Comment
	if err := json.Unmarshal([]byte(stdout), &printed); err != nil {
		t.Fatalf("cannot unmarshal printed output %q: %v", stdout, err)
	}
	if printed.ID != 7 {
		t.Errorf("printed id = %d, want 7", printed.ID)
	}
}

// TestUpdateProcessToWithoutFileReturnsError mirrors the create-side regression: the from/to
// without a file anchor must produce the real, readable error message, not a blank one, and
// send no request.
func TestUpdateProcessToWithoutFileReturnsError(t *testing.T) {
	withCommentEditOptions(t, &updateOptions, func() {
		updateOptions.PullRequestID.Value = "42"
		updateOptions.Comment = "updated comment"
		updateOptions.To = 5
	})

	var requestCount int
	cmd := setupTest(t, func(http.ResponseWriter, *http.Request) { requestCount++ }, false)

	err := updateProcess(cmd, []string{"7"})
	if err == nil {
		t.Fatal("updateProcess() expected an error, got nil")
	}
	if !strings.Contains(err.Error(), "cannot specify from/to without a file") {
		t.Errorf("error = %q, want it to contain the real message instead of a blank/generic one", err.Error())
	}
	if requestCount != 0 {
		t.Errorf("expected no HTTP request when the anchor is invalid, got %d", requestCount)
	}
}

func TestUpdateProcessAPIError(t *testing.T) {
	withCommentEditOptions(t, &updateOptions, func() {
		updateOptions.PullRequestID.Value = "42"
		updateOptions.Comment = "updated comment"
	})

	cmd := setupTest(t, func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusNotFound)
		_, _ = w.Write([]byte(`{"type":"error","error":{"message":"comment not found"}}`))
	}, false)

	err := updateProcess(cmd, []string{"7"})
	if err == nil {
		t.Fatal("updateProcess() expected an error, got nil")
	}
	if !strings.Contains(err.Error(), "failed to update comment") {
		t.Errorf("error = %q, want it to mention the failed update", err.Error())
	}
	if !strings.Contains(err.Error(), "comment not found") {
		t.Errorf("error = %q, want it to contain the BitBucket error message", err.Error())
	}
}

func TestUpdateProcessDryRun(t *testing.T) {
	withCommentEditOptions(t, &updateOptions, func() {
		updateOptions.PullRequestID.Value = "42"
		updateOptions.Comment = "updated comment"
	})

	var requestCount int
	cmd := setupTest(t, func(http.ResponseWriter, *http.Request) { requestCount++ }, true)

	if err := updateProcess(cmd, []string{"7"}); err != nil {
		t.Fatalf("updateProcess() error = %v", err)
	}
	if requestCount != 0 {
		t.Errorf("expected no HTTP request in dry-run mode, got %d", requestCount)
	}
}
