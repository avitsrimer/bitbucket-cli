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
	"github.com/avitsrimer/bitbucket-cli/internal/repository"
	"github.com/avitsrimer/bitbucket-cli/internal/user"
	"github.com/avitsrimer/bitbucket-cli/internal/workspace"
	"github.com/gildas/go-core"
	"github.com/go-pkgz/lgr"
	"github.com/spf13/cobra"
)

type PullRequestCreator struct {
	Title             string      `json:"title"`
	Description       string      `json:"description,omitempty"`
	Source            Endpoint    `json:"source"`
	Destination       *Endpoint   `json:"destination,omitempty"`
	Reviewers         []user.User `json:"reviewers,omitempty"`
	CloseSourceBranch bool        `json:"close_source_branch,omitempty"`
	Draft             bool        `json:"draft,omitempty"`
}

var createCmd = &cobra.Command{
	Use:     "create",
	Aliases: []string{"add", "new"},
	Short:   "create a pullrequest",
	Args:    cobra.NoArgs,
	RunE:    createProcess,
}

var createOptions struct {
	Title             string
	Description       string
	DescriptionFile   string
	Source            *common.EnumFlag
	Destination       *common.EnumFlag
	Reviewers         []string
	CloseSourceBranch bool
	Draft             bool
}

func init() {
	Command.AddCommand(createCmd)

	createOptions.Source = common.NewEnumFlagWithFunc("", branch.GetBranchNames)
	createOptions.Destination = common.NewEnumFlagWithFunc("", branch.GetBranchNames)

	createCmd.Flags().StringVar(&createOptions.Title, "title", "", "Title of the pullrequest")
	createCmd.Flags().StringVar(&createOptions.Description, "description", "", "Description of the pullrequest")
	registerDescriptionFileFlag(createCmd, &createOptions.DescriptionFile)
	createCmd.Flags().Var(createOptions.Source, "source", "Source branch of the pullrequest")
	createCmd.Flags().Var(createOptions.Destination, "destination", "Destination branch of the pullrequest")
	createCmd.Flags().StringSliceVar(&createOptions.Reviewers, "reviewer", nil, "Reviewer(s) of the pullrequest. Can be specified multiple times, or as a comma-separated list. Can be the user Account ID, UUID, name, or nickname. If the first reviewer is `default`, the command will try to find the default reviewers from the repository or project settings.")
	createCmd.Flags().BoolVar(&createOptions.CloseSourceBranch, "close-source-branch", false, "Close the source branch of the pullrequest")
	createCmd.Flags().BoolVar(&createOptions.Draft, "draft", false, "Create the pullrequest as a draft")
	_ = createCmd.MarkFlagRequired("title")
	_ = createCmd.MarkFlagRequired("source")
	_ = createCmd.RegisterFlagCompletionFunc(createOptions.Source.CompletionFunc("source"))
	_ = createCmd.RegisterFlagCompletionFunc(createOptions.Destination.CompletionFunc("destination"))
	_ = createCmd.RegisterFlagCompletionFunc("reviewer", reviewerCompletionFunc)
}

func createProcess(cmd *cobra.Command, args []string) (err error) {
	ctx := cmd.Context()

	profile, err := profile.GetProfileFromCommand(ctx, cmd)
	if err != nil {
		return fmt.Errorf("cannot get profile: %w", err)
	}

	repository, err := repository.GetRepository(cmd.Context(), cmd)
	if err != nil {
		return fmt.Errorf("cannot get repository: %w", err)
	}

	if createOptions.Title == "" {
		return errors.New("argument title is missing")
	}

	description, err := resolveDescriptionBody(cmd, createOptions.Description, createOptions.DescriptionFile)
	if err != nil {
		return err
	}

	payload := PullRequestCreator{
		Title:             createOptions.Title,
		Description:       description,
		Source:            Endpoint{Branch: Branch{Name: createOptions.Source.Value}},
		CloseSourceBranch: createOptions.CloseSourceBranch,
		Draft:             createOptions.Draft,
	}
	if createOptions.Destination.Value != "" {
		payload.Destination = &Endpoint{Branch: Branch{Name: createOptions.Destination.Value}}
	}

	lgr.Printf("[DEBUG] using repository: %s", repository)

	if len(createOptions.Reviewers) > 0 && createOptions.Reviewers[0] != "default" {
		payload.Reviewers, err = resolveExplicitReviewers(ctx, cmd, profile, repository, createOptions.Reviewers)
		if err != nil {
			return err
		}
	} else {
		payload.Reviewers, err = resolveCreateDefaultReviewers(ctx, cmd, repository)
		if err != nil {
			return err
		}
	}

	uripath := repository.GetPath("pullrequests")

	lgr.Printf("[DEBUG] creating pullrequest")
	if !common.WhatIfPayload(cmd, uripath, payload, "Creating pullrequest") {
		return nil
	}
	var pullrequest PullRequest

	err = profile.Post(
		cmd.Context(),
		uripath,
		payload,
		&pullrequest,
	)
	if err != nil {
		return fmt.Errorf("failed to create pullrequest: %w", err)
	}
	if err := profile.Print(cmd.Context(), cmd, pullrequest); err != nil {
		return fmt.Errorf("cannot print result: %w", err)
	}
	return nil
}

// resolveCreateDefaultReviewers resolves the effective default reviewers of repository, excluding
// the current user when known
func resolveCreateDefaultReviewers(ctx context.Context, cmd *cobra.Command, repository *repository.Repository) ([]user.User, error) {
	reviewers, err := effectiveDefaultReviewers(ctx, cmd, repository)
	if err != nil {
		return nil, err
	}
	return core.Map(reviewers, func(reviewer project.Reviewer) user.User { return reviewer.User }), nil
}

// resolveExplicitReviewers resolves the --reviewer values (already known not to be "default")
// to their matching workspace members, falling back to a direct user lookup. A value that
// resolves to neither is an error: by default (or with --stop-on-error/ErrorProcessing
// StopOnError) it aborts immediately naming the offending value, so no pullrequest is created;
// with --warn-on-error/WarnOnError or --ignore-errors/IgnoreErrors it is tolerated (warned or
// silently skipped) and the pullrequest is created with only the resolved reviewers.
func resolveExplicitReviewers(ctx context.Context, cmd *cobra.Command, currentProfile *profile.Profile, repository *repository.Repository, values []string) ([]user.User, error) {
	workspaceSlug, wsErr := repository.GetWorkspaceSlug(ctx, cmd)
	if wsErr != nil {
		return nil, fmt.Errorf("cannot get workspace: %w", wsErr)
	}
	members, membersErr := workspace.GetMembers(ctx, cmd, workspaceSlug)
	values, err := expandAllReviewers(values, members, membersErr)
	if err != nil {
		return nil, err
	}
	reviewers := make([]user.User, 0, len(values))
	var errs []error
	for _, reviewerNameOrID := range values {
		matches := core.Filter(members, func(member workspace.Member) bool { return matchesMember(member, reviewerNameOrID) })
		if len(matches) > 0 && !slices.ContainsFunc(reviewers, func(u user.User) bool { return u.ID == matches[0].User.ID }) {
			lgr.Printf("[DEBUG] adding reviewer: %s", matches[0].User.ID)
			reviewers = append(reviewers, matches[0].User)
			continue
		}
		if len(matches) > 0 {
			lgr.Printf("[DEBUG] reviewer %s (%s) already added, skipping duplicate", matches[0].User.ID, matches[0].User.Nickname)
			continue
		}
		reviewerUser, userErr := user.GetUser(ctx, cmd, reviewerNameOrID)
		if userErr == nil {
			lgr.Printf("[DEBUG] adding reviewer: %s", reviewerNameOrID)
			reviewers = append(reviewers, *reviewerUser)
			continue
		}
		reviewerErr := fmt.Errorf("reviewer %s is not a member of the workspace", reviewerNameOrID)
		lgr.Printf("[ERROR] %s", reviewerErr)
		if currentProfile.ShouldStopOnError(cmd) {
			return nil, reviewerErr
		}
		errs = append(errs, reviewerErr)
	}
	if err := common.TolerateErrors(cmd, currentProfile, errs, "resolve these reviewers"); err != nil {
		return nil, err //nolint:wrapcheck // TolerateErrors returns the same joined error verbatim (or nil); wrapping would prefix it with redundant noise
	}
	return reviewers, nil
}
