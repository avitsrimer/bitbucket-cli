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
	ValidArgsFunction: prcommon.PullRequestIDValidArgs,
	RunE:              createProcess,
}

var createOptions commentEditOptions

func init() {
	Command.AddCommand(createCmd)

	registerCommentEditFlags(createCmd, &createOptions, "Comment of the pullrequest")
}

func createProcess(cmd *cobra.Command, args []string) (err error) {
	pullRequestID := args[0]
	if validateErr := common.ValidatePathIdentifier("pullrequest-id", pullRequestID); validateErr != nil {
		return fmt.Errorf("cannot create comment: %w", validateErr)
	}

	payload, err := createOptions.payload(cmd)
	if err != nil {
		return err
	}

	ctx := cmd.Context()

	profile, err := profile.GetProfileFromCommand(ctx, cmd)
	if err != nil {
		return fmt.Errorf("cannot get profile: %w", err)
	}

	repository, err := repository.GetRepository(ctx, cmd)
	if err != nil {
		return fmt.Errorf("cannot get repository: %w", err)
	}

	// validateFileAnchor's diffstat GET already 404s identically for a nonexistent pull request,
	// so ExistsPullRequest would be a second, redundant round trip whenever --file is set; it only
	// runs here when there is no anchor to validate the pull request's existence instead.
	if payload.Anchor != nil {
		if anchorErr := validateFileAnchor(ctx, cmd, repository, pullRequestID, payload.Anchor); anchorErr != nil {
			return anchorErr
		}
	} else if err = prcommon.ExistsPullRequest(ctx, cmd, repository, pullRequestID); err != nil {
		return fmt.Errorf("cannot create comment: %w", err)
	}

	uripath := repository.GetPath("pullrequests", pullRequestID, "comments")

	lgr.Printf("[DEBUG] creating pullrequest comment")
	if !common.WhatIfPayload(cmd, uripath, payload, "Creating comment for pullrequest %s", pullRequestID) {
		return nil
	}
	var comment Comment

	err = profile.Post(
		ctx,
		uripath,
		payload,
		&comment,
	)
	if err != nil {
		return fmt.Errorf("failed to create comment for pullrequest %s: %w", pullRequestID, err)
	}
	if err := profile.Print(ctx, cmd, comment); err != nil {
		return fmt.Errorf("cannot print result: %w", err)
	}
	return nil
}
