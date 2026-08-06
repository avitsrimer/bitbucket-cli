package task

import (
	"encoding/json"
	"net/http"
	"strings"
	"testing"

	"github.com/avitsrimer/bitbucket-cli/internal/testutil"
)

func TestGetProcess(t *testing.T) {
	tests := []struct {
		name          string
		handler       http.HandlerFunc
		dryRun        bool
		wantErrSubstr []string
		validate      func(t *testing.T, requests []*http.Request, stdout string)
	}{
		{
			name: "success",
			handler: func(w http.ResponseWriter, _ *http.Request) {
				w.Header().Set("Content-Type", "application/json")
				_, _ = w.Write([]byte(`{"id":7,"content":{"raw":"please fix"},"state":"RESOLVED"}`))
			},
			validate: func(t *testing.T, requests []*http.Request, stdout string) {
				t.Helper()
				if len(requests) != 1 {
					t.Fatalf("expected exactly 1 request, got %d", len(requests))
				}
				wantPath := "/2.0/repositories/" + testutil.FixtureRepositoryFlag + "/pullrequests/42/tasks/7"
				if requests[0].URL.Path != wantPath {
					t.Errorf("path = %s, want %s", requests[0].URL.Path, wantPath)
				}
				if requests[0].Method != http.MethodGet {
					t.Errorf("method = %s, want GET", requests[0].Method)
				}
				var got Task
				if err := json.Unmarshal([]byte(stdout), &got); err != nil {
					t.Fatalf("cannot unmarshal printed output %q: %v", stdout, err)
				}
				if got.Content.Raw != "please fix" {
					t.Errorf("printed task content = %q, want %q", got.Content.Raw, "please fix")
				}
			},
		},
		{
			name: "api error",
			handler: func(w http.ResponseWriter, _ *http.Request) {
				w.Header().Set("Content-Type", "application/json")
				w.WriteHeader(http.StatusNotFound)
				_, _ = w.Write([]byte(`{"type":"error","error":{"message":"task not found"}}`))
			},
			wantErrSubstr: []string{"failed to get pullrequest task 7", "task not found"},
		},
		{
			name:    "dry run",
			handler: func(http.ResponseWriter, *http.Request) {},
			dryRun:  true,
			validate: func(t *testing.T, requests []*http.Request, _ string) {
				t.Helper()
				if len(requests) != 0 {
					t.Errorf("expected no HTTP request in dry-run mode, got %d", len(requests))
				}
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var requests []*http.Request
			cmd := setupTest(t, func(w http.ResponseWriter, r *http.Request) {
				requests = append(requests, r)
				tt.handler(w, r)
			}, tt.dryRun)

			var err error
			stdout := testutil.CaptureStdout(t, func() {
				err = getProcess(cmd, []string{"42", "7"})
			})

			if len(tt.wantErrSubstr) > 0 {
				if err == nil {
					t.Fatal("getProcess() expected an error, got nil")
				}
				for _, substr := range tt.wantErrSubstr {
					if !strings.Contains(err.Error(), substr) {
						t.Errorf("error = %q, want it to contain %q", err.Error(), substr)
					}
				}
				return
			}
			if err != nil {
				t.Fatalf("getProcess() error = %v", err)
			}
			if tt.validate != nil {
				tt.validate(t, requests, stdout)
			}
		})
	}
}
