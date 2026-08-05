package common

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/gildas/go-logger"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestResolveWorktreeGitDirReturnsFolderWhenGitPathIsAFolder(t *testing.T) {
	log := logger.Create("test", &logger.NilStream{})
	tempDir := t.TempDir()
	gitPath := filepath.Join(tempDir, ".git")
	require.NoError(t, os.Mkdir(gitPath, 0o750))

	folder := resolveWorktreeGitDir(log, tempDir, gitPath)
	assert.Equal(t, tempDir, folder)
}

func TestResolveWorktreeGitDirReturnsFolderWhenGitPathDoesNotExist(t *testing.T) {
	log := logger.Create("test", &logger.NilStream{})
	tempDir := t.TempDir()
	gitPath := filepath.Join(tempDir, ".git")

	folder := resolveWorktreeGitDir(log, tempDir, gitPath)
	assert.Equal(t, tempDir, folder)
}

func TestResolveWorktreeGitDirFollowsRelativeGitDirFile(t *testing.T) {
	log := logger.Create("test", &logger.NilStream{})
	tempDir := t.TempDir()
	gitPath := filepath.Join(tempDir, ".git")
	require.NoError(t, os.WriteFile(gitPath, []byte("gitdir: ../real-git-dir\n"), 0o600))

	folder := resolveWorktreeGitDir(log, tempDir, gitPath)
	assert.Equal(t, filepath.Join(tempDir, "..", "real-git-dir"), folder)
}

func TestResolveWorktreeGitDirFollowsAbsoluteGitDirFile(t *testing.T) {
	log := logger.Create("test", &logger.NilStream{})
	tempDir := t.TempDir()
	gitPath := filepath.Join(tempDir, ".git")
	require.NoError(t, os.WriteFile(gitPath, []byte("gitdir: /some/absolute/path\n"), 0o600))

	folder := resolveWorktreeGitDir(log, tempDir, gitPath)
	assert.Equal(t, "/some/absolute/path", folder)
}
