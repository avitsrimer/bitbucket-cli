package workspace

import (
	"strings"

	"github.com/avitsrimer/bitbucket-cli/internal/common"
	"github.com/avitsrimer/bitbucket-cli/internal/user"
	"github.com/gildas/go-core"
	"github.com/spf13/cobra"
)

type Member struct {
	Type      string       `json:"type"`
	User      user.User    `json:"user"`
	Workspace Workspace    `json:"workspace"`
	Links     common.Links `json:"links"`
}

var memberColumns = common.Columns[Member]{
	{Name: "id", DefaultSorter: false, Compare: func(a, b Member) bool {
		return strings.ToLower(a.User.ID.String()) < strings.ToLower(b.User.ID.String())
	}},
	{Name: "name", DefaultSorter: true, Compare: func(a, b Member) bool {
		return strings.ToLower(a.User.Name) < strings.ToLower(b.User.Name)
	}},
	{Name: "username", DefaultSorter: false, Compare: func(a, b Member) bool {
		return strings.ToLower(a.User.Username) < strings.ToLower(b.User.Username)
	}},
	{Name: "workspace", DefaultSorter: false, Compare: func(a, b Member) bool {
		return strings.ToLower(a.Workspace.Slug) < strings.ToLower(b.Workspace.Slug)
	}},
}

// GetHeaders gets the header for a table
//
// implements common.Tableable
func (member Member) GetHeaders(cmd *cobra.Command) []string {
	if cmd != nil && cmd.Flag("columns") != nil && cmd.Flag("columns").Changed {
		if values, err := cmd.Flags().GetStringSlice("columns"); err == nil {
			return core.Map(values, func(column string) string { return strings.ReplaceAll(column, "_", " ") })
		}
	}
	return []string{"ID", "Name", "Workspace"}
}

// GetRow gets the row for a table
//
// implements common.Tableable
func (member Member) GetRow(headers []string) []string {
	var row []string

	for _, header := range headers {
		switch common.NormalizeColumnKey(header) {
		case "id":
			row = append(row, member.User.ID.String())
		case "name":
			row = append(row, member.User.Name)
		case "username":
			row = append(row, member.User.Username)
		case "workspace":
			row = append(row, member.Workspace.Slug)
		default:
			row = append(row, " ")
		}
	}
	return row
}

type Members []Member

// GetHeaders gets the header for a table
//
// implements common.Tableables
func (members Members) GetHeaders(cmd *cobra.Command) []string {
	return Member{}.GetHeaders(cmd)
}

// GetRowAt gets the row for a table
//
// implements common.Tableables
func (members Members) GetRowAt(index int, headers []string) []string {
	if index < 0 || index >= len(members) {
		return []string{}
	}
	return members[index].GetRow(headers)
}

// Size gets the number of elements
//
// implements common.Tableables
func (members Members) Size() int {
	return len(members)
}
