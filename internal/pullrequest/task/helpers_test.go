package task

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

func TestMain(m *testing.M) {
	os.Exit(runTests(m))
}

// runTests points WorkspaceCache/RepositoryCache at a scratch temp directory instead of the real
// os.UserCacheDir() for the duration of this test binary, removing it afterward so the suite
// never reads or writes the developer's actual cache.
func runTests(m *testing.M) int {
	tempDir, err := os.MkdirTemp("", "bitbucket-cli-test-cache-*")
	if err != nil {
		panic("helpers_test: cannot create temp cache dir: " + err.Error())
	}
	defer func() { _ = os.RemoveAll(tempDir) }()

	oldWorkspaceCache, oldRepositoryCache := workspace.WorkspaceCache, repository.RepositoryCache
	workspace.WorkspaceCache = common.NewCacheAt[workspace.Workspace](tempDir, time.Minute)
	repository.RepositoryCache = common.NewCacheAt[repository.Repository](tempDir, time.Minute)
	defer func() {
		workspace.WorkspaceCache = oldWorkspaceCache
		repository.RepositoryCache = oldRepositoryCache
	}()

	return m.Run()
}

// newFixtureRepository builds a Repository with the fields Repository.UnmarshalJSON's Validate
// call requires (ID, Name, FullName) set, so priming RepositoryCache actually survives the
// on-disk JSON round-trip instead of only ever being read back from an in-memory shortcut.
func newFixtureRepository(t *testing.T, ws *workspace.Workspace) repository.Repository {
	t.Helper()
	id, err := common.ParseUUID("{22222222-2222-2222-2222-222222222222}")
	if err != nil {
		t.Fatalf("cannot parse fixture repository uuid: %v", err)
	}
	return repository.Repository{
		ID:        id,
		Name:      "Widgets",
		FullName:  fixtureRepositoryFlag,
		Slug:      fixtureRepositorySlug,
		Workspace: ws,
	}
}

// setupTest primes the workspace/repository caches, points the profile client at a fresh
// httptest server, and returns a standalone command carrying the flags this package's RunE
// functions read (profile, repository, output, dry-run).
func setupTest(t *testing.T, handler http.HandlerFunc, dryRun bool) *cobra.Command {
	t.Helper()

	ws := workspace.Workspace{Slug: fixtureWorkspaceSlug}
	repo := newFixtureRepository(t, &ws)
	if err := workspace.WorkspaceCache.Set(fixtureWorkspaceSlug, ws); err != nil {
		t.Fatalf("cannot prime workspace cache: %v", err)
	}
	if err := repository.RepositoryCache.Set(fixtureRepositoryFlag, repo); err != nil {
		t.Fatalf("cannot prime repository cache: %v", err)
	}

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

// captureStdout redirects os.Stdout for the duration of fn and returns what was written; used
// to assert on profile.Print's rendered output (it writes straight to os.Stdout).
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
