package prcommon

import (
	"context"
	"fmt"
	"strconv"
	"strings"

	"github.com/avitsrimer/bitbucket-cli/internal/common"
	"github.com/avitsrimer/bitbucket-cli/internal/profile"
	"github.com/avitsrimer/bitbucket-cli/internal/repository"
	"github.com/gildas/go-core"
	"github.com/go-pkgz/lgr"
	"github.com/spf13/cobra"
)

type PullRequestID struct {
	ID int `json:"id"`
}

// GetPullRequestIDsWithState gets the pullrequest Ids for completion for a given state
func GetPullRequestIDsWithState(context context.Context, cmd *cobra.Command, state string) (ids []string, err error) {
	repository, err := repository.GetRepository(cmd.Context(), cmd)
	if err != nil {
		return []string{}, fmt.Errorf("cannot get repository: %w", err)
	}
	return GetPullRequestIDsFromRepositoryWithState(context, cmd, repository, state)
}

// GetPullRequestIDsFromRepositoryWithState gets the pullrequest Ids for completion for a given
// state and repository.
//
// This uses GetAllUnbounded, not GetAll: cmd here is frequently the very command whose own
// --limit flag is meant to bound a *different*, later query (e.g. `pr commits`/`pr activities`
// use this to resolve an omitted pullrequest-id argument before fetching commits/activities with
// their own --limit). GetAll would apply that same --limit to this enumeration too, so e.g. `pr
// commits --limit 1` with 4 open pull requests would silently return only 1 id instead of the 4
// needed to trip the "too many pullrequests" ambiguity check below/in callers.
func GetPullRequestIDsFromRepositoryWithState(context context.Context, cmd *cobra.Command, repository *repository.Repository, state string) (ids []string, err error) {
	lgr.Printf("[DEBUG] getting %s pullrequests", state)
	pullrequests, err := profile.GetAllUnbounded[PullRequestID](
		context,
		cmd,
		repository.GetPath("pullrequests?state="+state),
	)
	if err != nil {
		lgr.Printf("[ERROR] failed to get %s pullrequests: %v", state, err)
		return []string{}, err
	}

	ids = core.Map(pullrequests, func(pullrequest PullRequestID) string { return strconv.Itoa(pullrequest.ID) })
	core.Sort(ids, func(a, b string) bool { return strings.ToLower(a) < strings.ToLower(b) })
	return ids, nil
}

// GetPullRequestIDs gets the IDs of the pullrequests
//
// First only the open pullrequests are fetched, if none are found, all pullrequests are fetched
func GetPullRequestIDs(context context.Context, cmd *cobra.Command) (ids []string, err error) {
	ids, err = GetPullRequestIDsWithState(context, cmd, "OPEN")
	if err != nil {
		return []string{}, err
	}
	if len(ids) > 0 {
		return ids, nil
	}
	return GetPullRequestIDsWithState(context, cmd, "ALL")
}

// PullRequestIDValidArgs is the ValidArgsFunction shared by every comment/task subcommand whose
// only positional is <pullrequest-id> (create, list, and the len(args)==0 case of delete):
// completes open pullrequest ids via GetPullRequestIDs, offering nothing once the positional is
// already filled.
func PullRequestIDValidArgs(cmd *cobra.Command, args []string, toComplete string) ([]string, cobra.ShellCompDirective) {
	if len(args) != 0 {
		return nil, cobra.ShellCompDirectiveNoFileComp
	}

	ids, err := GetPullRequestIDs(cmd.Context(), cmd)
	if err != nil {
		return []string{}, cobra.ShellCompDirectiveNoFileComp
	}
	return common.FilterValidArgs(ids, args, toComplete), cobra.ShellCompDirectiveNoFileComp
}

// ExistsPullRequest validates via a GET that pullRequestID names an existing pull request in repo,
// returning the same error a write against that id would produce. Every mutating command whose
// target sub-resource does not exist yet (pullrequest merge/approve/decline/..., a new comment or
// task) calls this as part of its full preflight, so a nonexistent pull request id fails
// identically whether or not --dry-run is set, instead of dry-run's skipped write silently hiding
// the failure. A command whose target IS an existing sub-resource (updating/reopening/resolving/
// deleting a comment or task) instead GETs that sub-resource directly, which validates the parent
// pull request's existence too.
func ExistsPullRequest(ctx context.Context, cmd *cobra.Command, repo *repository.Repository, pullRequestID string) error {
	currentProfile, err := profile.GetProfileFromCommand(ctx, cmd)
	if err != nil {
		return fmt.Errorf("cannot get profile: %w", err)
	}
	if err := currentProfile.Get(ctx, repo.GetPath("pullrequests", pullRequestID), nil); err != nil {
		return fmt.Errorf("failed to get pullrequest %s: %w", pullRequestID, err)
	}
	return nil
}

// ExistsSubResource validates via a GET that subResourceID names an existing sub-resource of the
// pull request identified by pullRequestID in repo, reached at
// repo.GetPath("pullrequests", pullRequestID, pathSegment, subResourceID) (e.g. pathSegment
// "comments" or "tasks"), returning the same error a write against that sub-resource would
// produce -- this also validates the parent pull request's existence too. noun names the
// sub-resource kind (e.g. "comment", "task") in the wrapped error. A command whose target does
// NOT yet exist (a new comment or task) calls ExistsPullRequest instead.
func ExistsSubResource(ctx context.Context, cmd *cobra.Command, repo *repository.Repository, pathSegment, noun, pullRequestID, subResourceID string) error {
	currentProfile, err := profile.GetProfileFromCommand(ctx, cmd)
	if err != nil {
		return fmt.Errorf("cannot get profile: %w", err)
	}
	if err := currentProfile.Get(ctx, repo.GetPath("pullrequests", pullRequestID, pathSegment, subResourceID), nil); err != nil {
		return fmt.Errorf("failed to get %s %s of pullrequest %s: %w", noun, subResourceID, pullRequestID, err)
	}
	return nil
}
