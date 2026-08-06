package workspace

import (
	"fmt"

	"github.com/avitsrimer/bitbucket-cli/internal/common"
	"github.com/avitsrimer/bitbucket-cli/internal/profile"
	"github.com/go-pkgz/lgr"
	"github.com/spf13/cobra"
)

var getCmd = &cobra.Command{
	Use:               "get [flags] [<workspace-slug-or-id>]",
	Aliases:           []string{"show", "info", "display"},
	Short:             "get a workspace by its <workspace-slug-or-id>, or the current workspace by default",
	Args:              cobra.RangeArgs(0, 1),
	ValidArgsFunction: getValidArgs,
	RunE:              getProcess,
}

var getOptions struct {
	Columns *common.EnumSliceFlag
}

func init() {
	Command.AddCommand(getCmd)

	getOptions.Columns = common.NewEnumSliceFlag(columns.Columns()...)
	getCmd.Flags().Var(getOptions.Columns, "columns", "Comma-separated list of columns to display")
	_ = getCmd.RegisterFlagCompletionFunc(getOptions.Columns.CompletionFunc("columns"))
}

func getValidArgs(cmd *cobra.Command, args []string, toComplete string) ([]string, cobra.ShellCompDirective) {
	if len(args) != 0 {
		return nil, cobra.ShellCompDirectiveNoFileComp
	}

	slugs, err := GetWorkspaceAllowedSlugs(cmd.Context(), cmd, args, toComplete)
	if err != nil {
		cobra.CompErrorln(err.Error())
		return []string{}, cobra.ShellCompDirectiveError
	}
	return common.FilterValidArgs(slugs, args, toComplete), cobra.ShellCompDirectiveNoFileComp
}

func getProcess(cmd *cobra.Command, args []string) error {
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

	lgr.Printf("[DEBUG] displaying workspace %s", target.Slug)
	if !common.WhatIf(cmd, "Showing workspace "+target.Slug) {
		return nil
	}

	profileCurrent, err := profile.GetProfileFromCommand(cmd.Context(), cmd)
	if err != nil {
		return fmt.Errorf("cannot get profile: %w", err)
	}
	if err := profileCurrent.Print(cmd.Context(), cmd, *target); err != nil {
		return fmt.Errorf("cannot print result: %w", err)
	}
	return nil
}
