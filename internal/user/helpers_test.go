package user

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"path/filepath"
	"testing"

	"github.com/avitsrimer/bitbucket-cli/internal/profile"
	"github.com/spf13/cobra"
)

// setupTest points the profile client at a fresh httptest server and returns a standalone
// command carrying the flags this package's RunE functions read (profile, output, dry-run).
// profileName must be unique per test whose code path touches UserCache (GetMe/GetUser), so
// cache entries left behind by one test never leak into another.
func setupTest(t *testing.T, profileName string, handler http.HandlerFunc, dryRun bool) *cobra.Command {
	t.Helper()

	server := httptest.NewServer(handler)
	t.Cleanup(server.Close)

	apiRoot, err := url.Parse(server.URL)
	if err != nil {
		t.Fatalf("cannot parse test server URL: %v", err)
	}

	testProfile := &profile.Profile{Name: profileName, APIRoot: apiRoot, AccessToken: "dummy-token", OutputFormat: "json"}
	oldProfiles, oldCurrent := profile.Profiles, profile.Current
	profile.Profiles = append(profile.Profiles, testProfile)
	profile.Current = testProfile
	t.Cleanup(func() {
		profile.Profiles = oldProfiles
		profile.Current = oldCurrent
	})

	cmd := &cobra.Command{Use: "test"}
	cmd.SetContext(context.Background())
	cmd.Flags().String("profile", "", "")
	cmd.Flags().String("output", "", "")
	cmd.Flags().Bool("dry-run", dryRun, "")
	return cmd
}

// removeCacheEntry deletes the on-disk mirror of a UserCache entry so the test run does not
// leave residue behind in the real os.UserCacheDir().
func removeCacheEntry(key string) {
	dir, err := os.UserCacheDir()
	if err != nil {
		return
	}
	sum := sha256.Sum256([]byte(key))
	_ = os.Remove(filepath.Join(dir, "bitbucket", hex.EncodeToString(sum[:])))
}

// captureStdout redirects os.Stdout for the duration of fn and returns what was written; used
// to assert on profile.Print's rendered output (it writes straight to os.Stdout).
func captureStdout(t *testing.T, fn func()) string {
	t.Helper()

	r, w, err := os.Pipe()
	if err != nil {
		t.Fatalf("cannot create pipe: %v", err)
	}
	original := os.Stdout
	os.Stdout = w

	fn()

	_ = w.Close()
	os.Stdout = original

	data, err := io.ReadAll(r)
	if err != nil {
		t.Fatalf("cannot read captured stdout: %v", err)
	}
	return string(data)
}
