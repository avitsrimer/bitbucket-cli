package common

import (
	"bufio"
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"os/signal"
	"strings"
	"syscall"

	"github.com/spf13/cobra"
)

// openControllingTTY opens the process's controlling terminal for ReadSecret to prompt on,
// bypassing the process's own stdin/stdout entirely -- e.g. a `bb profile create -u me
// --password-stdin < token.txt` invocation still has a controlling terminal even though stdin
// itself is redirected. It is a package-level variable, not a plain call to os.OpenFile, purely so
// tests can substitute a fake reader without a real terminal device: ReadSecret's own read/trim/
// error-handling logic is then exercised without ever touching a tty or invoking stty, which is
// what makes it possible to unit test at all (there is no controlling terminal in CI).
var openControllingTTY = func() (io.ReadCloser, error) {
	return os.OpenFile("/dev/tty", os.O_RDWR, 0)
}

// StdinIsInteractive reports whether cmd's input stream is the process's own, unmodified os.Stdin
// and that stdin is a real character-device terminal, as opposed to a pipe, a redirect, or (in
// tests) a stand-in reader installed via cmd.SetIn: only then is it safe to block on an interactive
// prompt instead of hanging forever or reading something nobody meant as an answer.
// common.Confirm, common.ConfirmInteractive, and the `profile create`/`profile update` secret
// prompt all gate their prompting on this.
func StdinIsInteractive(cmd *cobra.Command) bool {
	if cmd.InOrStdin() != os.Stdin {
		return false
	}
	info, err := os.Stdin.Stat()
	return err == nil && info.Mode()&os.ModeCharDevice != 0
}

// ReadSecret prompts on stderr with prompt and reads a single line typed at the controlling
// terminal with local echo disabled, e.g. for a password or API token entered interactively
// instead of passed on the command line. A trailing newline is printed once the (invisible) input
// is done, so the terminal cursor lands on its own line the same way it would after a normal,
// echoed Enter.
//
// It reads from /dev/tty directly rather than cmd.InOrStdin(): that keeps this prompt usable even
// when the calling command's own stdin has been repurposed for something else, and disabling echo
// only makes sense against a real terminal device in the first place.
//
// Terminal echo is toggled without depending on golang.org/x/term (banned in this codebase): it
// shells out to `stty -echo`/`stty echo` with the child process's stdin wired directly to the
// opened tty file, so the setting is applied to that terminal device regardless of this process's
// own stdin/stdout/stderr. Echo is always restored before returning, including on a read error.
//
// Callers must check StdinIsInteractive (or an equivalent guard) before calling ReadSecret: a
// missing controlling terminal (no /dev/tty -- CI, or any other non-interactive environment) is
// reported as an error including nonInteractiveHint, the caller-supplied text naming its own
// non-interactive alternative (e.g. "use --password-stdin or --access-token-stdin instead"),
// rather than left to hang. common is the lower layer here (profile imports it, not the other way
// around), so it names no flag of its own callers'.
//
// A SIGINT/SIGTERM received while echo is disabled is caught and handled the same way as a normal
// return: echo is restored, then the process exits with status 130 (the conventional 128+SIGINT),
// via exitProcess. Without this, Ctrl-C at the prompt would kill the process before ReadSecret's
// own defer-based restore ever runs -- a signal terminates immediately, deferred calls never get
// a chance -- leaving the user's shell with local echo permanently disabled until they run `stty
// sane`/`reset` themselves. Aborting a password prompt is routine, not exceptional, so this must
// not be able to break the terminal.
func ReadSecret(prompt, nonInteractiveHint string) (secret string, err error) {
	tty, err := openControllingTTY()
	if err != nil {
		return "", fmt.Errorf("cannot prompt for a secret: no controlling terminal available (%w); %s", err, nonInteractiveHint)
	}
	defer func() { _ = tty.Close() }()

	// Only a real terminal device can have its echo setting toggled; a fake reader substituted in
	// tests (see openControllingTTY's doc comment) is read from as-is, echo untouched, and no
	// interrupt guard is armed for it either -- there is no echo state a test's fake tty needs
	// protected.
	if file, ok := tty.(*os.File); ok {
		if echoErr := setTTYEcho(file, false); echoErr != nil {
			return "", fmt.Errorf("cannot disable terminal echo: %w", echoErr)
		}
		defer func() { _ = setTTYEcho(file, true) }()
		defer armInterruptGuard(file)()
	}

	fmt.Fprintf(os.Stderr, "%s ", prompt)
	line, readErr := bufio.NewReader(tty).ReadString('\n')
	fmt.Fprintln(os.Stderr)
	if readErr != nil && !errors.Is(readErr, io.EOF) {
		return "", fmt.Errorf("cannot read secret: %w", readErr)
	}
	secret = strings.TrimRight(line, "\r\n")
	if secret == "" {
		return "", errors.New("no secret entered")
	}
	return secret, nil
}

// exitProcess is a package-level variable (not a direct call to os.Exit) purely so tests can
// observe armInterruptGuard's signal path firing without actually terminating the test binary.
var exitProcess = os.Exit

// armInterruptGuard arms a SIGINT/SIGTERM handler for as long as tty's echo is disabled,
// restoring echo and exiting with status 130 the instant either signal arrives, and returns a
// disarm function the caller must defer immediately (before the echo-restore defer below it runs
// normally, so the two never race to restore echo twice). A signal received after disarm runs
// takes its usual default action instead -- by the time ReadSecret's own defers are unwinding,
// echo is either already restored or about to be, so there is nothing left for this guard to
// protect.
func armInterruptGuard(tty *os.File) (disarm func()) {
	signals := make(chan os.Signal, 1)
	signal.Notify(signals, os.Interrupt, syscall.SIGTERM)
	done := make(chan struct{})

	go func() {
		select {
		case <-signals:
			_ = setTTYEcho(tty, true)
			fmt.Fprintln(os.Stderr)
			exitProcess(130)
		case <-done:
		}
	}()

	return func() {
		signal.Stop(signals)
		close(done)
	}
}

// setTTYEcho enables or disables local echo on tty by shelling out to stty, wiring its stdin
// directly to tty so the setting is applied to that terminal device regardless of this process's
// own stdin. Both arguments passed to stty are fixed string literals (never built from user input),
// so this is safe despite running an external command with a variable argument list. It runs
// against context.Background(): toggling echo is a fast, synchronous, local operation with no
// caller-supplied context to thread through ReadSecret's own signature.
func setTTYEcho(tty *os.File, echo bool) error {
	var sttyCmd *exec.Cmd
	if echo {
		sttyCmd = exec.CommandContext(context.Background(), "stty", "echo")
	} else {
		sttyCmd = exec.CommandContext(context.Background(), "stty", "-echo")
	}
	sttyCmd.Stdin = tty
	if err := sttyCmd.Run(); err != nil {
		return fmt.Errorf("stty failed: %w", err)
	}
	return nil
}

// stdinIsInteractive is a package-level variable (not a direct call to StdinIsInteractive)
// purely so tests can force ReadSecretFromStdin's interactive-input guard down its rejecting
// path without a real controlling terminal, the same seam openControllingTTY provides for
// ReadSecret's own tty dependency: there is no way to make os.Stdin a character device in CI.
var stdinIsInteractive = StdinIsInteractive

// ReadSecretFromStdin reads the entirety of cmd.InOrStdin() -- not just its first line -- and
// trims trailing whitespace/newline, for --password-stdin/--access-token-stdin/
// --client-secret-stdin: gh- and docker-style flags that let a secret be piped in (e.g. `op read
// op://vault/item/token | bb profile create -n work -u me@corp.com --password-stdin`) instead of
// typed on the command line, where it would land in shell history.
//
// It refuses outright, instead of blocking forever, when cmd's input is a real, interactive
// terminal: io.ReadAll only returns once it sees EOF, which an interactive terminal never sends on
// a bare Enter (only Ctrl-D does) -- so `bb profile create -u me --password-stdin` typed without a
// redirect would otherwise hang with the secret echoed in clear text on the terminal, exactly what
// this flag exists to avoid. docker/gh's own -stdin flags reject the same way; ReadSecret (the
// no-echo interactive prompt) is the documented alternative for a real terminal.
func ReadSecretFromStdin(cmd *cobra.Command) (string, error) {
	if stdinIsInteractive(cmd) {
		return "", errors.New("refusing to read a secret from an interactive terminal: redirect or pipe the value in instead (e.g. `... | bb ...`), or omit the -stdin flag to be prompted with echo disabled")
	}
	data, err := io.ReadAll(cmd.InOrStdin())
	if err != nil {
		return "", fmt.Errorf("cannot read secret from stdin: %w", err)
	}
	secret := strings.TrimRight(string(data), " \t\r\n")
	if secret == "" {
		return "", errors.New("no data read from stdin")
	}
	return secret, nil
}
