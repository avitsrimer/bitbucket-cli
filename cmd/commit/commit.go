package commit

import (
	"encoding/json"
	"strings"
	"time"

	"github.com/gildas/bitbucket-cli/cmd/common"
	"github.com/gildas/bitbucket-cli/cmd/repository"
	"github.com/gildas/bitbucket-cli/cmd/user"
	"github.com/gildas/go-core"
	"github.com/gildas/go-errors"
	"github.com/spf13/cobra"
)

type Commit struct {
	Hash       string                `json:"hash"               mapstructure:"hash"`
	Author     user.Author           `json:"author"             mapstructure:"author"`
	Message    string                `json:"message"            mapstructure:"message"`
	Summary    *common.RenderedText  `json:"summary,omitempty"  mapstructure:"summary"`
	Rendered   *RenderedMessage      `json:"rendered,omitempty" mapstructure:"rendered"`
	Parents    []CommitReference     `json:"parents,omitempty"  mapstructure:"parents"`
	Date       time.Time             `json:"date"               mapstructure:"date"`
	Repository repository.Repository `json:"repository"         mapstructure:"repository"`
	Links      common.Links          `json:"links"              mapstructure:"links"`
}

type RenderedMessage struct {
	Message common.RenderedText `json:"message" mapstructure:"message"`
}

var columns = common.Columns[Commit]{
	{Name: "hash", DefaultSorter: false, Compare: func(a, b Commit) bool {
		return strings.Compare(strings.ToLower(a.Hash), strings.ToLower(b.Hash)) == -1
	}},
	{Name: "longhash", DefaultSorter: false, Compare: func(a, b Commit) bool {
		return strings.Compare(strings.ToLower(a.Message), strings.ToLower(b.Message)) == -1
	}},
	{Name: "author", DefaultSorter: false, Compare: func(a, b Commit) bool {
		return strings.Compare(strings.ToLower(a.Author.User.Name), strings.ToLower(b.Author.User.Name)) == -1
	}},
	{Name: "message", DefaultSorter: false, Compare: func(a, b Commit) bool {
		return strings.Compare(strings.ToLower(a.Message), strings.ToLower(b.Message)) == -1
	}},
	{Name: "date", DefaultSorter: true, Compare: func(a, b Commit) bool {
		return a.Date.Before(b.Date)
	}},
	{Name: "repository", DefaultSorter: false, Compare: func(a, b Commit) bool {
		return strings.Compare(strings.ToLower(a.Repository.Name), strings.ToLower(b.Repository.Name)) == -1
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
		switch strings.ToLower(header) {
		case "hash":
			row = append(row, commit.GetShortHash())
		case "longhash", "fullhash":
			row = append(row, commit.Hash)
		case "author":
			row = append(row, commit.Author.User.Name)
		case "message":
			row = append(row, commit.Message)
		case "date":
			row = append(row, commit.Date.Format("2006-01-02 15:04:05"))
		case "repository":
			row = append(row, commit.Repository.Name)
		}
	}
	return row
}

// GetShortHash gets the short hash of this commit
func (commit Commit) GetShortHash() string {
	if len(commit.Hash) > 7 {
		return commit.Hash[:7]
	}
	return commit.Hash
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
		Date:      commit.Date.Format("2006-01-02T15:04:05.999999999-07:00"),
	})
	return data, errors.JSONMarshalError.Wrap(err)
}
