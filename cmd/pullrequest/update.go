package pullrequest

import (
	"context"
	"fmt"
	"os"
	"slices"
	"strings"

	"github.com/gildas/bitbucket-cli/cmd/branch"
	"github.com/gildas/bitbucket-cli/cmd/common"
	"github.com/gildas/bitbucket-cli/cmd/profile"
	"github.com/gildas/bitbucket-cli/cmd/project/reviewer"
	prcommon "github.com/gildas/bitbucket-cli/cmd/pullrequest/common"
	"github.com/gildas/bitbucket-cli/cmd/repository"
	"github.com/gildas/bitbucket-cli/cmd/user"
	"github.com/gildas/bitbucket-cli/cmd/workspace"
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
	AddReviewers      *common.EnumSliceFlag
	RemoveReviewers   *common.EnumSliceFlag
	CloseSourceBranch bool
}

func init() {
	Command.AddCommand(updateCmd)

	updateOptions.Destination = common.NewEnumFlagWithFunc(updateCmd, "", branch.GetBranchNames)
	updateOptions.AddReviewers = common.NewEnumSliceFlagWithAllAllowedAndFunc(updateCmd, GetReviewerNicknames)
	updateOptions.RemoveReviewers = common.NewEnumSliceFlagWithAllAllowedAndFunc(updateCmd, GetReviewerNicknames)

	updateCmd.Flags().StringVar(&updateOptions.Title, "title", "", "Title of the pullrequest")
	updateCmd.Flags().StringVar(&updateOptions.Description, "description", "", "Description of the pullrequest")
	updateCmd.Flags().Var(updateOptions.Destination, "destination", "Destination branch of the pullrequest")
	updateCmd.Flags().Var(updateOptions.AddReviewers, "add-reviewer", "Reviewer(s) to add to the pullrequest. Can be specified multiple times, or as a comma-separated list. Can be the user Account ID, UUID, name, or nickname.")
	updateCmd.Flags().Var(updateOptions.RemoveReviewers, "remove-reviewer", "Reviewer(s) to remove from the pullrequest. Can be specified multiple times, or as a comma-separated list. Can be the user Account ID, UUID, name, or nickname.")
	updateCmd.Flags().BoolVar(&updateOptions.CloseSourceBranch, "close-source-branch", false, "Close the source branch after merging")

	_ = updateCmd.RegisterFlagCompletionFunc(updateOptions.Destination.CompletionFunc("destination"))
	_ = updateCmd.RegisterFlagCompletionFunc(updateOptions.AddReviewers.CompletionFunc("add-reviewer"))
	_ = updateCmd.RegisterFlagCompletionFunc(updateOptions.RemoveReviewers.CompletionFunc("remove-reviewer"))
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
		cmd,
		repository.GetPath("pullrequests", args[0]),
		&pullrequest,
	)
	if err != nil {
		return fmt.Errorf("failed to get pullrequest %s: %w", args[0], err)
	}
	lgr.Printf("[DEBUG] fetched pullrequest %s", args[0])
	lgr.Printf("[DEBUG] pullrequest %s details", args[0])

	updateWanted := applySimpleFieldUpdates(cmd, &pullrequest)

	var pullrequestWorkspace *workspace.Workspace
	if pullrequest.Destination.Repository != nil {
		lgr.Printf("[DEBUG] getting workspace of pullrequest destination repository %s", pullrequest.Destination.Repository.FullName)
		lgr.Printf("[DEBUG] pullrequest destination repository details")
		pullrequestWorkspace, err = pullrequest.Destination.Repository.GetWorkspace(cmd.Context(), cmd)
	} else {
		lgr.Printf("[DEBUG] getting current workspace")
		pullrequestWorkspace, err = repository.GetWorkspace(cmd.Context(), cmd)
	}
	if err != nil {
		lgr.Printf("[ERROR] failed to get workspace of pullrequest destination repository: %v", err)
		return fmt.Errorf("failed to get workspace of pullrequest destination repository: %w", err)
	}
	lgr.Printf("[DEBUG] pullrequest workspace: %s", pullrequestWorkspace)

	isMember := func(member workspace.Member, id string) bool {
		if parsedID, uuidErr := common.ParseUUID(id); uuidErr == nil {
			return member.User.ID == parsedID
		}
		return member.User.AccountID == id || strings.EqualFold(member.User.Nickname, id) || strings.EqualFold(member.User.Name, id)
	}

	if removeRequestedReviewers(cmd, &pullrequest, isMember) {
		updateWanted = true
	}

	added, err := addRequestedReviewers(cmd.Context(), cmd, &pullrequest, pullrequestWorkspace, isMember)
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
		cmd,
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

// removeRequestedReviewers removes reviewers listed in --remove-reviewer from pullrequest and
// reports whether anything changed
func removeRequestedReviewers(cmd *cobra.Command, pullrequest *PullRequest, isMember func(workspace.Member, string) bool) bool {
	if !cmd.Flag("remove-reviewer").Changed || len(updateOptions.RemoveReviewers.Values) == 0 {
		return false
	}
	updateWanted := false
	for _, reviewerNameOrID := range updateOptions.RemoveReviewers.Values {
		found := -1
		for index, reviewer := range pullrequest.Reviewers {
			if isMember(workspace.Member{User: reviewer}, reviewerNameOrID) {
				found = index
				break
			}
		}
		if found != -1 {
			pullrequest.Reviewers = append(pullrequest.Reviewers[:found], pullrequest.Reviewers[found+1:]...)
			updateWanted = true
		}
	}
	return updateWanted
}

// resolveDefaultReviewers replaces the "default" sentinel in --add-reviewer with the effective
// default reviewers of the pullrequest's source repository, excluding the current user
func resolveDefaultReviewers(ctx context.Context, cmd *cobra.Command, pullrequest *PullRequest) error {
	lgr.Printf("[DEBUG] finding current user")
	me, meErr := user.GetMe(ctx, cmd)
	if meErr != nil {
		// RAT (repo scoped tokens) do not have access to that API endpoint usually
		lgr.Printf("[WARN] failed to get current user, this may be a RAT client. Error: %s", meErr.Error())
	} else {
		lgr.Printf("[DEBUG] current user: %s (%s)", me.Username, me.ID)
	}

	// Find the default reviewers from the repo or project settings
	lgr.Printf("[DEBUG] no reviewers in the repository, trying to get effective default reviewers from the repository")
	reviewers, err := pullrequest.Source.Repository.GetEffectiveDefaultReviewers(ctx, cmd)
	if err != nil {
		lgr.Printf("[ERROR] failed to get default reviewers: %v", err)
		return fmt.Errorf("cannot get default reviewers: %w", err)
	}
	lgr.Printf("[DEBUG] found %d default reviewers", len(reviewers))

	if me != nil {
		// Removing myself from the reviewers since I cannot be a reviewer of my own pullrequest
		reviewers = core.Filter(reviewers, func(reviewer reviewer.Reviewer) bool { return reviewer.User.ID != me.ID })
		lgr.Printf("[DEBUG] filtered reviewers to remove current user: %d reviewers remaining", len(reviewers))
	}

	// Replace the first reviewer with the list of default reviewers and appends the rest
	updateOptions.AddReviewers.Values = append(
		core.Map(reviewers, func(reviewer reviewer.Reviewer) string { return reviewer.User.ID.String() }),
		updateOptions.AddReviewers.Values[1:]...,
	)
	return nil
}

// addRequestedReviewers adds reviewers listed in --add-reviewer (resolving the "default" sentinel
// first) and reports whether anything changed
func addRequestedReviewers(ctx context.Context, cmd *cobra.Command, pullrequest *PullRequest, pullrequestWorkspace *workspace.Workspace, isMember func(workspace.Member, string) bool) (bool, error) {
	if !cmd.Flag("add-reviewer").Changed || len(updateOptions.AddReviewers.Values) == 0 {
		return false, nil
	}

	if updateOptions.AddReviewers.Values[0] == "default" {
		if err := resolveDefaultReviewers(ctx, cmd, pullrequest); err != nil {
			return false, err
		}
	}

	updateWanted := false
	lgr.Printf("[DEBUG] getting all members from workspace %s", pullrequestWorkspace)
	members, _ := pullrequestWorkspace.GetMembers(ctx, cmd)
	lgr.Printf("[DEBUG] found %d members in workspace %s", len(members), pullrequestWorkspace)
	for _, reviewerNameOrID := range updateOptions.AddReviewers.Values {
		lgr.Printf("[DEBUG] processing reviewer to add: %s", reviewerNameOrID)
		matches := core.Filter(members, func(member workspace.Member) bool { return isMember(member, reviewerNameOrID) })
		if len(matches) > 0 {
			if !slices.ContainsFunc(pullrequest.Reviewers, func(u user.User) bool { return u.ID == matches[0].User.ID }) {
				lgr.Printf("[DEBUG] adding reviewer: %s (%s)", matches[0].User.ID, matches[0].User.Nickname)
				pullrequest.Reviewers = append(pullrequest.Reviewers, matches[0].User)
				updateWanted = true
			} else {
				lgr.Printf("[DEBUG] reviewer %s (%s) is already a reviewer, skipping", matches[0].User.ID, matches[0].User.Nickname)
			}
		} else {
			lgr.Printf("[ERROR] reviewer ID %s is not a member of workspace %s", reviewerNameOrID, pullrequestWorkspace)
			fmt.Fprintf(os.Stderr, "Reviewer %s is not a member of workspace %s\n", reviewerNameOrID, pullrequestWorkspace)
		}
	}
	return updateWanted, nil
}
