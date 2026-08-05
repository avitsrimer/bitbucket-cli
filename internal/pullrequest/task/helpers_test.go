package task

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
	"github.com/avitsrimer/bitbucket-cli/internal/repository"
	"github.com/avitsrimer/bitbucket-cli/internal/workspace"
	"github.com/spf13/cobra"
)

// fixtureWorkspaceSlug/fixtureRepositorySlug identify a repository primed once, directly into
// RepositoryCache/WorkspaceCache, so every test in this package resolves it without ever hitting
// the network for workspace/repository lookups; only the call under test hits the httptest server.
const (
	fixtureWorkspaceSlug  = "acme"
	fixtureRepositorySlug = "widgets"
	fixtureRepositoryFlag = fixtureWorkspaceSlug + "/" + fixtureRepositorySlug
)

// setupTest primes the workspace/repository caches, points the profile client at a fresh
// httptest server, and returns a standalone command carrying the flags this package's RunE
// functions read (profile, repository, output, dry-run).
func setupTest(t *testing.T, handler http.HandlerFunc, dryRun bool) *cobra.Command {
	t.Helper()

	ws := workspace.Workspace{Slug: fixtureWorkspaceSlug}
	repo := repository.Repository{Slug: fixtureRepositorySlug, Workspace: &ws}
	if err := workspace.WorkspaceCache.Set(ws, fixtureWorkspaceSlug); err != nil {
		t.Fatalf("cannot prime workspace cache: %v", err)
	}
	if err := repository.RepositoryCache.Set(repo, fixtureRepositoryFlag); err != nil {
		t.Fatalf("cannot prime repository cache: %v", err)
	}
	t.Cleanup(func() {
		removeCacheEntry(fixtureWorkspaceSlug)
		removeCacheEntry(fixtureRepositoryFlag)
	})

	server := httptest.NewServer(handler)
	t.Cleanup(server.Close)

	apiRoot, err := url.Parse(server.URL)
	if err != nil {
		t.Fatalf("cannot parse test server URL: %v", err)
	}

	testProfile := &profile.Profile{Name: "task-test", APIRoot: apiRoot, AccessToken: "dummy-token", OutputFormat: "json"}
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
	cmd.Flags().String("repository", fixtureRepositoryFlag, "")
	cmd.Flags().String("output", "", "")
	cmd.Flags().Bool("dry-run", dryRun, "")
	return cmd
}

// removeCacheEntry deletes the on-disk mirror of a primed cache entry so the test run does not
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
