package pullrequest

import (
	"net/http"
	"slices"
	"strings"
	"testing"

	"github.com/avitsrimer/bitbucket-cli/internal/testutil"
	"github.com/spf13/cobra"
)

// threePageHandler serves pull request ids over three pages linked by "next" (page 1 -> 2 -> 3),
// recording every request, so a test can tell a single bounded request from a full enumeration.
func threePageHandler(requests *[]*http.Request) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		*requests = append(*requests, r)
		w.Header().Set("Content-Type", "application/json")
		next := func(page string) string {
			return `,"next":"http://` + r.Host + r.URL.Path + `?state=` + r.URL.Query().Get("state") + `&page=` + page + `"`
		}
		switch r.URL.Query().Get("page") {
		case "":
			_, _ = w.Write([]byte(`{"values":[{"id":42},{"id":7}]` + next("2") + `}`))
		case "2":
			_, _ = w.Write([]byte(`{"values":[{"id":5}]` + next("3") + `}`))
		default:
			_, _ = w.Write([]byte(`{"values":[{"id":3}]}`))
		}
	}
}

// pullRequestIDCompletionCommands lists every command of the real pullrequest tree completing a
// <pullrequest-id> positional, with the state its completion queries.
var pullRequestIDCompletionCommands = []struct {
	name  string
	state string
}{
	{"get", "ALL"},
	{"update", "ALL"},
	{"commits", "OPEN"},
	{"activities", "OPEN"},
	{"approve", "OPEN"},
	{"unapprove", "OPEN"},
	{"decline", "OPEN"},
	{"request-changes", "OPEN"},
	{"remove-request-changes", "OPEN"},
	{"merge", "OPEN"},
	{"patch", "OPEN"},
	{"merge-status", "OPEN"},
	{"diff", "OPEN"},
}

// completionOf returns the ValidArgsFunction wired on the real pullrequest subcommand name.
func completionOf(t *testing.T, name string) cobra.CompletionFunc {
	t.Helper()
	for _, sub := range Command.Commands() {
		if sub.Name() == name {
			if sub.ValidArgsFunction == nil {
				t.Fatalf("pullrequest %s has no ValidArgsFunction", name)
			}
			return sub.ValidArgsFunction
		}
	}
	t.Fatalf("pullrequest %s command not found", name)
	return nil
}

// TestPullRequestIDCompletionIsBounded proves every pull request id ValidArgsFunction wired on the
// pullrequest command tree issues exactly one bounded request (the 50 most recently updated pull
// requests of its state) and never follows "next", whatever --limit/--page-length cmd carries.
func TestPullRequestIDCompletionIsBounded(t *testing.T) {
	for _, tt := range pullRequestIDCompletionCommands {
		t.Run(tt.name, func(t *testing.T) {
			var requests []*http.Request
			cmd := setupTestNamed(t, "bounded-completion", threePageHandler(&requests), false)
			for name, value := range map[string]string{"page-length": "10", "limit": "1"} {
				if err := cmd.Flags().Set(name, value); err != nil {
					t.Fatalf("cannot set %s flag: %v", name, err)
				}
			}

			ids, directive := completionOf(t, tt.name)(cmd, nil, "")
			if directive != cobra.ShellCompDirectiveNoFileComp {
				t.Errorf("directive = %v, want %v", directive, cobra.ShellCompDirectiveNoFileComp)
			}
			if want := []string{"42", "7"}; !slices.Equal(ids, want) {
				t.Errorf("ids = %v, want %v (first page only)", ids, want)
			}
			if len(requests) != 1 {
				pages := make([]string, 0, len(requests))
				for _, r := range requests {
					pages = append(pages, r.URL.RawQuery)
				}
				t.Fatalf("got %d requests %q, want exactly 1 bounded request", len(requests), pages)
			}
			wantPath := "/2.0/repositories/" + testutil.FixtureRepositoryFlag + "/pullrequests"
			if requests[0].URL.Path != wantPath {
				t.Errorf("request path = %s, want %s", requests[0].URL.Path, wantPath)
			}
			query := requests[0].URL.Query()
			for key, value := range map[string]string{"state": tt.state, "sort": "-updated_on", "pagelen": "50"} {
				if got := query.Get(key); got != value {
					t.Errorf("query %s = %q, want %q (raw query %q)", key, got, value, requests[0].URL.RawQuery)
				}
			}
		})
	}
}

// TestPullRequestIDCompletionError proves an API failure yields no candidates, the error
// directive, a single request, and the failure reported to the shell via CompErrorln.
func TestPullRequestIDCompletionError(t *testing.T) {
	for _, tt := range pullRequestIDCompletionCommands {
		t.Run(tt.name, func(t *testing.T) {
			var requests int
			cmd := setupTestNamed(t, "completion-error", func(w http.ResponseWriter, _ *http.Request) {
				requests++
				w.Header().Set("Content-Type", "application/json")
				w.WriteHeader(http.StatusInternalServerError)
				_, _ = w.Write([]byte(`{"type":"error","error":{"message":"boom"}}`))
			}, false)

			var ids []string
			var directive cobra.ShellCompDirective
			stderr := testutil.CaptureStderr(t, func() {
				ids, directive = completionOf(t, tt.name)(cmd, nil, "")
			})
			if ids == nil || len(ids) != 0 {
				t.Errorf("ids = %#v, want an empty non-nil slice", ids)
			}
			if directive != cobra.ShellCompDirectiveError {
				t.Errorf("directive = %v, want %v", directive, cobra.ShellCompDirectiveError)
			}
			if requests != 1 {
				t.Errorf("got %d requests, want exactly 1", requests)
			}
			if want := "cannot get " + tt.state + " pullrequests: "; !strings.Contains(stderr, want) || !strings.Contains(stderr, "boom") {
				t.Errorf("stderr = %q, want it to contain %q and the API message", stderr, want)
			}
		})
	}
}

func TestPullRequestIDCompletionFiltersByPrefix(t *testing.T) {
	cmd := setupTestNamed(t, "completion-prefix", func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"values":[{"id":42},{"id":7},{"id":4}]}`))
	}, false)

	ids, directive := completionOf(t, "get")(cmd, nil, "4")
	if directive != cobra.ShellCompDirectiveNoFileComp {
		t.Errorf("directive = %v, want %v", directive, cobra.ShellCompDirectiveNoFileComp)
	}
	if want := []string{"4", "42"}; !slices.Equal(ids, want) {
		t.Errorf("ids = %v, want %v", ids, want)
	}
}
