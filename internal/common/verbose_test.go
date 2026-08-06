package common

import (
	"io"
	"os"
	"testing"

	"github.com/spf13/cobra"
)

// captureStderr redirects os.Stderr for the duration of fn and returns what was written.
func captureStderr(t *testing.T, fn func()) string {
	t.Helper()

	r, w, err := os.Pipe()
	if err != nil {
		t.Fatalf("cannot create pipe: %v", err)
	}
	original := os.Stderr
	os.Stderr = w
	defer func() { os.Stderr = original }()

	captured := make(chan string, 1)
	go func() {
		data, _ := io.ReadAll(r)
		captured <- string(data)
	}()

	fn()

	_ = w.Close()
	return <-captured
}

// TestVerboseDoesNotPrintWhenExplicitlySetToFalse reproduces major finding #4: Verbose keyed off
// Changed alone, so an explicit --verbose=false (Changed is true, value is "false") still
// printed, identically to --verbose=true.
func TestVerboseDoesNotPrintWhenExplicitlySetToFalse(t *testing.T) {
	cmd := &cobra.Command{Use: "test"}
	cmd.Flags().Bool("verbose", false, "")
	if err := cmd.Flags().Set("verbose", "false"); err != nil {
		t.Fatalf("cannot set --verbose=false: %v", err)
	}
	if !cmd.Flags().Changed("verbose") {
		t.Fatal("flag must be Changed for this regression to be meaningful")
	}

	stderr := captureStderr(t, func() { Verbose(cmd, "should not print") })

	if stderr != "" {
		t.Errorf("stderr = %q, want empty: --verbose=false must not print", stderr)
	}
}

// TestVerbosePrintsToStderrNotStdout reproduces the other half of major finding #4: Verbose wrote
// to stdout, which corrupts -o json/csv output written to the same stream.
func TestVerbosePrintsToStderrNotStdout(t *testing.T) {
	cmd := &cobra.Command{Use: "test"}
	cmd.Flags().Bool("verbose", false, "")
	if err := cmd.Flags().Set("verbose", "true"); err != nil {
		t.Fatalf("cannot set --verbose=true: %v", err)
	}

	var stdout string
	stderr := captureStderr(t, func() {
		r, w, err := os.Pipe()
		if err != nil {
			t.Fatalf("cannot create pipe: %v", err)
		}
		original := os.Stdout
		os.Stdout = w
		captured := make(chan string, 1)
		go func() {
			data, _ := io.ReadAll(r)
			captured <- string(data)
		}()

		Verbose(cmd, "hello %s", "world")

		_ = w.Close()
		os.Stdout = original
		stdout = <-captured
	})

	if stdout != "" {
		t.Errorf("stdout = %q, want empty: Verbose must never write to stdout", stdout)
	}
	if stderr == "" {
		t.Error("stderr is empty, want the verbose message written there")
	}
}

// TestVerboseDoesNotPanicWithoutAnInheritedVerboseFlag reproduces the nil-deref half of major
// finding #4: a command with no "verbose" flag anywhere in its inherited chain made
// cmd.Flag("verbose") return nil, and dereferencing its Changed field panicked.
func TestVerboseDoesNotPanicWithoutAnInheritedVerboseFlag(t *testing.T) {
	cmd := &cobra.Command{Use: "test"}

	if !assertNotPanics(t, func() { Verbose(cmd, "should not panic") }) {
		t.Error("Verbose panicked on a command with no inherited verbose flag")
	}
}

func assertNotPanics(t *testing.T, fn func()) (ok bool) {
	t.Helper()
	defer func() {
		if r := recover(); r != nil {
			t.Errorf("panicked: %v", r)
			ok = false
		}
	}()
	fn()
	return true
}
