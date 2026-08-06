package comment

import (
	"encoding/json"
	"net/http"
	"strings"
	"testing"

	"github.com/avitsrimer/bitbucket-cli/internal/testutil"
)

// TestUpdateProcess covers updateProcess's success, API-error, and dry-run paths.
func TestUpdateProcess(t *testing.T) {
	tests := []struct {
		name          string
		handler       http.HandlerFunc
		dryRun        bool
		wantErrSubstr []string
		validate      func(t *testing.T, requests []*http.Request, gotBody CommentPayload, stdout string)
	}{
		{
			name: "success",
			handler: func(w http.ResponseWriter, _ *http.Request) {
				w.Header().Set("Content-Type", "application/json")
				_, _ = w.Write([]byte(`{"id":7,"content":{"raw":"updated comment"}}`))
			},
			validate: func(t *testing.T, requests []*http.Request, gotBody CommentPayload, stdout string) {
				t.Helper()
				if len(requests) != 1 {
					t.Fatalf("expected exactly 1 request, got %d", len(requests))
				}
				wantPath := "/2.0/repositories/" + testutil.FixtureRepositoryFlag + "/pullrequests/42/comments/7"
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
			},
		},
		{
			name: "api error",
			handler: func(w http.ResponseWriter, _ *http.Request) {
				w.Header().Set("Content-Type", "application/json")
				w.WriteHeader(http.StatusNotFound)
				_, _ = w.Write([]byte(`{"type":"error","error":{"message":"comment not found"}}`))
			},
			wantErrSubstr: []string{"failed to update comment", "comment not found"},
		},
		{
			name:    "dry run",
			handler: func(http.ResponseWriter, *http.Request) {},
			dryRun:  true,
			validate: func(t *testing.T, requests []*http.Request, _ CommentPayload, _ string) {
				t.Helper()
				if len(requests) != 0 {
					t.Errorf("expected no HTTP request in dry-run mode, got %d", len(requests))
				}
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			withCommentEditOptions(t, &updateOptions, func() {
				updateOptions.PullRequestID.Value = "42"
				updateOptions.Comment = "updated comment"
			})

			var requests []*http.Request
			var gotBody CommentPayload
			cmd := setupTest(t, func(w http.ResponseWriter, r *http.Request) {
				requests = append(requests, r)
				_ = json.NewDecoder(r.Body).Decode(&gotBody)
				tt.handler(w, r)
			}, tt.dryRun)

			var err error
			stdout := testutil.CaptureStdout(t, func() {
				err = updateProcess(cmd, []string{"7"})
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

// TestUpdateProcessToWithoutFileReturnsError mirrors the create-side check: --to without --file
// produces the real, readable error message, not a blank one, and sends no request.
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
