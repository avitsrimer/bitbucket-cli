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
	Use:               "get [flags] <pipeline> <pipeline-step-uuid-or-name>",
	Aliases:           []string{"show", "info", "display"},
	Short:             "get a pipeline step by its UUID or name",
	Args:              cobra.ExactArgs(2),
	ValidArgsFunction: pipelineAndStepValidArgs,
	RunE:              getProcess,
}

func init() {
	Command.AddCommand(getCmd)

	common.RegisterColumnsFlag(getCmd, columns)
}

func getProcess(cmd *cobra.Command, args []string) error {
	pipelineID, stepArg := args[0], args[1]
	if err := common.ValidatePathIdentifier("pipeline", pipelineID); err != nil {
		return fmt.Errorf("cannot get step: %w", err)
	}
	if err := common.ValidatePathIdentifier("pipeline-step-uuid-or-name", stepArg); err != nil {
		return fmt.Errorf("cannot get step: %w", err)
	}

	repo, err := repository.GetRepository(cmd.Context(), cmd)
	if err != nil {
		return fmt.Errorf("cannot get repository: %w", err)
	}

	lgr.Printf("[DEBUG] displaying pipeline step %s for pipeline %s", stepArg, pipelineID)
	if !common.WhatIf(cmd, "Showing pipeline step %s for pipeline %s", stepArg, pipelineID) {
		return nil
	}

	stepID, err := resolveStepID(cmd.Context(), cmd, repo, pipelineID, stepArg)
	if err != nil {
		return fmt.Errorf("cannot resolve step %s: %w", stepArg, err)
	}

	profileCurrent, err := profile.GetProfileFromCommand(cmd.Context(), cmd)
	if err != nil {
		return fmt.Errorf("cannot get profile: %w", err)
	}

	var target Step
	if err := profileCurrent.Get(cmd.Context(), repo.GetPath("pipelines", pipelineID, "steps", stepID), &target); err != nil {
		return fmt.Errorf("cannot get step %s: %w", stepArg, err)
	}
	if err := profileCurrent.Print(cmd.Context(), cmd, target); err != nil {
		return fmt.Errorf("cannot print result: %w", err)
	}
	return nil
}
