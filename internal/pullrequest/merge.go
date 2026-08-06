package pullrequest

import (
	"fmt"

	"github.com/avitsrimer/bitbucket-cli/internal/common"
	"github.com/avitsrimer/bitbucket-cli/internal/profile"
	prcommon "github.com/avitsrimer/bitbucket-cli/internal/pullrequest/common"
	"github.com/avitsrimer/bitbucket-cli/internal/repository"
	"github.com/go-pkgz/lgr"
	"github.com/spf13/cobra"
)

var mergeCmd = &cobra.Command{
	Use:               "merge [flags] <pullrequest-id>",
	Short:             "merge a pullrequest by its <pullrequest-id>. If not provided, it will try to merge the only open pullrequest.",
	Args:              cobra.MaximumNArgs(1),
	ValidArgsFunction: mergeValidArgs,
	RunE:              mergeProcess,
}

var mergeOptions struct {
	Async             bool
	Message           string
	MergeStrategy     *common.EnumFlag
	CloseSourceBranch bool
}

func init() {
	Command.AddCommand(mergeCmd)

	mergeOptions.MergeStrategy = common.NewEnumFlag("+merge_commit", "squash", "fast_forward")
	mergeCmd.Flags().StringVar(&mergeOptions.Message, "message", "", "Message of the merge")
	mergeCmd.Flags().BoolVar(&mergeOptions.CloseSourceBranch, "close-source-branch", false, "Close the source branch of the pullrequest")
	mergeCmd.Flags().BoolVar(&mergeOptions.Async, "async", false, "Perform the merge asynchronously")
	mergeCmd.Flags().Var(mergeOptions.MergeStrategy, "merge-strategy", "Merge strategy to use. Possible values are \"merge_commit\", \"squash\" or \"fast_forward\"")
	_ = mergeCmd.RegisterFlagCompletionFunc(mergeOptions.MergeStrategy.CompletionFunc("merge-strategy"))
}

func mergeValidArgs(cmd *cobra.Command, args []string, toComplete string) ([]string, cobra.ShellCompDirective) {
	return openPullRequestIDsCompletion(cmd, args, toComplete)
}

func mergeProcess(cmd *cobra.Command, args []string) (err error) {
	profile, err := profile.GetProfileFromCommand(cmd.Context(), cmd)
	if err != nil {
		return fmt.Errorf("cannot merge pull request: %w", err)
	}

	repository, err := repository.GetRepository(cmd.Context(), cmd)
	if err != nil {
		return fmt.Errorf("cannot merge pull request: %w", err)
	}

	pullRequestID, err := GetPullRequestIDFromArgs(cmd.Context(), cmd, repository, args)
	if err != nil {
		return fmt.Errorf("cannot merge pull request: %w", err)
	}

	if err = prcommon.ExistsPullRequest(cmd.Context(), cmd, repository, pullRequestID); err != nil {
		return fmt.Errorf("cannot merge pull request: %w", err)
	}

	uripath := repository.GetPath("pullrequests", pullRequestID, "merge")

	if mergeOptions.Async {
		uripath += "?async=true"
	}

	payload := struct {
		Message           string `json:"message,omitempty"`
		CloseSourceBranch bool   `json:"close_source_branch"`
		MergeStrategy     string `json:"merge_strategy"`
	}{
		Message:           mergeOptions.Message,
		CloseSourceBranch: mergeOptions.CloseSourceBranch,
		MergeStrategy:     mergeOptions.MergeStrategy.String(),
	}

	lgr.Printf("[DEBUG] merging pullrequest %s", pullRequestID)
	if !common.WhatIfPayload(cmd, uripath, payload, "Merging pullrequest %s", pullRequestID) {
		return nil
	}

	if mergeOptions.Async {
		result, asyncErr := profile.PostWithResult(cmd.Context(), uripath, payload)
		if asyncErr != nil {
			return fmt.Errorf("failed to merge pull request %s: %w", pullRequestID, asyncErr)
		}
		status, asyncErr := NewPullRequestMergeStatusFromLocation(result.Headers.Get("Location"))
		if asyncErr != nil {
			return fmt.Errorf("failed to get merge status for pull request %s: %w", pullRequestID, asyncErr)
		}
		lgr.Printf("[DEBUG] merge request accepted, task ID: %s", status.ID)
		if err = profile.Print(cmd.Context(), cmd, status); err != nil {
			return fmt.Errorf("cannot print result: %w", err)
		}
		return nil
	}

	var pullrequest PullRequest

	err = profile.Post(cmd.Context(), uripath, payload, &pullrequest)
	if err != nil {
		return fmt.Errorf("failed to merge pull request %s: %w", pullRequestID, err)
	}
	if err := profile.Print(cmd.Context(), cmd, pullrequest); err != nil {
		return fmt.Errorf("cannot print result: %w", err)
	}
	return nil
}
