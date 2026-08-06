package comment

import (
	"encoding/json"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/avitsrimer/bitbucket-cli/internal/testutil"
)

// TestUpdateProcess covers updateProcess's success, API-error, and dry-run paths.
func TestUpdateProcess(t *testing.T) {
	tests := []struct {
		name          string
		handleGet     http.HandlerFunc
		handleWrite   http.HandlerFunc
		dryRun        bool
		wantErrSubstr []string
		validate      func(t *testing.T, requests []*http.Request, gotBody CommentPayload, stdout string)
	}{
		{
			name:      "success",
			handleGet: okGetHandler,
			handleWrite: func(w http.ResponseWriter, _ *http.Request) {
				w.Header().Set("Content-Type", "application/json")
				_, _ = w.Write([]byte(`{"id":7,"content":{"raw":"updated comment"}}`))
			},
			validate: func(t *testing.T, requests []*http.Request, gotBody CommentPayload, stdout string) {
				t.Helper()
				if len(requests) != 2 {
					t.Fatalf("expected exactly 2 requests (preflight GET, comment PUT), got %d", len(requests))
				}
				if requests[0].Method != http.MethodGet {
					t.Errorf("first request method = %s, want GET (preflight existence check)", requests[0].Method)
				}
				wantPath := "/2.0/repositories/" + testutil.FixtureRepositoryFlag + "/pullrequests/42/comments/7"
				if requests[1].URL.Path != wantPath {
					t.Errorf("path = %s, want %s", requests[1].URL.Path, wantPath)
				}
				if requests[1].Method != http.MethodPut {
					t.Errorf("method = %s, want PUT", requests[1].Method)
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
			},
		},
		{
			name:      "api error",
			handleGet: okGetHandler,
			handleWrite: func(w http.ResponseWriter, _ *http.Request) {
				w.Header().Set("Content-Type", "application/json")
				w.WriteHeader(http.StatusBadRequest)
				_, _ = w.Write([]byte(`{"type":"error","error":{"message":"pull request is not open"}}`))
			},
			wantErrSubstr: []string{"failed to update comment", "pull request is not open"},
		},
		{
			name: "preflight error: comment does not exist",
			handleGet: func(w http.ResponseWriter, _ *http.Request) {
				w.Header().Set("Content-Type", "application/json")
				w.WriteHeader(http.StatusNotFound)
				_, _ = w.Write([]byte(`{"type":"error","error":{"message":"comment not found"}}`))
			},
			handleWrite:   func(http.ResponseWriter, *http.Request) {},
			wantErrSubstr: []string{"cannot update comment", "failed to get comment 7 of pullrequest 42", "comment not found"},
			validate: func(t *testing.T, requests []*http.Request, _ CommentPayload, _ string) {
				t.Helper()
				if len(requests) != 1 || requests[0].Method != http.MethodGet {
					t.Errorf("requests = %v, want exactly one preflight GET and no comment PUT", requests)
				}
			},
		},
		{
			name:        "dry run",
			handleGet:   okGetHandler,
			handleWrite: func(http.ResponseWriter, *http.Request) {},
			dryRun:      true,
			validate: func(t *testing.T, requests []*http.Request, _ CommentPayload, _ string) {
				t.Helper()
				if len(requests) != 1 || requests[0].Method != http.MethodGet {
					t.Errorf("requests = %v, want exactly one preflight GET and no comment PUT in dry-run mode", requests)
				}
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			withCommentEditOptions(t, &updateOptions, func() {
				updateOptions.Comment = "updated comment"
			})

			var requests []*http.Request
			var gotBody CommentPayload
			cmd := setupTest(t, createProcessHandler(t, &requests, &gotBody, tt.handleGet, tt.handleWrite), tt.dryRun)

			var err error
			stdout := testutil.CaptureStdout(t, func() {
				err = updateProcess(cmd, []string{"42", "7"})
			})

			if len(tt.wantErrSubstr) > 0 {
				if err == nil {
					t.Fatal("updateProcess() expected an error, got nil")
				}
				for _, substr := range tt.wantErrSubstr {
					if !strings.Contains(err.Error(), substr) {
						t.Errorf("error = %q, want it to contain %q", err.Error(), substr)
					}
				}
				if tt.validate != nil {
					tt.validate(t, requests, gotBody, stdout)
				}
				return
			}
			if err != nil {
				t.Fatalf("updateProcess() error = %v", err)
			}
			if tt.validate != nil {
				tt.validate(t, requests, gotBody, stdout)
			}
		})
	}
}

// TestUpdateProcessEmptyCommentBodyErrors mirrors the create-side check: an empty --comment value
// fails FR-6's full preflight before any HTTP request is sent.
func TestUpdateProcessEmptyCommentBodyErrors(t *testing.T) {
	withCommentEditOptions(t, &updateOptions, func() {
		updateOptions.Comment = "   "
	})

	var requestCount int
	cmd := setupTest(t, func(http.ResponseWriter, *http.Request) { requestCount++ }, false)

	err := updateProcess(cmd, []string{"42", "7"})
	if err == nil {
		t.Fatal("updateProcess() expected an error, got nil")
	}
	if !strings.Contains(err.Error(), "comment body is empty") {
		t.Errorf("error = %q, want it to mention the empty comment body", err.Error())
	}
	if requestCount != 0 {
		t.Errorf("expected no HTTP request for an empty comment body, got %d", requestCount)
	}
}

// TestUpdateProcessToWithoutFileReturnsError mirrors the create-side check: --to without --file
// produces the real, readable error message, not a blank one, and sends no request.
func TestUpdateProcessToWithoutFileReturnsError(t *testing.T) {
	withCommentEditOptions(t, &updateOptions, func() {
		updateOptions.Comment = "updated comment"
		updateOptions.To = 5
	})

	var requestCount int
	cmd := setupTest(t, func(http.ResponseWriter, *http.Request) { requestCount++ }, false)

	err := updateProcess(cmd, []string{"42", "7"})
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

// TestUpdateProcessCommentFromFileVerbatim mirrors the create-side check: --comment-file's
// content lands in the PUT body verbatim, including backticks and $().
func TestUpdateProcessCommentFromFileVerbatim(t *testing.T) {
	body := "Actually, run `go vet ./...` too, and check $(git status) before merging.\n"
	path := filepath.Join(t.TempDir(), "comment.md")
	if err := os.WriteFile(path, []byte(body), 0o600); err != nil {
		t.Fatalf("cannot write fixture file: %v", err)
	}

	withCommentEditOptions(t, &updateOptions, func() {
		updateOptions.Comment = ""
		updateOptions.CommentFile = path
	})

	var gotBody CommentPayload
	cmd := setupTest(t, createProcessHandler(t, &[]*http.Request{}, &gotBody, okGetHandler, func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"id":7}`))
	}), false)

	testutil.CaptureStdout(t, func() {
		if err := updateProcess(cmd, []string{"42", "7"}); err != nil {
			t.Fatalf("updateProcess() error = %v", err)
		}
	})

	if gotBody.Content.Raw != body {
		t.Errorf("posted content.raw = %q, want %q (verbatim)", gotBody.Content.Raw, body)
	}
}

// TestUpdateProcessEmptyCommentFileBodyErrors mirrors the create-side check: an empty
// --comment-file body fails the same preflight check as an empty --comment value.
func TestUpdateProcessEmptyCommentFileBodyErrors(t *testing.T) {
	path := filepath.Join(t.TempDir(), "empty.md")
	if err := os.WriteFile(path, []byte("\n"), 0o600); err != nil {
		t.Fatalf("cannot write fixture file: %v", err)
	}

	withCommentEditOptions(t, &updateOptions, func() {
		updateOptions.Comment = ""
		updateOptions.CommentFile = path
	})

	var requestCount int
	cmd := setupTest(t, func(http.ResponseWriter, *http.Request) { requestCount++ }, false)

	err := updateProcess(cmd, []string{"42", "7"})
	if err == nil {
		t.Fatal("updateProcess() expected an error, got nil")
	}
	if !strings.Contains(err.Error(), "comment body is empty") {
		t.Errorf("error = %q, want it to mention the empty comment body", err.Error())
	}
	if requestCount != 0 {
		t.Errorf("expected no HTTP request for an empty comment-file body, got %d", requestCount)
	}
}
