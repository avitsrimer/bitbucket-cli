package pullrequest

import (
	"fmt"
	"io"
	"os"

	"github.com/avitsrimer/bitbucket-cli/internal/common"
	"github.com/avitsrimer/bitbucket-cli/internal/profile"
	prcommon "github.com/avitsrimer/bitbucket-cli/internal/pullrequest/common"
	"github.com/avitsrimer/bitbucket-cli/internal/repository"
	"github.com/go-pkgz/lgr"
	"github.com/spf13/cobra"
)

var patchCmd = &cobra.Command{
	Use:               "patch [flags] <pullrequest-id>",
	Short:             "show the patch of a pull request by its <pullrequest-id>. If not provided, it will try to show the patch of the only open pullrequest.",
	Args:              cobra.MaximumNArgs(1),
	ValidArgsFunction: validPatchArgs,
	RunE:              patchProcess,
}

func init() {
	Command.AddCommand(patchCmd)
}

func validPatchArgs(cmd *cobra.Command, args []string, toComplete string) ([]string, cobra.ShellCompDirective) {
	if len(args) != 0 {
		return nil, cobra.ShellCompDirectiveNoFileComp
	}

	ids, err := prcommon.GetPullRequestIDsWithState(cmd.Context(), cmd, "OPEN")
	if err != nil {
		cobra.CompErrorln(err.Error())
		return []string{}, cobra.ShellCompDirectiveError
	}
	return common.FilterValidArgs(ids, args, toComplete), cobra.ShellCompDirectiveNoFileComp
}

func patchProcess(cmd *cobra.Command, args []string) error {
	profile, err := profile.GetProfileFromCommand(cmd.Context(), cmd)
	if err != nil {
		return fmt.Errorf("cannot get profile: %w", err)
	}

	repository, err := repository.GetRepository(cmd.Context(), cmd)
	if err != nil {
		return fmt.Errorf("cannot show patch of pull request: %w", err)
	}

	pullRequestID, err := GetPullRequestIDFromArgs(cmd.Context(), cmd, repository, args)
	if err != nil {
		return fmt.Errorf("cannot show patch of pull request: %w", err)
	}

	lgr.Printf("[DEBUG] displaying patch for pull request ID: %s", pullRequestID)
	if !common.WhatIf(cmd, "Showing patch for pull request "+pullRequestID) {
		return nil
	}

	patch, err := profile.GetRaw(cmd.Context(), repository.GetPath("pullrequests", pullRequestID, "patch"))
	if err != nil {
		return fmt.Errorf("cannot get resource: %w", err)
	}

	if _, err := io.Copy(os.Stdout, patch); err != nil {
		return fmt.Errorf("cannot write output: %w", err)
	}
	return nil
}
