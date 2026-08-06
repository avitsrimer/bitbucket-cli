package workspace

import (
	"strings"
	"testing"

	"github.com/avitsrimer/bitbucket-cli/internal/profile"
	"github.com/spf13/cobra"
)

// TestGetWorkspaceNameErrorNamesAllThreeWaysWhenEveryRungFails proves that when the flag, the git
// remote, and the profile default all come up empty, the error names every way to supply the
// value instead of a bare "argument workspace is missing", mirroring the repository package's
// GetRepositoryName message (FR-12).
func TestGetWorkspaceNameErrorNamesAllThreeWaysWhenEveryRungFails(t *testing.T) {
	chdirToFakeGitConfig(t, "[core]\n\tbare = false\n")

	oldCurrent := profile.Current
	profile.Current = &profile.Profile{Name: "p"}
	t.Cleanup(func() { profile.Current = oldCurrent })

	cmd := &cobra.Command{Use: "test"}
	cmd.Flags().String("workspace", "", "")

	_, err := GetWorkspaceName(t.Context(), cmd)
	if err == nil {
		t.Fatal("GetWorkspaceName() expected an error when every rung is empty, got nil")
	}
	for _, want := range []string{"--workspace", "git", "default-workspace"} {
		if !strings.Contains(err.Error(), want) {
			t.Errorf("GetWorkspaceName() error = %q, want it to mention %q", err.Error(), want)
		}
	}
}

// TestGetWorkspaceNameLogsWarnOnProfileLoadError proves that when the flag and the git remote both
// come up empty and resolving the profile itself fails (an invalid --profile value here), the
// underlying profile error is logged as a [WARN] rather than silently discarded, so the real cause
// is still visible even though GetWorkspaceName still returns its own generic
// "argument workspace is missing" error.
func TestGetWorkspaceNameLogsWarnOnProfileLoadError(t *testing.T) {
	chdirToFakeGitConfig(t, "[core]\n\tbare = false\n")

	oldCurrent, oldProfiles := profile.Current, profile.Profiles
	profile.Current = nil
	profile.Profiles = nil
	t.Cleanup(func() {
		profile.Current = oldCurrent
		profile.Profiles = oldProfiles
	})

	cmd := &cobra.Command{Use: "test"}
	cmd.Flags().String("workspace", "", "")
	cmd.Flags().String("profile", "bogus-profile", "")

	logs := captureLog(t)

	_, err := GetWorkspaceName(t.Context(), cmd)
	if err == nil {
		t.Fatal("GetWorkspaceName() expected an error, got nil")
	}
	if !strings.Contains(err.Error(), "argument workspace is missing") {
		t.Errorf("GetWorkspaceName() error = %q, want the generic missing-argument message", err.Error())
	}
	if !strings.Contains(logs.String(), "WARN") || !strings.Contains(logs.String(), "argument profile is invalid") {
		t.Errorf("logs = %q, want a [WARN] line surfacing the profile load error", logs.String())
	}
}
