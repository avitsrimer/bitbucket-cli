package commit

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/avitsrimer/bitbucket-cli/internal/common"
	"github.com/avitsrimer/bitbucket-cli/internal/profile"
	"github.com/avitsrimer/bitbucket-cli/internal/repository"
	"github.com/avitsrimer/bitbucket-cli/internal/user"
	"github.com/gildas/go-core"
	"github.com/spf13/cobra"
)

type Commit struct {
	Hash       string                `json:"hash"`
	Author     user.Author           `json:"author"`
	Message    string                `json:"message"`
	Summary    *common.RenderedText  `json:"summary,omitempty"`
	Rendered   *RenderedMessage      `json:"rendered,omitempty"`
	Parents    []CommitReference     `json:"parents,omitempty"`
	Date       time.Time             `json:"date"`
	Repository repository.Repository `json:"repository"`
	Links      common.Links          `json:"links"`
}

type RenderedMessage struct {
	Message common.RenderedText `json:"message"`
}

// Command represents this folder's command
var Command = &cobra.Command{
	Use:   "commit",
	Short: "Manage commits",
	Run:   common.SubcommandRequired("Commit"),
}

// shortHashLength is the number of characters a short hash is truncated to.
const shortHashLength = 7

// shortHash truncates hash to shortHashLength characters, or returns it unchanged if it is
// already shorter than that.
func shortHash(hash string) string {
	if len(hash) > shortHashLength {
		return hash[:shortHashLength]
	}
	return hash
}

var columns = common.Columns[Commit]{
	{Name: "hash", DefaultSorter: false, Compare: func(a, b Commit) bool {
		return strings.ToLower(a.Hash) < strings.ToLower(b.Hash)
	}},
	{Name: "longhash", DefaultSorter: false, Compare: func(a, b Commit) bool {
		return strings.ToLower(a.Hash) < strings.ToLower(b.Hash)
	}},
	{Name: "author", DefaultSorter: false, Compare: func(a, b Commit) bool {
		return strings.ToLower(a.Author.User.Name) < strings.ToLower(b.Author.User.Name)
	}},
	{Name: "message", DefaultSorter: false, Compare: func(a, b Commit) bool {
		return strings.ToLower(a.Message) < strings.ToLower(b.Message)
	}},
	{Name: "date", DefaultSorter: true, Compare: func(a, b Commit) bool {
		return a.Date.Before(b.Date)
	}},
	{Name: "repository", DefaultSorter: false, Compare: func(a, b Commit) bool {
		return strings.ToLower(a.Repository.Name) < strings.ToLower(b.Repository.Name)
	}},
}

// GetType gets the type of this commit
//
// implements core.TypeCarrier
func (commit Commit) GetType() string {
	return "commit"
}

// GetColumnDefinitions gets the column definitions for commits
func (commit Commit) GetColumnDefinitions() common.Columns[Commit] {
	return columns
}

// GetHeaders gets the header for a table
//
// implements common.Tableable
func (commit Commit) GetHeaders(cmd *cobra.Command) []string {
	if cmd != nil && cmd.Flag("columns") != nil && cmd.Flag("columns").Changed {
		if columns, err := cmd.Flags().GetStringSlice("columns"); err == nil {
			return core.Map(columns, func(column string) string { return strings.ReplaceAll(column, "_", " ") })
		}
	}
	return []string{"Hash", "Date", "Author", "Message"}
}

// GetRow gets the row for a table
//
// implements common.Tableable
func (commit Commit) GetRow(headers []string) []string {
	var row []string

	for _, header := range headers {
		switch common.NormalizeColumnKey(header) {
		case "hash":
			row = append(row, commit.GetShortHash())
		case "longhash":
			row = append(row, commit.Hash)
		case "author":
			row = append(row, commit.Author.User.Name)
		case "message":
			row = append(row, commit.Message)
		case "date":
			row = append(row, commit.dateCell())
		case "repository":
			row = append(row, commit.Repository.Name)
		default:
			row = append(row, " ")
		}
	}
	return row
}

// GetShortHash gets the short hash of this commit
func (commit Commit) GetShortHash() string {
	return shortHash(commit.Hash)
}

// dateCell returns Date formatted with common.TableTimeFormat, or " " when it is zero.
func (commit Commit) dateCell() string {
	if commit.Date.IsZero() {
		return " "
	}
	return commit.Date.Format(common.TableTimeFormat)
}

// GetLatestCommit gets the single most recent commit of the repository, purely against the
// BitBucket API: it never touches a local git working directory. BitBucket's commits endpoint
// returns commits newest first, so requesting a one-item page and taking its only element is
// enough.
func GetLatestCommit(ctx context.Context, cmd *cobra.Command) (*Commit, error) {
	repo, err := repository.GetRepository(ctx, cmd)
	if err != nil {
		return nil, fmt.Errorf("cannot get repository: %w", err)
	}
	profileCurrent, err := profile.GetProfileFromCommand(ctx, cmd)
	if err != nil {
		return nil, fmt.Errorf("cannot get profile: %w", err)
	}
	var page profile.PaginatedResources[Commit]
	if err := profileCurrent.Get(ctx, repo.GetPath("commits")+"?pagelen=1", &page); err != nil {
		return nil, fmt.Errorf("cannot get latest commit: %w", err)
	}
	if len(page.Values) == 0 {
		return nil, errors.New("no commit found")
	}
	return &page.Values[0], nil
}

// GetCommitByHash gets a commit by its hash.
//
// This deliberately requests the plural "commits/{revision}" endpoint and decodes a
// {"values":[...]} page, not the singular "commit/{revision}" form: despite what Bitbucket's own
// API documentation says, the singular endpoint returns a list-shaped body, so decoding it
// straight into a Commit silently yields an all-zero value with no error. The plural form is
// documented (and observed) to behave the same way but is unambiguous either way, since it is
// always list-shaped.
func GetCommitByHash(ctx context.Context, cmd *cobra.Command, hash string) (*Commit, error) {
	repo, err := repository.GetRepository(ctx, cmd)
	if err != nil {
		return nil, fmt.Errorf("cannot get repository: %w", err)
	}
	profileCurrent, err := profile.GetProfileFromCommand(ctx, cmd)
	if err != nil {
		return nil, fmt.Errorf("cannot get profile: %w", err)
	}
	var page profile.PaginatedResources[Commit]
	if err := profileCurrent.Get(ctx, repo.GetPath("commits", hash), &page); err != nil {
		return nil, fmt.Errorf("cannot get commit %s: %w", hash, err)
	}
	if len(page.Values) == 0 {
		return nil, fmt.Errorf("commit %s not found", hash)
	}
	return &page.Values[0], nil
}

// String gets a string representation of this commit
//
// implements fmt.Stringer
func (commit Commit) String() string {
	return commit.Hash
}

// MarshalJSON implements the json.Marshaler interface.
func (commit Commit) MarshalJSON() (data []byte, err error) {
	type surrogate Commit

	data, err = json.Marshal(struct {
		Type string `json:"type"`
		surrogate
		Date string `json:"date"`
	}{
		Type:      commit.GetType(),
		surrogate: surrogate(commit),
		Date:      commit.Date.Format(common.JSONTimeFormat),
	})
	if err != nil {
		return nil, fmt.Errorf("cannot marshal json: %w", err)
	}
	return data, nil
}
