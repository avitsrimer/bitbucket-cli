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
// functions read: everything testutil.SetupProfile registers, plus this package's own columns and
// sort flags. The pipeline and step ids are read as positionals, not flags, so they are passed
// directly in the args slice at each call site instead of being registered here.
func setupTest(t *testing.T, handler http.HandlerFunc, dryRun bool) *cobra.Command {
	t.Helper()

	testutil.PrimeFixtureCaches(t)
	cmd := testutil.SetupProfile(t, "test", handler)
	if dryRun {
		_ = cmd.Flags().Set("dry-run", "true")
	}
	cmd.Flags().StringSlice("columns", []string{}, "")
	// "" mirrors columns' own lack of a DefaultSorter (no column here is marked DefaultSorter, so
	// the real listCmd's --sort flag defaults to "" too), which is what makes listProcess skip
	// sorting entirely and preserve the API's own execution order by default.
	cmd.Flags().String("sort", "", "")
	return cmd
}
