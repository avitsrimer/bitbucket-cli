package common

import "github.com/spf13/cobra"

// StringFlagValue reads cmd's own string-valued flag named name directly, rather than a
// package-level variable a flag was bound to at registration time, so a RunE-style function
// behaves identically whether cmd is the real command or a standalone test command carrying its
// own flag. Returns "" when the flag was never registered on cmd or a read error occurs, so
// callers can treat either the same as "not provided".
func StringFlagValue(cmd *cobra.Command, name string) string {
	if cmd.Flags().Lookup(name) == nil {
		return ""
	}
	value, err := cmd.Flags().GetString(name)
	if err != nil {
		return ""
	}
	return value
}

// StringArrayFlagValue reads cmd's own string-array-valued flag named name directly, the same
// way StringFlagValue reads a plain string flag. Returns nil when the flag was never registered
// on cmd or a read error occurs.
func StringArrayFlagValue(cmd *cobra.Command, name string) []string {
	if cmd.Flags().Lookup(name) == nil {
		return nil
	}
	values, err := cmd.Flags().GetStringArray(name)
	if err != nil {
		return nil
	}
	return values
}
