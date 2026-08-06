package pullrequest

import (
	"context"
	"fmt"
	"net/url"
	"strings"

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

	// fetchActivityPages, not profile.GetAll: an unknown activity kind is dropped by
	// filterUnknownActivityKinds below, after the fetch. Fetching with GetAll's own
	// --limit-bounded pagination would apply --limit to the RAW page (known and unknown kinds
	// together), so a feed containing unrecognized entries could return fewer than --limit known
	// activities, or even zero, despite --limit activities actually existing. fetchActivityPages
	// instead stops paginating once it has collected --limit KNOWN activities (or the feed is
	// exhausted, when --limit is unset), so --limit still bounds the number of requests made.
	// activityLimit below then trims the last page's overshoot down to exactly --limit, exactly
	// like GetAll would have applied it to an all-known feed.
	pageLength, limit := activityPageLengthAndLimit(cmd, currentProfile.DefaultPageLength)
	activities, err := fetchActivityPages(cmd.Context(), currentProfile, uripath, pageLength, limit)
	if err != nil {
		return err
	}
	activities = filterUnknownActivityKinds(pullRequestID, activities)
	activities = activityLimit(cmd, activities)
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

// activityPageLengthAndLimit reads cmd's own --page-length and --limit flags, mirroring
// profile.resolvePageLengthAndLimit (unexported, so not reusable directly): pageLength defaults to
// defaultPageLength (the profile's own default), overridden by --page-length when explicitly set;
// limit is 0 (unbounded) unless --limit is explicitly set to a positive value. Unlike an internal
// id-resolution query, activitiesProcess's own --limit legitimately bounds THIS query's output, so
// it is always honored here -- there is no GetAllUnbounded-style case to avoid. When limit is
// smaller than pageLength, pageLength shrinks to it so the final page does not overfetch.
func activityPageLengthAndLimit(cmd *cobra.Command, defaultPageLength int) (pageLength, limit int) {
	pageLength = defaultPageLength
	if flag := cmd.Flag("page-length"); flag != nil && flag.Changed {
		if length, err := cmd.Flags().GetInt("page-length"); err == nil && length > 0 {
			pageLength = length
		}
	}
	if flag := cmd.Flag("limit"); flag != nil && flag.Changed {
		if limitValue, err := cmd.Flags().GetInt("limit"); err == nil && limitValue > 0 {
			limit = limitValue
		}
	}
	if limit > 0 && (pageLength == 0 || limit < pageLength) {
		pageLength = limit
	}
	return pageLength, limit
}

// fetchActivityPages fetches the activity feed at uripath page by page via currentProfile,
// stopping as soon as it has collected limit KNOWN-kind activities (or the feed is exhausted, when
// limit is 0) -- restoring --limit's original round-trip-bounding behavior (see the comment at its
// call site) without re-introducing the bug --limit counting unknown-kind entries caused: an
// unrecognized activity kind is never excluded from the returned slice here, only from the count
// this function stops on, so filterUnknownActivityKinds (run once, by the caller, on the returned
// slice) still sees -- and warns about -- every unknown-kind entry actually fetched, deduped across
// every page instead of per page.
func fetchActivityPages(ctx context.Context, currentProfile *profile.Profile, uripath string, pageLength, limit int) ([]Activity, error) {
	var activities []Activity
	if pageLength > 0 && !strings.Contains(uripath, "pagelen") {
		separator := "?"
		if strings.Contains(uripath, "?") {
			separator = "&"
		}
		uripath = fmt.Sprintf("%s%spagelen=%d", uripath, separator, pageLength)
	}

	knownCount := 0
	for {
		var paginated profile.PaginatedResources[Activity]
		if getErr := currentProfile.Get(ctx, uripath, &paginated); getErr != nil {
			return nil, fmt.Errorf("cannot get activities: %w", getErr)
		}
		activities = append(activities, paginated.Values...)
		for _, activity := range paginated.Values {
			if activity.unknownVariant == "" {
				knownCount++
			}
		}
		if limit > 0 && knownCount >= limit {
			return activities, nil
		}
		if paginated.Next == "" {
			return activities, nil
		}
		uripath = paginated.Next
	}
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

// activityLimit truncates activities to cmd's own --limit flag, when explicitly set to a positive
// value, mirroring the truncation profile.GetAll would have applied during pagination -- but
// applied here, after filterUnknownActivityKinds, so an unrecognized activity kind never counts
// against the limit a caller placed on the KNOWN activities they actually want.
func activityLimit(cmd *cobra.Command, activities []Activity) []Activity {
	limitFlag := cmd.Flag("limit")
	if limitFlag == nil || !limitFlag.Changed {
		return activities
	}
	limit, err := cmd.Flags().GetInt("limit")
	if err != nil || limit <= 0 || limit >= len(activities) {
		return activities
	}
	return activities[:limit]
}
