package cmd

import (
	"bytes"
	"os"
	"path/filepath"
	"testing"

	"github.com/avitsrimer/bitbucket-cli/skill"
	"github.com/spf13/cobra"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// newInstallSkillTestCmd builds a standalone *cobra.Command carrying its own --to and --dry-run
// flags (never the real, process-global installSkillCmd), so it exercises installSkillProcess
// exactly the way a real invocation would while staying isolated from RootCmd/cobra.OnInitialize.
func newInstallSkillTestCmd() (cmd *cobra.Command, out *bytes.Buffer) {
	out = &bytes.Buffer{}
	cmd = &cobra.Command{Use: "install-skill", RunE: installSkillProcess}
	cmd.Flags().String("to", "", "path to a .claude folder (default ~/.claude)")
	cmd.Flags().Bool("dry-run", false, "")
	cmd.SetOut(out)
	cmd.SetErr(&bytes.Buffer{})
	return cmd, out
}

func TestInstallSkillFreshInstall(t *testing.T) {
	tmp := t.TempDir()
	cmd, out := newInstallSkillTestCmd()
	require.NoError(t, cmd.Flags().Set("to", tmp))

	require.NoError(t, cmd.RunE(cmd, nil))

	dest := filepath.Join(tmp, "skills", "bitbucket-cli")
	assert.Contains(t, out.String(), "installed bitbucket-cli skill to "+dest)

	dirInfo, err := os.Stat(dest)
	require.NoError(t, err)
	assert.Equal(t, os.FileMode(0o750), dirInfo.Mode().Perm(), "skill dir should be 0o750")

	skillPath := filepath.Join(dest, "SKILL.md")
	fileInfo, err := os.Stat(skillPath)
	require.NoError(t, err)
	assert.Equal(t, os.FileMode(0o600), fileInfo.Mode().Perm(), "skill file should be 0o600")
}

func TestInstallSkillEveryEmbeddedFileLandsByteIdentical(t *testing.T) {
	tmp := t.TempDir()
	cmd, _ := newInstallSkillTestCmd()
	require.NoError(t, cmd.Flags().Set("to", tmp))

	require.NoError(t, cmd.RunE(cmd, nil))

	dest := filepath.Join(tmp, "skills", "bitbucket-cli")
	assertEmbeddedFilesLandByteIdentical(t, dest)
}

// assertEmbeddedFilesLandByteIdentical walks the embedded skill tree and asserts every file it
// contains landed at dest with byte-identical content.
func assertEmbeddedFilesLandByteIdentical(t *testing.T, dest string) {
	t.Helper()
	const root = "bitbucket-cli"
	entries, err := skill.Files.ReadDir(root)
	require.NoError(t, err)
	for _, entry := range entries {
		if entry.IsDir() {
			continue
		}
		want, err := skill.Files.ReadFile(root + "/" + entry.Name())
		require.NoError(t, err)
		got, err := os.ReadFile(filepath.Join(dest, entry.Name()))
		require.NoError(t, err)
		assert.Equal(t, want, got, "installed %s should equal the embedded bytes", entry.Name())
	}
}

func TestInstallSkillOverwritesStaleDestination(t *testing.T) {
	tmp := t.TempDir()
	dest := filepath.Join(tmp, "skills", "bitbucket-cli")
	require.NoError(t, os.MkdirAll(dest, 0o755))
	stale := filepath.Join(dest, "stale.txt")
	require.NoError(t, os.WriteFile(stale, []byte("old"), 0o644))
	skillPath := filepath.Join(dest, "SKILL.md")
	require.NoError(t, os.WriteFile(skillPath, []byte("stale skill body"), 0o644))

	cmd, _ := newInstallSkillTestCmd()
	require.NoError(t, cmd.Flags().Set("to", tmp))
	require.NoError(t, cmd.RunE(cmd, nil))

	_, err := os.Stat(stale)
	assert.True(t, os.IsNotExist(err), "stale file should be removed by os.RemoveAll")

	embedded, err := skill.Files.ReadFile("bitbucket-cli/SKILL.md")
	require.NoError(t, err)
	got, err := os.ReadFile(skillPath)
	require.NoError(t, err)
	assert.Equal(t, embedded, got, "stale SKILL.md should be replaced with the embedded bytes")
}

func TestInstallSkillToFlagHonored(t *testing.T) {
	tmp := t.TempDir()
	custom := filepath.Join(tmp, "custom-claude-dir")
	cmd, _ := newInstallSkillTestCmd()
	require.NoError(t, cmd.Flags().Set("to", custom))

	require.NoError(t, cmd.RunE(cmd, nil))

	assert.FileExists(t, filepath.Join(custom, "skills", "bitbucket-cli", "SKILL.md"))
}

func TestInstallSkillDefaultsToHomeClaude(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	cmd, _ := newInstallSkillTestCmd()

	require.NoError(t, cmd.RunE(cmd, nil))

	assert.FileExists(t, filepath.Join(home, ".claude", "skills", "bitbucket-cli", "SKILL.md"))
}

func TestInstallSkillDryRunWritesNothing(t *testing.T) {
	tmp := t.TempDir()
	cmd, out := newInstallSkillTestCmd()
	require.NoError(t, cmd.Flags().Set("to", tmp))
	require.NoError(t, cmd.Flags().Set("dry-run", "true"))

	require.NoError(t, cmd.RunE(cmd, nil))

	dest := filepath.Join(tmp, "skills", "bitbucket-cli")
	_, err := os.Stat(dest)
	assert.True(t, os.IsNotExist(err), "dry-run must not create the destination at all")
	assert.NotContains(t, out.String(), "installed bitbucket-cli skill to")
}

func TestInstallSkillErrorPathWrapsDestinationName(t *testing.T) {
	tmp := t.TempDir()
	// a regular file in the parent chain makes os.MkdirAll fail with ENOTDIR
	blocker := filepath.Join(tmp, "blocker")
	require.NoError(t, os.WriteFile(blocker, []byte("not a dir"), 0o644))

	cmd, _ := newInstallSkillTestCmd()
	require.NoError(t, cmd.Flags().Set("to", blocker))

	err := cmd.RunE(cmd, nil)
	require.Error(t, err)
	assert.ErrorContains(t, err, "blocker", "error should name the offending destination path")
}
