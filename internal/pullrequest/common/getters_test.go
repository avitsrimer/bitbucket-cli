package prcommon

import (
	"context"
	"net/http"
	"os"
	"slices"
	"strings"
	"testing"

	"github.com/avitsrimer/bitbucket-cli/internal/testutil"
	"github.com/spf13/cobra"
)

func TestMain(m *testing.M) {
	os.Exit(testutil.TempCaches(m))
}

// setupTest primes the fixture workspace/repository caches and points the profile client at a
// fresh httptest server, returning a standalone command carrying the flags the getters read
// (profile, repository, output).
func setupTest(t *testing.T, handler http.HandlerFunc) *cobra.Command {
	t.Helper()
	testutil.PrimeFixtureCaches(t)
	return testutil.SetupProfile(t, "prcommon-test", handler)
}

// pagedHandler answers every pullrequests request with the given ids and a "next" link, so a test
// can assert completion never follows it. requests records each request in order.
func pagedHandler(requests *[]*http.Request, idsByState map[string]string) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		*requests = append(*requests, r)
		w.Header().Set("Content-Type", "application/json")
		next := "http://" + r.Host + r.URL.Path + "?state=" + r.URL.Query().Get("state") + "&page=2"
		_, _ = w.Write([]byte(`{"values":[` + idsByState[r.URL.Query().Get("state")] + `],"next":"` + next + `"}`))
	}
}

// assertBoundedRequest checks that r is the single bounded completion request for state.
func assertBoundedRequest(t *testing.T, r *http.Request, state string) {
	t.Helper()
	wantPath := "/2.0/repositories/" + testutil.FixtureRepositoryFlag + "/pullrequests"
	if r.URL.Path != wantPath {
		t.Errorf("request path = %s, want %s", r.URL.Path, wantPath)
	}
	query := r.URL.Query()
	want := map[string]string{"state": state, "sort": "-updated_on", "pagelen": "50", "page": ""}
	for key, value := range want {
		if got := query.Get(key); got != value {
			t.Errorf("query %s = %q, want %q (raw query %q)", key, got, value, r.URL.RawQuery)
		}
	}
}

func TestCompletePullRequestIDsWithState(t *testing.T) {
	tests := []struct {
		name  string
		state string
		flags map[string]string
	}{
		{name: "open", state: "OPEN"},
		{name: "all", state: "ALL"},
		{name: "open ignores page-length", state: "OPEN", flags: map[string]string{"page-length": "10"}},
		{name: "all ignores limit", state: "ALL", flags: map[string]string{"limit": "1"}},
		{name: "open ignores page-length and limit", state: "OPEN", flags: map[string]string{"page-length": "10", "limit": "1"}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var requests []*http.Request
			cmd := setupTest(t, pagedHandler(&requests, map[string]string{tt.state: `{"id":42},{"id":7},{"id":13}`}))
			for name, value := range tt.flags {
				if err := cmd.Flags().Set(name, value); err != nil {
					t.Fatalf("cannot set %s flag: %v", name, err)
				}
			}

			ids, err := CompletePullRequestIDsWithState(context.Background(), cmd, tt.state)
			if err != nil {
				t.Fatalf("CompletePullRequestIDsWithState() error = %v", err)
			}
			want := []string{"13", "42", "7"}
			if !slices.Equal(ids, want) {
				t.Errorf("ids = %v, want %v", ids, want)
			}
			if len(requests) != 1 {
				t.Fatalf("expected exactly 1 request (next must not be followed), got %d", len(requests))
			}
			assertBoundedRequest(t, requests[0], tt.state)
		})
	}
}

func TestCompletePullRequestIDs(t *testing.T) {
	tests := []struct {
		name       string
		idsByState map[string]string
		wantStates []string
		wantIDs    []string
	}{
		{
			name:       "open pull requests are returned without fallback",
			idsByState: map[string]string{"OPEN": `{"id":1}`, "ALL": `{"id":1},{"id":2}`},
			wantStates: []string{"OPEN"},
			wantIDs:    []string{"1"},
		},
		{
			name:       "falls back to bounded all when none open",
			idsByState: map[string]string{"OPEN": ``, "ALL": `{"id":5},{"id":3}`},
			wantStates: []string{"OPEN", "ALL"},
			wantIDs:    []string{"3", "5"},
		},
		{
			name:       "no pull requests at all",
			idsByState: map[string]string{},
			wantStates: []string{"OPEN", "ALL"},
			wantIDs:    []string{},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var requests []*http.Request
			cmd := setupTest(t, pagedHandler(&requests, tt.idsByState))
			if err := cmd.Flags().Set("limit", "1"); err != nil {
				t.Fatalf("cannot set limit flag: %v", err)
			}

			ids, err := CompletePullRequestIDs(context.Background(), cmd)
			if err != nil {
				t.Fatalf("CompletePullRequestIDs() error = %v", err)
			}
			if !slices.Equal(ids, tt.wantIDs) {
				t.Errorf("ids = %v, want %v", ids, tt.wantIDs)
			}
			if len(requests) != len(tt.wantStates) {
				t.Fatalf("got %d requests, want %d (one per state %v)", len(requests), len(tt.wantStates), tt.wantStates)
			}
			for i, state := range tt.wantStates {
				assertBoundedRequest(t, requests[i], state)
			}
		})
	}
}

func TestCompletePullRequestIDsAPIError(t *testing.T) {
	tests := []struct {
		name     string
		complete func(context.Context, *cobra.Command) ([]string, error)
	}{
		{name: "with state", complete: func(ctx context.Context, cmd *cobra.Command) ([]string, error) {
			return CompletePullRequestIDsWithState(ctx, cmd, "OPEN")
		}},
		{name: "open then all", complete: CompletePullRequestIDs},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var requests int
			cmd := setupTest(t, func(w http.ResponseWriter, r *http.Request) {
				requests++
				w.Header().Set("Content-Type", "application/json")
				w.WriteHeader(http.StatusInternalServerError)
				_, _ = w.Write([]byte(`{"type":"error","error":{"message":"boom"}}`))
			})

			_, err := tt.complete(context.Background(), cmd)
			if err == nil {
				t.Fatal("expected an error, got nil")
			}
			if !strings.Contains(err.Error(), "boom") {
				t.Errorf("error = %q, want it to contain the BitBucket error message", err.Error())
			}
			if requests != 1 {
				t.Errorf("got %d requests, want exactly 1 (an error must not fall back)", requests)
			}
		})
	}
}

// finitePagedHandler serves each state's ids over two pages linked by "next" (page 1 -> 2), so an
// unbounded enumeration terminates and shows up as two requests for that state.
func finitePagedHandler(requests *[]*http.Request, idsByState map[string]string) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		*requests = append(*requests, r)
		w.Header().Set("Content-Type", "application/json")
		state := r.URL.Query().Get("state")
		if r.URL.Query().Get("page") == "2" {
			_, _ = w.Write([]byte(`{"values":[]}`))
			return
		}
		next := "http://" + r.Host + r.URL.Path + "?state=" + state + "&page=2"
		_, _ = w.Write([]byte(`{"values":[` + idsByState[state] + `],"next":"` + next + `"}`))
	}
}

func TestPullRequestIDValidArgsIsBounded(t *testing.T) {
	tests := []struct {
		name       string
		idsByState map[string]string
		wantStates []string
		wantIDs    []string
	}{
		{
			name:       "open pull requests",
			idsByState: map[string]string{"OPEN": `{"id":42},{"id":7}`},
			wantStates: []string{"OPEN"},
			wantIDs:    []string{"42", "7"},
		},
		{
			name:       "falls back to bounded all when none open",
			idsByState: map[string]string{"ALL": `{"id":5},{"id":3}`},
			wantStates: []string{"OPEN", "ALL"},
			wantIDs:    []string{"3", "5"},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var requests []*http.Request
			cmd := setupTest(t, finitePagedHandler(&requests, tt.idsByState))
			if err := cmd.Flags().Set("page-length", "10"); err != nil {
				t.Fatalf("cannot set page-length flag: %v", err)
			}

			ids, directive := PullRequestIDValidArgs(cmd, nil, "")
			if directive != cobra.ShellCompDirectiveNoFileComp {
				t.Errorf("directive = %v, want %v", directive, cobra.ShellCompDirectiveNoFileComp)
			}
			if !slices.Equal(ids, tt.wantIDs) {
				t.Errorf("ids = %v, want %v", ids, tt.wantIDs)
			}
			if len(requests) != len(tt.wantStates) {
				queries := make([]string, 0, len(requests))
				for _, r := range requests {
					queries = append(queries, r.URL.RawQuery)
				}
				t.Fatalf("got %d requests %q, want one bounded request per state %v", len(requests), queries, tt.wantStates)
			}
			for i, state := range tt.wantStates {
				assertBoundedRequest(t, requests[i], state)
			}
		})
	}

	t.Run("no request once the positional is filled", func(t *testing.T) {
		cmd := setupTest(t, func(http.ResponseWriter, *http.Request) {
			t.Error("no HTTP request expected")
		})
		ids, directive := PullRequestIDValidArgs(cmd, []string{"42"}, "")
		if ids != nil || directive != cobra.ShellCompDirectiveNoFileComp {
			t.Errorf("got (%v, %v), want (nil, %v)", ids, directive, cobra.ShellCompDirectiveNoFileComp)
		}
	})
}
