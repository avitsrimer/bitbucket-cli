package commit

import (
	"fmt"

	"github.com/avitsrimer/bitbucket-cli/internal/common"
	"github.com/avitsrimer/bitbucket-cli/internal/profile"
	"github.com/gildas/go-core"
	"github.com/go-pkgz/lgr"
	"github.com/spf13/cobra"
)

var listCmd = &cobra.Command{
	Use:   "list",
	Short: "list the commits of the current repository",
	Args:  cobra.NoArgs,
	RunE:  listProcess,
}

var listOptions struct {
	Query   string
	Include []string
	Exclude []string
	Columns *common.EnumSliceFlag
	SortBy  *common.EnumFlag
}

func init() {
	Command.AddCommand(listCmd)

	listOptions.Columns = common.NewEnumSliceFlagWithAllAllowed(columns.Columns()...)
	listOptions.SortBy = common.NewEnumFlag(columns.Sorters()...)
	listCmd.Flags().StringVar(&listOptions.Query, "query", "", "Query string to filter commits")
	listCmd.Flags().StringSliceVar(&listOptions.Include, "include", []string{}, "List of commit hashes/branches to include")
	listCmd.Flags().StringSliceVar(&listOptions.Exclude, "exclude", []string{}, "List of commit hashes/branches to exclude")
	listCmd.Flags().Var(listOptions.Columns, "columns", "Comma-separated list of columns to display")
	listCmd.Flags().Var(listOptions.SortBy, "sort", "Column to sort by")
	listCmd.Flags().Int("page-length", 0, "Number of items per page to retrieve from Bitbucket. Default is the profile's default page length")
	listCmd.Flags().Int("limit", 0, "Maximum total number of commits to retrieve. Default is to retrieve all of them")
	_ = listCmd.RegisterFlagCompletionFunc(listOptions.Columns.CompletionFunc("columns"))
	_ = listCmd.RegisterFlagCompletionFunc(listOptions.SortBy.CompletionFunc("sort"))
}

func listProcess(cmd *cobra.Command, args []string) error {
	lgr.Printf("[DEBUG] listing commits")
	if !common.WhatIf(cmd, "Showing commits") {
		return nil
	}

	commits, err := GetCommits(cmd.Context(), cmd)
	if err != nil {
		return fmt.Errorf("cannot get commits: %w", err)
	}
	if len(commits) == 0 {
		fmt.Println("No commit found")
		return nil
	}
	if sortFlag := cmd.Flag("sort"); sortFlag != nil && sortFlag.Changed {
		core.Sort(commits, columns.SortBy(listOptions.SortBy.Value))
	}
	if err := profile.Current.Print(cmd.Context(), cmd, Commits(commits)); err != nil {
		return fmt.Errorf("cannot print result: %w", err)
	}
	return nil
}
