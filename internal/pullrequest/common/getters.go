package prcommon

import (
	"context"
	"fmt"
	"strconv"
	"strings"

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
func GetPullRequestIDs(context context.Context, cmd *cobra.Command, args []string, toComplete string) (ids []string, err error) {
	ids, err = GetPullRequestIDsWithState(context, cmd, "OPEN")
	if err != nil {
		return []string{}, err
	}
	if len(ids) > 0 {
		return ids, nil
	}
	return GetPullRequestIDsWithState(context, cmd, "ALL")
}
