package profile_test

import (
	"testing"

	"github.com/avitsrimer/bitbucket-cli/internal/common"
	"github.com/avitsrimer/bitbucket-cli/internal/profile"
	"github.com/spf13/cobra"
)

// newErrorProcessingTestCmd builds a bare command carrying the same stop-on-error/warn-on-error/
// ignore-errors trio the real root command registers as persistent flags (see internal/cmd/root.go),
// optionally marking one of them Changed to the given value.
func newErrorProcessingTestCmd(changedFlag string, changedValue bool) *cobra.Command {
	cmd := &cobra.Command{Use: "test"}
	cmd.Flags().Bool("stop-on-error", false, "")
	cmd.Flags().Bool("warn-on-error", false, "")
	cmd.Flags().Bool("ignore-errors", false, "")
	if changedFlag != "" {
		if changedValue {
			_ = cmd.Flags().Set(changedFlag, "true")
		} else {
			// still mark Changed with the flag's zero value, distinguishing "explicitly passed
			// --flag=false" from "the flag was never mentioned on the command line at all"
			_ = cmd.Flags().Set(changedFlag, "false")
		}
	}
	return cmd
}

// TestShouldStopOnErrorPrecedenceMatrix pins the full precedence order ShouldStopOnError,
// ShouldWarnOnError, and ShouldIgnoreErrors must agree on: an explicit --stop-on-error always
// wins; otherwise an explicit --warn-on-error/--ignore-errors wins over the profile's configured
// ErrorProcessing (this is the regression: before the fix, ShouldStopOnError's fallback checked
// only profile.ErrorProcessing == StopOnError, which is ErrorProcessing's zero value, so it
// reported "stop" even when --warn-on-error/--ignore-errors were explicitly given and the profile
// had no ErrorProcessing configured at all); with no flag given, the profile's ErrorProcessing
// decides; and with neither a flag nor a configured ErrorProcessing, the default is to stop.
func TestShouldStopOnErrorPrecedenceMatrix(t *testing.T) {
	tests := []struct {
		name            string
		errorProcessing common.ErrorProcessing
		changedFlag     string
		changedValue    bool
		wantStop        bool
		wantWarn        bool
		wantIgnore      bool
	}{
		{
			name:     "no flag, no profile setting: defaults to stop",
			wantStop: true,
		},
		{
			name:            "no flag, profile set to WarnOnError: warns",
			errorProcessing: common.WarnOnError,
			wantWarn:        true,
		},
		{
			name:            "no flag, profile set to IgnoreErrors: ignores",
			errorProcessing: common.IgnoreErrors,
			wantIgnore:      true,
		},
		{
			name:            "no flag, profile explicitly StopOnError: stops",
			errorProcessing: common.StopOnError,
			wantStop:        true,
		},
		{
			// The core regression: an explicit --warn-on-error must override a profile that
			// defaults to StopOnError (the zero value, indistinguishable from "unset").
			name:         "--warn-on-error wins over a profile with no ErrorProcessing set",
			changedFlag:  "warn-on-error",
			changedValue: true,
			wantWarn:     true,
		},
		{
			name:         "--ignore-errors wins over a profile with no ErrorProcessing set",
			changedFlag:  "ignore-errors",
			changedValue: true,
			wantIgnore:   true,
		},
		{
			// --warn-on-error must also override a profile explicitly configured to StopOnError.
			name:            "--warn-on-error wins over a profile explicitly set to StopOnError",
			errorProcessing: common.StopOnError,
			changedFlag:     "warn-on-error",
			changedValue:    true,
			wantWarn:        true,
		},
		{
			// --stop-on-error always wins the *stop* decision, even over a profile configured to
			// warn: every call site checks ShouldStopOnError first and only consults
			// ShouldWarnOnError/ShouldIgnoreErrors when it is false, so ShouldWarnOnError
			// independently still reports the profile's own setting here -- it is
			// ShouldStopOnError's result that actually takes precedence in practice.
			name:            "--stop-on-error wins over a profile set to WarnOnError",
			errorProcessing: common.WarnOnError,
			changedFlag:     "stop-on-error",
			changedValue:    true,
			wantStop:        true,
			wantWarn:        true,
		},
		{
			name:         "explicit --stop-on-error=false with no profile setting: does not stop",
			changedFlag:  "stop-on-error",
			changedValue: false,
			wantStop:     false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			p := profile.Profile{Name: "test", ErrorProcessing: tt.errorProcessing}
			cmd := newErrorProcessingTestCmd(tt.changedFlag, tt.changedValue)

			if got := p.ShouldStopOnError(cmd); got != tt.wantStop {
				t.Errorf("ShouldStopOnError() = %v, want %v", got, tt.wantStop)
			}
			if got := p.ShouldWarnOnError(cmd); got != tt.wantWarn {
				t.Errorf("ShouldWarnOnError() = %v, want %v", got, tt.wantWarn)
			}
			if got := p.ShouldIgnoreErrors(cmd); got != tt.wantIgnore {
				t.Errorf("ShouldIgnoreErrors() = %v, want %v", got, tt.wantIgnore)
			}
		})
	}
}
