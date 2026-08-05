package pullrequest

import (
	"net/http"
	"strings"
	"testing"

	"github.com/avitsrimer/bitbucket-cli/internal/repository"
)

// TestGetPullRequestIDFromArgsIgnoresAmbientLimitFlag is a regression test: GetPullRequestIDFromArgs
// resolves an omitted pullrequest-id from the repository's own open pull requests, called with the
// very same cmd whose own, unrelated --limit flag bounds a different query (e.g. `pr commits
// --limit 1`'s eventual commits fetch, or `pr activities --limit 1`'s activities fetch). Before
// GetPullRequestIDsFromRepositoryWithState switched to GetAllUnbounded, GetAll's ambient --limit
// sniffing truncated this resolution query too: with 4 open pull requests and --limit 1, exactly 1
// id came back, the "too many pullrequests" guard below never tripped, and the command silently
// operated on an arbitrary pull request instead of refusing to guess.
func TestGetPullRequestIDFromArgsIgnoresAmbientLimitFlag(t *testing.T) {
	var openRequests int
	cmd := setupTestNamed(t, "limit-flag-id-resolution", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		if r.URL.Query().Get("state") == "OPEN" {
			openRequests++
			_, _ = w.Write([]byte(`{"values":[{"id":1},{"id":2},{"id":3},{"id":4}]}`))
		}
	}, false)
	// mirrors commits.go/activities.go registering their own --limit flag on the same cmd used to
	// resolve the omitted pullrequest-id argument
	cmd.Flags().Int("limit", 0, "")
	if err := cmd.Flags().Set("limit", "1"); err != nil {
		t.Fatalf("cannot set limit flag: %v", err)
	}

	repo, err := repository.GetRepository(cmd.Context(), cmd)
	if err != nil {
		t.Fatalf("cannot get repository: %v", err)
	}

	_, err = GetPullRequestIDFromArgs(cmd.Context(), cmd, repo, nil)
	if err == nil {
		t.Fatal("GetPullRequestIDFromArgs() expected an error for 4 open pull requests, got nil")
	}
	if !strings.Contains(err.Error(), "too many pullrequests") {
		t.Errorf("error = %q, want it to contain %q", err.Error(), "too many pullrequests")
	}
	if openRequests != 1 {
		t.Errorf("open pull requests queried %d times, want exactly 1", openRequests)
	}
}
