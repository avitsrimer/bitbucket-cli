package cmd

import (
	"fmt"
	"io/fs"
	"os"
	"path/filepath"

	"github.com/avitsrimer/bitbucket-cli/internal/common"
	"github.com/avitsrimer/bitbucket-cli/skill"
	"github.com/spf13/cobra"
)

// skillName is the directory name the embedded skill tree lives under (skill/bitbucket-cli in
// the source checkout) and the name it is installed as under <to>/skills/.
const skillName = "bitbucket-cli"

// installSkillUnsupportedFlags are the root-persistent flags that install-skill inherits but has
// no use for: it does not touch a profile, workspace, or repository, does not batch several
// items (so the multi-error-handling flags are meaningless), and prints a single plain
// confirmation line rather than a Tableable a --output format could apply to.
var installSkillUnsupportedFlags = []string{
	"profile", "workspace", "repository", "output", "stop-on-error", "warn-on-error", "ignore-errors",
}

var installSkillCmd = &cobra.Command{
	Use:   "install-skill [flags]",
	Short: "install the bitbucket-cli Claude skill",
	Long: "Write the embedded bitbucket-cli Claude skill to <to>/skills/bitbucket-cli, defaulting " +
		"to ~/.claude when --to is unset. Re-running always overwrites the destination directory " +
		"wholesale -- any files added under it since the last install are deleted along with it, " +
		"not merged -- so re-installs are idempotent and pick up whatever skill content shipped " +
		"with this build of bb. Supports --dry-run: it reports the destination it would write to " +
		"and exits without touching the filesystem at all.",
	Args:    cobra.NoArgs,
	PreRunE: common.DisableUnsupportedFlags("install-skill", installSkillUnsupportedFlags...),
	RunE:    installSkillProcess,
}

func init() {
	installSkillCmd.Flags().String("to", "", "path to a .claude folder (default ~/.claude)")
	_ = installSkillCmd.MarkFlagDirname("to")
	installSkillCmd.SetHelpFunc(common.HideUnsupportedFlags(installSkillUnsupportedFlags...))
}

// installSkillProcess resolves --to (defaulting to os.UserHomeDir()/.claude), then writes the
// embedded skill tree to <to>/skills/bitbucket-cli, replacing any existing content there so
// re-installs are idempotent.
//
// The embedded tree is staged into a temporary sibling directory first and only swapped into
// place (via os.RemoveAll of the old dest followed by os.Rename of the staging directory) once
// every file has been written successfully. This keeps a pre-existing installation completely
// untouched if anything fails partway through -- reading an embedded file or writing a staged
// copy of it -- instead of the previous behavior of removing the destination up front and
// possibly leaving no skill installed at all on a mid-write failure.
func installSkillProcess(cmd *cobra.Command, _ []string) error {
	to := common.StringFlagValue(cmd, "to")
	if to == "" {
		home, err := os.UserHomeDir()
		if err != nil {
			return fmt.Errorf("resolve home dir: %w", err)
		}
		to = filepath.Join(home, ".claude")
	}
	skillsDir := filepath.Join(to, "skills")
	dest := filepath.Join(skillsDir, skillName)

	if !common.WhatIf(cmd, "Installing the bitbucket-cli skill to %s", dest) {
		return nil
	}

	if err := os.MkdirAll(skillsDir, 0o750); err != nil {
		return fmt.Errorf("create skills dir %s: %w", skillsDir, err)
	}

	// os.MkdirTemp only exists here to hand out a guaranteed-unique name under skillsDir; the
	// directory it creates (always mode 0o700) is immediately replaced by an os.Mkdir at that
	// same name so the staged tree's root ends up 0o750, matching every subdirectory MkdirAll
	// creates under it below.
	staged, err := os.MkdirTemp(skillsDir, ".bitbucket-cli-tmp-*")
	if err != nil {
		return fmt.Errorf("create staging dir: %w", err)
	}
	defer func() { _ = os.RemoveAll(staged) }() // no-op once the rename below has moved staged to dest
	if err = os.RemoveAll(staged); err != nil {
		return fmt.Errorf("reset staging dir %s: %w", staged, err)
	}
	if err = os.Mkdir(staged, 0o750); err != nil {
		return fmt.Errorf("create staging dir %s: %w", staged, err)
	}

	err = fs.WalkDir(skill.Files, skillName, func(path string, d fs.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		rel := path[len(skillName):]
		target := filepath.Join(staged, filepath.FromSlash(rel))
		if d.IsDir() {
			if mkErr := os.MkdirAll(target, 0o750); mkErr != nil {
				return fmt.Errorf("create dir %s: %w", target, mkErr)
			}
			return nil
		}
		data, readErr := skill.Files.ReadFile(path)
		if readErr != nil {
			return fmt.Errorf("read embedded %s: %w", path, readErr)
		}
		if writeErr := os.WriteFile(target, data, 0o600); writeErr != nil {
			return fmt.Errorf("write %s: %w", target, writeErr)
		}
		return nil
	})
	if err != nil {
		return fmt.Errorf("install skill: %w", err)
	}

	if err := os.RemoveAll(dest); err != nil {
		return fmt.Errorf("remove existing skill dir %s: %w", dest, err)
	}
	if err := os.Rename(staged, dest); err != nil {
		return fmt.Errorf("move staged skill into place at %s: %w", dest, err)
	}

	fmt.Fprintf(cmd.OutOrStdout(), "installed %s skill to %s\n", skillName, dest)
	return nil
}
