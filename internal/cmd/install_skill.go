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

var installSkillCmd = &cobra.Command{
	Use:   "install-skill [flags]",
	Short: "install the bitbucket-cli Claude skill",
	Long: "Write the embedded bitbucket-cli Claude skill to <to>/skills/bitbucket-cli, defaulting " +
		"to ~/.claude when --to is unset. Re-running always overwrites the destination, so " +
		"re-installs are idempotent and pick up whatever skill content shipped with this build " +
		"of bb.",
	Args: cobra.NoArgs,
	RunE: installSkillProcess,
}

func init() {
	installSkillCmd.Flags().String("to", "", "path to a .claude folder (default ~/.claude)")
	_ = installSkillCmd.MarkFlagDirname("to")
}

// installSkillProcess resolves --to (defaulting to os.UserHomeDir()/.claude), then writes the
// embedded skill tree to <to>/skills/bitbucket-cli, removing any existing content there first so
// re-installs are idempotent.
func installSkillProcess(cmd *cobra.Command, _ []string) error {
	to := common.StringFlagValue(cmd, "to")
	if to == "" {
		home, err := os.UserHomeDir()
		if err != nil {
			return fmt.Errorf("resolve home dir: %w", err)
		}
		to = filepath.Join(home, ".claude")
	}
	dest := filepath.Join(to, "skills", skillName)

	if !common.WhatIf(cmd, "Installing the bitbucket-cli skill to %s", dest) {
		return nil
	}

	if err := os.RemoveAll(dest); err != nil {
		return fmt.Errorf("remove existing skill dir %s: %w", dest, err)
	}

	err := fs.WalkDir(skill.Files, skillName, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		rel := path[len(skillName):]
		target := filepath.Join(dest, filepath.FromSlash(rel))
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

	fmt.Fprintf(cmd.OutOrStdout(), "installed %s skill to %s\n", skillName, dest)
	return nil
}
