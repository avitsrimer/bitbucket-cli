package branch

import (
	"encoding/json"
	"fmt"
	"strings"

	"github.com/avitsrimer/bitbucket-cli/internal/commit"
	"github.com/avitsrimer/bitbucket-cli/internal/common"
	"github.com/spf13/cobra"
)

type Branch struct {
	Name                 string        `json:"name"`
	Target               commit.Commit `json:"target"`
	Links                common.Links  `json:"links"`
	MergeStrategies      []string      `json:"merge_strategies,omitempty"`
	DefaultMergeStrategy string        `json:"default_merge_strategy,omitempty"`
}

// Command represents this folder's command
var Command = &cobra.Command{
	Use:   "branch",
	Short: "Manage branches",
	Run:   common.SubcommandRequired("Branch"),
}

var columns = common.Columns[Branch]{
	{Name: "name", DefaultSorter: true, Compare: func(a, b Branch) bool {
		return strings.ToLower(a.Name) < strings.ToLower(b.Name)
	}},
	{Name: "target", DefaultSorter: false, Compare: func(a, b Branch) bool {
		return strings.ToLower(a.Target.Hash) < strings.ToLower(b.Target.Hash)
	}},
	{Name: "default_merge_strategy", DefaultSorter: false, Compare: func(a, b Branch) bool {
		return strings.ToLower(a.DefaultMergeStrategy) < strings.ToLower(b.DefaultMergeStrategy)
	}},
	{Name: "merge_strategies", DefaultSorter: false, Compare: func(a, b Branch) bool {
		return strings.ToLower(strings.Join(a.MergeStrategies, ",")) < strings.ToLower(strings.Join(b.MergeStrategies, ","))
	}},
}

// GetType returns the branch type
func (branch Branch) GetType() string {
	return "branch"
}

// GetHeaders gets the header for a table
//
// implements common.Tableable
func (branch Branch) GetHeaders(cmd *cobra.Command) []string {
	return common.HeadersFromFlag(cmd, "Name")
}

// GetRow gets the row for a table
//
// implements common.Tableable
func (branch Branch) GetRow(headers []string) []string {
	var row []string

	for _, header := range headers {
		switch common.NormalizeColumnKey(header) {
		case "name":
			row = append(row, branch.Name)
		case "target":
			row = append(row, branch.Target.Hash)
		case "default_merge_strategy":
			row = append(row, branch.DefaultMergeStrategy)
		case "merge_strategies":
			row = append(row, strings.Join(branch.MergeStrategies, ", "))
		default:
			row = append(row, " ")
		}
	}
	return row
}

// String gets a string representation of this Branch
//
// implements fmt.Stringer
func (branch Branch) String() string {
	return branch.Name
}

// MarshalJSON custom JSON marshaling for Branch
//
// implements json.Marshaler
func (branch Branch) MarshalJSON() ([]byte, error) {
	type surrogate Branch
	data, err := json.Marshal(struct {
		Type string `json:"type"`
		surrogate
	}{
		Type:      branch.GetType(),
		surrogate: surrogate(branch),
	})
	if err != nil {
		return nil, fmt.Errorf("cannot marshal json: %w", err)
	}
	return data, nil
}

// UnmarshalJSON custom JSON unmarshalling for Branch
//
// implements json.Unmarshaler
func (branch *Branch) UnmarshalJSON(data []byte) error {
	type surrogate Branch
	var inner struct {
		Type string `json:"type"`
		surrogate
	}

	if err := json.Unmarshal(data, &inner); err != nil {
		return fmt.Errorf("cannot unmarshal json: %w", err)
	}
	if inner.Type != branch.GetType() {
		return fmt.Errorf("invalid type %s, expected %s", inner.Type, branch.GetType())
	}
	*branch = Branch(inner.surrogate)
	return nil
}
