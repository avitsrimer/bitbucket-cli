package common

import (
	"encoding/json"
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

// WhatIfPayload behaves exactly like WhatIf (same gate, same "Dry run: <format>" line, same single
// report) but additionally, only when the gate trips, echoes the resolved request a real
// invocation would have sent: targetPath always, and payload's indented JSON encoding whenever
// payload is non-nil. Every mutating command calls this once it has finished resolving its
// request (validating identifiers, fetching related resources, building the request body) so a
// dry run reports what the real call would actually send instead of a fixed, input-independent
// line. A payload carrying a secret (e.g. a pipeline trigger's variables) must be redacted by the
// caller before it reaches here -- this function only renders what it is given.
func WhatIfPayload(cmd *cobra.Command, targetPath string, payload any, format string, args ...any) (proceed bool) {
	if WhatIf(cmd, format, args...) {
		return true
	}
	fmt.Fprintf(os.Stderr, "Dry run: target %s\n", targetPath)
	if payload != nil {
		data, err := json.MarshalIndent(payload, "", "  ")
		if err != nil {
			fmt.Fprintf(os.Stderr, "Dry run: cannot render payload: %s\n", err)
		} else {
			fmt.Fprintf(os.Stderr, "Dry run: payload\n%s\n", data)
		}
	}
	return false
}

// isDryRun reports whether cmd carries a dry-run/noop/whatif flag set to true. Shared by WhatIf
// (which also prints the "would do X" message) and Confirm/ConfirmInteractive (which short-circuit
// before prompting, leaving the caller's own WhatIf check to report the dry-run message once).
func isDryRun(cmd *cobra.Command) bool {
	for _, name := range []string{"dry-run", "noop", "whatif"} {
		flag := cmd.Flag(name)
		if flag != nil && flag.Value != nil && flag.Value.String() == "true" {
			return true
		}
	}
	return false
}
