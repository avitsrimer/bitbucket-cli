package common

import (
	"fmt"
	"io"
	"os"

	"github.com/spf13/cobra"
)

// ReadBodyFromFileOrStdin returns the verbatim content of path, or of cmd's stdin (via
// cmd.InOrStdin()) when path is "-". Content is returned exactly as read, with no trimming: a
// markdown comment or pull request description body containing trailing whitespace, backticks, or
// shell-metacharacter-looking text (the exact hazard --comment-file/--description-file exist to
// route around) must reach the request payload unchanged.
//
// The returned error, on failure, names path so a bad --comment-file/--description-file value is
// diagnosable without a debugger.
func ReadBodyFromFileOrStdin(cmd *cobra.Command, path string) (string, error) {
	if path == "-" {
		data, err := io.ReadAll(cmd.InOrStdin())
		if err != nil {
			return "", fmt.Errorf("cannot read from stdin: %w", err)
		}
		return string(data), nil
	}
	data, err := os.ReadFile(path) //nolint:gosec // G304: path is the value the invoking user deliberately typed as --comment-file/--description-file, the same access a shell "cat <path>" would have; there is no separate untrusted-input boundary to cross
	if err != nil {
		return "", fmt.Errorf("cannot read %s: %w", path, err)
	}
	return string(data), nil
}
