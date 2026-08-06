package common

import (
	"errors"
	"io"
	"os"
	"os/signal"
	"strings"
	"syscall"
	"testing"
	"time"

	"github.com/spf13/cobra"
)

// withFakeControllingTTY substitutes openControllingTTY with a stub returning tty/err instead of
// opening a real /dev/tty, restoring the original on cleanup. This is what makes ReadSecret's
// read/trim/error-handling logic testable at all: there is no controlling terminal to open in CI,
// and a fake substituted here is never a *os.File, so ReadSecret's stty toggle is skipped
// entirely -- only the read/trim/error paths are exercised, matching this package's own doc
// comment on openControllingTTY.
func withFakeControllingTTY(t *testing.T, tty io.ReadCloser, err error) {
	t.Helper()
	original := openControllingTTY
	openControllingTTY = func() (io.ReadCloser, error) { return tty, err }
	t.Cleanup(func() { openControllingTTY = original })
}

// fakeTTY adapts a strings.Reader into the io.ReadCloser openControllingTTY returns, deliberately
// not a *os.File so ReadSecret never attempts to toggle echo against it via stty.
type fakeTTY struct {
	io.Reader
}

func (fakeTTY) Close() error { return nil }

func TestReadSecretReadsAndTrims(t *testing.T) {
	tests := []struct {
		name       string
		ttyContent string
		wantSecret string
	}{
		{name: "trims a trailing newline", ttyContent: "s3cr3t\n", wantSecret: "s3cr3t"},
		{name: "trims a trailing CRLF", ttyContent: "s3cr3t\r\n", wantSecret: "s3cr3t"},
		{name: "keeps reading to EOF when there is no trailing newline", ttyContent: "s3cr3t", wantSecret: "s3cr3t"},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			withFakeControllingTTY(t, fakeTTY{strings.NewReader(tc.ttyContent)}, nil)

			secret, err := ReadSecret("Password:", "use --password-stdin or --access-token-stdin instead")
			if err != nil {
				t.Fatalf("ReadSecret() error = %v, want nil", err)
			}
			if secret != tc.wantSecret {
				t.Errorf("ReadSecret() = %q, want %q", secret, tc.wantSecret)
			}
		})
	}
}

func TestReadSecretRejectsAnEmptyLine(t *testing.T) {
	withFakeControllingTTY(t, fakeTTY{strings.NewReader("\n")}, nil)

	_, err := ReadSecret("Password:", "use --password-stdin or --access-token-stdin instead")
	if err == nil {
		t.Fatal("ReadSecret() error = nil, want an error for an empty line")
	}
	if !strings.Contains(err.Error(), "no secret entered") {
		t.Errorf("ReadSecret() error = %q, want it to mention that no secret was entered", err)
	}
}

func TestReadSecretErrorsWhenNoControllingTTYIsAvailable(t *testing.T) {
	withFakeControllingTTY(t, nil, errors.New("no such device or address"))

	_, err := ReadSecret("Password:", "use --password-stdin or --access-token-stdin instead")
	if err == nil {
		t.Fatal("ReadSecret() error = nil, want an error when no controlling terminal is available")
	}
	if !strings.Contains(err.Error(), "--password-stdin") || !strings.Contains(err.Error(), "--access-token-stdin") {
		t.Errorf("ReadSecret() error = %q, want it to name --password-stdin and --access-token-stdin as the non-interactive alternative", err)
	}
}

// TestArmInterruptGuardRestoresEchoAndExitsOnSignal proves the FINAL CRITICAL GATE's priority-2
// finding: without this guard, a SIGINT/SIGTERM received while ReadSecret has terminal echo
// disabled kills the process before its own defer-based restore ever runs, leaving the user's
// shell with echo permanently off. armInterruptGuard must intervene: restore echo (best-effort;
// here the fake tty is a pipe, not a real terminal, so setTTYEcho's own stty call fails and is
// ignored, exactly as armInterruptGuard's caller does) and exit with status 130.
func TestArmInterruptGuardRestoresEchoAndExitsOnSignal(t *testing.T) {
	r, w, err := os.Pipe()
	if err != nil {
		t.Fatalf("cannot create pipe: %v", err)
	}
	t.Cleanup(func() { _ = r.Close(); _ = w.Close() })

	originalExit := exitProcess
	exited := make(chan int, 1)
	exitProcess = func(code int) { exited <- code }
	t.Cleanup(func() { exitProcess = originalExit })

	disarm := armInterruptGuard(w)
	t.Cleanup(disarm)

	if err := syscall.Kill(syscall.Getpid(), syscall.SIGTERM); err != nil {
		t.Fatalf("cannot send SIGTERM to self: %v", err)
	}

	select {
	case code := <-exited:
		if code != 130 {
			t.Errorf("exitProcess called with %d, want 130", code)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("armInterruptGuard did not call exitProcess after SIGTERM, terminal echo would stay disabled forever")
	}
}

// TestArmInterruptGuardDisarmPreventsExitAfterNormalReturn proves disarm() (the pattern ReadSecret
// itself defers immediately after arming the guard) stops the handler from firing at all once the
// prompt completes normally, so a signal arriving after a normal ReadSecret return cannot trigger
// a spurious exit.
func TestArmInterruptGuardDisarmPreventsExitAfterNormalReturn(t *testing.T) {
	r, w, err := os.Pipe()
	if err != nil {
		t.Fatalf("cannot create pipe: %v", err)
	}
	t.Cleanup(func() { _ = r.Close(); _ = w.Close() })

	originalExit := exitProcess
	exited := make(chan int, 1)
	exitProcess = func(code int) { exited <- code }
	t.Cleanup(func() { exitProcess = originalExit })

	disarm := armInterruptGuard(w)
	disarm() // simulates ReadSecret returning normally before any signal arrives

	// signal.Stop (inside disarm) hands SIGTERM back to its default action -- process
	// termination -- once nothing is left registered for it, so a throwaway Notify absorbs the
	// signal this test is about to send instead of letting it kill the test binary; it also lets
	// the test prove *something* observed the signal, ruling out a broken test (as opposed to a
	// working disarm) as the reason armInterruptGuard's own exitProcess never fires.
	absorbed := make(chan os.Signal, 1)
	signal.Notify(absorbed, syscall.SIGTERM)
	t.Cleanup(func() { signal.Stop(absorbed) })

	if err := syscall.Kill(syscall.Getpid(), syscall.SIGTERM); err != nil {
		t.Fatalf("cannot send SIGTERM to self: %v", err)
	}

	select {
	case code := <-exited:
		t.Fatalf("exitProcess called with %d after disarm, want the guard to have stopped listening", code)
	case <-absorbed:
		// expected: the throwaway handler saw it, armInterruptGuard's own did not.
	case <-time.After(2 * time.Second):
		t.Fatal("signal was not observed at all; test setup is broken")
	}
}

// withFakeStdinIsInteractive substitutes stdinIsInteractive with a stub returning interactive for
// the duration of the test, restoring the original on cleanup -- the seam ReadSecretFromStdin's
// own interactivity guard reads, mirroring withFakeControllingTTY's rationale: there is no way to
// make os.Stdin a real character device in CI.
func withFakeStdinIsInteractive(t *testing.T, interactive bool) {
	t.Helper()
	original := stdinIsInteractive
	stdinIsInteractive = func(*cobra.Command) bool { return interactive }
	t.Cleanup(func() { stdinIsInteractive = original })
}

// TestReadSecretFromStdinRejectsAnInteractiveTerminal reproduces the FINAL CRITICAL GATE's
// priority-2 finding: --password-stdin/--access-token-stdin with a real, interactive terminal
// stdin used to hang in io.ReadAll (which only returns on EOF, and a terminal never sends one on a
// bare Enter) with the secret echoed in clear text while the user typed it. ReadSecretFromStdin
// must refuse outright instead, naming the redirect/pipe alternative, and must never even attempt
// the read.
func TestReadSecretFromStdinRejectsAnInteractiveTerminal(t *testing.T) {
	withFakeStdinIsInteractive(t, true)

	cmd := &cobra.Command{Use: "test"}
	reader := &countingReader{Reader: strings.NewReader("s3cr3t\n")}
	cmd.SetIn(reader)

	_, err := ReadSecretFromStdin(cmd)
	if err == nil {
		t.Fatal("ReadSecretFromStdin() error = nil, want an error when stdin is an interactive terminal")
	}
	if !strings.Contains(err.Error(), "interactive") {
		t.Errorf("ReadSecretFromStdin() error = %q, want it to mention the interactive-terminal rejection", err)
	}
	if reader.reads != 0 {
		t.Errorf("cmd.InOrStdin() was read from %d time(s), want zero: the guard must reject before ever reading", reader.reads)
	}
}

// countingReader wraps an io.Reader and counts how many times Read was called, so a test can
// prove a guard rejected before ever attempting to read.
type countingReader struct {
	io.Reader
	reads int
}

func (c *countingReader) Read(p []byte) (int, error) {
	c.reads++
	return c.Reader.Read(p) //nolint:wrapcheck // test double forwarding verbatim to the wrapped io.Reader; wrapping would prefix io.EOF and mislead callers checking errors.Is(err, io.EOF)
}
