package workspace

import (
	"strings"
	"testing"

	"github.com/avitsrimer/bitbucket-cli/internal/common"
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

	oldCurrent, oldProfiles, oldConfig := profile.Current, profile.Profiles, common.CurrentConfig()
	profile.Current = nil
	profile.Profiles = nil
	t.Cleanup(func() {
		profile.Current = oldCurrent
		profile.Profiles = oldProfiles
		common.SetCurrentConfig(oldConfig)
	})

	cmd := &cobra.Command{Use: "test"}
	cmd.Flags().String("workspace", "", "")
	cmd.Flags().String("profile", "bogus-profile", "")

	// Warm up common.CurrentConfig() before installing captureLog's buffer: profile.Profiles.Load
	// (reached below via GetWorkspaceName -> profile.GetProfileFromCommand) only calls
	// common.Initialize -- which unconditionally resets the global lgr logger to os.Stderr -- when
	// common.CurrentConfig() is nil. Relying on an earlier test in the package having already
	// primed it left this test order-dependent (it failed under -run and -shuffle); priming it here
	// up front keeps it self-sufficient regardless of run order.
	if err := common.Initialize(cmd); err != nil {
		t.Fatalf("cannot warm up config: %v", err)
	}

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
