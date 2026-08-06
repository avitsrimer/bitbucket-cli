package task

import (
	"fmt"

	"github.com/avitsrimer/bitbucket-cli/internal/common"
	"github.com/avitsrimer/bitbucket-cli/internal/profile"
	"github.com/avitsrimer/bitbucket-cli/internal/repository"
	"github.com/go-pkgz/lgr"
	"github.com/spf13/cobra"
)

var getCmd = &cobra.Command{
	Use:               "get [flags] <pullrequest-id> <task-id>",
	Aliases:           []string{"show", "info", "display"},
	Short:             "get a pullrequest task by its <task-id> on the pullrequest identified by <pullrequest-id>.",
	Args:              cobra.ExactArgs(2),
	ValidArgsFunction: pullRequestAndTaskIDValidArgs,
	RunE:              getProcess,
}

func init() {
	Command.AddCommand(getCmd)

	common.RegisterColumnsFlag(getCmd, columns)
}

func getProcess(cmd *cobra.Command, args []string) (err error) {
	pullRequestID, taskID := args[0], args[1]
	if validateErr := common.ValidatePathIdentifier("pullrequest-id", pullRequestID); validateErr != nil {
		return fmt.Errorf("cannot get task: %w", validateErr)
	}

	profile, err := profile.GetProfileFromCommand(cmd.Context(), cmd)
	if err != nil {
		return fmt.Errorf("cannot get profile: %w", err)
	}

	repository, err := repository.GetRepository(cmd.Context(), cmd)
	if err != nil {
		return fmt.Errorf("cannot get repository: %w", err)
	}

	lgr.Printf("[DEBUG] displaying pullrequest task %s", taskID)
	if !common.WhatIf(cmd, "Showing pullrequest task "+taskID) {
		return nil
	}

	var task Task

	err = profile.Get(
		cmd.Context(),
		repository.GetPath("pullrequests", pullRequestID, "tasks", taskID),
		&task,
	)
	if err != nil {
		return fmt.Errorf("failed to get pullrequest task %s: %w", taskID, err)
	}
	if err := profile.Print(cmd.Context(), cmd, task); err != nil {
		return fmt.Errorf("cannot print result: %w", err)
	}
	return nil
}
