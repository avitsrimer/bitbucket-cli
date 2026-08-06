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
	Columns *common.EnumSliceFlag
	SortBy  *common.EnumFlag
}

func init() {
	Command.AddCommand(listCmd)

	listOptions.Columns, listOptions.SortBy = common.RegisterListFlags(listCmd, columns, "commits")
	// --query/--include/--exclude have no package-level destination: GetCommits reads them
	// directly off the passed cmd (see commitsQuery), so bound variables here would only ever be
	// write-only state.
	listCmd.Flags().String("query", "", "Query string to filter commits")
	listCmd.Flags().StringSlice("include", []string{}, "List of commit hashes/branches to include")
	listCmd.Flags().StringSlice("exclude", []string{}, "List of commit hashes/branches to exclude")
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
	if sortValue := common.SortFlagValue(cmd); sortValue != "" {
		core.Sort(commits, columns.SortBy(sortValue))
	}
	if err := profile.Current.Print(cmd.Context(), cmd, Commits(commits)); err != nil {
		return fmt.Errorf("cannot print result: %w", err)
	}
	return nil
}
