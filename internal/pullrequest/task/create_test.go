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

// taskCreateHandler wraps handleGet/handleWrite the same way comment package's
// createProcessHandler does: a GET (the preflight pull request existence check) is dispatched to
// handleGet, and any other method to handleWrite, decoding its body into gotBody first.
func taskCreateHandler(requests *[]*http.Request, gotBody *TaskCreator, handleGet, handleWrite http.HandlerFunc) http.HandlerFunc {
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

func okGetHandler(w http.ResponseWriter, _ *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	_, _ = w.Write([]byte(`{"id":42}`))
}

// TestCreateProcess covers createProcess's success, API-error, and dry-run paths.
func TestCreateProcess(t *testing.T) {
	tests := []struct {
		name          string
		handleGet     http.HandlerFunc
		handleWrite   http.HandlerFunc
		dryRun        bool
		wantErrSubstr []string
		validate      func(t *testing.T, requests []*http.Request, gotBody TaskCreator, stdout string)
	}{
		{
			name:      "success",
			handleGet: okGetHandler,
			handleWrite: func(w http.ResponseWriter, _ *http.Request) {
				w.Header().Set("Content-Type", "application/json")
				_, _ = w.Write([]byte(`{"id":99,"content":{"raw":"please fix"}}`))
			},
			validate: func(t *testing.T, requests []*http.Request, gotBody TaskCreator, stdout string) {
				t.Helper()
				if len(requests) != 2 {
					t.Fatalf("expected exactly 2 requests (preflight GET, task POST), got %d", len(requests))
				}
				if requests[0].Method != http.MethodGet {
					t.Errorf("first request method = %s, want GET (preflight existence check)", requests[0].Method)
				}
				wantPath := "/2.0/repositories/" + testutil.FixtureRepositoryFlag + "/pullrequests/42/tasks"
				if requests[1].URL.Path != wantPath {
					t.Errorf("path = %s, want %s", requests[1].URL.Path, wantPath)
				}
				if requests[1].Method != http.MethodPost {
					t.Errorf("method = %s, want POST", requests[1].Method)
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
			name:      "api error",
			handleGet: okGetHandler,
			handleWrite: func(w http.ResponseWriter, _ *http.Request) {
				w.Header().Set("Content-Type", "application/json")
				w.WriteHeader(http.StatusBadRequest)
				_, _ = w.Write([]byte(`{"type":"error","error":{"message":"pull request is not open"}}`))
			},
			wantErrSubstr: []string{"failed to create pull request task on pull request 42", "pull request is not open"},
		},
		{
			name: "preflight error: pull request does not exist",
			handleGet: func(w http.ResponseWriter, _ *http.Request) {
				w.Header().Set("Content-Type", "application/json")
				w.WriteHeader(http.StatusNotFound)
				_, _ = w.Write([]byte(`{"type":"error","error":{"message":"pull request not found"}}`))
			},
			handleWrite:   func(http.ResponseWriter, *http.Request) {},
			wantErrSubstr: []string{"cannot create task", "failed to get pullrequest 42", "pull request not found"},
			validate: func(t *testing.T, requests []*http.Request, _ TaskCreator, _ string) {
				t.Helper()
				if len(requests) != 1 || requests[0].Method != http.MethodGet {
					t.Errorf("requests = %v, want exactly one preflight GET and no task POST", requests)
				}
			},
		},
		{
			name:        "dry run",
			handleGet:   okGetHandler,
			handleWrite: func(http.ResponseWriter, *http.Request) {},
			dryRun:      true,
			validate: func(t *testing.T, requests []*http.Request, _ TaskCreator, _ string) {
				t.Helper()
				if len(requests) != 1 || requests[0].Method != http.MethodGet {
					t.Errorf("requests = %v, want exactly one preflight GET and no task POST in dry-run mode", requests)
				}
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			runTaskCreateProcessCase(t, tt.handleGet, tt.handleWrite, tt.dryRun, tt.wantErrSubstr, tt.validate)
		})
	}
}

// runTaskCreateProcessCase drives one TestCreateProcess table entry: sets up createOptions and the
// fixture server, runs createProcess, and asserts either the expected error substrings or success,
// then hands requests/gotBody/stdout to validate (when given) either way.
func runTaskCreateProcessCase(t *testing.T, handleGet, handleWrite http.HandlerFunc, dryRun bool, wantErrSubstr []string, validate func(t *testing.T, requests []*http.Request, gotBody TaskCreator, stdout string)) {
	t.Helper()
	withCreateOptions(t, func() {
		createOptions.Content = "please fix"
		createOptions.CommentID.Value = ""
		createOptions.Pending = false
	})

	var requests []*http.Request
	var gotBody TaskCreator
	cmd := setupTest(t, taskCreateHandler(&requests, &gotBody, handleGet, handleWrite), dryRun)

	var err error
	stdout := testutil.CaptureStdout(t, func() {
		err = createProcess(cmd, []string{"42"})
	})

	if len(wantErrSubstr) > 0 {
		if err == nil {
			t.Fatal("createProcess() expected an error, got nil")
		}
		for _, substr := range wantErrSubstr {
			if !strings.Contains(err.Error(), substr) {
				t.Errorf("error = %q, want it to contain %q", err.Error(), substr)
			}
		}
	} else if err != nil {
		t.Fatalf("createProcess() error = %v", err)
	}
	if validate != nil {
		validate(t, requests, gotBody, stdout)
	}
}

// TestCreateProcessEmptyContentErrors verifies that an empty --content value (which passes
// cobra's MarkFlagRequired check, since that only requires the flag be set) fails FR-6's full
// preflight before any HTTP request is sent.
func TestCreateProcessEmptyContentErrors(t *testing.T) {
	withCreateOptions(t, func() {
		createOptions.Content = "   "
		createOptions.CommentID.Value = ""
		createOptions.Pending = false
	})

	var requestCount int
	cmd := setupTest(t, func(http.ResponseWriter, *http.Request) { requestCount++ }, false)

	err := createProcess(cmd, []string{"42"})
	if err == nil {
		t.Fatal("createProcess() expected an error, got nil")
	}
	if !strings.Contains(err.Error(), "task content is empty") {
		t.Errorf("error = %q, want it to mention the empty task content", err.Error())
	}
	if requestCount != 0 {
		t.Errorf("expected no HTTP request for empty task content, got %d", requestCount)
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
