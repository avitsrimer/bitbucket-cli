package pullrequest

import (
	"fmt"
	"net/url"

	"github.com/avitsrimer/bitbucket-cli/internal/common"
	"github.com/avitsrimer/bitbucket-cli/internal/profile"
	prcommon "github.com/avitsrimer/bitbucket-cli/internal/pullrequest/common"
	"github.com/avitsrimer/bitbucket-cli/internal/repository"
	"github.com/gildas/go-core"
	"github.com/go-pkgz/lgr"
	"github.com/spf13/cobra"
)

// Activities describes a list of Activity
type Activities []Activity

// GetHeaders gets the headers for the list command
//
// implements common.Tableables
func (activities Activities) GetHeaders(cmd *cobra.Command) []string {
	return Activity{}.GetHeaders(cmd)
}

// GetRowAt gets the row for the list command
//
// implements common.Tableables
func (activities Activities) GetRowAt(index int, headers []string) []string {
	if index < 0 || index >= len(activities) {
		return []string{}
	}
	return activities[index].GetRow(headers)
}

// Size gets the number of elements
//
// implements common.Tableables
func (activities Activities) Size() int {
	return len(activities)
}

var activitiesCmd = &cobra.Command{
	Use:               "activities",
	Short:             "List all activities of a pullrequest",
	Args:              cobra.MaximumNArgs(1),
	ValidArgsFunction: activitiesValidArgs,
	RunE:              activitiesProcess,
}

var activitiesOptions struct {
	Query string
}

func init() {
	Command.AddCommand(activitiesCmd)

	activitiesCmd.Flags().StringVar(&activitiesOptions.Query, "query", "", "Query string to filter activities")
	common.RegisterListFlags(activitiesCmd, activityColumns, "activities")
}

func activitiesValidArgs(cmd *cobra.Command, args []string, toComplete string) ([]string, cobra.ShellCompDirective) {
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

func activitiesProcess(cmd *cobra.Command, args []string) (err error) {
	currentProfile, err := profile.GetProfileFromCommand(cmd.Context(), cmd)
	if err != nil {
		return fmt.Errorf("cannot list activities for pull request: %w", err)
	}

	repository, err := repository.GetRepository(cmd.Context(), cmd)
	if err != nil {
		return fmt.Errorf("cannot list activities for pull request: %w", err)
	}

	pullRequestID, err := GetPullRequestIDFromArgs(cmd.Context(), cmd, repository, args)
	if err != nil {
		return fmt.Errorf("cannot list activities for pull request: %w", err)
	}

	uripath := repository.GetPath(fmt.Sprintf("pullrequests/%s/activity", pullRequestID))

	if activitiesOptions.Query != "" {
		uripath = fmt.Sprintf("%s?q=%s", uripath, url.QueryEscape(activitiesOptions.Query))
	}

	lgr.Printf("[DEBUG] listing all activities from repository %s with profile %s", repository, currentProfile)
	if !common.WhatIf(cmd, fmt.Sprintf("Showing activities for pullrequest %s in repository %s with profile %s", pullRequestID, repository, currentProfile)) {
		return nil
	}

	activities, err := profile.GetAll[Activity](cmd.Context(), cmd, uripath)
	if err != nil {
		return err
	}
	activities = filterUnknownActivityKinds(pullRequestID, activities)
	if len(activities) == 0 {
		lgr.Printf("[DEBUG] no activities found")
		return nil
	}
	if sortValue := common.SortFlagValue(cmd); sortValue != "" {
		core.Sort(activities, activityColumns.SortBy(sortValue))
	}
	if err := currentProfile.Print(
		cmd.Context(),
		cmd,
		Activities(activities),
	); err != nil {
		return fmt.Errorf("cannot print result: %w", err)
	}
	return nil
}

// filterUnknownActivityKinds drops any activity Activity.UnmarshalJSON could not match to a known
// variant (Approval/ChangesRequested/Comment/Update) from the result, so an activity kind
// BitBucket adds later renders every activity this command DOES recognize instead of blinding the
// whole list. It emits exactly one [WARN] per distinct unknown kind, deduped via a local map local
// to this call (never package-level state, so concurrent invocations under `go test -race` never
// share it).
func filterUnknownActivityKinds(pullRequestID string, activities []Activity) []Activity {
	known := make([]Activity, 0, len(activities))
	warnedKinds := map[string]struct{}{}
	for _, activity := range activities {
		if activity.unknownVariant == "" {
			known = append(known, activity)
			continue
		}
		if _, alreadyWarned := warnedKinds[activity.unknownVariant]; !alreadyWarned {
			warnedKinds[activity.unknownVariant] = struct{}{}
			lgr.Printf("[WARN] pullrequest %s activity feed contains an unrecognized activity kind %q, skipping matching entries", pullRequestID, activity.unknownVariant)
		}
	}
	return known
}
