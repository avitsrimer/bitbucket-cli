package comment

import (
	"net/http"
	"strings"
	"testing"

	"github.com/avitsrimer/bitbucket-cli/internal/testutil"
)

// TestReopenProcess covers reopenProcess's success, API-error, preflight-error, and dry-run paths.
func TestReopenProcess(t *testing.T) {
	tests := []struct {
		name          string
		handleGet     http.HandlerFunc
		handleWrite   http.HandlerFunc
		dryRun        bool
		wantErrSubstr []string
		wantRequests  int
		wantMethod    string
	}{
		{
			name:         "success",
			handleGet:    okGetHandler,
			handleWrite:  func(http.ResponseWriter, *http.Request) {},
			wantRequests: 2,
			wantMethod:   http.MethodDelete,
		},
		{
			name:      "api error",
			handleGet: okGetHandler,
			handleWrite: func(w http.ResponseWriter, _ *http.Request) {
				w.Header().Set("Content-Type", "application/json")
				w.WriteHeader(http.StatusBadRequest)
				_, _ = w.Write([]byte(`{"type":"error","error":{"message":"comment is not resolved"}}`))
			},
			wantErrSubstr: []string{"failed to reopen pullrequest comment 7", "comment is not resolved"},
			wantRequests:  2,
		},
		{
			name: "preflight error: comment does not exist",
			handleGet: func(w http.ResponseWriter, _ *http.Request) {
				w.Header().Set("Content-Type", "application/json")
				w.WriteHeader(http.StatusNotFound)
				_, _ = w.Write([]byte(`{"type":"error","error":{"message":"comment not found"}}`))
			},
			handleWrite:   func(http.ResponseWriter, *http.Request) {},
			wantErrSubstr: []string{"cannot reopen comment", "failed to get comment 7 of pullrequest 42", "comment not found"},
			wantRequests:  1,
		},
		{
			name:         "dry run",
			handleGet:    okGetHandler,
			handleWrite:  func(http.ResponseWriter, *http.Request) {},
			dryRun:       true,
			wantRequests: 1,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var requests []*http.Request
			cmd := setupTest(t, func(w http.ResponseWriter, r *http.Request) {
				requests = append(requests, r)
				if r.Method == http.MethodGet {
					tt.handleGet(w, r)
					return
				}
				tt.handleWrite(w, r)
			}, tt.dryRun)

			err := reopenProcess(cmd, []string{"42", "7"})

			if len(tt.wantErrSubstr) > 0 {
				if err == nil {
					t.Fatal("reopenProcess() expected an error, got nil")
				}
				for _, substr := range tt.wantErrSubstr {
					if !strings.Contains(err.Error(), substr) {
						t.Errorf("error = %q, want it to contain %q", err.Error(), substr)
					}
				}
			} else if err != nil {
				t.Fatalf("reopenProcess() error = %v", err)
			}

			if len(requests) != tt.wantRequests {
				t.Fatalf("expected exactly %d requests, got %d", tt.wantRequests, len(requests))
			}
			if requests[0].Method != http.MethodGet {
				t.Errorf("first request method = %s, want GET (preflight existence check)", requests[0].Method)
			}
			if tt.wantMethod != "" {
				last := requests[len(requests)-1]
				if last.Method != tt.wantMethod {
					t.Errorf("last request method = %s, want %s", last.Method, tt.wantMethod)
				}
				wantPath := "/2.0/repositories/" + testutil.FixtureRepositoryFlag + "/pullrequests/42/comments/7/resolve"
				if last.URL.Path != wantPath {
					t.Errorf("path = %s, want %s", last.URL.Path, wantPath)
				}
			}
		})
	}
}

// TestResolveProcess covers resolveProcess's success, API-error, preflight-error, and dry-run
// paths.
func TestResolveProcess(t *testing.T) {
	tests := []struct {
		name          string
		handleGet     http.HandlerFunc
		handleWrite   http.HandlerFunc
		dryRun        bool
		wantErrSubstr []string
		wantRequests  int
		wantMethod    string
	}{
		{
			name:         "success",
			handleGet:    okGetHandler,
			handleWrite:  func(http.ResponseWriter, *http.Request) {},
			wantRequests: 2,
			wantMethod:   http.MethodPost,
		},
		{
			name:      "api error",
			handleGet: okGetHandler,
			handleWrite: func(w http.ResponseWriter, _ *http.Request) {
				w.Header().Set("Content-Type", "application/json")
				w.WriteHeader(http.StatusBadRequest)
				_, _ = w.Write([]byte(`{"type":"error","error":{"message":"comment already resolved"}}`))
			},
			wantErrSubstr: []string{"failed to resolve pullrequest comment 7", "comment already resolved"},
			wantRequests:  2,
		},
		{
			name: "preflight error: comment does not exist",
			handleGet: func(w http.ResponseWriter, _ *http.Request) {
				w.Header().Set("Content-Type", "application/json")
				w.WriteHeader(http.StatusNotFound)
				_, _ = w.Write([]byte(`{"type":"error","error":{"message":"comment not found"}}`))
			},
			handleWrite:   func(http.ResponseWriter, *http.Request) {},
			wantErrSubstr: []string{"cannot resolve comment", "failed to get comment 7 of pullrequest 42", "comment not found"},
			wantRequests:  1,
		},
		{
			name:         "dry run",
			handleGet:    okGetHandler,
			handleWrite:  func(http.ResponseWriter, *http.Request) {},
			dryRun:       true,
			wantRequests: 1,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var requests []*http.Request
			cmd := setupTest(t, func(w http.ResponseWriter, r *http.Request) {
				requests = append(requests, r)
				if r.Method == http.MethodGet {
					tt.handleGet(w, r)
					return
				}
				tt.handleWrite(w, r)
			}, tt.dryRun)

			err := resolveProcess(cmd, []string{"42", "7"})

			if len(tt.wantErrSubstr) > 0 {
				if err == nil {
					t.Fatal("resolveProcess() expected an error, got nil")
				}
				for _, substr := range tt.wantErrSubstr {
					if !strings.Contains(err.Error(), substr) {
						t.Errorf("error = %q, want it to contain %q", err.Error(), substr)
					}
				}
			} else if err != nil {
				t.Fatalf("resolveProcess() error = %v", err)
			}

			if len(requests) != tt.wantRequests {
				t.Fatalf("expected exactly %d requests, got %d", tt.wantRequests, len(requests))
			}
			if requests[0].Method != http.MethodGet {
				t.Errorf("first request method = %s, want GET (preflight existence check)", requests[0].Method)
			}
			if tt.wantMethod != "" {
				last := requests[len(requests)-1]
				if last.Method != tt.wantMethod {
					t.Errorf("last request method = %s, want %s", last.Method, tt.wantMethod)
				}
				wantPath := "/2.0/repositories/" + testutil.FixtureRepositoryFlag + "/pullrequests/42/comments/7/resolve"
				if last.URL.Path != wantPath {
					t.Errorf("path = %s, want %s", last.URL.Path, wantPath)
				}
			}
		})
	}
}
