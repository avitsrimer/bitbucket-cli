package comment

import (
	"fmt"

	"github.com/gildas/bitbucket-cli/cmd/common"
	"github.com/gildas/bitbucket-cli/cmd/profile"
	prcommon "github.com/gildas/bitbucket-cli/cmd/pullrequest/common"
	"github.com/gildas/bitbucket-cli/cmd/repository"
	"github.com/go-pkgz/lgr"
	"github.com/spf13/cobra"
)

var resolveCmd = &cobra.Command{
	Use:               "resolve [flags] <comment-id>",
	Short:             "resolve a pullrequest comment by its <comment-id>.",
	Args:              cobra.ExactArgs(1),
	ValidArgsFunction: resolveValidArgs,
	RunE:              resolveProcess,
}

var resolveOptions struct {
	PullRequestID *common.EnumFlag
}

func init() {
	Command.AddCommand(resolveCmd)

	resolveOptions.PullRequestID = common.NewEnumFlagWithFunc(resolveCmd, "", prcommon.GetPullRequestIDs)
	resolveCmd.Flags().Var(resolveOptions.PullRequestID, "pullrequest", "Pullrequest to resolve comments from")
	_ = resolveCmd.MarkFlagRequired("pullrequest")
	_ = resolveCmd.RegisterFlagCompletionFunc(resolveOptions.PullRequestID.CompletionFunc("pullrequest"))
}

func resolveValidArgs(cmd *cobra.Command, args []string, toComplete string) ([]string, cobra.ShellCompDirective) {
	if len(args) != 0 {
		return nil, cobra.ShellCompDirectiveNoFileComp
	}

	commentIDs, err := GetPullRequestCommentIDs(cmd.Context(), cmd, args, toComplete)
	if err != nil {
		return []string{}, cobra.ShellCompDirectiveNoFileComp
	}
	return commentIDs, cobra.ShellCompDirectiveNoFileComp
}

func resolveProcess(cmd *cobra.Command, args []string) (err error) {
	profile, err := profile.GetProfileFromCommand(cmd.Context(), cmd)
	if err != nil {
		return fmt.Errorf("cannot get profile: %w", err)
	}

	repository, err := repository.GetRepository(cmd.Context(), cmd)
	if err != nil {
		return fmt.Errorf("cannot get repository: %w", err)
	}

	if !common.WhatIf(cmd, "Resolving comment %s from pullrequest %s", args[0], resolveOptions.PullRequestID.Value) {
		return nil
	}

	err = profile.Post(
		cmd.Context(),
		cmd,
		repository.GetPath("pullrequests", resolveOptions.PullRequestID.Value, "comments", args[0], "resolve"),
		nil,
		nil,
	)
	if err != nil {
		return fmt.Errorf("failed to resolve pullrequest comment %s: %w", args[0], err)
	}
	lgr.Printf("[DEBUG] pullrequest comment %s resolved", args[0])
	return nil
}
