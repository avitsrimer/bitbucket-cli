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

func init() {
	Command.AddCommand(membersCmd)

	common.RegisterListFlags(membersCmd, memberColumns, "members")
}

func membersProcess(cmd *cobra.Command, args []string) error {
	var workspaceSlug string
	var err error

	// The workspace here is only ever used to build the /workspaces/{slug}/members request path
	// (via GetMembers), so an explicit argument is used as-is and the no-argument case resolves
	// the slug with no API call (GetWorkspaceName) instead of fetching a Workspace object.
	if len(args) == 0 {
		workspaceSlug, err = GetWorkspaceName(cmd.Context(), cmd)
		if err != nil {
			return fmt.Errorf("cannot get current workspace: %w", err)
		}
	} else {
		workspaceSlug = args[0]
	}

	lgr.Printf("[DEBUG] listing members of workspace %s", workspaceSlug)
	if !common.WhatIf(cmd, "Showing members of workspace "+workspaceSlug) {
		return nil
	}

	members, err := GetMembers(cmd.Context(), cmd, workspaceSlug)
	if err != nil {
		return fmt.Errorf("failed to retrieve members of workspace %s: %w", workspaceSlug, err)
	}
	if len(members) == 0 {
		fmt.Println("No member found")
		return nil
	}
	if sortValue := common.SortFlagValue(cmd); sortValue != "" {
		core.Sort(members, memberColumns.SortBy(sortValue))
	}
	if err := profile.Current.Print(cmd.Context(), cmd, Members(members)); err != nil {
		return fmt.Errorf("cannot print result: %w", err)
	}
	return nil
}
