package artifact

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"

	"github.com/avitsrimer/bitbucket-cli/internal/common"
	"github.com/avitsrimer/bitbucket-cli/internal/profile"
	"github.com/avitsrimer/bitbucket-cli/internal/repository"
	"github.com/go-pkgz/lgr"
	"github.com/spf13/cobra"
)

var downloadCmd = &cobra.Command{
	Use:     "download [flags] <name...>",
	Aliases: []string{"get", "fetch"},
	Short:   "download artifacts by their <name>",
	Long: "Download one or more artifacts by their <name> to --destination (default: the current " +
		"directory). Each artifact is written under the base name of <name> (any directory " +
		"components it carries are stripped, so a name cannot write outside the destination " +
		"directory), overwriting a file already there and creating it otherwise; the destination " +
		"file is only created or replaced once its whole download has completed successfully, so " +
		"a failed download never leaves a stray empty or partial file behind. The destination " +
		"directory itself is never created and must already exist.",
	Args:              cobra.MinimumNArgs(1),
	ValidArgsFunction: downloadValidArgs,
	RunE:              downloadProcess,
}

func init() {
	Command.AddCommand(downloadCmd)

	downloadCmd.Flags().String("destination", "", "Destination folder to download the artifact(s) to. Defaults to the current folder; the folder must already exist")
	_ = downloadCmd.MarkFlagDirname("destination")
}

func downloadValidArgs(cmd *cobra.Command, args []string, toComplete string) ([]string, cobra.ShellCompDirective) {
	if len(args) != 0 {
		return nil, cobra.ShellCompDirectiveNoFileComp
	}

	names, err := GetArtifactNames(cmd.Context(), cmd)
	if err != nil {
		cobra.CompErrorln(err.Error())
		return []string{}, cobra.ShellCompDirectiveError
	}
	return common.FilterValidArgs(names, args, toComplete), cobra.ShellCompDirectiveNoFileComp
}

func downloadProcess(cmd *cobra.Command, args []string) error {
	profileCurrent, err := profile.GetProfileFromCommand(cmd.Context(), cmd)
	if err != nil {
		return fmt.Errorf("cannot get profile: %w", err)
	}

	repo, err := repository.GetRepository(cmd.Context(), cmd)
	if err != nil {
		return fmt.Errorf("cannot get repository: %w", err)
	}

	destination := destinationFlagValue(cmd)

	var errs []error
	for _, name := range args {
		if common.WhatIf(cmd, "Downloading artifact %s to %s", name, displayDestination(destination)) {
			if err := downloadOne(cmd, profileCurrent, repo, destination, name); err != nil {
				if profileCurrent.ShouldStopOnError(cmd) {
					return fmt.Errorf("failed to download artifact %s: %w", name, err)
				}
				errs = append(errs, err)
				continue
			}
			lgr.Printf("[DEBUG] artifact %s downloaded", name)
		}
	}
	joined := errors.Join(errs...)
	if joined != nil && profileCurrent.ShouldWarnOnError(cmd) {
		fmt.Fprintf(os.Stderr, "Failed to download these artifacts: %s\n", joined)
		return nil
	}
	if profileCurrent.ShouldIgnoreErrors(cmd) {
		lgr.Printf("[WARN] failed to download these artifacts, but ignoring errors: %s", joined)
		return nil
	}
	return joined
}

// destinationFlagValue reads cmd's own --destination flag directly (rather than binding it to a
// package-level variable, which would only ever be populated on the real downloadCmd instance), so
// downloadProcess behaves the same whether cmd is downloadCmd itself or a standalone test command
// carrying its own --destination flag.
func destinationFlagValue(cmd *cobra.Command) string {
	flag := cmd.Flag("destination")
	if flag == nil {
		return ""
	}
	value, err := cmd.Flags().GetString("destination")
	if err != nil {
		return ""
	}
	return value
}

// displayDestination returns destination for the WhatIf message, or "." (the current directory,
// destination's actual default) when it is empty, so the dry-run message never shows a blank
// destination.
func displayDestination(destination string) string {
	if destination == "" {
		return "."
	}
	return destination
}

// downloadOne downloads a single artifact by name into destDir (defaulting to the current
// directory), naming the local file after filepath.Base(name) -- stripping any directory
// components name carries, so a "../"-laden name can never write outside destDir. The artifact is
// streamed into a temporary file in destDir and only renamed over the final destination path once
// the download completes successfully, so a failed attempt neither leaves a stray file behind nor
// corrupts a file already at that destination.
func downloadOne(cmd *cobra.Command, profileCurrent *profile.Profile, repo *repository.Repository, destDir, name string) error {
	if destDir == "" {
		destDir = "."
	}
	destPath := filepath.Join(destDir, filepath.Base(name))

	tempFile, err := os.CreateTemp(destDir, ".artifact-*.tmp")
	if err != nil {
		return fmt.Errorf("cannot create temporary file in %s: %w", destDir, err)
	}
	tempPath := tempFile.Name()
	defer func() {
		_ = tempFile.Close()
		_ = os.Remove(tempPath) // no-op once the temp file has already been renamed to destPath
	}()

	if _, err := profileCurrent.Download(cmd.Context(), repo.GetPath("downloads", name), tempFile); err != nil {
		return fmt.Errorf("cannot download artifact %s: %w", name, err)
	}
	if err := tempFile.Close(); err != nil {
		return fmt.Errorf("cannot finalize destination file %s: %w", destPath, err)
	}
	if err := os.Rename(tempPath, destPath); err != nil {
		return fmt.Errorf("cannot move downloaded artifact to %s: %w", destPath, err)
	}
	return nil
}
