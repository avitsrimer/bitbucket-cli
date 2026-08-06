package task

import (
	"encoding/json"
	"net/http"
	"strings"
	"testing"

	"github.com/avitsrimer/bitbucket-cli/internal/testutil"
)

// withUpdateOptions saves/restores the package-level updateOptions (bound to updateCmd's flags at
// init) so tests can set the values they need without leaking state across other tests.
func withUpdateOptions(t *testing.T, mutate func()) {
	t.Helper()
	oldContent := updateOptions.Content
	oldStateValue := updateOptions.State.Value
	t.Cleanup(func() {
		updateOptions.Content = oldContent
		updateOptions.State.Value = oldStateValue
	})
	mutate()
}

// updateHandler dispatches a GET (the preflight task existence check) to handleGet and any other
// method to handleWrite, decoding its body into gotBody first.
func updateHandler(requests *[]*http.Request, gotBody *TaskUpdater, handleGet, handleWrite http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		*requests = append(*requests, r)
		if r.Method == http.MethodGet {
			handleGet(w, r)
			return
		}
		_ = json.NewDecoder(r.Body).Decode(gotBody)
		handleWrite(w, r)
	}
}

// TestUpdateProcess covers updateProcess's success, API-error, preflight-error, and dry-run paths.
func TestUpdateProcess(t *testing.T) {
	tests := []struct {
		name          string
		handleGet     http.HandlerFunc
		handleWrite   http.HandlerFunc
		dryRun        bool
		wantErrSubstr []string
		validate      func(t *testing.T, requests []*http.Request, gotBody TaskUpdater, stdout string)
	}{
		{
			name:      "success",
			handleGet: okGetHandler,
			handleWrite: func(w http.ResponseWriter, _ *http.Request) {
				w.Header().Set("Content-Type", "application/json")
				_, _ = w.Write([]byte(`{"id":7,"content":{"raw":"updated content"}}`))
			},
			validate: func(t *testing.T, requests []*http.Request, gotBody TaskUpdater, stdout string) {
				t.Helper()
				if len(requests) != 2 {
					t.Fatalf("expected exactly 2 requests (preflight GET, task PUT), got %d", len(requests))
				}
				if requests[0].Method != http.MethodGet {
					t.Errorf("first request method = %s, want GET (preflight existence check)", requests[0].Method)
				}
				wantPath := "/2.0/repositories/" + testutil.FixtureRepositoryFlag + "/pullrequests/42/tasks/7"
				if requests[1].URL.Path != wantPath {
					t.Errorf("path = %s, want %s", requests[1].URL.Path, wantPath)
				}
				if requests[1].Method != http.MethodPut {
					t.Errorf("method = %s, want PUT", requests[1].Method)
				}
				if gotBody.Content == nil || gotBody.Content.Raw != "updated content" {
					t.Errorf("posted content = %+v, want %q", gotBody.Content, "updated content")
				}
				var updated Task
				if err := json.Unmarshal([]byte(stdout), &updated); err != nil {
					t.Fatalf("cannot unmarshal printed output %q: %v", stdout, err)
				}
				if updated.ID != 7 {
					t.Errorf("printed task id = %d, want 7", updated.ID)
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
			wantErrSubstr: []string{"failed to update pull request task 7 on pull request 42", "pull request is not open"},
		},
		{
			name: "preflight error: task does not exist",
			handleGet: func(w http.ResponseWriter, _ *http.Request) {
				w.Header().Set("Content-Type", "application/json")
				w.WriteHeader(http.StatusNotFound)
				_, _ = w.Write([]byte(`{"type":"error","error":{"message":"task not found"}}`))
			},
			handleWrite:   func(http.ResponseWriter, *http.Request) {},
			wantErrSubstr: []string{"failed to get task 7 of pullrequest 42", "task not found"},
			validate: func(t *testing.T, requests []*http.Request, _ TaskUpdater, _ string) {
				t.Helper()
				if len(requests) != 1 || requests[0].Method != http.MethodGet {
					t.Errorf("requests = %v, want exactly one preflight GET and no task PUT", requests)
				}
			},
		},
		{
			name:        "dry run",
			handleGet:   okGetHandler,
			handleWrite: func(http.ResponseWriter, *http.Request) {},
			dryRun:      true,
			validate: func(t *testing.T, requests []*http.Request, _ TaskUpdater, _ string) {
				t.Helper()
				if len(requests) != 1 || requests[0].Method != http.MethodGet {
					t.Errorf("requests = %v, want exactly one preflight GET and no task PUT in dry-run mode", requests)
				}
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			runTaskUpdateProcessCase(t, tt.handleGet, tt.handleWrite, tt.dryRun, tt.wantErrSubstr, tt.validate)
		})
	}
}

// runTaskUpdateProcessCase drives one TestUpdateProcess table entry: sets up updateOptions and the
// fixture server, runs updateProcess, and asserts either the expected error substrings or success,
// then hands requests/gotBody/stdout to validate (when given) either way.
func runTaskUpdateProcessCase(t *testing.T, handleGet, handleWrite http.HandlerFunc, dryRun bool, wantErrSubstr []string, validate func(t *testing.T, requests []*http.Request, gotBody TaskUpdater, stdout string)) {
	t.Helper()
	withUpdateOptions(t, func() {
		updateOptions.Content = "updated content"
		updateOptions.State.Value = ""
	})

	var requests []*http.Request
	var gotBody TaskUpdater
	cmd := setupTest(t, updateHandler(&requests, &gotBody, handleGet, handleWrite), dryRun)

	var err error
	stdout := testutil.CaptureStdout(t, func() {
		err = updateProcess(cmd, []string{"42", "7"})
	})

	if len(wantErrSubstr) > 0 {
		if err == nil {
			t.Fatal("updateProcess() expected an error, got nil")
		}
		for _, substr := range wantErrSubstr {
			if !strings.Contains(err.Error(), substr) {
				t.Errorf("error = %q, want it to contain %q", err.Error(), substr)
			}
		}
	} else if err != nil {
		t.Fatalf("updateProcess() error = %v", err)
	}
	if validate != nil {
		validate(t, requests, gotBody, stdout)
	}
}
