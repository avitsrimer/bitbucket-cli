package common

import (
	"bufio"
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"strings"

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
// prompt instead of hanging forever or reading something nobody meant as an answer. common.Confirm
// and the `profile create`/`profile update` secret prompt both gate their prompting on this.
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
func ReadSecret(prompt, nonInteractiveHint string) (secret string, err error) {
	tty, err := openControllingTTY()
	if err != nil {
		return "", fmt.Errorf("cannot prompt for a secret: no controlling terminal available (%w); %s", err, nonInteractiveHint)
	}
	defer func() { _ = tty.Close() }()

	// Only a real terminal device can have its echo setting toggled; a fake reader substituted in
	// tests (see openControllingTTY's doc comment) is read from as-is, echo untouched.
	if file, ok := tty.(*os.File); ok {
		if echoErr := setTTYEcho(file, false); echoErr != nil {
			return "", fmt.Errorf("cannot disable terminal echo: %w", echoErr)
		}
		defer func() { _ = setTTYEcho(file, true) }()
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

// ReadSecretFromStdin reads the entirety of cmd.InOrStdin() -- not just its first line -- and
// trims trailing whitespace/newline, for --password-stdin/--access-token-stdin: gh- and
// docker-style flags that let a secret be piped in (e.g. `op read op://vault/item/token | bb
// profile create -n work -u me@corp.com --password-stdin`) instead of typed on the command line,
// where it would land in shell history.
func ReadSecretFromStdin(cmd *cobra.Command) (string, error) {
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
