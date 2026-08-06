package cmd_test

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
)

// TestVersionStampedViaLdflags builds the bb binary the same way the Makefile does
// (-ldflags -X .../internal/cmd.version=<rev>) and verifies the git describe revision
// shows up in --version output, i.e. the ldflags stamping path actually works end to end.
func TestVersionStampedViaLdflags(t *testing.T) {
	if testing.Short() {
		t.Skip("builds a full binary; skipped in short mode")
	}

	revOut, err := exec.Command("git", "describe", "--tags", "--always", "--dirty").CombinedOutput()
	require.NoError(t, err, "git describe failed: %s", revOut)
	rev := strings.TrimSpace(string(revOut))
	require.NotEmpty(t, rev)

	binary := filepath.Join(t.TempDir(), "bb-smoke")
	build := exec.Command("go", "build",
		"-ldflags", "-X github.com/avitsrimer/bitbucket-cli/internal/cmd.version="+rev,
		"-o", binary,
		"github.com/avitsrimer/bitbucket-cli/cmd/bb",
	)
	build.Env = append(os.Environ(), "GOTOOLCHAIN=local")
	buildOut, err := build.CombinedOutput()
	require.NoError(t, err, "go build failed: %s", buildOut)

	versionOut, err := exec.Command(binary, "--version").CombinedOutput()
	require.NoError(t, err, "bb --version failed: %s", versionOut)
	require.Contains(t, string(versionOut), rev)
}

// TestVersionWorksWithoutHomeDirectory reproduces major finding #10: root.go's init() called
// cobra.CheckErr on os.UserConfigDir()'s error, which aborts the whole process (os.Exit) the
// instant $HOME (and XDG_CONFIG_HOME) are unset -- as in many containers/CI runners -- even for
// `bb --version`, though configDir only ever feeds a --config flag help string that never gets
// rendered by --version. The error must be ignored, not fatal.
func TestVersionWorksWithoutHomeDirectory(t *testing.T) {
	if testing.Short() {
		t.Skip("builds a full binary; skipped in short mode")
	}

	binary := filepath.Join(t.TempDir(), "bb-smoke-no-home")
	build := exec.Command("go", "build", "-o", binary, "github.com/avitsrimer/bitbucket-cli/cmd/bb")
	build.Env = append(os.Environ(), "GOTOOLCHAIN=local")
	buildOut, err := build.CombinedOutput()
	require.NoError(t, err, "go build failed: %s", buildOut)

	cmd := exec.Command(binary, "--version")
	// Strip every environment variable os.UserConfigDir consults on any platform (HOME on
	// darwin/linux, XDG_CONFIG_HOME on linux, AppData on windows), simulating a container/CI
	// runner with no home directory configured at all.
	env := make([]string, 0, len(os.Environ()))
	for _, kv := range os.Environ() {
		switch {
		case strings.HasPrefix(kv, "HOME="),
			strings.HasPrefix(kv, "XDG_CONFIG_HOME="),
			strings.HasPrefix(kv, "AppData="):
			continue
		}
		env = append(env, kv)
	}
	cmd.Env = env

	out, err := cmd.CombinedOutput()
	require.NoError(t, err, "bb --version without $HOME set must still succeed, got: %s", out)
}
