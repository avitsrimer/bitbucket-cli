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
