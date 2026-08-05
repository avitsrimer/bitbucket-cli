package task

import (
	"fmt"

	"github.com/gildas/bitbucket-cli/cmd/common"
	"github.com/gildas/bitbucket-cli/cmd/profile"
	prcommon "github.com/gildas/bitbucket-cli/cmd/pullrequest/common"
	"github.com/gildas/bitbucket-cli/cmd/repository"
	"github.com/go-pkgz/lgr"
	"github.com/spf13/cobra"
)

type TaskUpdator struct {
	Content *ContentUpdator `json:"content,omitempty" mapstructure:"content,omitempty"`
	State   string          `json:"state,omitempty"   mapstructure:"state,omitempty"`
}

type ContentUpdator struct {
	Raw string `json:"raw" mapstructure:"raw"`
}

var updateCmd = &cobra.Command{
	Use:               "update [flags] <task-id>",
	Aliases:           []string{"edit"},
	Short:             "update a pullrequest task by its <task-id>.",
	Args:              cobra.ExactArgs(1),
	ValidArgsFunction: updateValidArgs,
	RunE:              updateProcess,
}

var updateOptions struct {
	PullRequestID *common.EnumFlag
	Content       string
	State         *common.EnumFlag
}

func init() {
	Command.AddCommand(updateCmd)

	updateOptions.PullRequestID = common.NewEnumFlagWithFunc(updateCmd, "", prcommon.GetPullRequestIDs)
	updateOptions.State = common.NewEnumFlag("RESOLVED", "UNRESOLVED")
	updateCmd.Flags().Var(updateOptions.PullRequestID, "pullrequest", "Pullrequest to update tasks to")
	updateCmd.Flags().StringVar(&updateOptions.Content, "content", "", "Updated content of the task")
	updateCmd.Flags().Var(updateOptions.State, "state", "Updated state of the task. Can be one of RESOLVED or UNRESOLVED")
	_ = updateCmd.MarkFlagRequired("pullrequest")
	_ = updateCmd.RegisterFlagCompletionFunc(updateOptions.PullRequestID.CompletionFunc("pullrequest"))
	_ = updateCmd.RegisterFlagCompletionFunc(updateOptions.State.CompletionFunc("state"))
}

func updateValidArgs(cmd *cobra.Command, args []string, toComplete string) ([]string, cobra.ShellCompDirective) {
	if len(args) != 0 {
		return nil, cobra.ShellCompDirectiveNoFileComp
	}

	taskIDs, err := GetPullRequestTaskIDs(cmd.Context(), cmd, updateOptions.PullRequestID.Value)
	if err != nil {
		return []string{}, cobra.ShellCompDirectiveNoFileComp
	}
	return taskIDs, cobra.ShellCompDirectiveNoFileComp
}

func updateProcess(cmd *cobra.Command, args []string) error {
	profile, err := profile.GetProfileFromCommand(cmd.Context(), cmd)
	if err != nil {
		return fmt.Errorf("cannot get profile: %w", err)
	}

	repository, err := repository.GetRepository(cmd.Context(), cmd)
	if err != nil {
		return fmt.Errorf("cannot get repository: %w", err)
	}

	taskID := args[0]

	taskUpdator := TaskUpdator{}
	if updateOptions.Content != "" {
		taskUpdator.Content = &ContentUpdator{
			Raw: updateOptions.Content,
		}
	}
	if cmd.Flags().Changed("state") && updateOptions.State.Value != "" {
		taskUpdator.State = updateOptions.State.Value
	}

	lgr.Printf("[DEBUG] updating pullrequest task %s on pullrequest %s", taskID, updateOptions.PullRequestID.Value)
	if !common.WhatIf(cmd, fmt.Sprintf("Updating pullrequest task %s on pullrequest %s", taskID, updateOptions.PullRequestID.Value)) {
		return nil
	}

	var updated Task

	err = profile.Put(
		cmd.Context(),
		cmd,
		repository.GetPath("pullrequests", updateOptions.PullRequestID.Value, "tasks", taskID),
		taskUpdator,
		&updated,
	)
	if err != nil {
		return fmt.Errorf("failed to update pull request task %s on pull request %s: %w", taskID, updateOptions.PullRequestID.Value, err)
	}
	if err := profile.Print(cmd.Context(), cmd, updated); err != nil {
		return fmt.Errorf("cannot print result: %w", err)
	}
	return nil
}
