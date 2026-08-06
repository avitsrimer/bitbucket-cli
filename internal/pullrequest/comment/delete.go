package comment

import (
	"fmt"

	"github.com/avitsrimer/bitbucket-cli/internal/common"
	prcommon "github.com/avitsrimer/bitbucket-cli/internal/pullrequest/common"
	"github.com/avitsrimer/bitbucket-cli/internal/repository"
	"github.com/spf13/cobra"
)

var deleteCmd = &cobra.Command{
	Use:               "delete [flags] <comment-id...>",
	Aliases:           []string{"remove", "rm"},
	Short:             "delete pullrequest comments by their <comment-id>.",
	Args:              cobra.MinimumNArgs(1),
	ValidArgsFunction: deleteValidArgs,
	RunE:              deleteProcess,
}

var deleteOptions struct {
	PullRequestID *common.EnumFlag
}

func init() {
	Command.AddCommand(deleteCmd)

	deleteOptions.PullRequestID = common.NewEnumFlagWithFunc("", prcommon.GetPullRequestIDs)
	deleteCmd.Flags().Var(deleteOptions.PullRequestID, "pullrequest", "Pullrequest to delete comments from")
	_ = deleteCmd.MarkFlagRequired("pullrequest")
	_ = deleteCmd.RegisterFlagCompletionFunc(deleteOptions.PullRequestID.CompletionFunc("pullrequest"))
}

func deleteValidArgs(cmd *cobra.Command, args []string, toComplete string) ([]string, cobra.ShellCompDirective) {
	if len(args) != 0 {
		return nil, cobra.ShellCompDirectiveNoFileComp
	}

	commentIDs, err := GetPullRequestCommentIDs(cmd.Context(), cmd, args, toComplete)
	if err != nil {
		return []string{}, cobra.ShellCompDirectiveNoFileComp
	}
	return commentIDs, cobra.ShellCompDirectiveNoFileComp
}

func deleteProcess(cmd *cobra.Command, args []string) error {
	repo, err := repository.GetRepository(cmd.Context(), cmd)
	if err != nil {
		return fmt.Errorf("cannot get repository: %w", err)
	}

	return prcommon.DeleteSubResources(cmd, repo, deleteOptions.PullRequestID.Value, "comments", args, "comment", "comments") //nolint:wrapcheck // DeleteSubResources returns the same joined error TolerateErrors produces (or nil); wrapping would prefix it with redundant noise
}
