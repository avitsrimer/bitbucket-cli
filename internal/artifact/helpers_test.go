package artifact

import (
	"net/http"
	"os"
	"testing"

	"github.com/avitsrimer/bitbucket-cli/internal/testutil"
	"github.com/spf13/cobra"
)

// internal/testutil imports internal/repository, internal/user, and internal/workspace (for their
// caches), but none of them import internal/artifact, so importing testutil from this internal
// ("package artifact") test package does not cycle -- same reasoning as internal/commit,
// internal/branch, and internal/pipeline's own test files (see those packages' helpers_test.go).

func TestMain(m *testing.M) {
	os.Exit(testutil.TempCaches(m))
}

// setupTest points the profile client at a fresh httptest server, primes the fixture workspace and
// repository caches, and returns a standalone command carrying the flags this package's RunE
// functions read: everything testutil.SetupProfile registers, plus this package's own query,
// columns, sort, and destination flags.
func setupTest(t *testing.T, handler http.HandlerFunc, dryRun bool) *cobra.Command {
	t.Helper()

	testutil.PrimeFixtureCaches(t)
	cmd := testutil.SetupProfile(t, "test", handler)
	if dryRun {
		_ = cmd.Flags().Set("dry-run", "true")
	}
	cmd.Flags().String("query", "", "")
	cmd.Flags().String("columns", "", "")
	cmd.Flags().String("sort", "", "")
	cmd.Flags().String("destination", "", "")
	return cmd
}
