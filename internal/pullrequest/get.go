package pullrequest

import (
	"fmt"

	"github.com/avitsrimer/bitbucket-cli/internal/common"
	"github.com/avitsrimer/bitbucket-cli/internal/profile"
	"github.com/avitsrimer/bitbucket-cli/internal/repository"
	"github.com/go-pkgz/lgr"
	"github.com/spf13/cobra"
)

var getCmd = &cobra.Command{
	Use:               "get [flags] <pullrequest-id>",
	Aliases:           []string{"show", "info", "display"},
	Short:             "get a pullrequest by its <pullrequest-id>.",
	Args:              cobra.ExactArgs(1),
	ValidArgsFunction: allPullRequestIDsCompletion,
	RunE:              getProcess,
}

func init() {
	Command.AddCommand(getCmd)

	common.RegisterColumnsFlag(getCmd, columns)
}

func getProcess(cmd *cobra.Command, args []string) error {
	if err := common.ValidatePathIdentifier("pullrequest-id", args[0]); err != nil {
		return fmt.Errorf("cannot get pullrequest: %w", err)
	}

	profile, err := profile.GetProfileFromCommand(cmd.Context(), cmd)
	if err != nil {
		return fmt.Errorf("cannot get profile: %w", err)
	}

	repository, err := repository.GetRepository(cmd.Context(), cmd)
	if err != nil {
		return fmt.Errorf("cannot get repository: %w", err)
	}

	lgr.Printf("[DEBUG] displaying pull request %s", args[0])
	if !common.WhatIf(cmd, "Showing pull request "+args[0]) {
		return nil
	}
	var pullrequest PullRequest

	err = profile.Get(
		cmd.Context(),
		repository.GetPath("pullrequests", args[0]),
		&pullrequest,
	)
	if err != nil {
		return fmt.Errorf("failed to get pullrequest %s: %w", args[0], err)
	}

	if err := profile.Print(cmd.Context(), cmd, pullrequest); err != nil {
		return fmt.Errorf("cannot print result: %w", err)
	}
	return nil
}
