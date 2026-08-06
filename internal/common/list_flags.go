package common

import "github.com/spf13/cobra"

// RegisterColumnsFlag registers the --columns flag against colDefs's column table.
func RegisterColumnsFlag[T any](cmd *cobra.Command, colDefs Columns[T]) {
	columnsFlag := NewEnumSliceFlagWithAllAllowed(colDefs.Columns()...)
	cmd.Flags().Var(columnsFlag, "columns", "Comma-separated list of columns to display")
	_ = cmd.RegisterFlagCompletionFunc(columnsFlag.CompletionFunc("columns"))
}

// RegisterSortFlag registers the --sort flag against colDefs's column table. Read the resolved
// value back with SortFlagValue.
func RegisterSortFlag[T any](cmd *cobra.Command, colDefs Columns[T]) {
	sortFlag := NewEnumFlag(colDefs.Sorters()...)
	cmd.Flags().Var(sortFlag, "sort", "Column to sort by")
	_ = cmd.RegisterFlagCompletionFunc(sortFlag.CompletionFunc("sort"))
}

// RegisterListFlags registers the --columns, --sort, --page-length, and --limit flags shared by
// every list-style subcommand against colDefs's column table.
func RegisterListFlags[T any](cmd *cobra.Command, colDefs Columns[T], noun string) {
	RegisterColumnsFlag(cmd, colDefs)
	RegisterSortFlag(cmd, colDefs)
	cmd.Flags().Int("page-length", 0, "Number of items per page to retrieve from Bitbucket. Default is the profile's default page length")
	cmd.Flags().Int("limit", 0, "Maximum total number of "+noun+" to retrieve. Default is to retrieve all of them")
}

// SortFlagValue reads cmd's own --sort flag via StringFlagValue. It deliberately does not gate
// on the flag's Changed state: an EnumFlag's default String() value is already the column marked
// DefaultSorter, so returning that unconditionally is what makes default sorting (as advertised
// by --help) actually happen. Returns "" only when --sort was never registered on cmd, so
// callers can skip sorting instead of calling Columns.SortBy with an empty sorter name.
func SortFlagValue(cmd *cobra.Command) string {
	return StringFlagValue(cmd, "sort")
}
