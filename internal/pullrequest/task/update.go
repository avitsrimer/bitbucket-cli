package task

import (
	"fmt"

	"github.com/avitsrimer/bitbucket-cli/internal/common"
	"github.com/avitsrimer/bitbucket-cli/internal/profile"
	"github.com/avitsrimer/bitbucket-cli/internal/repository"
	"github.com/go-pkgz/lgr"
	"github.com/spf13/cobra"
)

type TaskUpdater struct {
	Content *ContentUpdater `json:"content,omitempty"`
	State   string          `json:"state,omitempty"`
}

type ContentUpdater struct {
	Raw string `json:"raw"`
}

var updateCmd = &cobra.Command{
	Use:               "update [flags] <pullrequest-id> <task-id>",
	Aliases:           []string{"edit"},
	Short:             "update a pullrequest task by its <task-id> on the pullrequest identified by <pullrequest-id>.",
	Args:              cobra.ExactArgs(2),
	ValidArgsFunction: pullRequestAndTaskIDValidArgs,
	RunE:              updateProcess,
}

var updateOptions struct {
	Content string
	State   *common.EnumFlag
}

func init() {
	Command.AddCommand(updateCmd)

	updateOptions.State = common.NewEnumFlag("RESOLVED", "UNRESOLVED")
	updateCmd.Flags().StringVar(&updateOptions.Content, "content", "", "Updated content of the task")
	updateCmd.Flags().Var(updateOptions.State, "state", "Updated state of the task. Can be one of RESOLVED or UNRESOLVED")
	_ = updateCmd.RegisterFlagCompletionFunc(updateOptions.State.CompletionFunc("state"))
}

func updateProcess(cmd *cobra.Command, args []string) error {
	pullRequestID, taskID := args[0], args[1]
	if validateErr := common.ValidatePathIdentifier("pullrequest-id", pullRequestID); validateErr != nil {
		return fmt.Errorf("cannot update task: %w", validateErr)
	}

	profile, err := profile.GetProfileFromCommand(cmd.Context(), cmd)
	if err != nil {
		return fmt.Errorf("cannot get profile: %w", err)
	}

	repository, err := repository.GetRepository(cmd.Context(), cmd)
	if err != nil {
		return fmt.Errorf("cannot get repository: %w", err)
	}

	uripath := repository.GetPath("pullrequests", pullRequestID, "tasks", taskID)
	if err = profile.Get(cmd.Context(), uripath, nil); err != nil {
		return fmt.Errorf("failed to get task %s of pullrequest %s: %w", taskID, pullRequestID, err)
	}

	taskUpdater := TaskUpdater{}
	if updateOptions.Content != "" {
		taskUpdater.Content = &ContentUpdater{
			Raw: updateOptions.Content,
		}
	}
	if cmd.Flags().Changed("state") && updateOptions.State.Value != "" {
		taskUpdater.State = updateOptions.State.Value
	}

	lgr.Printf("[DEBUG] updating pullrequest task %s on pullrequest %s", taskID, pullRequestID)
	if !common.WhatIfPayload(cmd, uripath, taskUpdater, "Updating pullrequest task %s on pullrequest %s", taskID, pullRequestID) {
		return nil
	}

	var updated Task

	err = profile.Put(
		cmd.Context(),
		uripath,
		taskUpdater,
		&updated,
	)
	if err != nil {
		return fmt.Errorf("failed to update pull request task %s on pull request %s: %w", taskID, pullRequestID, err)
	}
	if err := profile.Print(cmd.Context(), cmd, updated); err != nil {
		return fmt.Errorf("cannot print result: %w", err)
	}
	return nil
}
