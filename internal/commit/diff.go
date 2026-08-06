package commit

import (
	"fmt"
	"io"
	"os"

	"github.com/avitsrimer/bitbucket-cli/internal/common"
	"github.com/avitsrimer/bitbucket-cli/internal/profile"
	"github.com/avitsrimer/bitbucket-cli/internal/repository"
	"github.com/go-pkgz/lgr"
	"github.com/spf13/cobra"
)

var diffCmd = &cobra.Command{
	Use:               "diff [flags] <commit-hash> [<commit-hash>]",
	Short:             "show the diff of a commit, or the diff between two commits",
	Args:              cobra.RangeArgs(1, 2),
	ValidArgsFunction: diffValidArgs,
	RunE:              diffProcess,
}

var diffOptions struct {
	Stat bool
}

func init() {
	Command.AddCommand(diffCmd)

	diffCmd.Flags().BoolVar(&diffOptions.Stat, "stat", false, "show only the diffstat")
}

func diffValidArgs(cmd *cobra.Command, args []string, toComplete string) ([]string, cobra.ShellCompDirective) {
	if len(args) >= 2 {
		return nil, cobra.ShellCompDirectiveNoFileComp
	}

	hashes, err := GetCommitHashes(cmd.Context(), cmd, args, toComplete)
	if err != nil {
		cobra.CompErrorln(err.Error())
		return []string{}, cobra.ShellCompDirectiveError
	}
	return common.FilterValidArgs(hashes, args, toComplete), cobra.ShellCompDirectiveNoFileComp
}

func diffProcess(cmd *cobra.Command, args []string) error {
	spec := args[0]
	if len(args) > 1 {
		spec = args[0] + ".." + args[1]
	}

	lgr.Printf("[DEBUG] displaying diff for %s", spec)
	if !common.WhatIf(cmd, "Showing diff for "+spec) {
		return nil
	}

	repo, err := repository.GetRepository(cmd.Context(), cmd)
	if err != nil {
		return fmt.Errorf("cannot get repository: %w", err)
	}

	uripath := repo.GetPath("diff", spec)
	if diffOptions.Stat {
		uripath = repo.GetPath("diffstat", spec)
	}

	profileCurrent, err := profile.GetProfileFromCommand(cmd.Context(), cmd)
	if err != nil {
		return fmt.Errorf("cannot get profile: %w", err)
	}
	diff, err := profileCurrent.GetRaw(cmd.Context(), uripath)
	if err != nil {
		return fmt.Errorf("cannot get diff: %w", err)
	}

	if _, err := io.Copy(os.Stdout, diff); err != nil {
		return fmt.Errorf("cannot write diff: %w", err)
	}
	return nil
}
