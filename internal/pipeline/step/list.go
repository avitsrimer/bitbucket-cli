package step

import (
	"fmt"

	"github.com/avitsrimer/bitbucket-cli/internal/common"
	"github.com/avitsrimer/bitbucket-cli/internal/profile"
	"github.com/avitsrimer/bitbucket-cli/internal/repository"
	"github.com/gildas/go-core"
	"github.com/go-pkgz/lgr"
	"github.com/spf13/cobra"
)

var listCmd = &cobra.Command{
	Use:   "list",
	Short: "list the steps of a pipeline",
	Args:  cobra.NoArgs,
	RunE:  listProcess,
}

var listOptions struct {
	Columns *common.EnumSliceFlag
	SortBy  *common.EnumFlag
}

func init() {
	Command.AddCommand(listCmd)

	registerPipelineFlag(listCmd, "Pipeline to list steps from")
	listOptions.Columns = common.NewEnumSliceFlagWithAllAllowed(columns.Columns()...)
	listOptions.SortBy = common.NewEnumFlag(columns.Sorters()...)
	listCmd.Flags().Var(listOptions.Columns, "columns", "Comma-separated list of columns to display")
	listCmd.Flags().Var(listOptions.SortBy, "sort", "Column to sort by")
	listCmd.Flags().Int("page-length", 0, "Number of items per page to retrieve from Bitbucket. Default is the profile's default page length")
	listCmd.Flags().Int("limit", 0, "Maximum total number of steps to retrieve. Default is to retrieve all of them")
	_ = listCmd.RegisterFlagCompletionFunc(listOptions.Columns.CompletionFunc("columns"))
	_ = listCmd.RegisterFlagCompletionFunc(listOptions.SortBy.CompletionFunc("sort"))
}

func listProcess(cmd *cobra.Command, args []string) error {
	pipelineID := pipelineFlagValue(cmd)

	repo, err := repository.GetRepository(cmd.Context(), cmd)
	if err != nil {
		return fmt.Errorf("cannot get repository: %w", err)
	}

	lgr.Printf("[DEBUG] listing steps for pipeline %s", pipelineID)
	if !common.WhatIf(cmd, "Showing steps for pipeline %s", pipelineID) {
		return nil
	}

	steps, err := profile.GetAll[Step](cmd.Context(), cmd, repo.GetPath("pipelines", pipelineID, "steps"))
	if err != nil {
		return fmt.Errorf("cannot get steps: %w", err)
	}
	if len(steps) == 0 {
		fmt.Println("No step found")
		return nil
	}
	if sortFlag := cmd.Flag("sort"); sortFlag != nil && sortFlag.Changed {
		core.Sort(steps, columns.SortBy(listOptions.SortBy.Value))
	}
	if err := profile.Current.Print(cmd.Context(), cmd, Steps(steps)); err != nil {
		return fmt.Errorf("cannot print result: %w", err)
	}
	return nil
}
