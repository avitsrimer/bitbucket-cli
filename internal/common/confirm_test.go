package common_test

import (
	"errors"
	"io"
	"os"
	"strings"
	"testing"

	"github.com/avitsrimer/bitbucket-cli/internal/common"
	"github.com/spf13/cobra"
)

// newConfirmCmd returns a bare command carrying the "dry-run" and "force" flags Confirm reads.
func newConfirmCmd(t *testing.T) *cobra.Command {
	t.Helper()
	cmd := &cobra.Command{Use: "test"}
	cmd.Flags().Bool("dry-run", false, "")
	cmd.Flags().Bool("force", false, "")
	return cmd
}

// swapStdin temporarily replaces the package-level os.Stdin with r, restoring the original on
// cleanup. r's Stat() mode determines whether Confirm's non-interactive check fires: an os.Pipe
// read end reports as a named pipe (ModeCharDevice unset), reliably simulating a redirected,
// non-interactive stdin regardless of the ambient test environment's own stdin.
func swapStdin(t *testing.T, r *os.File) {
	t.Helper()
	original := os.Stdin
	os.Stdin = r
	t.Cleanup(func() { os.Stdin = original })
}

func TestConfirmYesProceeds(t *testing.T) {
	for _, answer := range []string{"y\n", "Y\n", "yes\n", "YES\n"} {
		cmd := newConfirmCmd(t)
		cmd.SetIn(strings.NewReader(answer))

		proceed, err := common.Confirm(cmd, "proceed?")
		if err != nil {
			t.Fatalf("Confirm() error = %v, want nil", err)
		}
		if !proceed {
			t.Errorf("Confirm(%q) proceed = false, want true", answer)
		}
	}
}

func TestConfirmNoDeclines(t *testing.T) {
	for _, answer := range []string{"n\n", "no\n", "\n", "anything else\n"} {
		cmd := newConfirmCmd(t)
		cmd.SetIn(strings.NewReader(answer))

		proceed, err := common.Confirm(cmd, "proceed?")
		if err != nil {
			t.Fatalf("Confirm() error = %v, want nil", err)
		}
		if proceed {
			t.Errorf("Confirm(%q) proceed = true, want false", answer)
		}
	}
}

// poisonReader fails the test if it is ever read from, proving a short-circuit (--force for
// Confirm, --dry-run for either) skipped the prompt entirely instead of merely happening to
// decline it.
type poisonReader struct{ t *testing.T }

func (p poisonReader) Read([]byte) (int, error) {
	p.t.Fatal("read from stdin when the prompt should have been skipped")
	return 0, io.EOF
}

func TestConfirmForceSkipsPrompt(t *testing.T) {
	cmd := newConfirmCmd(t)
	if err := cmd.Flags().Set("force", "true"); err != nil {
		t.Fatalf("cannot set force flag: %v", err)
	}
	cmd.SetIn(poisonReader{t})

	proceed, err := common.Confirm(cmd, "proceed?")
	if err != nil {
		t.Fatalf("Confirm() error = %v, want nil", err)
	}
	if !proceed {
		t.Error("Confirm() proceed = false, want true with --force")
	}
}

func TestConfirmDryRunWithoutForceOrTTYDoesNotError(t *testing.T) {
	cmd := newConfirmCmd(t)
	if err := cmd.Flags().Set("dry-run", "true"); err != nil {
		t.Fatalf("cannot set dry-run flag: %v", err)
	}
	// cmd.InOrStdin() is left as the real os.Stdin (no SetIn), swapped to a pipe whose write end is
	// never used: if the dry-run short-circuit did not fire before any read, this test would hang
	// instead of failing cleanly.
	r, w, pipeErr := os.Pipe()
	if pipeErr != nil {
		t.Fatalf("cannot create pipe: %v", pipeErr)
	}
	t.Cleanup(func() { _ = w.Close() })
	swapStdin(t, r)

	proceed, err := common.Confirm(cmd, "proceed?")
	if err != nil {
		t.Fatalf("Confirm() error = %v, want nil in dry-run mode", err)
	}
	if !proceed {
		t.Error("Confirm() proceed = false, want true in dry-run mode")
	}
}

func TestConfirmNonInteractiveWithoutForceErrors(t *testing.T) {
	cmd := newConfirmCmd(t)
	r, w, pipeErr := os.Pipe()
	if pipeErr != nil {
		t.Fatalf("cannot create pipe: %v", pipeErr)
	}
	t.Cleanup(func() { _ = w.Close() })
	swapStdin(t, r)

	proceed, err := common.Confirm(cmd, "proceed?")
	if err == nil {
		t.Fatal("Confirm() error = nil, want an error for non-interactive stdin without --force")
	}
	if proceed {
		t.Error("Confirm() proceed = true, want false alongside the error")
	}
}

func TestConfirmInteractiveYesProceeds(t *testing.T) {
	for _, answer := range []string{"y\n", "Y\n", "yes\n", "YES\n"} {
		cmd := newConfirmCmd(t)
		cmd.SetIn(strings.NewReader(answer))

		proceed, err := common.ConfirmInteractive(cmd, "proceed?")
		if err != nil {
			t.Fatalf("ConfirmInteractive() error = %v, want nil", err)
		}
		if !proceed {
			t.Errorf("ConfirmInteractive(%q) proceed = false, want true", answer)
		}
	}
}

func TestConfirmInteractiveNoDeclines(t *testing.T) {
	for _, answer := range []string{"n\n", "no\n", "\n", "anything else\n"} {
		cmd := newConfirmCmd(t)
		cmd.SetIn(strings.NewReader(answer))

		proceed, err := common.ConfirmInteractive(cmd, "proceed?")
		if err != nil {
			t.Fatalf("ConfirmInteractive() error = %v, want nil", err)
		}
		if proceed {
			t.Errorf("ConfirmInteractive(%q) proceed = true, want false", answer)
		}
	}
}

// TestConfirmInteractiveAnswerWithoutTrailingNewlineIsNotAnError pins the predicate that
// distinguishes a real (if unterminated) answer from an EOF-without-input non-answer:
// strings.NewReader("y") returns ("y", io.EOF), which must proceed exactly like "y\n" does, not be
// mistaken for nobody having answered at all.
func TestConfirmInteractiveAnswerWithoutTrailingNewlineIsNotAnError(t *testing.T) {
	cmd := newConfirmCmd(t)
	cmd.SetIn(strings.NewReader("y"))

	proceed, err := common.ConfirmInteractive(cmd, "proceed?")
	if err != nil {
		t.Fatalf("ConfirmInteractive() error = %v, want nil: an answer with no trailing newline is a real answer, not EOF-without-input", err)
	}
	if !proceed {
		t.Error("ConfirmInteractive() proceed = false, want true")
	}
}

// TestConfirmInteractiveForceFlagDoesNotSkipPrompt proves ConfirmInteractive never reads a
// registered "force" flag at all: unlike Confirm, setting it true still prompts, and a decline
// still declines instead of --force-style proceeding unconditionally.
func TestConfirmInteractiveForceFlagDoesNotSkipPrompt(t *testing.T) {
	for _, tc := range []struct {
		answer string
		want   bool
	}{
		{"y\n", true},
		{"n\n", false},
	} {
		cmd := newConfirmCmd(t)
		if err := cmd.Flags().Set("force", "true"); err != nil {
			t.Fatalf("cannot set force flag: %v", err)
		}
		cmd.SetIn(strings.NewReader(tc.answer))

		proceed, err := common.ConfirmInteractive(cmd, "proceed?")
		if err != nil {
			t.Fatalf("ConfirmInteractive() error = %v, want nil", err)
		}
		if proceed != tc.want {
			t.Errorf("ConfirmInteractive(%q) with force=true, proceed = %v, want %v (a registered+set force flag must not skip the prompt)", tc.answer, proceed, tc.want)
		}
	}
}

func TestConfirmInteractiveNonInteractiveErrors(t *testing.T) {
	cmd := newConfirmCmd(t)
	r, w, pipeErr := os.Pipe()
	if pipeErr != nil {
		t.Fatalf("cannot create pipe: %v", pipeErr)
	}
	t.Cleanup(func() { _ = w.Close() })
	swapStdin(t, r)

	proceed, err := common.ConfirmInteractive(cmd, "proceed?")
	if err == nil {
		t.Fatal("ConfirmInteractive() error = nil, want an error for non-interactive stdin")
	}
	if proceed {
		t.Error("ConfirmInteractive() proceed = true, want false alongside the error")
	}
	if got, want := err.Error(), "proceed?: merging requires an interactive terminal"; got != want {
		t.Errorf("error = %q, want %q", got, want)
	}
	if strings.Contains(err.Error(), "force") {
		t.Errorf("error = %q, must not mention force: ConfirmInteractive has no bypass", err.Error())
	}
}

func TestConfirmInteractiveEOFWithoutInputErrors(t *testing.T) {
	cmd := newConfirmCmd(t)
	cmd.SetIn(strings.NewReader(""))

	proceed, err := common.ConfirmInteractive(cmd, "proceed?")
	if err == nil {
		t.Fatal("ConfirmInteractive() error = nil, want an error for EOF with no input")
	}
	if proceed {
		t.Error("ConfirmInteractive() proceed = true, want false alongside the error")
	}
	if got, want := err.Error(), "proceed?: merging requires an interactive terminal"; got != want {
		t.Errorf("error = %q, want %q", got, want)
	}
}

// errPoisonRead simulates a non-EOF read failure, e.g. EIO after the process loses its
// controlling terminal mid-prompt.
var errPoisonRead = errors.New("simulated read failure")

// poisonErrReader always fails with errPoisonRead instead of returning any input or io.EOF.
type poisonErrReader struct{}

func (poisonErrReader) Read([]byte) (int, error) {
	return 0, errPoisonRead
}

// TestConfirmInteractiveNonEOFReadErrorErrors pins the fix for the non-EOF read-error case: a read
// failure that isn't io.EOF must still be treated as nobody having answered, not fall through to a
// silent decline the way a "" answer normally would.
func TestConfirmInteractiveNonEOFReadErrorErrors(t *testing.T) {
	cmd := newConfirmCmd(t)
	cmd.SetIn(poisonErrReader{})

	proceed, err := common.ConfirmInteractive(cmd, "proceed?")
	if err == nil {
		t.Fatal("ConfirmInteractive() error = nil, want an error for a non-EOF read error")
	}
	if proceed {
		t.Error("ConfirmInteractive() proceed = true, want false alongside the error")
	}
	if got, want := err.Error(), "proceed?: merging requires an interactive terminal"; got != want {
		t.Errorf("error = %q, want %q", got, want)
	}
}

func TestConfirmInteractiveDryRunSkipsPrompt(t *testing.T) {
	cmd := newConfirmCmd(t)
	if err := cmd.Flags().Set("dry-run", "true"); err != nil {
		t.Fatalf("cannot set dry-run flag: %v", err)
	}
	cmd.SetIn(poisonReader{t})

	proceed, err := common.ConfirmInteractive(cmd, "proceed?")
	if err != nil {
		t.Fatalf("ConfirmInteractive() error = %v, want nil in dry-run mode", err)
	}
	if !proceed {
		t.Error("ConfirmInteractive() proceed = false, want true in dry-run mode")
	}
}
