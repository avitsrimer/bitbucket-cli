package step

import (
	"net/http"
	"os"
	"testing"

	"github.com/avitsrimer/bitbucket-cli/internal/testutil"
	"github.com/spf13/cobra"
)

// internal/testutil imports internal/repository (for RepositoryCache/WorkspaceCache), but neither
// it nor internal/repository imports internal/pipeline/step, so importing testutil from this
// internal ("package step") test package does not cycle -- same precedent as internal/pipeline,
// internal/commit, and internal/branch (see those packages' helpers_test.go).

func TestMain(m *testing.M) {
	os.Exit(testutil.TempCaches(m))
}

// setupTest points the profile client at a fresh httptest server, primes the fixture workspace and
// repository caches, and returns a standalone command carrying the flags this package's RunE
// functions read: everything testutil.SetupProfile registers, plus this package's own pipeline,
// columns, and sort flags.
func setupTest(t *testing.T, handler http.HandlerFunc, dryRun bool) *cobra.Command {
	t.Helper()

	testutil.PrimeFixtureCaches(t)
	cmd := testutil.SetupProfile(t, "test", handler)
	if dryRun {
		_ = cmd.Flags().Set("dry-run", "true")
	}
	cmd.Flags().String("pipeline", "", "")
	cmd.Flags().StringSlice("columns", []string{}, "")
	cmd.Flags().String("sort", "id", "")
	return cmd
}
