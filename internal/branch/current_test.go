package branch

import (
	"os"
	"os/exec"
	"strings"
	"testing"
)

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
	dir := t.TempDir()
	chdir(t, dir)

	if _, err := GetCurrentBranch(t.Context()); err == nil {
		t.Error("GetCurrentBranch() expected an error outside a git repository, got nil")
	}
}
