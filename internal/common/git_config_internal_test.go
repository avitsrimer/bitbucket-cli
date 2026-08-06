package common

import (
	"context"
	"io"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestResolveWorktreeGitDirReturnsFolderWhenGitPathIsAFolder(t *testing.T) {
	tempDir := t.TempDir()
	gitPath := filepath.Join(tempDir, ".git")
	require.NoError(t, os.Mkdir(gitPath, 0o750))

	folder := resolveWorktreeGitDir(tempDir, gitPath)
	assert.Equal(t, tempDir, folder)
}

func TestResolveWorktreeGitDirReturnsFolderWhenGitPathDoesNotExist(t *testing.T) {
	tempDir := t.TempDir()
	gitPath := filepath.Join(tempDir, ".git")

	folder := resolveWorktreeGitDir(tempDir, gitPath)
	assert.Equal(t, tempDir, folder)
}

func TestResolveWorktreeGitDirFollowsRelativeGitDirFile(t *testing.T) {
	tempDir := t.TempDir()
	gitPath := filepath.Join(tempDir, ".git")
	require.NoError(t, os.WriteFile(gitPath, []byte("gitdir: ../real-git-dir\n"), 0o600))

	folder := resolveWorktreeGitDir(tempDir, gitPath)
	assert.Equal(t, filepath.Join(tempDir, "..", "real-git-dir"), folder)
}

func TestResolveWorktreeGitDirFollowsAbsoluteGitDirFile(t *testing.T) {
	tempDir := t.TempDir()
	gitPath := filepath.Join(tempDir, ".git")
	require.NoError(t, os.WriteFile(gitPath, []byte("gitdir: /some/absolute/path\n"), 0o600))

	folder := resolveWorktreeGitDir(tempDir, gitPath)
	assert.Equal(t, "/some/absolute/path", folder)
}

// TestOpenGitConfigFollowsWorktreeGitDirFileWithoutDoublingGitSegment reproduces the regression
// where OpenGitConfig joined ".git/config" onto resolveWorktreeGitDir's return value
// unconditionally: for a worktree, that value already IS the real git directory (e.g.
// .git/worktrees/<name>), so the doubled path never exists, the open fails, and the loop's
// walk-up silently finds and opens the *parent* repository's own .git/config instead -- which
// exists, so no error is ever returned; the caller just silently gets the wrong repository's
// config. Asserting on the config content (not just "no error") is what catches that: an assertion
// that only checked err == nil would pass against the parent repository's config too.
func TestOpenGitConfigFollowsWorktreeGitDirFileWithoutDoublingGitSegment(t *testing.T) {
	root := t.TempDir()
	parentRepo := filepath.Join(root, "parent-repo")
	worktreeGitDir := filepath.Join(parentRepo, ".git", "worktrees", "feature")
	require.NoError(t, os.MkdirAll(worktreeGitDir, 0o750))
	require.NoError(t, os.WriteFile(filepath.Join(parentRepo, ".git", "config"), []byte("[core]\n\tfrom = parent\n"), 0o600))
	require.NoError(t, os.WriteFile(filepath.Join(worktreeGitDir, "config"), []byte("[core]\n\tfrom = worktree\n"), 0o600))

	worktreeCheckout := filepath.Join(parentRepo, "feature-worktree")
	require.NoError(t, os.MkdirAll(worktreeCheckout, 0o750))
	require.NoError(t, os.WriteFile(filepath.Join(worktreeCheckout, ".git"), []byte("gitdir: ../.git/worktrees/feature\n"), 0o600))

	oldWd, err := os.Getwd()
	require.NoError(t, err)
	defer func() { require.NoError(t, os.Chdir(oldWd)) }()
	require.NoError(t, os.Chdir(worktreeCheckout))

	file, err := OpenGitConfig(context.Background())
	require.NoError(t, err)
	defer func() { _ = file.Close() }()

	content, err := io.ReadAll(file)
	require.NoError(t, err)
	assert.Contains(t, string(content), "from = worktree")
	assert.NotContains(t, string(content), "from = parent")
}
