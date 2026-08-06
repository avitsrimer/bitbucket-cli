package comment

import (
	"net/http"
	"os"
	"testing"

	"github.com/avitsrimer/bitbucket-cli/internal/testutil"
	"github.com/spf13/cobra"
)

func TestMain(m *testing.M) {
	os.Exit(testutil.TempCaches(m))
}

// setupTest primes the fixture workspace/repository caches, points the profile client at a fresh
// httptest server, and returns a standalone command carrying the flags this package's RunE
// functions read (profile, repository, output, dry-run, pending).
func setupTest(t *testing.T, handler http.HandlerFunc, dryRun bool) *cobra.Command {
	t.Helper()

	testutil.PrimeFixtureCaches(t)
	cmd := testutil.SetupProfile(t, "comment-test", handler)
	if dryRun {
		_ = cmd.Flags().Set("dry-run", "true")
	}
	return cmd
}

// withCommentEditOptions saves/restores a commentEditOptions value (createOptions or
// updateOptions, both package-level singletons bound to their command's flags at init) so tests
// can set the values they need without leaking state across other tests.
func withCommentEditOptions(t *testing.T, options *commentEditOptions, mutate func()) {
	t.Helper()
	old := *options
	t.Cleanup(func() {
		*options = old
	})
	mutate()
}
