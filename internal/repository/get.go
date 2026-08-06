package repository

import (
	"fmt"

	"github.com/avitsrimer/bitbucket-cli/internal/common"
	"github.com/avitsrimer/bitbucket-cli/internal/profile"
	"github.com/go-pkgz/lgr"
	"github.com/spf13/cobra"
)

var getCmd = &cobra.Command{
	Use:               "get [flags] [<repository-slug-or-uuid>]",
	Aliases:           []string{"show", "info", "display"},
	Short:             "get a repository by its <repository-slug-or-uuid>, or the current repository by default",
	Args:              cobra.RangeArgs(0, 1),
	ValidArgsFunction: getValidArgs,
	PreRunE:           common.DisableUnsupportedFlags("repository", "repository"),
	RunE:              getProcess,
}

func init() {
	Command.AddCommand(getCmd)

	common.RegisterColumnsFlag(getCmd, columns)
	getCmd.SetHelpFunc(common.HideUnsupportedFlags("repository"))
}

func getValidArgs(cmd *cobra.Command, args []string, toComplete string) ([]string, cobra.ShellCompDirective) {
	if len(args) != 0 {
		return nil, cobra.ShellCompDirectiveNoFileComp
	}

	slugs, err := GetRepositorySlugs(cmd.Context(), cmd)
	if err != nil {
		cobra.CompErrorln(err.Error())
		return []string{}, cobra.ShellCompDirectiveError
	}
	return common.FilterValidArgs(slugs, args, toComplete), cobra.ShellCompDirectiveNoFileComp
}

func getProcess(cmd *cobra.Command, args []string) error {
	var target *Repository
	var err error

	if len(args) == 0 {
		target, err = GetRepository(cmd.Context(), cmd)
		if err != nil {
			return fmt.Errorf("cannot get current repository: %w", err)
		}
	} else {
		target, err = GetRepositoryBySlugOrID(cmd.Context(), cmd, args[0])
		if err != nil {
			return fmt.Errorf("cannot get repository %s: %w", args[0], err)
		}
	}

	lgr.Printf("[DEBUG] displaying repository %s", target.Slug)
	if !common.WhatIf(cmd, "Showing repository "+target.Slug) {
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
