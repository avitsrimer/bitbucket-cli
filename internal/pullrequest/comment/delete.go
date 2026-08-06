package comment

import (
	"fmt"

	"github.com/avitsrimer/bitbucket-cli/internal/common"
	prcommon "github.com/avitsrimer/bitbucket-cli/internal/pullrequest/common"
	"github.com/avitsrimer/bitbucket-cli/internal/repository"
	"github.com/spf13/cobra"
)

var deleteCmd = &cobra.Command{
	Use:               "delete [flags] <pullrequest-id> <comment-id...>",
	Aliases:           []string{"remove", "rm"},
	Short:             "delete pullrequest comments by their <comment-id> on the pullrequest identified by <pullrequest-id>.",
	Args:              cobra.MinimumNArgs(2),
	ValidArgsFunction: deleteValidArgs,
	RunE:              deleteProcess,
}

func init() {
	Command.AddCommand(deleteCmd)
}

func deleteValidArgs(cmd *cobra.Command, args []string, toComplete string) ([]string, cobra.ShellCompDirective) {
	if len(args) == 0 {
		return prcommon.PullRequestIDValidArgs(cmd, args, toComplete)
	}

	commentIDs, err := GetPullRequestCommentIDs(cmd.Context(), cmd, args[0])
	if err != nil {
		return []string{}, cobra.ShellCompDirectiveNoFileComp
	}
	return common.FilterValidArgs(commentIDs, args[1:], toComplete), cobra.ShellCompDirectiveNoFileComp
}

func deleteProcess(cmd *cobra.Command, args []string) error {
	pullRequestID, commentIDs := args[0], args[1:]
	if err := common.ValidatePathIdentifier("pullrequest-id", pullRequestID); err != nil {
		return fmt.Errorf("cannot delete comments: %w", err)
	}

	repo, err := repository.GetRepository(cmd.Context(), cmd)
	if err != nil {
		return fmt.Errorf("cannot get repository: %w", err)
	}

	return prcommon.DeleteSubResources(cmd, repo, pullRequestID, "comments", commentIDs, "comment", "comments") //nolint:wrapcheck // DeleteSubResources returns the same joined error TolerateErrors produces (or nil); wrapping would prefix it with redundant noise
}
