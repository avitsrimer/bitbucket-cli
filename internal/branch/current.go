package branch

import (
	"context"
	"errors"
	"fmt"
	"os/exec"
	"strings"
)

// GetCurrentBranch returns the name of the branch checked out in the current working directory,
// resolved by shelling out to `git symbolic-ref --short HEAD` (explicit argv, no shell -- the same
// approach as repository.Clone's git invocation). It returns an error when HEAD does not point at
// a branch (a detached HEAD) or the working directory is not inside a git repository at all; git
// itself reports both cases as a non-zero exit.
func GetCurrentBranch(ctx context.Context) (string, error) {
	output, err := exec.CommandContext(ctx, "git", "symbolic-ref", "--short", "HEAD").Output()
	if err != nil {
		var exitErr *exec.ExitError
		if errors.As(err, &exitErr) {
			if stderr := strings.TrimSpace(string(exitErr.Stderr)); stderr != "" {
				return "", fmt.Errorf("cannot determine current branch: %s: %w", stderr, err)
			}
		}
		return "", fmt.Errorf("cannot determine current branch: %w", err)
	}
	name := strings.TrimSpace(string(output))
	if name == "" {
		return "", errors.New("cannot determine current branch: git returned an empty name")
	}
	return name, nil
}
