package cmd

import (
	"bytes"
	"io/fs"
	"os"
	"path/filepath"
	"runtime"
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

// TestInstallSkillLeavesNoStagingDirBehind proves the temp staging directory created for the
// atomic swap never lingers in <to>/skills after a successful install -- only the final
// "bitbucket-cli" entry should remain.
func TestInstallSkillLeavesNoStagingDirBehind(t *testing.T) {
	tmp := t.TempDir()
	cmd, _ := newInstallSkillTestCmd()
	require.NoError(t, cmd.Flags().Set("to", tmp))

	require.NoError(t, cmd.RunE(cmd, nil))

	skillsDir := filepath.Join(tmp, "skills")
	entries, err := os.ReadDir(skillsDir)
	require.NoError(t, err)
	names := make([]string, 0, len(entries))
	for _, entry := range entries {
		names = append(names, entry.Name())
	}
	assert.Equal(t, []string{"bitbucket-cli"}, names, "no staging directory should remain in %s", skillsDir)
}

func TestInstallSkillEveryEmbeddedFileLandsByteIdentical(t *testing.T) {
	tmp := t.TempDir()
	cmd, _ := newInstallSkillTestCmd()
	require.NoError(t, cmd.Flags().Set("to", tmp))

	require.NoError(t, cmd.RunE(cmd, nil))

	dest := filepath.Join(tmp, "skills", "bitbucket-cli")
	assertEmbeddedFilesLandByteIdentical(t, dest)
}

// assertEmbeddedFilesLandByteIdentical walks the ENTIRE embedded skill tree, at any depth, via
// fs.WalkDir and asserts every file it contains landed at dest with byte-identical content. A
// single top-level ReadDir would silently stop proving anything for a file nested under a
// subdirectory (e.g. skill/bitbucket-cli/references/foo.md), so the tree is always walked in
// full instead.
func assertEmbeddedFilesLandByteIdentical(t *testing.T, dest string) {
	t.Helper()
	const root = "bitbucket-cli"
	fileCount := 0
	err := fs.WalkDir(skill.Files, root, func(path string, d fs.DirEntry, walkErr error) error {
		require.NoError(t, walkErr)
		if d.IsDir() {
			return nil
		}
		fileCount++
		rel := path[len(root):]
		want, readErr := skill.Files.ReadFile(path)
		require.NoError(t, readErr)
		got, statErr := os.ReadFile(filepath.Join(dest, filepath.FromSlash(rel)))
		require.NoError(t, statErr)
		assert.Equal(t, want, got, "installed %s should equal the embedded bytes", path)
		return nil
	})
	require.NoError(t, err)
	require.NotZero(t, fileCount, "expected fs.WalkDir to find at least one embedded file under %s", root)
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
	assert.True(t, os.IsNotExist(err), "stale file should be removed by the RemoveAll+Rename swap")

	embedded, err := skill.Files.ReadFile("bitbucket-cli/SKILL.md")
	require.NoError(t, err)
	got, err := os.ReadFile(skillPath)
	require.NoError(t, err)
	assert.Equal(t, embedded, got, "stale SKILL.md should be replaced with the embedded bytes")
}

// TestInstallSkillFailedSwapLeavesExistingDestUntouched proves that when staging the new tree
// fails -- here, because <to>/skills has no write permission, so os.MkdirTemp itself cannot
// create the staging directory -- the pre-existing destination is left completely intact: the
// swap (os.RemoveAll of dest followed by os.Rename of the staging dir into place) never runs
// until every embedded file has already been written successfully into a sibling staging
// directory, so a failure anywhere before that point cannot half-delete or half-overwrite an
// existing installation the way the previous RemoveAll-dest-first implementation could.
func TestInstallSkillFailedSwapLeavesExistingDestUntouched(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("permission-bit based read-only directories behave differently on windows")
	}
	if os.Geteuid() == 0 {
		t.Skip("root ignores directory write permission bits")
	}

	tmp := t.TempDir()
	skillsDir := filepath.Join(tmp, "skills")
	dest := filepath.Join(skillsDir, "bitbucket-cli")
	require.NoError(t, os.MkdirAll(dest, 0o750))
	existingSkillPath := filepath.Join(dest, "SKILL.md")
	const existingContent = "existing installed skill body, must survive a failed re-install"
	require.NoError(t, os.WriteFile(existingSkillPath, []byte(existingContent), 0o600))

	// Read-only (no write bit): os.MkdirTemp can no longer create a new entry under skillsDir,
	// so staging fails before dest is touched at all.
	require.NoError(t, os.Chmod(skillsDir, 0o500))
	t.Cleanup(func() { _ = os.Chmod(skillsDir, 0o750) })

	cmd, _ := newInstallSkillTestCmd()
	require.NoError(t, cmd.Flags().Set("to", tmp))

	err := cmd.RunE(cmd, nil)
	require.Error(t, err, "expected the staging directory creation to fail on a read-only skills dir")

	got, readErr := os.ReadFile(existingSkillPath)
	require.NoError(t, readErr, "the existing installation must still be present after a failed re-install")
	assert.Equal(t, existingContent, string(got), "the existing SKILL.md must be untouched, not partially overwritten")
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

// TestInstallSkillCommandRejectsUnsupportedGlobalFlags proves installSkillCmd's PreRunE (the
// exact closure common.DisableUnsupportedFlags returns, captured at package init time) rejects
// each of the root-persistent flags install-skill has no use for --
// --profile/--workspace/--repository/--output/--stop-on-error/--warn-on-error/--ignore-errors.
// DisableUnsupportedFlags's returned closure reads whatever *cobra.Command it is called with, so
// a synthetic probe command carrying just the one flag under test (mirroring cobra's own
// post-ParseFlags merged state, the same way internal/common/unsupported_flags_test.go does)
// exercises it without touching the real, process-global installSkillCmd's flags or command
// tree.
func TestInstallSkillCommandRejectsUnsupportedGlobalFlags(t *testing.T) {
	require.NotNil(t, installSkillCmd.PreRunE, "installSkillCmd should reject unsupported inherited flags via PreRunE")

	for _, name := range installSkillUnsupportedFlags {
		t.Run(name, func(t *testing.T) {
			probe := &cobra.Command{Use: "probe"}
			probe.Flags().String(name, "", "")
			require.NoError(t, probe.Flags().Set(name, "x"))

			err := installSkillCmd.PreRunE(probe, nil)
			assert.Errorf(t, err, "expected --%s to be rejected", name)
		})
	}
}

// TestInstallSkillCommandHidesUnsupportedGlobalFlags proves installSkillCmd's help function (the
// exact closure common.HideUnsupportedFlags returns, captured at package init time) marks each
// unsupported flag Hidden before delegating to the parent's own help function. It is exercised
// against a synthetic parent/child pair, not the real RootCmd/installSkillCmd, so this test
// cannot perturb global command-tree state shared with other tests in this package.
func TestInstallSkillCommandHidesUnsupportedGlobalFlags(t *testing.T) {
	helpFn := installSkillCmd.HelpFunc()
	require.NotNil(t, helpFn, "installSkillCmd should have a help function that hides unsupported flags")

	root := &cobra.Command{Use: "root"}
	root.SetHelpFunc(func(*cobra.Command, []string) {})
	child := &cobra.Command{Use: "install-skill"}
	for _, name := range installSkillUnsupportedFlags {
		child.Flags().String(name, "", "")
	}
	root.AddCommand(child)

	helpFn(child, nil)

	for _, name := range installSkillUnsupportedFlags {
		flag := child.Flags().Lookup(name)
		require.NotNilf(t, flag, "expected --%s to be registered", name)
		assert.Truef(t, flag.Hidden, "expected --%s to be hidden from help", name)
	}
}
