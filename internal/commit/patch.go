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

var patchCmd = &cobra.Command{
	Use:               "patch <commit-hash> <commit-hash>",
	Short:             "show the patch between two commits",
	Args:              cobra.ExactArgs(2),
	ValidArgsFunction: patchValidArgs,
	RunE:              patchProcess,
}

func init() {
	Command.AddCommand(patchCmd)
}

func patchValidArgs(cmd *cobra.Command, args []string, toComplete string) ([]string, cobra.ShellCompDirective) {
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

func patchProcess(cmd *cobra.Command, args []string) error {
	// Each hash is validated on its own, before being joined with the literal ".." separator
	// below: repo.GetPath("patch", spec) is a bare path.Join with no escaping, so an unvalidated
	// hash could splice extra path segments into the request. The joined spec itself legitimately
	// contains ".." (BitBucket's own two-commit patch syntax), so ValidatePathIdentifier is never
	// called on spec, only on each hash that goes into it.
	for _, hash := range args {
		if err := common.ValidatePathIdentifier("commit-hash", hash); err != nil {
			return fmt.Errorf("cannot get patch: %w", err)
		}
	}

	spec := args[0] + ".." + args[1]

	lgr.Printf("[DEBUG] displaying patch for %s", spec)
	if !common.WhatIf(cmd, "Showing patch for "+spec) {
		return nil
	}

	repo, err := repository.GetRepository(cmd.Context(), cmd)
	if err != nil {
		return fmt.Errorf("cannot get repository: %w", err)
	}

	profileCurrent, err := profile.GetProfileFromCommand(cmd.Context(), cmd)
	if err != nil {
		return fmt.Errorf("cannot get profile: %w", err)
	}
	patch, err := profileCurrent.GetRaw(cmd.Context(), repo.GetPath("patch", spec))
	if err != nil {
		return fmt.Errorf("cannot get patch: %w", err)
	}

	if _, err := io.Copy(os.Stdout, patch); err != nil {
		return fmt.Errorf("cannot write patch: %w", err)
	}
	return nil
}
