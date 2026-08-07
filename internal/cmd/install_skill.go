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
		"to $CLAUDE_CONFIG_DIR (or ~/.claude if that's unset) when --to is unset. Re-running always " +
		"overwrites the destination directory " +
		"wholesale -- any files added under it since the last install are deleted along with it, " +
		"not merged -- so re-installs are idempotent and pick up whatever skill content shipped " +
		"with this build of bb. Supports --dry-run: it reports the destination it would write to " +
		"and exits without touching the filesystem at all.",
	Args:    cobra.NoArgs,
	PreRunE: common.DisableUnsupportedFlags("install-skill", installSkillUnsupportedFlags...),
	RunE:    installSkillProcess,
}

func init() {
	installSkillCmd.Flags().String("to", "", "path to a .claude folder (default $CLAUDE_CONFIG_DIR or ~/.claude)")
	_ = installSkillCmd.MarkFlagDirname("to")
	installSkillCmd.SetHelpFunc(common.HideUnsupportedFlags(installSkillUnsupportedFlags...))
}

// defaultInstallSkillTo resolves the --to default: $CLAUDE_CONFIG_DIR when set (matching the
// personal-projects workflow of launching Claude Code with CLAUDE_CONFIG_DIR pointed at a
// non-default config directory), otherwise os.UserHomeDir()/.claude.
func defaultInstallSkillTo() (string, error) {
	if dir := os.Getenv("CLAUDE_CONFIG_DIR"); dir != "" {
		return dir, nil
	}
	home, err := os.UserHomeDir()
	if err != nil {
		return "", fmt.Errorf("resolve home dir: %w", err)
	}
	return filepath.Join(home, ".claude"), nil
}

// dirMode is the permission every directory this command creates ends up with: rwxr-x---.
const dirMode = 0o750

// chmodDir forces path to dirMode right after it was freshly created: Mkdir/MkdirAll's mode
// argument is masked by the process umask when the directory doesn't already exist, so a
// restrictive umask (e.g. 077) would otherwise silently produce narrower directory permissions
// than intended. Only ever call this on a directory this process just created -- never on one
// that may have pre-existed with deliberately different permissions.
func chmodDir(path string) error {
	if err := os.Chmod(path, dirMode); err != nil {
		return fmt.Errorf("chmod %s: %w", path, err)
	}
	return nil
}

// ensureSkillsDir creates skillsDir (and any missing parents) if it doesn't already exist. A
// freshly created directory is chmod'd to dirMode to stay deterministic under a restrictive
// umask; a pre-existing skillsDir is left exactly as found, so a caller-managed <to>/skills with
// deliberately different permissions is never overwritten by an install.
func ensureSkillsDir(skillsDir string) error {
	existed := true
	if _, statErr := os.Stat(skillsDir); os.IsNotExist(statErr) {
		existed = false
	}
	if err := os.MkdirAll(skillsDir, dirMode); err != nil {
		return fmt.Errorf("create skills dir %s: %w", skillsDir, err)
	}
	if !existed {
		if err := chmodDir(skillsDir); err != nil {
			return err
		}
	}
	return nil
}

// stageSkillTree creates a fresh, empty staging directory under skillsDir and returns its path.
// The directory os.MkdirTemp hands back is immediately reset and recreated at dirMode (rather
// than the 0o700 MkdirTemp always uses) so it matches every subdirectory created under it below.
func stageSkillTree(skillsDir string) (staged string, err error) {
	staged, err = os.MkdirTemp(skillsDir, ".bitbucket-cli-tmp-*")
	if err != nil {
		return "", fmt.Errorf("create staging dir: %w", err)
	}
	if err = os.RemoveAll(staged); err != nil {
		return "", fmt.Errorf("reset staging dir %s: %w", staged, err)
	}
	if err = os.Mkdir(staged, dirMode); err != nil {
		return "", fmt.Errorf("create staging dir %s: %w", staged, err)
	}
	if err := chmodDir(staged); err != nil {
		return "", err
	}
	return staged, nil
}

// installSkillProcess resolves --to (defaulting to $CLAUDE_CONFIG_DIR or os.UserHomeDir()/.claude),
// then writes the embedded skill tree to <to>/skills/bitbucket-cli, replacing any existing content
// there so re-installs are idempotent.
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
		var err error
		to, err = defaultInstallSkillTo()
		if err != nil {
			return err
		}
	}
	skillsDir := filepath.Join(to, "skills")
	dest := filepath.Join(skillsDir, skillName)

	if !common.WhatIf(cmd, "Installing the bitbucket-cli skill to %s", dest) {
		return nil
	}

	if err := ensureSkillsDir(skillsDir); err != nil {
		return err
	}

	staged, err := stageSkillTree(skillsDir)
	if err != nil {
		return err
	}
	defer func() { _ = os.RemoveAll(staged) }() // no-op once the rename below has moved staged to dest

	err = fs.WalkDir(skill.Files, skillName, func(path string, d fs.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		rel := path[len(skillName):]
		target := filepath.Join(staged, filepath.FromSlash(rel))
		if d.IsDir() {
			if mkErr := os.MkdirAll(target, dirMode); mkErr != nil {
				return fmt.Errorf("create dir %s: %w", target, mkErr)
			}
			if chmodErr := chmodDir(target); chmodErr != nil {
				return chmodErr
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
