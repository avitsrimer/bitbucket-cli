package pullrequest

import (
	"context"
	"errors"
	"fmt"
	"slices"

	"github.com/avitsrimer/bitbucket-cli/internal/branch"
	"github.com/avitsrimer/bitbucket-cli/internal/common"
	"github.com/avitsrimer/bitbucket-cli/internal/profile"
	"github.com/avitsrimer/bitbucket-cli/internal/project"
	prcommon "github.com/avitsrimer/bitbucket-cli/internal/pullrequest/common"
	"github.com/avitsrimer/bitbucket-cli/internal/repository"
	"github.com/avitsrimer/bitbucket-cli/internal/user"
	"github.com/avitsrimer/bitbucket-cli/internal/workspace"
	"github.com/gildas/go-core"
	"github.com/go-pkgz/lgr"
	"github.com/spf13/cobra"
)

var updateCmd = &cobra.Command{
	Use:               "update [flags] <pullrequest-id>",
	Aliases:           []string{"edit"},
	Short:             "update a pullrequest by its <pullrequest-id>.",
	Args:              cobra.ExactArgs(1),
	ValidArgsFunction: updateValidArgs,
	RunE:              updateProcess,
}

var updateOptions struct {
	Title             string
	Description       string
	Destination       *common.EnumFlag
	AddReviewers      []string
	RemoveReviewers   []string
	CloseSourceBranch bool
}

func init() {
	Command.AddCommand(updateCmd)

	updateOptions.Destination = common.NewEnumFlagWithFunc(updateCmd, "", branch.GetBranchNames)

	updateCmd.Flags().StringVar(&updateOptions.Title, "title", "", "Title of the pullrequest")
	updateCmd.Flags().StringVar(&updateOptions.Description, "description", "", "Description of the pullrequest")
	updateCmd.Flags().Var(updateOptions.Destination, "destination", "Destination branch of the pullrequest")
	updateCmd.Flags().StringSliceVar(&updateOptions.AddReviewers, "add-reviewer", nil, "Reviewer(s) to add to the pullrequest. Can be specified multiple times, or as a comma-separated list. Can be the user Account ID, UUID, name, or nickname. If the first reviewer is `default`, the command will try to find the default reviewers from the repository or project settings.")
	updateCmd.Flags().StringSliceVar(&updateOptions.RemoveReviewers, "remove-reviewer", nil, "Reviewer(s) to remove from the pullrequest. Can be specified multiple times, or as a comma-separated list. Can be the user Account ID, UUID, name, or nickname.")
	updateCmd.Flags().BoolVar(&updateOptions.CloseSourceBranch, "close-source-branch", false, "Close the source branch after merging")

	_ = updateCmd.RegisterFlagCompletionFunc(updateOptions.Destination.CompletionFunc("destination"))
	_ = updateCmd.RegisterFlagCompletionFunc("add-reviewer", reviewerCompletionFunc)
	_ = updateCmd.RegisterFlagCompletionFunc("remove-reviewer", reviewerCompletionFunc)
}

func updateValidArgs(cmd *cobra.Command, args []string, toComplete string) ([]string, cobra.ShellCompDirective) {
	if len(args) != 0 {
		return nil, cobra.ShellCompDirectiveNoFileComp
	}

	ids, err := prcommon.GetPullRequestIDsWithState(cmd.Context(), cmd, "ALL")
	if err != nil {
		cobra.CompErrorln(err.Error())
		return []string{}, cobra.ShellCompDirectiveError
	}
	return common.FilterValidArgs(ids, args, toComplete), cobra.ShellCompDirectiveNoFileComp
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

	var pullrequest PullRequest

	lgr.Printf("[DEBUG] fetching pullrequest %s", args[0])
	err = profile.Get(
		cmd.Context(),
		repository.GetPath("pullrequests", args[0]),
		&pullrequest,
	)
	if err != nil {
		return fmt.Errorf("failed to get pullrequest %s: %w", args[0], err)
	}
	lgr.Printf("[DEBUG] fetched pullrequest %s", args[0])
	lgr.Printf("[DEBUG] pullrequest %s details", args[0])

	updateWanted := applySimpleFieldUpdates(cmd, &pullrequest)

	pullrequestWorkspace, err := destinationWorkspace(cmd.Context(), cmd, &pullrequest, repository)
	if err != nil {
		return err
	}
	lgr.Printf("[DEBUG] pullrequest workspace: %s", pullrequestWorkspace)

	removed, err := removeRequestedReviewers(cmd.Context(), cmd, profile, &pullrequest)
	if err != nil {
		return err
	}
	if removed {
		updateWanted = true
	}

	added, err := addRequestedReviewers(cmd.Context(), cmd, profile, &pullrequest, pullrequestWorkspace)
	if err != nil {
		return err
	}
	if added {
		updateWanted = true
	}

	if !updateWanted {
		lgr.Printf("[DEBUG] no update options were changed, exiting")
		return nil
	}

	// Remove fields that should not be sent in update
	pullrequest.Summary.Type = ""
	pullrequest.Summary.Markup = ""
	pullrequest.Summary.HTML = ""

	lgr.Printf("[DEBUG] updating pullrequest %s", args[0])
	if !common.WhatIf(cmd, "Updating pullrequest %d", pullrequest.ID) {
		return nil
	}

	var updated PullRequest

	err = profile.Put(
		cmd.Context(),
		repository.GetPath("pullrequests", args[0]),
		pullrequest,
		&updated,
	)
	if err != nil {
		return fmt.Errorf("failed to update pullrequest %s: %w", args[0], err)
	}

	if err := profile.Print(cmd.Context(), cmd, updated); err != nil {
		return fmt.Errorf("cannot print result: %w", err)
	}
	return nil
}

// applySimpleFieldUpdates copies the flag-backed simple fields onto pullrequest and reports
// whether anything changed
func applySimpleFieldUpdates(cmd *cobra.Command, pullrequest *PullRequest) bool {
	updateWanted := false
	if cmd.Flag("title").Changed {
		pullrequest.Title = updateOptions.Title
		updateWanted = true
	}
	if cmd.Flag("description").Changed {
		pullrequest.Description = updateOptions.Description
		pullrequest.Summary.Raw = updateOptions.Description
		updateWanted = true
	}
	if cmd.Flag("destination").Changed {
		pullrequest.Destination = Endpoint{Branch: Branch{Name: updateOptions.Destination.Value}}
		updateWanted = true
	}
	if cmd.Flag("close-source-branch").Changed {
		pullrequest.CloseSourceBranch = updateOptions.CloseSourceBranch
		updateWanted = true
	}
	return updateWanted
}

// destinationWorkspace resolves the workspace that owns pr's destination repository, falling back
// to repo's own workspace when pr carries no destination repository.
func destinationWorkspace(ctx context.Context, cmd *cobra.Command, pr *PullRequest, repo *repository.Repository) (*workspace.Workspace, error) {
	var pullrequestWorkspace *workspace.Workspace
	var err error
	if pr.Destination.Repository != nil {
		lgr.Printf("[DEBUG] getting workspace of pullrequest destination repository %s", pr.Destination.Repository.FullName)
		lgr.Printf("[DEBUG] pullrequest destination repository details")
		pullrequestWorkspace, err = pr.Destination.Repository.GetWorkspace(ctx, cmd)
	} else {
		lgr.Printf("[DEBUG] getting current workspace")
		pullrequestWorkspace, err = repo.GetWorkspace(ctx, cmd)
	}
	if err != nil {
		lgr.Printf("[ERROR] failed to get workspace of pullrequest destination repository: %v", err)
		return nil, fmt.Errorf("failed to get workspace of pullrequest destination repository: %w", err)
	}
	return pullrequestWorkspace, nil
}

// removeRequestedReviewers removes reviewers listed in --remove-reviewer from pullrequest and
// reports whether anything changed. A value that does not match any current reviewer is an
// error: by default (or with --stop-on-error/ErrorProcessing StopOnError) it aborts immediately
// naming the offending value, so no PUT is sent; with --warn-on-error/WarnOnError or
// --ignore-errors/IgnoreErrors it is tolerated (warned or silently skipped) and the update
// proceeds with whichever reviewers were resolved.
func removeRequestedReviewers(ctx context.Context, cmd *cobra.Command, currentProfile *profile.Profile, pullrequest *PullRequest) (bool, error) { //nolint:unparam // ctx is accepted for signature consistency with addRequestedReviewers, which needs it; this function resolves reviewers purely from the in-memory pullrequest.Reviewers and never reaches the network
	if !cmd.Flag("remove-reviewer").Changed || len(updateOptions.RemoveReviewers) == 0 {
		return false, nil
	}
	updateWanted := false
	var errs []error
	for _, reviewerNameOrID := range updateOptions.RemoveReviewers {
		found := -1
		for index, reviewer := range pullrequest.Reviewers {
			if matchesMember(workspace.Member{User: reviewer}, reviewerNameOrID) {
				found = index
				break
			}
		}
		if found != -1 {
			pullrequest.Reviewers = append(pullrequest.Reviewers[:found], pullrequest.Reviewers[found+1:]...)
			updateWanted = true
			continue
		}
		reviewerErr := fmt.Errorf("reviewer %s is not a reviewer of the pullrequest", reviewerNameOrID)
		lgr.Printf("[ERROR] %s", reviewerErr)
		if currentProfile.ShouldStopOnError(cmd) {
			return false, reviewerErr
		}
		errs = append(errs, reviewerErr)
	}
	if err := tolerateReviewerErrors(cmd, currentProfile, errs, "remove these reviewers"); err != nil {
		return false, err
	}
	return updateWanted, nil
}

// resolveDefaultReviewers resolves the "default" sentinel in --add-reviewer to the effective
// default reviewers of the pullrequest's source repository (excluding the current user),
// returning the resolved reviewer values with the rest of the original --add-reviewer list
// appended. It reads updateOptions.AddReviewers but never writes to it, so repeated
// calls (e.g. across tests reusing the package-level singleton) are idempotent.
func resolveDefaultReviewers(ctx context.Context, cmd *cobra.Command, pullrequest *PullRequest) ([]string, error) {
	if pullrequest.Source.Repository == nil {
		return nil, errors.New("pullrequest has no source repository, cannot resolve default reviewers")
	}

	reviewers, err := effectiveDefaultReviewers(ctx, cmd, pullrequest.Source.Repository)
	if err != nil {
		return nil, err
	}

	// Replace the first reviewer with the list of default reviewers and append the rest
	return append(
		core.Map(reviewers, func(reviewer project.Reviewer) string { return reviewer.User.ID.String() }),
		updateOptions.AddReviewers[1:]...,
	), nil
}

// addRequestedReviewers adds reviewers listed in --add-reviewer (resolving the "default" and
// "all" sentinels first) and reports whether anything changed. A value that does not match any
// workspace member is an error: by default (or with --stop-on-error/ErrorProcessing
// StopOnError) it aborts immediately naming the offending value, so no PUT is sent; with
// --warn-on-error/WarnOnError or --ignore-errors/IgnoreErrors it is tolerated (warned or
// silently skipped) and the update proceeds with whichever reviewers were resolved.
func addRequestedReviewers(ctx context.Context, cmd *cobra.Command, currentProfile *profile.Profile, pullrequest *PullRequest, pullrequestWorkspace *workspace.Workspace) (bool, error) {
	if !cmd.Flag("add-reviewer").Changed || len(updateOptions.AddReviewers) == 0 {
		return false, nil
	}

	reviewerValues := updateOptions.AddReviewers
	if reviewerValues[0] == "default" {
		resolved, err := resolveDefaultReviewers(ctx, cmd, pullrequest)
		if err != nil {
			return false, err
		}
		reviewerValues = resolved
	}

	updateWanted := false
	lgr.Printf("[DEBUG] getting all members from workspace %s", pullrequestWorkspace)
	members, _ := pullrequestWorkspace.GetMembers(ctx, cmd)
	lgr.Printf("[DEBUG] found %d members in workspace %s", len(members), pullrequestWorkspace)
	reviewerValues = expandAllReviewers(reviewerValues, members)
	var errs []error
	for _, reviewerNameOrID := range reviewerValues {
		lgr.Printf("[DEBUG] processing reviewer to add: %s", reviewerNameOrID)
		matches := core.Filter(members, func(member workspace.Member) bool { return matchesMember(member, reviewerNameOrID) })
		if len(matches) > 0 {
			if !slices.ContainsFunc(pullrequest.Reviewers, func(u user.User) bool { return u.ID == matches[0].User.ID }) {
				lgr.Printf("[DEBUG] adding reviewer: %s (%s)", matches[0].User.ID, matches[0].User.Nickname)
				pullrequest.Reviewers = append(pullrequest.Reviewers, matches[0].User)
				updateWanted = true
			} else {
				lgr.Printf("[DEBUG] reviewer %s (%s) is already a reviewer, skipping", matches[0].User.ID, matches[0].User.Nickname)
			}
			continue
		}
		reviewerErr := fmt.Errorf("reviewer %s is not a member of workspace %s", reviewerNameOrID, pullrequestWorkspace)
		lgr.Printf("[ERROR] %s", reviewerErr)
		if currentProfile.ShouldStopOnError(cmd) {
			return false, reviewerErr
		}
		errs = append(errs, reviewerErr)
	}
	if err := tolerateReviewerErrors(cmd, currentProfile, errs, "resolve these reviewers"); err != nil {
		return false, err
	}
	return updateWanted, nil
}
