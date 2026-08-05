package comment

import (
	"encoding/json"
	"net/http"
	"strings"
	"testing"
)

func TestCreateProcessSuccess(t *testing.T) {
	withCommentEditOptions(t, &createOptions, func() {
		createOptions.PullRequestID.Value = "42"
		createOptions.Comment = "looks good"
	})

	var requests []*http.Request
	var gotBody CommentCreator
	cmd := setupTest(t, func(w http.ResponseWriter, r *http.Request) {
		requests = append(requests, r)
		if err := json.NewDecoder(r.Body).Decode(&gotBody); err != nil {
			t.Errorf("cannot decode request body: %v", err)
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"id":1,"content":{"raw":"looks good"}}`))
	}, false)

	stdout := captureStdout(t, func() {
		if err := createProcess(cmd, nil); err != nil {
			t.Fatalf("createProcess() error = %v", err)
		}
	})

	if len(requests) != 1 {
		t.Fatalf("expected exactly 1 request, got %d", len(requests))
	}
	wantPath := "/2.0/repositories/" + fixtureRepositoryFlag + "/pullrequests/42/comments"
	if requests[0].URL.Path != wantPath {
		t.Errorf("path = %s, want %s", requests[0].URL.Path, wantPath)
	}
	if requests[0].Method != http.MethodPost {
		t.Errorf("method = %s, want POST", requests[0].Method)
	}
	if gotBody.Content.Raw != "looks good" {
		t.Errorf("posted content.raw = %q, want %q", gotBody.Content.Raw, "looks good")
	}
	if gotBody.Anchor != nil {
		t.Errorf("posted anchor = %+v, want nil when --file is not set", gotBody.Anchor)
	}
	var printed Comment
	if err := json.Unmarshal([]byte(stdout), &printed); err != nil {
		t.Fatalf("cannot unmarshal printed output %q: %v", stdout, err)
	}
	if printed.Content.Raw != "looks good" {
		t.Errorf("printed content.raw = %q, want %q", printed.Content.Raw, "looks good")
	}
}

func TestCreateProcessWithFileAnchor(t *testing.T) {
	withCommentEditOptions(t, &createOptions, func() {
		createOptions.PullRequestID.Value = "42"
		createOptions.Comment = "fix this"
		createOptions.File = "main.go"
		createOptions.From = 10
		createOptions.To = 12
	})

	var gotBody CommentCreator
	cmd := setupTest(t, func(w http.ResponseWriter, r *http.Request) {
		if err := json.NewDecoder(r.Body).Decode(&gotBody); err != nil {
			t.Errorf("cannot decode request body: %v", err)
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"id":2}`))
	}, false)

	captureStdout(t, func() {
		if err := createProcess(cmd, nil); err != nil {
			t.Fatalf("createProcess() error = %v", err)
		}
	})

	if gotBody.Anchor == nil {
		t.Fatal("posted anchor = nil, want the --file/--from/--to anchor to be set")
	}
	if gotBody.Anchor.Path != "main.go" || gotBody.Anchor.From != 10 || gotBody.Anchor.To != 12 {
		t.Errorf("posted anchor = %+v, want {Path:main.go From:10 To:12}", gotBody.Anchor)
	}
}

// TestCreateProcessFromWithoutFileReturnsError is a regression test for the Task 4 fix: this
// used to be errors.RuntimeError.With("Cannot specify from/to without a file"), whose Text has
// zero %-verbs, so the rendered error was always just "Runtime Error" and the actual message was
// silently dropped. It must now be the real, readable message, and no request should be sent.
func TestCreateProcessFromWithoutFileReturnsError(t *testing.T) {
	withCommentEditOptions(t, &createOptions, func() {
		createOptions.PullRequestID.Value = "42"
		createOptions.Comment = "fix this"
		createOptions.From = 10
	})

	var requestCount int
	cmd := setupTest(t, func(http.ResponseWriter, *http.Request) { requestCount++ }, false)

	err := createProcess(cmd, nil)
	if err == nil {
		t.Fatal("createProcess() expected an error, got nil")
	}
	if !strings.Contains(err.Error(), "cannot specify from/to without a file") {
		t.Errorf("error = %q, want it to contain the real message instead of a blank/generic one", err.Error())
	}
	if requestCount != 0 {
		t.Errorf("expected no HTTP request when the anchor is invalid, got %d", requestCount)
	}
}

func TestCreateProcessAPIError(t *testing.T) {
	withCommentEditOptions(t, &createOptions, func() {
		createOptions.PullRequestID.Value = "42"
		createOptions.Comment = "looks good"
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
	if !strings.Contains(err.Error(), "failed to create comment") {
		t.Errorf("error = %q, want it to mention the failed create", err.Error())
	}
	if !strings.Contains(err.Error(), "pull request is not open") {
		t.Errorf("error = %q, want it to contain the BitBucket error message", err.Error())
	}
}

func TestCreateProcessDryRun(t *testing.T) {
	withCommentEditOptions(t, &createOptions, func() {
		createOptions.PullRequestID.Value = "42"
		createOptions.Comment = "looks good"
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
