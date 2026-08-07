package common

import (
	"bufio"
	"errors"
	"fmt"
	"io"
	"os"
	"strings"

	"github.com/spf13/cobra"
)

// promptYesNo writes prompt to stderr, suffixed with the shared " [y/N] " marker, then reads a
// single line from in. The returned error wraps whatever the underlying read returned (typically
// io.EOF when the stream ended before a newline was seen, still matched by errors.Is) -- Confirm
// and ConfirmInteractive decide for themselves whether that error matters.
func promptYesNo(in io.Reader, prompt string) (line string, err error) {
	fmt.Fprintf(os.Stderr, "%s [y/N] ", prompt)
	line, err = bufio.NewReader(in).ReadString('\n')
	if err != nil {
		return line, fmt.Errorf("cannot read confirmation answer: %w", err)
	}
	return line, nil
}

// Confirm asks the user a gh-style y/N question before a state-changing command proceeds, reading
// a single line from cmd.InOrStdin(). Only a trimmed, case-insensitive "y" or "yes" answers true;
// anything else -- including a bare Enter or an EOF with no input at all -- declines, matching the
// y/N convention that the prompt defaults to "no".
//
// --dry-run short-circuits before the prompt is ever shown: proceed is reported true without
// reading any input, leaving the caller's own common.WhatIf check to report what would have been
// done and stop before any request is sent.
//
// --force (read from cmd's own "force" flag, when registered) also skips the prompt and reports
// proceed as true unconditionally.
//
// Absent both, Confirm refuses to block forever on a prompt nobody can answer: non-interactive
// detection (os.Stdin.Stat's ModeCharDevice bit) applies only when cmd.InOrStdin() is the process's
// real os.Stdin -- never when it has been replaced (e.g. a test's cmd.SetIn(strings.NewReader(...))
// or a piped command's redirected stdin) -- and only then does a non-character-device stdin turn
// into an error asking for --force instead of a hanging read.
func Confirm(cmd *cobra.Command, prompt string) (proceed bool, err error) {
	if isDryRun(cmd) {
		return true, nil
	}
	if force, ferr := cmd.Flags().GetBool("force"); ferr == nil && force {
		return true, nil
	}

	in := cmd.InOrStdin()
	if in == os.Stdin && !StdinIsInteractive(cmd) {
		return false, fmt.Errorf("%s: input is not a terminal, use --force to skip confirmation", prompt)
	}

	line, _ := promptYesNo(in, prompt)
	answer := strings.ToLower(strings.TrimSpace(line))
	return answer == "y" || answer == "yes", nil
}

// ConfirmInteractive is Confirm's strict sibling for the one command in this fork that must never
// be automatable: pullrequest merge. It shares Confirm's prompt/read body via promptYesNo but
// differs from it in two deliberate ways:
//
//   - There is no --force escape hatch of any kind: the "force" flag, even when registered and
//     set on cmd, is never read. A merge is either confirmed by a human at the prompt or it does
//     not happen.
//   - An EOF with no input at all (e.g. /dev/null on stdin: it passes the character-device check
//     below since /dev/null IS a char device, but the read still returns nothing) is treated as an
//     error, not Confirm's silent decline -- nobody answered, so nothing may look like a handled
//     cancel. A real answer with no trailing newline (e.g. strings.NewReader("y")) still returns
//     that answer, not an error: only a wholly empty read counts.
//
// --dry-run still short-circuits first, for defense in depth: mergeProcess's own WhatIfPayload
// gate fires before ConfirmInteractive is ever reached under --dry-run, so this branch is
// currently unreachable from that call site, but ConfirmInteractive is not safe to call before an
// unconditional dry-run check just because today's one caller happens to order things that way.
//
// Non-interactive detection is identical to Confirm's: only cmd.InOrStdin() being the process's
// own, unreplaced os.Stdin and failing StdinIsInteractive's character-device check triggers the
// error below -- a test's cmd.SetIn stand-in bypasses that specific check, but an empty stand-in
// reader still hits the EOF-without-input rule.
func ConfirmInteractive(cmd *cobra.Command, prompt string) (proceed bool, err error) {
	if isDryRun(cmd) {
		return true, nil
	}

	in := cmd.InOrStdin()
	if in == os.Stdin && !StdinIsInteractive(cmd) {
		return false, fmt.Errorf("%s: merging requires an interactive terminal", prompt)
	}

	line, readErr := promptYesNo(in, prompt)
	if line == "" && errors.Is(readErr, io.EOF) {
		return false, fmt.Errorf("%s: merging requires an interactive terminal", prompt)
	}
	answer := strings.ToLower(strings.TrimSpace(line))
	return answer == "y" || answer == "yes", nil
}
