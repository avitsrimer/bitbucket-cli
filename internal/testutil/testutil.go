// Package testutil provides the test-harness helpers shared by every package that spins up an
// httptest server, primes the workspace/repository/user caches, captures os.Stdout/os.Stderr, or
// captures the global lgr logger. Because this package itself imports internal/repository,
// internal/user, and internal/workspace, only packages *outside* that trio (and outside anything
// they import) can import it without a cycle -- currently the pullrequest command tree and its
// comment/task/common subpackages, the user package, and the restored artifact, branch, commit,
// pipeline, pipeline/step, and pipeline/common packages. internal/repository and
// internal/workspace's own tests, which cannot import it, instead duplicate the specific helpers
// they need in a local helpers_test.go (e.g. internal/repository/helpers_test.go); their external
// (package foo_test) test files can still import it, since only the internal (package foo) test
// files sit inside the cycle -- see internal/workspace/allowed_slugs_test.go for that escape
// hatch in use.
package testutil

import (
	"bytes"
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
	"github.com/avitsrimer/bitbucket-cli/internal/user"
	"github.com/avitsrimer/bitbucket-cli/internal/workspace"
	"github.com/go-pkgz/lgr"
	"github.com/spf13/cobra"
)

// FixtureWorkspaceSlug and FixtureRepositorySlug/FixtureRepositoryFlag identify the repository
// NewFixtureRepository builds, shared by every package that primes RepositoryCache/WorkspaceCache
// for its tests instead of hitting the network for workspace/repository lookups.
const (
	FixtureWorkspaceSlug  = "acme"
	FixtureRepositorySlug = "widgets"
	FixtureRepositoryFlag = FixtureWorkspaceSlug + "/" + FixtureRepositorySlug
)

// TempCaches points WorkspaceCache, RepositoryCache, and UserCache at a scratch temp directory
// instead of the real os.UserCacheDir() for the duration of m.Run(), removing it afterward so the
// suite never reads or writes the developer's actual cache even if a test panics, times out, or
// the run is interrupted. Call it from TestMain:
//
//	func TestMain(m *testing.M) { os.Exit(testutil.TempCaches(m)) }
func TempCaches(m *testing.M) int {
	tempDir, err := os.MkdirTemp("", "bitbucket-cli-test-cache-*")
	if err != nil {
		panic("testutil: cannot create temp cache dir: " + err.Error())
	}
	defer func() { _ = os.RemoveAll(tempDir) }()

	oldWorkspaceCache, oldRepositoryCache, oldUserCache := workspace.WorkspaceCache, repository.RepositoryCache, user.UserCache
	workspace.WorkspaceCache = common.NewCacheAt[workspace.Workspace](tempDir, time.Minute)
	repository.RepositoryCache = common.NewCacheAt[repository.Repository](tempDir, time.Minute)
	user.UserCache = common.NewCacheAt[user.User](tempDir, time.Minute)
	defer func() {
		workspace.WorkspaceCache = oldWorkspaceCache
		repository.RepositoryCache = oldRepositoryCache
		user.UserCache = oldUserCache
	}()

	return m.Run()
}

// NewFixtureRepository builds a Repository with the fields Repository.UnmarshalJSON's Validate
// call requires (ID, Name, FullName) set, so priming RepositoryCache with it actually survives the
// on-disk JSON round-trip instead of only ever being read back from an in-memory shortcut.
func NewFixtureRepository(t testing.TB, ws *workspace.Workspace) repository.Repository {
	t.Helper()
	id, err := common.ParseUUID("{22222222-2222-2222-2222-222222222222}")
	if err != nil {
		t.Fatalf("cannot parse fixture repository uuid: %v", err)
	}
	return repository.Repository{
		ID:        id,
		Name:      "Widgets",
		FullName:  ws.Slug + "/" + FixtureRepositorySlug,
		Slug:      FixtureRepositorySlug,
		Workspace: ws,
	}
}

// PrimeFixtureCaches sets a fixture workspace (FixtureWorkspaceSlug) and its repository
// (NewFixtureRepository, keyed at FixtureRepositoryFlag) directly into WorkspaceCache and
// RepositoryCache, so tests resolve them without ever hitting the network for workspace/
// repository lookups; only the call under test reaches the per-test httptest server.
func PrimeFixtureCaches(t testing.TB) {
	t.Helper()
	ws := workspace.Workspace{Slug: FixtureWorkspaceSlug}
	repo := NewFixtureRepository(t, &ws)
	if err := workspace.WorkspaceCache.Set(FixtureWorkspaceSlug, ws); err != nil {
		t.Fatalf("cannot prime workspace cache: %v", err)
	}
	if err := repository.RepositoryCache.Set(FixtureRepositoryFlag, repo); err != nil {
		t.Fatalf("cannot prime repository cache: %v", err)
	}
}

// SetupProfile points the profile client at a fresh httptest server and returns a standalone
// command carrying the full flag set the pullrequest/comment/task/user RunE functions and their
// shared helpers read: profile, repository, output, dry-run, pending, page-length, limit, and the
// root command's stop-on-error/warn-on-error/ignore-errors trio (Profile.ShouldStopOnError and
// friends read them via cmd.Flag(name).Changed unconditionally, so any code path reaching them
// needs the flags to exist even when a test never sets them). profileName must be unique across a
// test binary whenever the code path under test touches a package-level cache keyed by profile
// name (e.g. UserCache via GetMe/GetUser), so entries left behind by one test never leak into
// another.
func SetupProfile(t testing.TB, profileName string, handler http.HandlerFunc) *cobra.Command {
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
	cmd.Flags().String("repository", FixtureRepositoryFlag, "")
	cmd.Flags().String("output", "", "")
	cmd.Flags().Bool("dry-run", false, "")
	cmd.Flags().Bool("pending", false, "")
	cmd.Flags().Int("page-length", 0, "")
	cmd.Flags().Int("limit", 0, "")
	cmd.Flags().Bool("stop-on-error", false, "")
	cmd.Flags().Bool("warn-on-error", false, "")
	cmd.Flags().Bool("ignore-errors", false, "")
	return cmd
}

// CaptureStdout redirects os.Stdout for the duration of fn and returns what was written; used to
// assert on profile.Print's rendered output (it writes straight to os.Stdout).
func CaptureStdout(t *testing.T, fn func()) string {
	t.Helper()
	return captureStream(t, &os.Stdout, fn)
}

// CaptureStderr is CaptureStdout for os.Stderr; used to assert on warnings written directly to
// stderr (e.g. tolerateReviewerErrors' --warn-on-error message), which profile.Print never touches.
func CaptureStderr(t *testing.T, fn func()) string {
	t.Helper()
	return captureStream(t, &os.Stderr, fn)
}

// CaptureLog redirects the global lgr logger to a buffer for the duration of the test, with
// [DEBUG] lines enabled, and returns that buffer. It restores whatever logger was active
// beforehand once the test ends -- rather than a hardcoded quiet baseline -- by wrapping the
// previous logger as a slog.Handler and forwarding every record to it: that previous logger's own
// Logf decides for itself whether a DEBUG line was really enabled, so a test run after this one
// sees exactly the logging behavior it would have without this call, whatever that was.
func CaptureLog(t *testing.T) *bytes.Buffer {
	t.Helper()
	previous := lgr.Default()
	var buf bytes.Buffer
	lgr.Setup(lgr.Out(&buf), lgr.Err(&buf), lgr.Debug)
	t.Cleanup(func() {
		lgr.Setup(lgr.Debug, lgr.SlogHandler(lgr.ToSlogHandler(previous)))
	})
	return &buf
}

// captureStream redirects *stream (os.Stdout or os.Stderr) for the duration of fn and returns
// what was written to it.
//
// The reader is drained on a goroutine started before fn runs, so output larger than the pipe
// buffer cannot deadlock the test, and *stream is restored via defer so a t.Fatalf inside fn
// (which calls runtime.Goexit, skipping any code after it) still leaves it intact for the rest of
// the test binary.
func captureStream(t *testing.T, stream **os.File, fn func()) string {
	t.Helper()

	r, w, err := os.Pipe()
	if err != nil {
		t.Fatalf("cannot create pipe: %v", err)
	}
	original := *stream
	*stream = w
	defer func() {
		*stream = original
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
