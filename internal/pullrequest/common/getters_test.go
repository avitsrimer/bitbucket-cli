package prcommon

import (
	"context"
	"errors"
	"net/http"
	"os"
	"slices"
	"strings"
	"testing"

	"github.com/avitsrimer/bitbucket-cli/internal/profile"
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
			cmd := setupTest(t, finitePagedHandler(&requests, map[string]string{tt.state: `{"id":42},{"id":7},{"id":13}`}))
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
			cmd := setupTest(t, finitePagedHandler(&requests, tt.idsByState))
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

// errorHandler fails every pullrequests request in failState with status and a BitBucket error
// payload, answering any other state with an empty page; requests records each request in order.
func errorHandler(requests *[]*http.Request, failState string, status int) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		*requests = append(*requests, r)
		w.Header().Set("Content-Type", "application/json")
		if r.URL.Query().Get("state") != failState {
			_, _ = w.Write([]byte(`{"values":[]}`))
			return
		}
		w.WriteHeader(status)
		_, _ = w.Write([]byte(`{"type":"error","error":{"message":"boom"}}`))
	}
}

func TestCompletePullRequestIDsAPIError(t *testing.T) {
	withState := func(ctx context.Context, cmd *cobra.Command) ([]string, error) {
		return CompletePullRequestIDsWithState(ctx, cmd, "OPEN")
	}
	tests := []struct {
		name       string
		complete   func(context.Context, *cobra.Command) ([]string, error)
		failState  string
		status     int
		wantStates []string
	}{
		{name: "with state", complete: withState, failState: "OPEN", status: http.StatusInternalServerError, wantStates: []string{"OPEN"}},
		{name: "with state not found", complete: withState, failState: "OPEN", status: http.StatusNotFound, wantStates: []string{"OPEN"}},
		{name: "open error does not fall back", complete: CompletePullRequestIDs, failState: "OPEN", status: http.StatusInternalServerError, wantStates: []string{"OPEN"}},
		{name: "none open then all error", complete: CompletePullRequestIDs, failState: "ALL", status: http.StatusInternalServerError, wantStates: []string{"OPEN", "ALL"}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var requests []*http.Request
			cmd := setupTest(t, errorHandler(&requests, tt.failState, tt.status))

			ids, err := tt.complete(context.Background(), cmd)
			if err == nil {
				t.Fatal("expected an error, got nil")
			}
			if ids == nil || len(ids) != 0 {
				t.Errorf("ids = %#v, want an empty non-nil slice", ids)
			}
			if prefix := "cannot get " + tt.failState + " pullrequests: "; !strings.HasPrefix(err.Error(), prefix) || !strings.Contains(err.Error(), "boom") {
				t.Errorf("error = %q, want prefix %q and the BitBucket error message", err.Error(), prefix)
			}
			var bberr *profile.BitBucketError
			if !errors.As(err, &bberr) {
				t.Errorf("error %q does not wrap a *profile.BitBucketError", err.Error())
			}
			if got, want := profile.IsNotFound(err), tt.status == http.StatusNotFound; got != want {
				t.Errorf("profile.IsNotFound(err) = %v, want %v", got, want)
			}
			if len(requests) != len(tt.wantStates) {
				t.Fatalf("got %d requests, want one per state %v", len(requests), tt.wantStates)
			}
			for i, state := range tt.wantStates {
				assertBoundedRequest(t, requests[i], state)
			}
		})
	}
}

func TestCompletePullRequestIDsWithStateResolutionError(t *testing.T) {
	tests := []struct {
		name       string
		flag       string
		value      string
		wantPrefix string
	}{
		{name: "repository", flag: "repository", value: "/missing-workspace", wantPrefix: "cannot get repository: "},
		{name: "profile", flag: "profile", value: "no-such-profile", wantPrefix: "cannot get profile: "},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cmd := setupTest(t, testutil.FailIfCalled(t, "an unresolvable "+tt.flag))
			if err := cmd.Flags().Set(tt.flag, tt.value); err != nil {
				t.Fatalf("cannot set %s flag: %v", tt.flag, err)
			}

			ids, err := CompletePullRequestIDsWithState(context.Background(), cmd, "OPEN")
			if err == nil || !strings.HasPrefix(err.Error(), tt.wantPrefix) {
				t.Fatalf("error = %v, want prefix %q", err, tt.wantPrefix)
			}
			if ids == nil || len(ids) != 0 {
				t.Errorf("ids = %#v, want an empty non-nil slice", ids)
			}
		})
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

	t.Run("filters by the typed prefix", func(t *testing.T) {
		var requests []*http.Request
		cmd := setupTest(t, finitePagedHandler(&requests, map[string]string{"OPEN": `{"id":42},{"id":7},{"id":4}`}))
		ids, directive := PullRequestIDValidArgs(cmd, nil, "4")
		if want := []string{"4", "42"}; !slices.Equal(ids, want) || directive != cobra.ShellCompDirectiveNoFileComp {
			t.Errorf("got (%v, %v), want (%v, %v)", ids, directive, want, cobra.ShellCompDirectiveNoFileComp)
		}
	})

	t.Run("an api error offers nothing silently without falling back", func(t *testing.T) {
		var requests []*http.Request
		cmd := setupTest(t, errorHandler(&requests, "OPEN", http.StatusInternalServerError))
		var ids []string
		var directive cobra.ShellCompDirective
		stderr := testutil.CaptureStderr(t, func() {
			ids, directive = PullRequestIDValidArgs(cmd, nil, "")
		})
		if ids == nil || len(ids) != 0 || directive != cobra.ShellCompDirectiveNoFileComp {
			t.Errorf("got (%#v, %v), want ([]string{}, %v)", ids, directive, cobra.ShellCompDirectiveNoFileComp)
		}
		if strings.Contains(stderr, "[Error]") {
			t.Errorf("stderr = %q, want no completion error reported", stderr)
		}
		if len(requests) != 1 {
			t.Fatalf("got %d requests, want exactly 1 (no ALL fallback after an OPEN error)", len(requests))
		}
		assertBoundedRequest(t, requests[0], "OPEN")
	})

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
