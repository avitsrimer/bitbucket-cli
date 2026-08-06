package workspace

import (
	"fmt"

	"github.com/avitsrimer/bitbucket-cli/internal/common"
	"github.com/avitsrimer/bitbucket-cli/internal/profile"
	"github.com/gildas/go-core"
	"github.com/go-pkgz/lgr"
	"github.com/spf13/cobra"
)

var listCmd = &cobra.Command{
	Use:   "list",
	Short: "list all workspaces for the current user",
	Args:  cobra.NoArgs,
	RunE:  listProcess,
}

var listOptions struct {
	Columns *common.EnumSliceFlag
	SortBy  *common.EnumFlag
}

func init() {
	Command.AddCommand(listCmd)

	listOptions.Columns, listOptions.SortBy = registerListFlags(listCmd, columns, "workspaces")
}

func listProcess(cmd *cobra.Command, args []string) error {
	lgr.Printf("[DEBUG] listing all workspaces")
	if !common.WhatIf(cmd, "Showing workspaces") {
		return nil
	}

	workspaces, err := profile.GetAll[Workspace](cmd.Context(), cmd, "/user/workspaces")
	if err != nil {
		return fmt.Errorf("failed to retrieve workspaces: %w", err)
	}
	if len(workspaces) == 0 {
		fmt.Println("No workspace found")
		return nil
	}
	if sortFlag := cmd.Flag("sort"); sortFlag != nil && sortFlag.Changed {
		core.Sort(workspaces, columns.SortBy(listOptions.SortBy.Value))
	}
	if err := profile.Current.Print(cmd.Context(), cmd, Workspaces(workspaces)); err != nil {
		return fmt.Errorf("cannot print result: %w", err)
	}
	return nil
}
