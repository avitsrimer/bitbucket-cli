package common

import "github.com/spf13/cobra"

// RegisterListFlags registers the --columns, --sort, --page-length, and --limit flags shared by
// every list-style subcommand against colDefs's column table, and returns the bound
// Columns/SortBy flag values for the caller to store.
func RegisterListFlags[T any](cmd *cobra.Command, colDefs Columns[T], noun string) (*EnumSliceFlag, *EnumFlag) {
	columnsFlag := NewEnumSliceFlagWithAllAllowed(colDefs.Columns()...)
	sortFlag := NewEnumFlag(colDefs.Sorters()...)
	cmd.Flags().Var(columnsFlag, "columns", "Comma-separated list of columns to display")
	cmd.Flags().Var(sortFlag, "sort", "Column to sort by")
	cmd.Flags().Int("page-length", 0, "Number of items per page to retrieve from Bitbucket. Default is the profile's default page length")
	cmd.Flags().Int("limit", 0, "Maximum total number of "+noun+" to retrieve. Default is to retrieve all of them")
	_ = cmd.RegisterFlagCompletionFunc(columnsFlag.CompletionFunc("columns"))
	_ = cmd.RegisterFlagCompletionFunc(sortFlag.CompletionFunc("sort"))
	return columnsFlag, sortFlag
}

// SortFlagValue reads cmd's own --sort flag directly, rather than a package-level SortBy.Value
// binding that is only ever populated on the real list command instance, so a listProcess-style
// function sorts identically whether cmd is the real command or a standalone test command
// carrying its own --sort flag. It deliberately does not gate on the flag's Changed state: an
// EnumFlag's default String() value is already the column marked DefaultSorter, so returning that
// unconditionally is what makes default sorting (as advertised by --help) actually happen.
// Returns "" only when --sort was never registered on cmd, so callers can skip sorting instead of
// calling Columns.SortBy with an empty sorter name.
func SortFlagValue(cmd *cobra.Command) string {
	if cmd.Flags().Lookup("sort") == nil {
		return ""
	}
	value, err := cmd.Flags().GetString("sort")
	if err != nil {
		return ""
	}
	return value
}
