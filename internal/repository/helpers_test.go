package repository

import (
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"testing"
	"time"

	"github.com/avitsrimer/bitbucket-cli/internal/common"
	"github.com/avitsrimer/bitbucket-cli/internal/profile"
	"github.com/avitsrimer/bitbucket-cli/internal/workspace"
	"github.com/spf13/cobra"
)

// setupTest and captureStdout are local to this package rather than shared via
// internal/testutil: internal/testutil imports internal/repository (for RepositoryCache), so any
// test file declared "package repository" (as opposed to "package repository_test") that imported
// internal/testutil would form an import cycle.

// testWorkspaceSlug is the workspace primed into WorkspaceCache by setupTest, so resolving it
// during a test never reaches the network.
const testWorkspaceSlug = "acme"

func TestMain(m *testing.M) {
	os.Exit(runTests(m))
}

// runTests points RepositoryCache and WorkspaceCache at a scratch temp directory instead of the
// real os.UserCacheDir() for the duration of this test binary, removing it afterward so the suite
// never reads or writes the developer's actual cache.
func runTests(m *testing.M) int {
	tempDir, err := os.MkdirTemp("", "bitbucket-cli-test-cache-*")
	if err != nil {
		panic("helpers_test: cannot create temp cache dir: " + err.Error())
	}
	defer func() { _ = os.RemoveAll(tempDir) }()

	oldRepositoryCache, oldWorkspaceCache := RepositoryCache, workspace.WorkspaceCache
	RepositoryCache = common.NewCacheAt[Repository](tempDir, time.Minute)
	workspace.WorkspaceCache = common.NewCacheAt[workspace.Workspace](tempDir, time.Minute)
	defer func() {
		RepositoryCache = oldRepositoryCache
		workspace.WorkspaceCache = oldWorkspaceCache
	}()

	return m.Run()
}

// setupTest points the profile client at a fresh httptest server, primes WorkspaceCache with
// testWorkspaceSlug, and returns a standalone command carrying the flags this package's RunE
// functions read: profile, output, dry-run, repository, workspace, role, page-length, limit, sort.
// profileName must be unique across the test binary so entries left behind by one test's
// profile.Profiles append never leak into another.
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

	if err := workspace.WorkspaceCache.Set(testWorkspaceSlug, workspace.Workspace{Slug: testWorkspaceSlug}); err != nil {
		t.Fatalf("cannot prime workspace cache: %v", err)
	}

	cmd := &cobra.Command{Use: "test"}
	cmd.SetContext(context.Background())
	cmd.Flags().String("profile", "", "")
	cmd.Flags().String("output", "", "")
	cmd.Flags().Bool("dry-run", dryRun, "")
	cmd.Flags().String("repository", "", "")
	cmd.Flags().String("workspace", testWorkspaceSlug, "")
	cmd.Flags().String("role", "owner", "")
	cmd.Flags().Int("page-length", 0, "")
	cmd.Flags().Int("limit", 0, "")
	cmd.Flags().String("sort", "", "")
	return cmd
}

// captureStdout redirects os.Stdout for the duration of fn and returns what was written; used to
// assert on profile.Print's rendered output (it writes straight to os.Stdout), as well as the
// fmt.Println "No <thing> found" empty-list message.
//
// The reader is drained on a goroutine started before fn runs, so output larger than the pipe
// buffer cannot deadlock the test, and os.Stdout is restored via defer so a t.Fatalf inside fn
// (which calls runtime.Goexit, skipping any code after it) still leaves stdout intact for the
// rest of the test binary.
func captureStdout(t *testing.T, fn func()) string {
	t.Helper()

	r, w, err := os.Pipe()
	if err != nil {
		t.Fatalf("cannot create pipe: %v", err)
	}
	original := os.Stdout
	os.Stdout = w
	defer func() {
		os.Stdout = original
	}()

	captured := make(chan string, 1)
	go func() {
		data, _ := io.ReadAll(r)
		captured <- string(data)
	}()

	fn()

	_ = w.Close()
	return <-captured
}
