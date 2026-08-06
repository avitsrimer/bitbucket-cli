package repository

import (
	"github.com/avitsrimer/bitbucket-cli/internal/common"
	"github.com/spf13/cobra"
)

// registerListFlags registers the --columns, --sort, --page-length, and --limit flags shared by
// every list-style subcommand in this package against colDefs's column table, and returns the
// bound Columns/SortBy flag values for the caller to store.
func registerListFlags[T any](cmd *cobra.Command, colDefs common.Columns[T], noun string) (*common.EnumSliceFlag, *common.EnumFlag) {
	columnsFlag := common.NewEnumSliceFlagWithAllAllowed(colDefs.Columns()...)
	sortFlag := common.NewEnumFlag(colDefs.Sorters()...)
	cmd.Flags().Var(columnsFlag, "columns", "Comma-separated list of columns to display")
	cmd.Flags().Var(sortFlag, "sort", "Column to sort by")
	cmd.Flags().Int("page-length", 0, "Number of items per page to retrieve from Bitbucket. Default is the profile's default page length")
	cmd.Flags().Int("limit", 0, "Maximum total number of "+noun+" to retrieve. Default is to retrieve all of them")
	_ = cmd.RegisterFlagCompletionFunc(columnsFlag.CompletionFunc("columns"))
	_ = cmd.RegisterFlagCompletionFunc(sortFlag.CompletionFunc("sort"))
	return columnsFlag, sortFlag
}
