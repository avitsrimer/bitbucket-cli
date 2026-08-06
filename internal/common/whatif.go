package common

import (
	"fmt"
	"os"

	"github.com/go-pkgz/lgr"
	"github.com/spf13/cobra"
)

// WhatIf prints what would be done by the command
//
// # If the DryRun flag is set, it prints what would be done by the command and tells the caller to not proceed
//
// otherwise it does nothing
func WhatIf(cmd *cobra.Command, format string, args ...any) (proceed bool) {
	if !isDryRun(cmd) {
		return true
	}
	lgr.Printf("[DEBUG] dry run: "+format, args...)
	fmt.Fprintf(os.Stderr, "Dry run: "+format+"\n", args...)
	return false
}

// isDryRun reports whether cmd carries a dry-run/noop/whatif flag set to true. Shared by WhatIf
// (which also prints the "would do X" message) and Confirm (which short-circuits before prompting,
// leaving the caller's own WhatIf check to report the dry-run message once).
func isDryRun(cmd *cobra.Command) bool {
	for _, name := range []string{"dry-run", "noop", "whatif"} {
		flag := cmd.Flag(name)
		if flag != nil && flag.Value != nil && flag.Value.String() == "true" {
			return true
		}
	}
	return false
}
