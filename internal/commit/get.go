package commit

import (
	"fmt"

	"github.com/avitsrimer/bitbucket-cli/internal/common"
	"github.com/avitsrimer/bitbucket-cli/internal/profile"
	"github.com/go-pkgz/lgr"
	"github.com/spf13/cobra"
)

var getCmd = &cobra.Command{
	Use:               "get [flags] [<commit-hash>]",
	Aliases:           []string{"show", "describe"},
	Short:             "get a commit by its <commit-hash>, or the latest commit by default",
	Args:              cobra.RangeArgs(0, 1),
	ValidArgsFunction: getValidArgs,
	RunE:              getProcess,
}

func init() {
	Command.AddCommand(getCmd)

	common.RegisterColumnsFlag(getCmd, columns)
}

func getValidArgs(cmd *cobra.Command, args []string, toComplete string) ([]string, cobra.ShellCompDirective) {
	if len(args) != 0 {
		return nil, cobra.ShellCompDirectiveNoFileComp
	}

	hashes, err := GetCommitHashes(cmd.Context(), cmd, args, toComplete)
	if err != nil {
		cobra.CompErrorln(err.Error())
		return []string{}, cobra.ShellCompDirectiveError
	}
	return common.FilterValidArgs(hashes, args, toComplete), cobra.ShellCompDirectiveNoFileComp
}

func getProcess(cmd *cobra.Command, args []string) error {
	var target *Commit
	var err error

	if len(args) == 0 {
		target, err = GetLatestCommit(cmd.Context(), cmd)
		if err != nil {
			return fmt.Errorf("cannot get latest commit: %w", err)
		}
	} else {
		// A hash of "", ".", or ".." must be rejected outright: repo.GetPath("commits", hash)
		// runs the segments through path.Join, which collapses any of these three away instead
		// of erroring, silently retargeting the request at a *different* endpoint (the commits
		// list, for an empty/"." hash) that then succeeds and prints the newest commit as if the
		// given hash had matched.
		if validateErr := common.ValidatePathIdentifier("commit-hash", args[0]); validateErr != nil {
			return fmt.Errorf("cannot get commit: %w", validateErr)
		}
		target, err = GetCommitByHash(cmd.Context(), cmd, args[0])
		if err != nil {
			return fmt.Errorf("cannot get commit %s: %w", args[0], err)
		}
	}

	lgr.Printf("[DEBUG] displaying commit %s", target.GetShortHash())
	if !common.WhatIf(cmd, "Showing commit "+target.GetShortHash()) {
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
