package workspace

import (
	"fmt"

	"github.com/avitsrimer/bitbucket-cli/internal/common"
	"github.com/avitsrimer/bitbucket-cli/internal/profile"
	"github.com/gildas/go-core"
	"github.com/go-pkgz/lgr"
	"github.com/spf13/cobra"
)

var membersCmd = &cobra.Command{
	Use:               "members [flags] [<workspace-slug-or-id>]",
	Short:             "list the members of a workspace, or the current workspace by default",
	Args:              cobra.RangeArgs(0, 1),
	ValidArgsFunction: getValidArgs,
	RunE:              membersProcess,
}

var membersOptions struct {
	Columns *common.EnumSliceFlag
	SortBy  *common.EnumFlag
}

func init() {
	Command.AddCommand(membersCmd)

	membersOptions.Columns, membersOptions.SortBy = registerListFlags(membersCmd, memberColumns, "members")
}

func membersProcess(cmd *cobra.Command, args []string) error {
	var target *Workspace
	var err error

	if len(args) == 0 {
		target, err = GetWorkspace(cmd.Context(), cmd)
		if err != nil {
			return fmt.Errorf("cannot get current workspace: %w", err)
		}
	} else {
		target, err = GetWorkspaceBySlugOrID(cmd.Context(), cmd, args[0])
		if err != nil {
			return fmt.Errorf("cannot get workspace %s: %w", args[0], err)
		}
	}

	lgr.Printf("[DEBUG] listing members of workspace %s", target.Slug)
	if !common.WhatIf(cmd, "Showing members of workspace "+target.Slug) {
		return nil
	}

	members, err := target.GetMembers(cmd.Context(), cmd)
	if err != nil {
		return fmt.Errorf("failed to retrieve members of workspace %s: %w", target.Slug, err)
	}
	if len(members) == 0 {
		fmt.Println("No member found")
		return nil
	}
	if sortFlag := cmd.Flag("sort"); sortFlag != nil && sortFlag.Changed {
		core.Sort(members, memberColumns.SortBy(membersOptions.SortBy.Value))
	}
	if err := profile.Current.Print(cmd.Context(), cmd, Members(members)); err != nil {
		return fmt.Errorf("cannot print result: %w", err)
	}
	return nil
}
