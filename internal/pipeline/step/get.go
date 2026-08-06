package step

import (
	"fmt"

	"github.com/avitsrimer/bitbucket-cli/internal/common"
	"github.com/avitsrimer/bitbucket-cli/internal/profile"
	"github.com/avitsrimer/bitbucket-cli/internal/repository"
	"github.com/go-pkgz/lgr"
	"github.com/spf13/cobra"
)

var getCmd = &cobra.Command{
	Use:               "get [flags] <pipeline-step-uuid-or-name>",
	Aliases:           []string{"show", "info", "display"},
	Short:             "get a pipeline step by its UUID or name",
	Args:              cobra.ExactArgs(1),
	ValidArgsFunction: stepValidArgs,
	RunE:              getProcess,
}

var getOptions struct {
	Columns *common.EnumSliceFlag
}

func init() {
	Command.AddCommand(getCmd)

	registerPipelineFlag(getCmd, "Pipeline to get the step from")
	getOptions.Columns = common.NewEnumSliceFlag(columns.Columns()...)
	getCmd.Flags().Var(getOptions.Columns, "columns", "Comma-separated list of columns to display")
	_ = getCmd.RegisterFlagCompletionFunc(getOptions.Columns.CompletionFunc("columns"))
}

func getProcess(cmd *cobra.Command, args []string) error {
	pipelineID := pipelineFlagValue(cmd)

	repo, err := repository.GetRepository(cmd.Context(), cmd)
	if err != nil {
		return fmt.Errorf("cannot get repository: %w", err)
	}

	lgr.Printf("[DEBUG] displaying pipeline step %s for pipeline %s", args[0], pipelineID)
	if !common.WhatIf(cmd, "Showing pipeline step %s for pipeline %s", args[0], pipelineID) {
		return nil
	}

	profileCurrent, err := profile.GetProfileFromCommand(cmd.Context(), cmd)
	if err != nil {
		return fmt.Errorf("cannot get profile: %w", err)
	}

	var target Step
	if err := profileCurrent.Get(cmd.Context(), repo.GetPath("pipelines", pipelineID, "steps", args[0]), &target); err != nil {
		return fmt.Errorf("cannot get step %s: %w", args[0], err)
	}
	if err := profileCurrent.Print(cmd.Context(), cmd, target); err != nil {
		return fmt.Errorf("cannot print result: %w", err)
	}
	return nil
}
