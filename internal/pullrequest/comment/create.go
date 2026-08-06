package comment

import (
	"fmt"

	"github.com/avitsrimer/bitbucket-cli/internal/common"
	"github.com/avitsrimer/bitbucket-cli/internal/profile"
	prcommon "github.com/avitsrimer/bitbucket-cli/internal/pullrequest/common"
	"github.com/avitsrimer/bitbucket-cli/internal/repository"
	"github.com/go-pkgz/lgr"
	"github.com/spf13/cobra"
)

var createCmd = &cobra.Command{
	Use:               "create [flags] <pullrequest-id>",
	Aliases:           []string{"add", "new"},
	Short:             "create a pullrequest comment on the pullrequest identified by <pullrequest-id>.",
	Args:              cobra.ExactArgs(1),
	ValidArgsFunction: createValidArgs,
	RunE:              createProcess,
}

var createOptions commentEditOptions

func init() {
	Command.AddCommand(createCmd)

	registerCommentEditFlags(createCmd, &createOptions, "Comment of the pullrequest")
}

func createValidArgs(cmd *cobra.Command, args []string, toComplete string) ([]string, cobra.ShellCompDirective) {
	if len(args) != 0 {
		return nil, cobra.ShellCompDirectiveNoFileComp
	}

	ids, err := prcommon.GetPullRequestIDs(cmd.Context(), cmd, args, toComplete)
	if err != nil {
		return []string{}, cobra.ShellCompDirectiveNoFileComp
	}
	return common.FilterValidArgs(ids, args, toComplete), cobra.ShellCompDirectiveNoFileComp
}

func createProcess(cmd *cobra.Command, args []string) (err error) {
	pullRequestID := args[0]
	if validateErr := common.ValidatePathIdentifier("pullrequest-id", pullRequestID); validateErr != nil {
		return fmt.Errorf("cannot create comment: %w", validateErr)
	}

	profile, err := profile.GetProfileFromCommand(cmd.Context(), cmd)
	if err != nil {
		return fmt.Errorf("cannot get profile: %w", err)
	}

	repository, err := repository.GetRepository(cmd.Context(), cmd)
	if err != nil {
		return fmt.Errorf("cannot get repository: %w", err)
	}

	payload, err := createOptions.payload(cmd)
	if err != nil {
		return err
	}

	lgr.Printf("[DEBUG] creating pullrequest comment")
	if !common.WhatIf(cmd, "Creating comment for pullrequest %s", pullRequestID) {
		return nil
	}
	var comment Comment

	err = profile.Post(
		cmd.Context(),
		repository.GetPath("pullrequests", pullRequestID, "comments"),
		payload,
		&comment,
	)
	if err != nil {
		return fmt.Errorf("failed to create comment for pullrequest %s: %w", pullRequestID, err)
	}
	if err := profile.Print(cmd.Context(), cmd, comment); err != nil {
		return fmt.Errorf("cannot print result: %w", err)
	}
	return nil
}
