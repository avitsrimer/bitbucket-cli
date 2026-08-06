package artifact

import (
	"fmt"
	"net/url"
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
		"a failed download never leaves a stray empty or partial file behind. Overwriting a file " +
		"preserves its existing permissions; a newly created file gets the normal 0666 minus your " +
		"umask, not a restricted mode. The destination directory itself is never created and must " +
		"already exist.",
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
	if destination == "" {
		destination = "." // the current directory: destination's actual default
	}

	var errs []error
	for _, name := range args {
		if common.WhatIf(cmd, "Downloading artifact %s to %s", name, destination) {
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
	return common.TolerateErrors(cmd, profileCurrent, errs, "download these artifacts") //nolint:wrapcheck // TolerateErrors returns the same joined error verbatim (or nil); wrapping would prefix it with redundant noise
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

// downloadOne downloads a single artifact by name into destDir, naming the local file after
// filepath.Base(name) -- stripping any directory components name carries, so a "../"-laden name
// can never write outside destDir. The artifact is streamed into a temporary file in destDir and
// only renamed over the final destination path once the download completes successfully, so a
// failed attempt neither leaves a stray file behind nor corrupts a file already at that
// destination. The temp file's mode is adjusted before the rename to match the file it replaces
// (or the process umask for a new file), so a download never silently downgrades an existing
// destination file to os.CreateTemp's owner-only 0600.
func downloadOne(cmd *cobra.Command, profileCurrent *profile.Profile, repo *repository.Repository, destDir, name string) error {
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

	// name is an arbitrary, user-facing filename -- it can legitimately contain "%", "?", spaces,
	// or other characters that are not safe to drop unescaped into a URL path segment (a bare "%"
	// makes the request silently target the API root instead of erroring; a "?" would be
	// mistaken for the query separator). url.PathEscape makes it survive both GetPath's path.Join
	// and resolveRequestURL's path/query split intact.
	if _, err := profileCurrent.Download(cmd.Context(), repo.GetPath("downloads", url.PathEscape(name)), tempFile); err != nil {
		return fmt.Errorf("cannot download artifact %s: %w", name, err)
	}
	if err := tempFile.Chmod(destinationFileMode(destPath)); err != nil {
		return fmt.Errorf("cannot set permissions on downloaded artifact %s: %w", destPath, err)
	}
	if err := tempFile.Close(); err != nil {
		return fmt.Errorf("cannot finalize destination file %s: %w", destPath, err)
	}
	if err := os.Rename(tempPath, destPath); err != nil {
		return fmt.Errorf("cannot move downloaded artifact to %s: %w", destPath, err)
	}
	return nil
}

// destinationFileMode returns the mode of the file already at destPath, so overwriting it
// preserves that mode; when destPath does not exist yet, it returns the mode the process's umask
// would leave from a request for 0o666 (matching what os.Create/os.WriteFile would produce for a
// brand-new file), rather than os.CreateTemp's fixed owner-only 0600.
func destinationFileMode(destPath string) os.FileMode {
	if info, err := os.Stat(destPath); err == nil {
		return info.Mode().Perm()
	}
	// os.CreateTemp always requests 0600 regardless of umask, so probe with os.OpenFile (which
	// does apply the umask, like os.Create) against a name guaranteed not to collide. The 0666
	// request is deliberate, not a hardening gap: it exists solely to read back what the umask
	// leaves of it, and the probe file itself is removed immediately below, never left on disk.
	probeName := filepath.Join(filepath.Dir(destPath), fmt.Sprintf(".artifact-mode-probe-%d", os.Getpid()))
	probe, err := os.OpenFile(probeName, os.O_RDWR|os.O_CREATE|os.O_EXCL, 0o666) //nolint:gosec // G302: probing the umask-adjusted default for a *new* file requires requesting 0666 (matching os.Create's own default); see comment above
	if err != nil {
		return 0o644
	}
	defer func() {
		_ = probe.Close()
		_ = os.Remove(probeName)
	}()
	if info, err := probe.Stat(); err == nil {
		return info.Mode().Perm()
	}
	return 0o644
}
