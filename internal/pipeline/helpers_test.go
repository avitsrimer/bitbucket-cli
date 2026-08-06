package pipeline

import (
	"io"
	"net/http"
	"os"
	"testing"

	"github.com/avitsrimer/bitbucket-cli/internal/testutil"
	"github.com/spf13/cobra"
)

// internal/testutil imports internal/repository (for RepositoryCache/WorkspaceCache), but neither
// it nor internal/repository imports internal/pipeline, so importing testutil from this internal
// ("package pipeline") test package does not cycle -- unlike internal/repository and
// internal/workspace's own test files, which need a local harness instead (see those packages'
// helpers_test.go).

func TestMain(m *testing.M) {
	os.Exit(testutil.TempCaches(m))
}

// setupTest points the profile client at a fresh httptest server, primes the fixture workspace and
// repository caches, and returns a standalone command carrying the flags this package's RunE
// functions read: everything testutil.SetupProfile registers, plus this package's own query,
// columns, and sort flags.
func setupTest(t *testing.T, handler http.HandlerFunc, dryRun bool) *cobra.Command {
	t.Helper()

	testutil.PrimeFixtureCaches(t)
	cmd := testutil.SetupProfile(t, "test", handler)
	if dryRun {
		_ = cmd.Flags().Set("dry-run", "true")
	}
	cmd.Flags().String("query", "", "")
	cmd.Flags().StringSlice("columns", []string{}, "")
	cmd.Flags().String("sort", "build_number", "")
	return cmd
}

// swapStdinToNonInteractivePipe temporarily replaces the package-level os.Stdin with the read end
// of a fresh pipe (whose write end is never closed or written to), restoring the original on
// cleanup. A pipe's read end reports as a named pipe (ModeCharDevice unset), reliably simulating a
// redirected, non-interactive stdin regardless of the ambient test environment's own stdin; a test
// that reads past the point it should have short-circuited hangs instead of failing silently.
func swapStdinToNonInteractivePipe(t *testing.T) {
	t.Helper()
	r, w, err := os.Pipe()
	if err != nil {
		t.Fatalf("cannot create pipe: %v", err)
	}
	t.Cleanup(func() { _ = w.Close() })
	original := os.Stdin
	os.Stdin = r
	t.Cleanup(func() { os.Stdin = original })
}

// poisonStdin fails the test immediately if it is ever read from, proving a --force/--dry-run path
// skipped the confirmation prompt entirely instead of merely happening to decline it.
type poisonStdin struct{ t *testing.T }

func (p poisonStdin) Read([]byte) (int, error) {
	p.t.Helper()
	p.t.Fatal("read from stdin despite --force or --dry-run")
	return 0, io.EOF
}
