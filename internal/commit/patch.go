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
	Use:               "patch <commit-hash-or-ref> <commit-hash-or-ref>",
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
	// Each hash/ref is validated on its own, before being joined with the literal ".." separator
	// below: repo.GetPath("patch", spec) is a bare path.Join with no escaping, so an unvalidated
	// hash/ref could splice extra path segments into the request. ValidatePathRef accepts a
	// multi-segment branch/tag ref (e.g. "release/1.0") in addition to a bare hash, validating each
	// '/'-delimited segment so no segment can be ".." for path.Join to collapse. The joined spec
	// itself legitimately contains ".." (BitBucket's own two-commit patch syntax), so ValidatePathRef
	// is never called on spec, only on each hash/ref that goes into it. Verified live against
	// Bitbucket's public API: GET /repositories/{workspace}/{repo_slug}/patch/{spec} returns the
	// expected patch for a spec built from a multi-segment branch name, raw slash and all -- unlike
	// GET .../commit/{revision} (see commit/get.go's own comment), which 404s on the same input.
	for _, hash := range args {
		if err := common.ValidatePathRef("commit-hash", hash); err != nil {
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
