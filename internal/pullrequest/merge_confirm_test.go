package pullrequest

import (
	"errors"
	"io"
	"net/http"
	"os"
	"strings"
	"testing"

	"github.com/avitsrimer/bitbucket-cli/internal/testutil"
)

// swapStdinToNonInteractivePipe temporarily replaces the package-level os.Stdin with the read end
// of a fresh pipe (whose write end is never closed or written to), restoring the original on
// cleanup. A pipe's read end reports as a named pipe (ModeCharDevice unset), reliably simulating a
// redirected, non-interactive stdin regardless of the ambient test environment's own stdin; a test
// that reads past the point it should have short-circuited hangs instead of failing silently.
func swapStdinToNonInteractivePipe(t *testing.T) {
	t.Helper()
	r, w, err := os.Pipe()
	if err != nil {
		t.Fatalf("cannot create pipe: %v", err)
	}
	t.Cleanup(func() { _ = w.Close() })
	original := os.Stdin
	os.Stdin = r
	t.Cleanup(func() { os.Stdin = original })
}

// poisonStdin fails the test immediately if it is ever read from, proving --dry-run skipped the
// confirmation prompt entirely instead of merely happening to decline it.
type poisonStdin struct{ t *testing.T }

func (p poisonStdin) Read([]byte) (int, error) {
	p.t.Helper()
	p.t.Fatal("read from stdin despite --dry-run")
	return 0, io.EOF
}

// errPoisonStdinRead simulates a non-EOF read failure, e.g. EIO after the process loses its
// controlling terminal mid-prompt.
var errPoisonStdinRead = errors.New("simulated read failure")

// poisonErrStdin always fails with errPoisonStdinRead instead of returning any input or io.EOF.
type poisonErrStdin struct{}

func (poisonErrStdin) Read([]byte) (int, error) {
	return 0, errPoisonStdinRead
}

// mergeConfirmHandler answers both requests mergeProcess issues when no positional pullrequest-id
// is given: the open-pullrequests listing (?state=OPEN) and the preflight/merge GETs and POST
// against pullrequests/42, all resolving to id 42. requests records every request seen, in order.
func mergeConfirmHandler(requests *[]*http.Request) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		*requests = append(*requests, r)
		w.Header().Set("Content-Type", "application/json")
		if r.URL.Query().Get("state") == "OPEN" {
			_, _ = w.Write([]byte(`{"values":[{"id":42}]}`))
			return
		}
		_, _ = w.Write([]byte(`{"id":42,"title":"Add feature"}`))
	}
}

func TestMergeProcessConfirmProceeds(t *testing.T) {
	for _, answer := range []string{"y\n", "yes\n"} {
		t.Run(testutil.DescribeArg(strings.TrimSuffix(answer, "\n")), func(t *testing.T) {
			withMergeOptions(t, func() {
				mergeOptions.Async = false
				mergeOptions.Message = ""
				mergeOptions.CloseSourceBranch = false
			})

			var requests []*http.Request
			cmd := setupTest(t, mergeConfirmHandler(&requests), false)
			cmd.SetIn(strings.NewReader(answer))

			if err := mergeProcess(cmd, []string{"42"}); err != nil {
				t.Fatalf("mergeProcess() error = %v", err)
			}

			var posts int
			for _, r := range requests {
				if r.Method == http.MethodPost {
					posts++
				}
			}
			if posts != 1 {
				t.Errorf("POST requests = %d, want exactly 1 for answer %q", posts, answer)
			}
		})
	}
}

func TestMergeProcessConfirmDeclines(t *testing.T) {
	for _, answer := range []string{"n\n", "\n"} {
		t.Run(testutil.DescribeArg(strings.TrimSuffix(answer, "\n")), func(t *testing.T) {
			withMergeOptions(t, func() {
				mergeOptions.Async = false
				mergeOptions.Message = ""
				mergeOptions.CloseSourceBranch = false
			})

			var requests []*http.Request
			cmd := setupTest(t, mergeConfirmHandler(&requests), false)
			cmd.SetIn(strings.NewReader(answer))

			stdout := testutil.CaptureStdout(t, func() {
				if err := mergeProcess(cmd, []string{"42"}); err != nil {
					t.Fatalf("mergeProcess() error = %v", err)
				}
			})

			if !strings.Contains(stdout, "Merge canceled") {
				t.Errorf("stdout = %q, want it to contain %q", stdout, "Merge canceled")
			}
			for _, r := range requests {
				if r.Method != http.MethodGet {
					t.Errorf("unexpected non-GET request %s %s, want zero write requests on decline", r.Method, r.URL.Path)
				}
			}
		})
	}
}

func TestMergeProcessConfirmDeclineAsyncSendsNothing(t *testing.T) {
	withMergeOptions(t, func() {
		mergeOptions.Async = true
		mergeOptions.Message = ""
		mergeOptions.CloseSourceBranch = false
	})

	var requests []*http.Request
	cmd := setupTest(t, mergeConfirmHandler(&requests), false)
	cmd.SetIn(strings.NewReader("n\n"))

	stdout := testutil.CaptureStdout(t, func() {
		if err := mergeProcess(cmd, []string{"42"}); err != nil {
			t.Fatalf("mergeProcess() error = %v", err)
		}
	})

	if !strings.Contains(stdout, "Merge canceled") {
		t.Errorf("stdout = %q, want it to contain %q", stdout, "Merge canceled")
	}
	for _, r := range requests {
		if r.Method != http.MethodGet {
			t.Errorf("unexpected non-GET request %s %s, want zero write requests on --async decline", r.Method, r.URL.Path)
		}
	}
}

// TestMergeProcessConfirmDryRunDoesNotPrompt guards the gate ordering itself: WhatIfPayload must
// still fire (and return) before ConfirmInteractive is ever reached under --dry-run. A poisoned
// stdin proves the prompt is never even attempted, not merely declined.
func TestMergeProcessConfirmDryRunDoesNotPrompt(t *testing.T) {
	withMergeOptions(t, func() {
		mergeOptions.Async = false
		mergeOptions.Message = ""
		mergeOptions.CloseSourceBranch = false
	})

	var requests []*http.Request
	cmd := setupTest(t, mergeConfirmHandler(&requests), true)
	cmd.SetIn(poisonStdin{t})

	if err := mergeProcess(cmd, []string{"42"}); err != nil {
		t.Fatalf("mergeProcess() error = %v", err)
	}
	if len(requests) != 1 || requests[0].Method != http.MethodGet {
		t.Errorf("requests = %v, want exactly one preflight GET and no merge POST in dry-run mode", requests)
	}
}

// TestMergeProcessConfirmPromptContainsResolvedID exercises the omitted-positional fallback (not
// branch-aware) together with the prompt: the resolved id must appear in the prompt text so a
// human confirming at the terminal actually sees which pull request is about to be merged.
func TestMergeProcessConfirmPromptContainsResolvedID(t *testing.T) {
	withMergeOptions(t, func() {
		mergeOptions.Async = false
		mergeOptions.Message = ""
		mergeOptions.CloseSourceBranch = false
	})

	var requests []*http.Request
	cmd := setupTest(t, mergeConfirmHandler(&requests), false)
	cmd.SetIn(strings.NewReader("n\n"))

	stderr := testutil.CaptureStderr(t, func() {
		if err := mergeProcess(cmd, nil); err != nil {
			t.Fatalf("mergeProcess() error = %v", err)
		}
	})

	if !strings.Contains(stderr, "Merge pullrequest 42?") {
		t.Errorf("stderr = %q, want the prompt to contain %q", stderr, "Merge pullrequest 42?")
	}
}

func TestMergeProcessConfirmEOFErrors(t *testing.T) {
	t.Run("empty SetIn reader (EOF-without-input rule)", func(t *testing.T) {
		withMergeOptions(t, func() {
			mergeOptions.Async = false
			mergeOptions.Message = ""
			mergeOptions.CloseSourceBranch = false
		})

		var requests []*http.Request
		cmd := setupTest(t, mergeConfirmHandler(&requests), false)
		cmd.SetIn(strings.NewReader(""))

		err := mergeProcess(cmd, []string{"42"})
		if err == nil {
			t.Fatal("mergeProcess() expected an error, got nil")
		}
		if !strings.Contains(err.Error(), "cannot confirm merge:") {
			t.Errorf("error = %q, want it to wrap as %q", err.Error(), "cannot confirm merge:")
		}
		for _, r := range requests {
			if r.Method != http.MethodGet {
				t.Errorf("unexpected non-GET request %s %s, want zero write requests", r.Method, r.URL.Path)
			}
		}
	})

	t.Run("non-interactive real stdin (mode-check rule)", func(t *testing.T) {
		withMergeOptions(t, func() {
			mergeOptions.Async = false
			mergeOptions.Message = ""
			mergeOptions.CloseSourceBranch = false
		})

		var requests []*http.Request
		cmd := setupTest(t, mergeConfirmHandler(&requests), false)
		swapStdinToNonInteractivePipe(t)

		err := mergeProcess(cmd, []string{"42"})
		if err == nil {
			t.Fatal("mergeProcess() expected an error, got nil")
		}
		if !strings.Contains(err.Error(), "cannot confirm merge:") {
			t.Errorf("error = %q, want it to wrap as %q", err.Error(), "cannot confirm merge:")
		}
		for _, r := range requests {
			if r.Method != http.MethodGet {
				t.Errorf("unexpected non-GET request %s %s, want zero write requests", r.Method, r.URL.Path)
			}
		}
	})

	t.Run("non-EOF read error (poisoned reader)", func(t *testing.T) {
		withMergeOptions(t, func() {
			mergeOptions.Async = false
			mergeOptions.Message = ""
			mergeOptions.CloseSourceBranch = false
		})

		var requests []*http.Request
		cmd := setupTest(t, mergeConfirmHandler(&requests), false)
		cmd.SetIn(poisonErrStdin{})

		err := mergeProcess(cmd, []string{"42"})
		if err == nil {
			t.Fatal("mergeProcess() expected an error, got nil")
		}
		if !strings.Contains(err.Error(), "cannot confirm merge:") {
			t.Errorf("error = %q, want it to wrap as %q", err.Error(), "cannot confirm merge:")
		}
		for _, r := range requests {
			if r.Method != http.MethodGet {
				t.Errorf("unexpected non-GET request %s %s, want zero write requests", r.Method, r.URL.Path)
			}
		}
	})
}

// TestMergeCmdHasNoForceFlag is a structural+behavioral guard against a --force flag ever being
// reintroduced on mergeCmd: this fork's merge confirmation has no bypass of any kind.
func TestMergeCmdHasNoForceFlag(t *testing.T) {
	if flag := mergeCmd.Flags().Lookup("force"); flag != nil {
		t.Errorf("mergeCmd has a registered --force flag, want none")
	}

	err := mergeCmd.ParseFlags([]string{"--force"})
	if err == nil {
		t.Fatal("mergeCmd.ParseFlags([--force]) error = nil, want an unknown-flag error")
	}
	if !strings.Contains(err.Error(), "unknown flag: --force") {
		t.Errorf("error = %q, want it to contain %q", err.Error(), "unknown flag: --force")
	}
}
