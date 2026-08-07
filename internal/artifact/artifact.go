package artifact

import (
	"context"
	"fmt"
	"strconv"
	"strings"

	"github.com/avitsrimer/bitbucket-cli/internal/common"
	"github.com/avitsrimer/bitbucket-cli/internal/profile"
	"github.com/avitsrimer/bitbucket-cli/internal/repository"
	"github.com/avitsrimer/bitbucket-cli/internal/user"
	"github.com/spf13/cobra"
)

// Artifact represents a downloadable build artifact attached to a repository.
type Artifact struct {
	Name      string       `json:"name"`
	Size      uint64       `json:"size"`
	Downloads uint64       `json:"downloads"`
	User      user.User    `json:"user"`
	Links     common.Links `json:"links"`
}

// Command represents this folder's command
var Command = &cobra.Command{
	Use:   "artifact",
	Short: "Manage artifacts",
	Run:   common.SubcommandRequired("Artifact"),
}

var columns = common.Columns[Artifact]{
	{Name: "name", DefaultSorter: true, Compare: func(a, b Artifact) bool {
		return strings.ToLower(a.Name) < strings.ToLower(b.Name)
	}},
	{Name: "size", DefaultSorter: false, Compare: func(a, b Artifact) bool {
		return a.Size < b.Size
	}},
	{Name: "downloads", DefaultSorter: false, Compare: func(a, b Artifact) bool {
		return a.Downloads < b.Downloads
	}},
	{Name: "owner", DefaultSorter: false, Compare: func(a, b Artifact) bool {
		return strings.ToLower(a.User.Name) < strings.ToLower(b.User.Name)
	}},
}

// GetHeaders gets the header for a table
//
// implements common.Tableable
func (artifact Artifact) GetHeaders(cmd *cobra.Command) []string {
	return common.HeadersFromFlag(cmd, "Name", "Size", "Downloads", "Owner")
}

// GetRow gets the row for a table
//
// implements common.Tableable
func (artifact Artifact) GetRow(headers []string) []string {
	var row []string

	for _, header := range headers {
		switch common.NormalizeColumnKey(header) {
		case "name":
			row = append(row, artifact.Name)
		case "size":
			row = append(row, strconv.FormatUint(artifact.Size, 10))
		case "downloads":
			row = append(row, strconv.FormatUint(artifact.Downloads, 10))
		case "owner":
			row = append(row, artifact.User.Name)
		default:
			row = append(row, " ")
		}
	}
	return row
}

// GetArtifactNames gets the names of every artifact attached to the current repository, sorted
// case-insensitively. It backs shell completion for an artifact name argument (download).
//
// This uses profile.GetAllUnbounded, not profile.GetAll: a completion/allowed-value getter must
// enumerate every matching artifact regardless of any --limit flag that happens to be registered
// on the cmd it is called with, rather than silently truncating the completion list.
func GetArtifactNames(ctx context.Context, cmd *cobra.Command) (names []string, err error) {
	repo, err := repository.GetRepository(ctx, cmd)
	if err != nil {
		return nil, fmt.Errorf("cannot get repository: %w", err)
	}

	artifacts, err := profile.GetAllUnbounded[Artifact](ctx, cmd, repo.GetPath("downloads"))
	if err != nil {
		return nil, fmt.Errorf("cannot get artifacts: %w", err)
	}
	names = common.Map(artifacts, func(artifact Artifact) string { return artifact.Name })
	common.Sort(names, func(a, b string) bool { return strings.ToLower(a) < strings.ToLower(b) })
	return names, nil
}
