package pullrequest

import (
	"net/http"
	"slices"
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
	// mirrors commits.go/activities.go setting their own --limit value on the same cmd used to
	// resolve the omitted pullrequest-id argument
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
	if !strings.Contains(err.Error(), "too many open pullrequests") {
		t.Errorf("error = %q, want it to contain %q", err.Error(), "too many open pullrequests")
	}
	if openRequests != 1 {
		t.Errorf("open pull requests queried %d times, want exactly 1", openRequests)
	}
}

// TestGetPullRequestIDFromArgsPagesThroughAllOpen guards the unbounded id-resolution path against
// the bounded completion getters: with open pull requests spread over two pages and --limit 1 on
// cmd, GetPullRequestIDFromArgs still follows "next" and lists the ids of both pages.
func TestGetPullRequestIDFromArgsPagesThroughAllOpen(t *testing.T) {
	var pages []string
	cmd := setupTestNamed(t, "id-resolution-pages", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		if r.URL.Query().Get("state") != "OPEN" {
			t.Errorf("unexpected state query: %q", r.URL.Query().Get("state"))
			return
		}
		pages = append(pages, r.URL.Query().Get("page"))
		if r.URL.Query().Get("page") == "2" {
			_, _ = w.Write([]byte(`{"values":[{"id":3}]}`))
			return
		}
		_, _ = w.Write([]byte(`{"values":[{"id":1},{"id":2}],"next":"http://` + r.Host + r.URL.Path + `?state=OPEN&page=2"}`))
	}, false)
	if err := cmd.Flags().Set("limit", "1"); err != nil {
		t.Fatalf("cannot set limit flag: %v", err)
	}

	repo, err := repository.GetRepository(cmd.Context(), cmd)
	if err != nil {
		t.Fatalf("cannot get repository: %v", err)
	}

	_, err = GetPullRequestIDFromArgs(cmd.Context(), cmd, repo, nil)
	if err == nil {
		t.Fatal("GetPullRequestIDFromArgs() expected an error for 3 open pull requests, got nil")
	}
	if want := "too many open pullrequests, specify one: 1, 2, 3"; !strings.Contains(err.Error(), want) {
		t.Errorf("error = %q, want it to contain %q", err.Error(), want)
	}
	if !slices.Equal(pages, []string{"", "2"}) {
		t.Errorf("requested pages = %q, want both pages", pages)
	}
}
