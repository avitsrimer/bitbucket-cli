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

// createProcessHandler wraps handler so a GET (the preflight pull request existence check) always
// succeeds with a minimal pullrequest body, leaving handler to see only the actual write request --
// unless handler itself wants to special-case the GET (e.g. to simulate a nonexistent pull
// request), in which case pass handleGet.
func createProcessHandler(t *testing.T, requests *[]*http.Request, gotBody *CommentPayload, handleGet, handleWrite http.HandlerFunc) http.HandlerFunc {
	t.Helper()
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
		validate      func(t *testing.T, requests []*http.Request, gotBody CommentPayload, stdout string)
	}{
		{
			name:      "success",
			handleGet: okGetHandler,
			handleWrite: func(w http.ResponseWriter, _ *http.Request) {
				w.Header().Set("Content-Type", "application/json")
				_, _ = w.Write([]byte(`{"id":1,"content":{"raw":"looks good"}}`))
			},
			validate: func(t *testing.T, requests []*http.Request, gotBody CommentPayload, stdout string) {
				t.Helper()
				if len(requests) != 2 {
					t.Fatalf("expected exactly 2 requests (preflight GET, comment POST), got %d", len(requests))
				}
				if requests[0].Method != http.MethodGet {
					t.Errorf("first request method = %s, want GET (preflight existence check)", requests[0].Method)
				}
				wantPath := "/2.0/repositories/" + testutil.FixtureRepositoryFlag + "/pullrequests/42/comments"
				if requests[1].URL.Path != wantPath {
					t.Errorf("path = %s, want %s", requests[1].URL.Path, wantPath)
				}
				if requests[1].Method != http.MethodPost {
					t.Errorf("method = %s, want POST", requests[1].Method)
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
			name:      "api error",
			handleGet: okGetHandler,
			handleWrite: func(w http.ResponseWriter, _ *http.Request) {
				w.Header().Set("Content-Type", "application/json")
				w.WriteHeader(http.StatusBadRequest)
				_, _ = w.Write([]byte(`{"type":"error","error":{"message":"pull request is not open"}}`))
			},
			wantErrSubstr: []string{"failed to create comment", "pull request is not open"},
		},
		{
			name: "preflight error: pull request does not exist",
			handleGet: func(w http.ResponseWriter, _ *http.Request) {
				w.Header().Set("Content-Type", "application/json")
				w.WriteHeader(http.StatusNotFound)
				_, _ = w.Write([]byte(`{"type":"error","error":{"message":"pull request not found"}}`))
			},
			handleWrite:   func(http.ResponseWriter, *http.Request) {},
			wantErrSubstr: []string{"cannot create comment", "failed to get pullrequest 42", "pull request not found"},
			validate: func(t *testing.T, requests []*http.Request, _ CommentPayload, _ string) {
				t.Helper()
				if len(requests) != 1 || requests[0].Method != http.MethodGet {
					t.Errorf("requests = %v, want exactly one preflight GET and no comment POST", requests)
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
					t.Errorf("requests = %v, want exactly one preflight GET and no comment POST in dry-run mode", requests)
				}
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			runCreateProcessCase(t, tt.handleGet, tt.handleWrite, tt.dryRun, tt.wantErrSubstr, tt.validate)
		})
	}
}

// runCreateProcessCase drives one TestCreateProcess table entry: sets up createOptions and the
// fixture server, runs createProcess, and asserts either the expected error substrings or success,
// then hands requests/gotBody/stdout to validate (when given) either way.
func runCreateProcessCase(t *testing.T, handleGet, handleWrite http.HandlerFunc, dryRun bool, wantErrSubstr []string, validate func(t *testing.T, requests []*http.Request, gotBody CommentPayload, stdout string)) {
	t.Helper()
	withCommentEditOptions(t, &createOptions, func() {
		createOptions.Comment = "looks good"
	})

	var requests []*http.Request
	var gotBody CommentPayload
	cmd := setupTest(t, createProcessHandler(t, &requests, &gotBody, handleGet, handleWrite), dryRun)

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

func TestCreateProcessWithFileAnchor(t *testing.T) {
	withCommentEditOptions(t, &createOptions, func() {
		createOptions.Comment = "fix this"
		createOptions.File = "main.go"
	})

	var gotBody CommentPayload
	cmd := setupTest(t, func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		switch {
		case strings.HasSuffix(r.URL.Path, "/diffstat"):
			_, _ = w.Write([]byte(`{"values":[{"old":{"path":"main.go"},"new":{"path":"main.go"}}]}`))
		case r.Method == http.MethodGet:
			_, _ = w.Write([]byte(`{"id":42}`))
		default:
			if err := json.NewDecoder(r.Body).Decode(&gotBody); err != nil {
				t.Errorf("cannot decode request body: %v", err)
			}
			_, _ = w.Write([]byte(`{"id":2}`))
		}
	}, false)
	if err := cmd.Flags().Set("from", "10"); err != nil {
		t.Fatalf("cannot set --from: %v", err)
	}
	if err := cmd.Flags().Set("line", "12"); err != nil {
		t.Fatalf("cannot set --line: %v", err)
	}

	testutil.CaptureStdout(t, func() {
		if err := createProcess(cmd, []string{"42"}); err != nil {
			t.Fatalf("createProcess() error = %v", err)
		}
	})

	if gotBody.Anchor == nil {
		t.Fatal("posted anchor = nil, want the --file/--from/--line anchor to be set")
	}
	if gotBody.Anchor.Path != "main.go" || gotBody.Anchor.From != 10 || gotBody.Anchor.To != 12 {
		t.Errorf("posted anchor = %+v, want {Path:main.go From:10 To:12}", gotBody.Anchor)
	}
}

// TestCreateProcessUnknownFileAnchorErrors verifies that a --file path not present in the pull
// request's diffstat fails the full preflight (FR-6), deliberately stricter than what the write
// endpoint itself might accept, and sends no comment POST.
func TestCreateProcessUnknownFileAnchorErrors(t *testing.T) {
	withCommentEditOptions(t, &createOptions, func() {
		createOptions.Comment = "fix this"
		createOptions.File = "does-not-exist.go"
	})

	var requests []*http.Request
	cmd := setupTest(t, func(w http.ResponseWriter, r *http.Request) {
		requests = append(requests, r)
		w.Header().Set("Content-Type", "application/json")
		switch {
		case strings.HasSuffix(r.URL.Path, "/diffstat"):
			_, _ = w.Write([]byte(`{"values":[{"old":{"path":"main.go"},"new":{"path":"main.go"}}]}`))
		case r.Method == http.MethodGet:
			_, _ = w.Write([]byte(`{"id":42}`))
		default:
			t.Error("no write request expected")
		}
	}, false)

	err := createProcess(cmd, []string{"42"})
	if err == nil {
		t.Fatal("createProcess() expected an error, got nil")
	}
	if !strings.Contains(err.Error(), `file "does-not-exist.go" is not part of the diff of pullrequest 42`) {
		t.Errorf("error = %q, want it to name the unknown file anchor", err.Error())
	}
	for _, req := range requests {
		if req.Method == http.MethodPost {
			t.Errorf("unexpected POST request: %s", req.URL.Path)
		}
	}
}

// TestCreateProcessEmptyCommentBodyErrors verifies that an empty --comment value (which passes
// cobra's MarkFlagRequired check, since that only requires the flag be set) fails FR-6's full
// preflight before any HTTP request is sent.
func TestCreateProcessEmptyCommentBodyErrors(t *testing.T) {
	withCommentEditOptions(t, &createOptions, func() {
		createOptions.Comment = "   "
	})

	var requestCount int
	cmd := setupTest(t, func(http.ResponseWriter, *http.Request) { requestCount++ }, false)

	err := createProcess(cmd, []string{"42"})
	if err == nil {
		t.Fatal("createProcess() expected an error, got nil")
	}
	if !strings.Contains(err.Error(), "comment body is empty") {
		t.Errorf("error = %q, want it to mention the empty comment body", err.Error())
	}
	if requestCount != 0 {
		t.Errorf("expected no HTTP request for an empty comment body, got %d", requestCount)
	}
}

// TestCreateProcessFromWithoutFileReturnsError verifies that --from without --file produces the
// real, readable "cannot specify --line/--from without --file" error message, and sends no
// request.
func TestCreateProcessFromWithoutFileReturnsError(t *testing.T) {
	withCommentEditOptions(t, &createOptions, func() {
		createOptions.Comment = "fix this"
	})

	var requestCount int
	cmd := setupTest(t, func(http.ResponseWriter, *http.Request) { requestCount++ }, false)
	if err := cmd.Flags().Set("from", "10"); err != nil {
		t.Fatalf("cannot set --from: %v", err)
	}

	err := createProcess(cmd, []string{"42"})
	if err == nil {
		t.Fatal("createProcess() expected an error, got nil")
	}
	if !strings.Contains(err.Error(), "cannot specify --line/--from without --file") {
		t.Errorf("error = %q, want it to contain the real message instead of a blank/generic one", err.Error())
	}
	if requestCount != 0 {
		t.Errorf("expected no HTTP request when the anchor is invalid, got %d", requestCount)
	}
}

// TestCreateProcessCommentFromFileVerbatim verifies that --comment-file's content lands in the
// posted comment body verbatim, including the shell-quoting hazard class (backticks and $()) that
// --comment-file exists to route around.
func TestCreateProcessCommentFromFileVerbatim(t *testing.T) {
	body := "Looks good, but run `go test ./...` and check $(pwd) first.\n"
	path := filepath.Join(t.TempDir(), "comment.md")
	if err := os.WriteFile(path, []byte(body), 0o600); err != nil {
		t.Fatalf("cannot write fixture file: %v", err)
	}

	withCommentEditOptions(t, &createOptions, func() {
		createOptions.Comment = ""
		createOptions.CommentFile = path
	})

	var gotBody CommentPayload
	cmd := setupTest(t, func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		if r.Method == http.MethodGet {
			_, _ = w.Write([]byte(`{"id":42}`))
			return
		}
		if err := json.NewDecoder(r.Body).Decode(&gotBody); err != nil {
			t.Errorf("cannot decode request body: %v", err)
		}
		_, _ = w.Write([]byte(`{"id":1}`))
	}, false)

	testutil.CaptureStdout(t, func() {
		if err := createProcess(cmd, []string{"42"}); err != nil {
			t.Fatalf("createProcess() error = %v", err)
		}
	})

	if gotBody.Content.Raw != body {
		t.Errorf("posted content.raw = %q, want %q (verbatim)", gotBody.Content.Raw, body)
	}
}

// TestCreateProcessCommentFromStdin verifies the --comment-file - stdin variant.
func TestCreateProcessCommentFromStdin(t *testing.T) {
	body := "stdin comment with `backticks` and $(command) untouched\n"

	withCommentEditOptions(t, &createOptions, func() {
		createOptions.Comment = ""
		createOptions.CommentFile = "-"
	})

	var gotBody CommentPayload
	cmd := setupTest(t, func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		if r.Method == http.MethodGet {
			_, _ = w.Write([]byte(`{"id":42}`))
			return
		}
		if err := json.NewDecoder(r.Body).Decode(&gotBody); err != nil {
			t.Errorf("cannot decode request body: %v", err)
		}
		_, _ = w.Write([]byte(`{"id":1}`))
	}, false)
	cmd.SetIn(strings.NewReader(body))

	testutil.CaptureStdout(t, func() {
		if err := createProcess(cmd, []string{"42"}); err != nil {
			t.Fatalf("createProcess() error = %v", err)
		}
	})

	if gotBody.Content.Raw != body {
		t.Errorf("posted content.raw = %q, want %q (verbatim)", gotBody.Content.Raw, body)
	}
}

// TestCreateProcessEmptyCommentFileBodyErrors verifies that a --comment-file pointing at an empty
// (or whitespace-only) file fails the same "comment body is empty" preflight check as an empty
// --comment value, and sends no request.
func TestCreateProcessEmptyCommentFileBodyErrors(t *testing.T) {
	path := filepath.Join(t.TempDir(), "empty.md")
	if err := os.WriteFile(path, []byte("   \n"), 0o600); err != nil {
		t.Fatalf("cannot write fixture file: %v", err)
	}

	withCommentEditOptions(t, &createOptions, func() {
		createOptions.Comment = ""
		createOptions.CommentFile = path
	})

	var requestCount int
	cmd := setupTest(t, func(http.ResponseWriter, *http.Request) { requestCount++ }, false)

	err := createProcess(cmd, []string{"42"})
	if err == nil {
		t.Fatal("createProcess() expected an error, got nil")
	}
	if !strings.Contains(err.Error(), "comment body is empty") {
		t.Errorf("error = %q, want it to mention the empty comment body", err.Error())
	}
	if requestCount != 0 {
		t.Errorf("expected no HTTP request for an empty comment-file body, got %d", requestCount)
	}
}

// TestCreateProcessCommentFileNonexistentErrors verifies that a --comment-file naming a
// nonexistent path fails with an error naming that path, and sends no request.
func TestCreateProcessCommentFileNonexistentErrors(t *testing.T) {
	path := filepath.Join(t.TempDir(), "does-not-exist.md")

	withCommentEditOptions(t, &createOptions, func() {
		createOptions.Comment = ""
		createOptions.CommentFile = path
	})

	var requestCount int
	cmd := setupTest(t, func(http.ResponseWriter, *http.Request) { requestCount++ }, false)

	err := createProcess(cmd, []string{"42"})
	if err == nil {
		t.Fatal("createProcess() expected an error, got nil")
	}
	if !strings.Contains(err.Error(), path) {
		t.Errorf("error = %q, want it to name the path %q", err.Error(), path)
	}
	if requestCount != 0 {
		t.Errorf("expected no HTTP request for an unreadable comment-file, got %d", requestCount)
	}
}
