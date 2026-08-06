package task

import (
	"fmt"

	"github.com/avitsrimer/bitbucket-cli/internal/common"
	prcommon "github.com/avitsrimer/bitbucket-cli/internal/pullrequest/common"
	"github.com/avitsrimer/bitbucket-cli/internal/repository"
	"github.com/spf13/cobra"
)

var deleteCmd = &cobra.Command{
	Use:               "delete [flags] <pullrequest-id> <task-id...>",
	Aliases:           []string{"remove", "rm"},
	Short:             "delete pullrequest tasks by their <task-id> on the pullrequest identified by <pullrequest-id>.",
	Args:              cobra.MinimumNArgs(2),
	ValidArgsFunction: deleteValidArgs,
	RunE:              deleteProcess,
}

func init() {
	Command.AddCommand(deleteCmd)
}

func deleteValidArgs(cmd *cobra.Command, args []string, toComplete string) ([]string, cobra.ShellCompDirective) {
	if len(args) == 0 {
		ids, err := prcommon.GetPullRequestIDs(cmd.Context(), cmd, args, toComplete)
		if err != nil {
			return []string{}, cobra.ShellCompDirectiveNoFileComp
		}
		return common.FilterValidArgs(ids, args, toComplete), cobra.ShellCompDirectiveNoFileComp
	}

	taskIDs, err := GetPullRequestTaskIDs(cmd.Context(), cmd, args[0])
	if err != nil {
		return []string{}, cobra.ShellCompDirectiveNoFileComp
	}
	return common.FilterValidArgs(taskIDs, args[1:], toComplete), cobra.ShellCompDirectiveNoFileComp
}

func deleteProcess(cmd *cobra.Command, args []string) error {
	pullRequestID, taskIDs := args[0], args[1:]
	if err := common.ValidatePathIdentifier("pullrequest-id", pullRequestID); err != nil {
		return fmt.Errorf("cannot delete tasks: %w", err)
	}

	repo, err := repository.GetRepository(cmd.Context(), cmd)
	if err != nil {
		return fmt.Errorf("cannot get repository: %w", err)
	}

	return prcommon.DeleteSubResources(cmd, repo, pullRequestID, "tasks", taskIDs, "task", "tasks") //nolint:wrapcheck // DeleteSubResources returns the same joined error TolerateErrors produces (or nil); wrapping would prefix it with redundant noise
}
