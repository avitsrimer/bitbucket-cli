package branch

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
	Short: "list the branches of the current repository",
	Args:  cobra.NoArgs,
	RunE:  listProcess,
}

func init() {
	Command.AddCommand(listCmd)

	common.RegisterListFlags(listCmd, columns, "branches")
	// --query has no package-level destination: GetBranches reads it directly off the passed cmd
	// (see branchesQuery), so a bound variable here would only ever be write-only state.
	listCmd.Flags().String("query", "", "Query string to filter branches")
}

func listProcess(cmd *cobra.Command, args []string) error {
	lgr.Printf("[DEBUG] listing branches")
	if !common.WhatIf(cmd, "Showing branches") {
		return nil
	}

	branches, err := GetBranches(cmd.Context(), cmd)
	if err != nil {
		return fmt.Errorf("cannot get branches: %w", err)
	}
	if len(branches) == 0 {
		fmt.Println("No branch found")
		return nil
	}
	if sortValue := common.SortFlagValue(cmd); sortValue != "" {
		core.Sort(branches, columns.SortBy(sortValue))
	}
	if err := profile.Current.Print(cmd.Context(), cmd, Branches(branches)); err != nil {
		return fmt.Errorf("cannot print result: %w", err)
	}
	return nil
}
