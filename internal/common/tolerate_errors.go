package common

import (
	"errors"
	"fmt"
	"os"

	"github.com/go-pkgz/lgr"
	"github.com/spf13/cobra"
)

// ErrorTolerance is satisfied by *profile.Profile (and profile.Profile): it decides, per
// --warn-on-error/--ignore-errors, how a batch of per-item failures from a multi-argument command
// should be handled. Defined here rather than accepting *profile.Profile directly so this package
// does not have to import internal/profile, which already imports internal/common.
type ErrorTolerance interface {
	ShouldWarnOnError(cmd *cobra.Command) bool
	ShouldIgnoreErrors(cmd *cobra.Command) bool
}

// TolerateErrors decides, given tolerance's ShouldWarnOnError/ShouldIgnoreErrors, whether errs
// (aggregated per-item failures from a multi-argument command, e.g. one entry per name/id that
// failed while the rest were still attempted) should be returned as a hard error, printed to
// stderr as a warning, or silently logged and ignored. summary describes the failed action in
// lowercase (e.g. "delete these comments", "download these artifacts") for both the stderr and
// log messages.
//
// It returns nil whenever errs joins to nil (nothing to tolerate in the first place) or whenever
// tolerance absorbs the joined error; callers must not call ShouldWarnOnError/ShouldIgnoreErrors
// themselves against a possibly-nil joined error, which is exactly the mistake this helper
// exists to prevent (see internal/artifact/download.go's [WARN] with a literal "%!s(<nil>)" that
// motivated lifting this out of three separate near-identical copies).
func TolerateErrors(cmd *cobra.Command, tolerance ErrorTolerance, errs []error, summary string) error {
	joined := errors.Join(errs...)
	if joined == nil {
		return nil
	}
	if tolerance.ShouldWarnOnError(cmd) {
		fmt.Fprintf(os.Stderr, "Failed to %s: %s\n", summary, joined)
		return nil
	}
	if tolerance.ShouldIgnoreErrors(cmd) {
		lgr.Printf("[WARN] failed to %s, but ignoring errors: %s", summary, joined)
		return nil
	}
	return joined
}
