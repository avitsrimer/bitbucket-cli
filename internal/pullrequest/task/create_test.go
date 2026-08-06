package task

import (
	"encoding/json"
	"net/http"
	"strings"
	"testing"

	"github.com/avitsrimer/bitbucket-cli/internal/testutil"
)

func withCreateOptions(t *testing.T, mutate func()) {
	t.Helper()
	oldContent := createOptions.Content
	oldCommentIDValue := createOptions.CommentID.Value
	oldPending := createOptions.Pending
	t.Cleanup(func() {
		createOptions.Content = oldContent
		createOptions.CommentID.Value = oldCommentIDValue
		createOptions.Pending = oldPending
	})
	mutate()
}

// TestCreateProcess covers createProcess's success, API-error, and dry-run paths.
func TestCreateProcess(t *testing.T) {
	tests := []struct {
		name          string
		handler       http.HandlerFunc
		dryRun        bool
		wantErrSubstr []string
		validate      func(t *testing.T, requests []*http.Request, gotBody TaskCreator, stdout string)
	}{
		{
			name: "success",
			handler: func(w http.ResponseWriter, _ *http.Request) {
				w.Header().Set("Content-Type", "application/json")
				_, _ = w.Write([]byte(`{"id":99,"content":{"raw":"please fix"}}`))
			},
			validate: func(t *testing.T, requests []*http.Request, gotBody TaskCreator, stdout string) {
				t.Helper()
				if len(requests) != 1 {
					t.Fatalf("expected exactly 1 request, got %d", len(requests))
				}
				wantPath := "/2.0/repositories/" + testutil.FixtureRepositoryFlag + "/pullrequests/42/tasks"
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
			},
		},
		{
			name: "api error",
			handler: func(w http.ResponseWriter, _ *http.Request) {
				w.Header().Set("Content-Type", "application/json")
				w.WriteHeader(http.StatusBadRequest)
				_, _ = w.Write([]byte(`{"type":"error","error":{"message":"pull request is not open"}}`))
			},
			wantErrSubstr: []string{"failed to create pull request task on pull request 42", "pull request is not open"},
		},
		{
			name:    "dry run",
			handler: func(http.ResponseWriter, *http.Request) {},
			dryRun:  true,
			validate: func(t *testing.T, requests []*http.Request, _ TaskCreator, _ string) {
				t.Helper()
				if len(requests) != 0 {
					t.Errorf("expected no HTTP request in dry-run mode, got %d", len(requests))
				}
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			withCreateOptions(t, func() {
				createOptions.Content = "please fix"
				createOptions.CommentID.Value = ""
				createOptions.Pending = false
			})

			var requests []*http.Request
			var gotBody TaskCreator
			cmd := setupTest(t, func(w http.ResponseWriter, r *http.Request) {
				requests = append(requests, r)
				_ = json.NewDecoder(r.Body).Decode(&gotBody)
				tt.handler(w, r)
			}, tt.dryRun)

			var err error
			stdout := testutil.CaptureStdout(t, func() {
				err = createProcess(cmd, []string{"42"})
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

func TestCreateProcessWithComment(t *testing.T) {
	withCreateOptions(t, func() {
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

	testutil.CaptureStdout(t, func() {
		if err := createProcess(cmd, []string{"42"}); err != nil {
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
		createOptions.Content = "please fix"
		createOptions.CommentID.Value = "not-a-number"
	})

	var requestCount int
	cmd := setupTest(t, func(http.ResponseWriter, *http.Request) { requestCount++ }, false)

	err := createProcess(cmd, []string{"42"})
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
