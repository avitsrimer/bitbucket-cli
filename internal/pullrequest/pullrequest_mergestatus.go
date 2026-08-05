package pullrequest

import (
	"errors"
	"fmt"
	"strconv"
	"strings"

	"github.com/avitsrimer/bitbucket-cli/internal/common"
	"github.com/gildas/go-core"
	"github.com/spf13/cobra"
)

// PullRequestMergeStatus describes the status of a pull request
type PullRequestMergeStatus struct {
	ID          string       `json:"id"`
	Status      string       `json:"task_status"`
	PullRequest PullRequest  `json:"merge_result"`
	Links       common.Links `json:"links"`
}

// NewPullRequestMergeStatusFromLocation creates a new PullRequestMergeStatus from a URL location
//
// The URL location is the URL returned in the Location header of the response from Bitbucket when we request to merge a pull request asynchronously
func NewPullRequestMergeStatusFromLocation(location string) (mergeStatus *PullRequestMergeStatus, err error) {
	// Format: https://api.bitbucket.org/2.0/repositories/<workspace_slug>/<repo_slug>/pullrequests/<pullrequest_id>/merge/task-status/<task_id>
	if location == "" {
		return nil, errors.New("failed to get the merge task URL from the Location header in the response from Bitbucket")
	}
	parts := strings.Split(location, "/")
	if len(parts) < 12 {
		return nil, fmt.Errorf("invalid merge task URL: %s", location)
	}
	taskID := parts[len(parts)-1]
	pullrequestID, err := strconv.Atoi(parts[len(parts)-4])
	if err != nil || pullrequestID < 0 {
		return nil, fmt.Errorf("invalid pull request ID: %s", parts[len(parts)-4])
	}
	return &PullRequestMergeStatus{ID: taskID, PullRequest: PullRequest{ID: uint64(pullrequestID)}}, nil
}

// GetHeaders gets the header for a table
//
// implements common.Tableable
func (status PullRequestMergeStatus) GetHeaders(cmd *cobra.Command) []string {
	if cmd != nil && cmd.Flag("columns") != nil && cmd.Flag("columns").Changed {
		if columns, err := cmd.Flags().GetStringSlice("columns"); err == nil {
			return core.Map(columns, func(column string) string { return strings.ReplaceAll(column, "_", " ") })
		}
	}
	return []string{"ID", "Pull Request", "Status"}
}

// GetRow gets the row for a table
//
// implements common.Tableable
func (status PullRequestMergeStatus) GetRow(headers []string) []string {
	var row []string

	for _, header := range headers {
		switch strings.ToLower(header) {
		case "id":
			row = append(row, status.ID)
		case "pull request", "pull_request", "pull-request", "pullrequest", "pr":
			row = append(row, strconv.FormatUint(status.PullRequest.ID, 10))
		case "status":
			if status.Status == "SUCCESS" {
				row = append(row, status.Status+"/"+status.PullRequest.State)
			} else {
				row = append(row, status.Status)
			}
		}
	}
	return row
}
