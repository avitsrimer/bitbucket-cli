package common

import (
	"fmt"

	"github.com/spf13/cobra"
)

// DisableUnsupportedFlags returns a cobra PreRunE function that rejects any of flagNames when the
// caller has changed it, because command does not support overriding that persistent flag's value
// (e.g. a command that already takes its target as a positional argument or resolves it from git
// config has no use for the root --repository flag).
func DisableUnsupportedFlags(command string, flagNames ...string) func(cmd *cobra.Command, args []string) error {
	return func(cmd *cobra.Command, args []string) error {
		for _, name := range flagNames {
			if cmd.Flags().Changed(name) {
				return fmt.Errorf("the --%s flag is not supported by the %s command", name, command)
			}
		}
		return nil
	}
}

// HideUnsupportedFlags returns a cobra SetHelpFunc function that hides flagNames from a command's
// help output before delegating to its parent's help function, so a flag DisableUnsupportedFlags
// rejects is not even advertised.
func HideUnsupportedFlags(flagNames ...string) func(cmd *cobra.Command, args []string) {
	return func(cmd *cobra.Command, args []string) {
		for _, name := range flagNames {
			_ = cmd.Flags().MarkHidden(name)
		}
		cmd.Parent().HelpFunc()(cmd, args)
	}
}
