package common

import (
	"bufio"
	"fmt"
	"os"
	"strings"

	"github.com/spf13/cobra"
)

// Confirm asks the user a gh-style y/N question before a state-changing command proceeds, reading
// a single line from cmd.InOrStdin(). Only a trimmed, case-insensitive "y" or "yes" answers true;
// anything else -- including a bare Enter -- declines, matching the y/N convention that the prompt
// defaults to "no".
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
	if in == os.Stdin {
		if info, statErr := os.Stdin.Stat(); statErr == nil && info.Mode()&os.ModeCharDevice == 0 {
			return false, fmt.Errorf("%s: input is not a terminal, use --force to skip confirmation", prompt)
		}
	}

	fmt.Fprintf(os.Stderr, "%s [y/N] ", prompt)
	line, _ := bufio.NewReader(in).ReadString('\n')
	answer := strings.ToLower(strings.TrimSpace(line))
	return answer == "y" || answer == "yes", nil
}
