package comment

import (
	"encoding/json"
	"net/http"
	"strings"
	"testing"

	"github.com/avitsrimer/bitbucket-cli/internal/testutil"
)

// TestCreateProcess covers createProcess's success, API-error, and dry-run paths.
func TestCreateProcess(t *testing.T) {
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
				_, _ = w.Write([]byte(`{"id":1,"content":{"raw":"looks good"}}`))
			},
			validate: func(t *testing.T, requests []*http.Request, gotBody CommentPayload, stdout string) {
				t.Helper()
				if len(requests) != 1 {
					t.Fatalf("expected exactly 1 request, got %d", len(requests))
				}
				wantPath := "/2.0/repositories/" + testutil.FixtureRepositoryFlag + "/pullrequests/42/comments"
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
			},
		},
		{
			name: "api error",
			handler: func(w http.ResponseWriter, _ *http.Request) {
				w.Header().Set("Content-Type", "application/json")
				w.WriteHeader(http.StatusBadRequest)
				_, _ = w.Write([]byte(`{"type":"error","error":{"message":"pull request is not open"}}`))
			},
			wantErrSubstr: []string{"failed to create comment", "pull request is not open"},
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
			withCommentEditOptions(t, &createOptions, func() {
				createOptions.PullRequestID.Value = "42"
				createOptions.Comment = "looks good"
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
				err = createProcess(cmd, nil)
			})

			if len(tt.wantErrSubstr) > 0 {
				if err == nil {
					t.Fatal("createProcess() expected an error, got nil")
				}
				for _, substr := range tt.wantErrSubstr {
					if !strings.Contains(err.Error(), substr) {
						t.Errorf("error = %q, want it to contain %q", err.Error(), substr)
					}
				}
				return
			}
			if err != nil {
				t.Fatalf("createProcess() error = %v", err)
			}
			if tt.validate != nil {
				tt.validate(t, requests, gotBody, stdout)
			}
		})
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

	var gotBody CommentPayload
	cmd := setupTest(t, func(w http.ResponseWriter, r *http.Request) {
		if err := json.NewDecoder(r.Body).Decode(&gotBody); err != nil {
			t.Errorf("cannot decode request body: %v", err)
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"id":2}`))
	}, false)

	testutil.CaptureStdout(t, func() {
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

// TestCreateProcessFromWithoutFileReturnsError verifies that --from/--to without --file produces
// the real, readable "cannot specify from/to without a file" error message, and sends no request.
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
