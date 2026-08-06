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
	Query   string
	Columns *common.EnumSliceFlag
	SortBy  *common.EnumFlag
}

func init() {
	Command.AddCommand(activitiesCmd)

	activitiesOptions.Columns = common.NewEnumSliceFlagWithAllAllowed(activityColumns.Columns()...)
	activitiesOptions.SortBy = common.NewEnumFlag(activityColumns.Sorters()...)
	activitiesCmd.Flags().StringVar(&activitiesOptions.Query, "query", "", "Query string to filter activities")
	activitiesCmd.Flags().Var(activitiesOptions.Columns, "columns", "Comma-separated list of columns to display")
	activitiesCmd.Flags().Var(activitiesOptions.SortBy, "sort", "Column to sort by")
	activitiesCmd.Flags().Int("page-length", 0, "Number of items per page to retrieve from Bitbucket. Default is the profile's default page length")
	activitiesCmd.Flags().Int("limit", 0, "Maximum total number of activities to retrieve. Default is to retrieve all of them")
	_ = activitiesCmd.RegisterFlagCompletionFunc(activitiesOptions.Columns.CompletionFunc("columns"))
	_ = activitiesCmd.RegisterFlagCompletionFunc(activitiesOptions.SortBy.CompletionFunc("sort"))
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
	if len(activities) == 0 {
		lgr.Printf("[DEBUG] no activities found")
		return nil
	}
	core.Sort(activities, activityColumns.SortBy(activitiesOptions.SortBy.Value))
	if err := currentProfile.Print(
		cmd.Context(),
		cmd,
		Activities(activities),
	); err != nil {
		return fmt.Errorf("cannot print result: %w", err)
	}
	return nil
}
