package common

import (
	"fmt"
	"os"

	"github.com/go-pkgz/lgr"
	"github.com/spf13/cobra"
)

// Verbose prints a message if the verbose flag is set.
//
// It checks the flag's actual value, not just whether it was Changed, so an explicit
// --verbose=false does not still print; it writes to stderr, not stdout, so it never corrupts
// -o json/csv output written to stdout; and it nil-checks the flag lookup, since a command with no
// inherited "verbose" flag would otherwise panic.
func Verbose(cmd *cobra.Command, format string, args ...any) {
	lgr.Printf("[DEBUG] "+format, args...)
	flag := cmd.Flag("verbose")
	if flag == nil || flag.Value == nil || flag.Value.String() != "true" {
		return
	}
	fmt.Fprintf(os.Stderr, format+"\n", args...)
}
