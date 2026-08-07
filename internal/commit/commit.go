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

// validateCommitRefs validates each of args as a diff/patch spec ref, before any of them are
// joined with the literal ".." separator into a spec: repo.GetPath("diff"/"patch", spec) is a
// bare path.Join with no escaping, so an unvalidated hash/ref could splice extra path segments
// into the request. ValidatePathRef accepts a multi-segment branch/tag ref (e.g. "release/1.0")
// in addition to a bare hash, validating each '/'-delimited segment so no segment can be ".." for
// path.Join to collapse. The joined spec itself legitimately contains ".." (BitBucket's own
// two-commit diff/patch syntax), so ValidatePathRef is never called on the joined spec, only on
// each hash/ref that goes into it. The argument name "commit-hash-or-ref" matches diff/patch's own
// <commit-hash-or-ref> Use string (unlike commit get's single, hash-only positional). Verified
// live against Bitbucket's public API: GET /repositories/{workspace}/{repo_slug}/diff|patch/{spec}
// returns the expected diff/patch for a spec built from a multi-segment branch name, raw slash and
// all -- unlike GET .../commit/{revision} (see commit/get.go's own comment), which 404s on the
// same input. noun ("diff" or "patch") names the operation in the wrapped error.
func validateCommitRefs(noun string, args []string) error {
	for _, hash := range args {
		if err := common.ValidatePathRef("commit-hash-or-ref", hash); err != nil {
			return fmt.Errorf("cannot get %s: %w", noun, err)
		}
	}
	return nil
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

// Ordering policy: no column here is marked DefaultSorter. BitBucket's commits endpoint returns
// commits newest-first (the same order `git log` prints); re-sorting that result ascending by
// date by default (as a prior revision did) silently reversed it to oldest-first with no way to
// opt back into the fetched order via --sort. Leaving every column here unmarked makes "no
// default sort, keep the server's own newest-first order" the actual default (see
// common.SortFlagValue/list.go), while --sort date remains available for an explicit
// oldest-first read.
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
	{Name: "date", DefaultSorter: false, Compare: func(a, b Commit) bool {
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

// Columns gets the column definitions for commits.
func Columns() common.Columns[Commit] {
	return columns
}

// GetHeaders gets the header for a table
//
// implements common.Tableable
func (commit Commit) GetHeaders(cmd *cobra.Command) []string {
	return common.HeadersFromFlag(cmd, "Hash", "Date", "Author", "Message")
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
			row = append(row, common.TimeCell(commit.Date))
		case "repository":
			row = append(row, commit.Repository.Name)
		default:
			row = append(row, common.EmptyCell)
		}
	}
	return row
}

// GetShortHash gets the short hash of this commit
func (commit Commit) GetShortHash() string {
	return shortHash(commit.Hash)
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
//
// Date is only formatted (and only included at all, via omitempty) when non-zero: a Commit built
// by hand rather than decoded from a real API response otherwise emits time.Time's own zero-value
// marshaling, "0001-01-01T00:00:00Z", into machine-readable JSON/YAML output.
func (commit Commit) MarshalJSON() (data []byte, err error) {
	type surrogate Commit

	data, err = json.Marshal(struct {
		Type string `json:"type"`
		surrogate
		Date *string `json:"date,omitempty"`
	}{
		Type:      commit.GetType(),
		surrogate: surrogate(commit),
		Date:      common.FormatOptionalTime(commit.Date),
	})
	if err != nil {
		return nil, fmt.Errorf("cannot marshal json: %w", err)
	}
	return data, nil
}
