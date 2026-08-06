package branch

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

// requireGit skips the calling test when a real git binary is not resolvable via PATH: these
// tests shell out to the actual git executable rather than a shim, so they cannot run in an
// environment without one (e.g. a minimal container image building this module).
func requireGit(t *testing.T) {
	t.Helper()
	if _, err := exec.LookPath("git"); err != nil {
		t.Skip("git not found on PATH, skipping test that requires a real git binary")
	}
}

// runGit runs a real git command with dir as its working directory, failing the test on error.
func runGit(t *testing.T, dir string, args ...string) string {
	t.Helper()
	cmd := exec.Command("git", args...)
	cmd.Dir = dir
	cmd.Env = append(os.Environ(),
		"GIT_AUTHOR_NAME=test", "GIT_AUTHOR_EMAIL=test@example.com",
		"GIT_COMMITTER_NAME=test", "GIT_COMMITTER_EMAIL=test@example.com",
	)
	output, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("git %v failed: %v\n%s", args, err, output)
	}
	return strings.TrimSpace(string(output))
}

// chdir changes the process's working directory to dir for the duration of the calling test,
// restoring the original directory via t.Cleanup.
func chdir(t *testing.T, dir string) {
	t.Helper()
	original, err := os.Getwd()
	if err != nil {
		t.Fatalf("cannot get working directory: %v", err)
	}
	if err := os.Chdir(dir); err != nil {
		t.Fatalf("cannot chdir to %s: %v", dir, err)
	}
	t.Cleanup(func() {
		if err := os.Chdir(original); err != nil {
			t.Fatalf("cannot restore working directory: %v", err)
		}
	})
}

func TestGetCurrentBranchOnABranch(t *testing.T) {
	requireGit(t)
	dir := t.TempDir()
	runGit(t, dir, "init")
	runGit(t, dir, "commit", "--allow-empty", "-m", "init")
	runGit(t, dir, "checkout", "-b", "feature-x")
	chdir(t, dir)

	name, err := GetCurrentBranch(t.Context())
	if err != nil {
		t.Fatalf("GetCurrentBranch() error = %v", err)
	}
	if name != "feature-x" {
		t.Errorf("GetCurrentBranch() = %q, want %q", name, "feature-x")
	}
}

func TestGetCurrentBranchDetachedHEAD(t *testing.T) {
	requireGit(t)
	dir := t.TempDir()
	runGit(t, dir, "init")
	runGit(t, dir, "commit", "--allow-empty", "-m", "init")
	hash := runGit(t, dir, "rev-parse", "HEAD")
	runGit(t, dir, "checkout", hash)
	chdir(t, dir)

	if _, err := GetCurrentBranch(t.Context()); err == nil {
		t.Error("GetCurrentBranch() expected an error for a detached HEAD, got nil")
	}
}

func TestGetCurrentBranchNotARepository(t *testing.T) {
	requireGit(t)
	dir := t.TempDir()
	chdir(t, dir)

	if _, err := GetCurrentBranch(t.Context()); err == nil {
		t.Error("GetCurrentBranch() expected an error outside a git repository, got nil")
	}
}

// TestGetCurrentBranchSurfacesGitStderr proves the error includes git's own stderr message, not
// just the bare "exit status N" that (*exec.ExitError).Error() alone would produce. It uses a
// fake "git" on PATH rather than the real binary, so it doesn't need requireGit and exercises the
// exact message a real "not a git repository"/detached-HEAD failure would carry.
func TestGetCurrentBranchSurfacesGitStderr(t *testing.T) {
	binDir := t.TempDir()
	const stderrMessage = "fatal: not a git repository (or any of the parent directories): .git"
	script := "#!/bin/sh\necho '" + stderrMessage + "' >&2\nexit 128\n"
	if err := os.WriteFile(filepath.Join(binDir, "git"), []byte(script), 0o755); err != nil {
		t.Fatalf("cannot write git shim: %v", err)
	}
	t.Setenv("PATH", binDir+string(os.PathListSeparator)+os.Getenv("PATH"))

	_, err := GetCurrentBranch(t.Context())
	if err == nil {
		t.Fatal("GetCurrentBranch() expected an error, got nil")
	}
	if !strings.Contains(err.Error(), stderrMessage) {
		t.Errorf("error = %q, want it to contain git's stderr message %q", err.Error(), stderrMessage)
	}
}
