package pipeline

import (
	"fmt"

	"github.com/avitsrimer/bitbucket-cli/internal/common"
	"github.com/avitsrimer/bitbucket-cli/internal/profile"
	"github.com/avitsrimer/bitbucket-cli/internal/repository"
	"github.com/go-pkgz/lgr"
	"github.com/spf13/cobra"
)

var stopCmd = &cobra.Command{
	Use:     "stop [flags] <pipeline-uuid-or-build-number>",
	Aliases: []string{"cancel", "abort"},
	Short:   "stop a running pipeline",
	Args:    cobra.ExactArgs(1),
	RunE:    stopProcess,
}

func init() {
	Command.AddCommand(stopCmd)

	stopCmd.Flags().Bool("force", false, "Skip the confirmation prompt")
}

func stopProcess(cmd *cobra.Command, args []string) error {
	pipelineID := args[0]
	if err := common.ValidatePathIdentifier("pipeline", pipelineID); err != nil {
		return fmt.Errorf("cannot stop pipeline: %w", err)
	}

	repo, err := repository.GetRepository(cmd.Context(), cmd)
	if err != nil {
		return fmt.Errorf("cannot get repository: %w", err)
	}

	profileCurrent, err := profile.GetProfileFromCommand(cmd.Context(), cmd)
	if err != nil {
		return fmt.Errorf("cannot get profile: %w", err)
	}

	if getErr := profileCurrent.Get(cmd.Context(), repo.GetPath("pipelines", pipelineID), nil); getErr != nil {
		return fmt.Errorf("cannot stop pipeline %s: %w", pipelineID, getErr)
	}

	lgr.Printf("[DEBUG] stopping pipeline %s", pipelineID)
	proceed, err := common.Confirm(cmd, fmt.Sprintf("Stop pipeline %s?", pipelineID))
	if err != nil {
		return fmt.Errorf("cannot confirm pipeline stop: %w", err)
	}
	if !proceed {
		fmt.Println("Stop canceled")
		return nil
	}

	uripath := repo.GetPath("pipelines", pipelineID, "stopPipeline")
	if !common.WhatIfPayload(cmd, uripath, nil, "Stopping pipeline %s", pipelineID) {
		return nil
	}

	result, err := profileCurrent.PostWithResult(cmd.Context(), uripath, nil)
	if err != nil {
		return fmt.Errorf("cannot stop pipeline %s: %w", pipelineID, err)
	}
	fmt.Printf("Pipeline %s stop request: %s\n", pipelineID, result.StatusText)
	return nil
}
