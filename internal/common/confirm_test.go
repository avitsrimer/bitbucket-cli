package common_test

import (
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

// poisonReader fails the test if it is ever read from, proving --force skipped the prompt entirely
// instead of merely happening to decline it.
type poisonReader struct{ t *testing.T }

func (p poisonReader) Read([]byte) (int, error) {
	p.t.Fatal("Confirm read from stdin despite --force")
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
