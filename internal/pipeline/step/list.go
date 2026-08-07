package step

import (
	"fmt"

	"github.com/avitsrimer/bitbucket-cli/internal/common"
	"github.com/avitsrimer/bitbucket-cli/internal/profile"
	"github.com/avitsrimer/bitbucket-cli/internal/repository"
	"github.com/go-pkgz/lgr"
	"github.com/spf13/cobra"
)

var listCmd = &cobra.Command{
	Use:               "list [flags] <pipeline>",
	Short:             "list the steps of a pipeline",
	Args:              cobra.ExactArgs(1),
	ValidArgsFunction: pipelineValidArgs,
	RunE:              listProcess,
}

func init() {
	Command.AddCommand(listCmd)

	common.RegisterListFlags(listCmd, columns, "steps")
}

func listProcess(cmd *cobra.Command, args []string) error {
	pipelineID := args[0]
	if err := common.ValidatePathIdentifier("pipeline", pipelineID); err != nil {
		return fmt.Errorf("cannot list steps: %w", err)
	}

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
	if sortValue := common.SortFlagValue(cmd); sortValue != "" {
		common.Sort(steps, columns.SortBy(sortValue))
	}
	if err := profile.Current.Print(cmd.Context(), cmd, Steps(steps)); err != nil {
		return fmt.Errorf("cannot print result: %w", err)
	}
	return nil
}
