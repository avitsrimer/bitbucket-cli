package pullrequest

import (
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"slices"
	"strings"
	"testing"

	"github.com/avitsrimer/bitbucket-cli/internal/testutil"
	"github.com/avitsrimer/bitbucket-cli/internal/user"
	"github.com/spf13/cobra"
)

func TestMain(m *testing.M) {
	os.Exit(testutil.TempCaches(m))
}

// setupTest points the profile client at a fresh httptest server and returns a standalone
// command carrying the flags runAction/mergeProcess read (profile, repository, output, dry-run).
func setupTest(t *testing.T, handler http.HandlerFunc, dryRun bool) *cobra.Command {
	t.Helper()
	return setupTestNamed(t, "test", handler, dryRun)
}

// setupTestNamed is setupTest with an explicit profile name; use it whenever a test's code path
// caches something keyed by profile name (e.g. user.UserCache) and must not collide with entries
// left behind by other tests sharing the "test" profile name.
func setupTestNamed(t *testing.T, profileName string, handler http.HandlerFunc, dryRun bool) *cobra.Command {
	t.Helper()

	testutil.PrimeFixtureCaches(t)
	cmd := testutil.SetupProfile(t, profileName, handler)
	// "id" mirrors columns' own DefaultSorter mark, the same default the real listCmd's --sort
	// flag carries (see common.RegisterListFlags), so listProcess sorts identically here and on
	// the real command when --sort is never passed.
	cmd.Flags().String("sort", "id", "")
	if dryRun {
		_ = cmd.Flags().Set("dry-run", "true")
	}
	return cmd
}

func TestRunActionSimpleActions(t *testing.T) {
	specs := []actionSpec{approveSpec, unapproveSpec, declineSpec, requestChangesSpec, removeRequestChangesSpec}

	for _, spec := range specs {
		t.Run(spec.name+"/success", func(t *testing.T) { testRunActionSuccess(t, spec) })
		t.Run(spec.name+"/preflight_error", func(t *testing.T) { testRunActionPreflightError(t, spec) })
		t.Run(spec.name+"/write_error", func(t *testing.T) { testRunActionWriteError(t, spec) })
		t.Run(spec.name+"/dry_run", func(t *testing.T) { testRunActionDryRun(t, spec) })
		t.Run(spec.name+"/invalid_id", func(t *testing.T) { testRunActionInvalidID(t, spec) })
	}
}

func testRunActionSuccess(t *testing.T, spec actionSpec) {
	t.Helper()

	var requests []*http.Request
	responseBody := []byte(`{"role":"REVIEWER","approved":true,"state":"approved","user":{"display_name":"Ada Lovelace"}}`)
	wantPath := "/2.0/repositories/" + testutil.FixtureRepositoryFlag + "/pullrequests/42/" + spec.endpoint

	cmd := setupTest(t, func(w http.ResponseWriter, r *http.Request) {
		requests = append(requests, r)
		w.Header().Set("Content-Type", "application/json")
		if r.Method == http.MethodGet {
			// the preflight existence check GETs the pullrequest itself, not spec.endpoint
			_, _ = w.Write([]byte(`{"id":42}`))
			return
		}
		if spec.post {
			_, _ = w.Write(responseBody)
		}
	}, false)

	call := func() {
		if err := runAction(cmd, []string{"42"}, spec); err != nil {
			t.Fatalf("runAction() error = %v", err)
		}
	}
	var stdout string
	if spec.post {
		stdout = testutil.CaptureStdout(t, call)
	} else {
		call()
	}

	if len(requests) != 2 {
		t.Fatalf("expected exactly 2 requests (preflight GET, write), got %d", len(requests))
	}
	if requests[0].Method != http.MethodGet {
		t.Errorf("first request method = %s, want GET (preflight existence check)", requests[0].Method)
	}
	wantGetPath := "/2.0/repositories/" + testutil.FixtureRepositoryFlag + "/pullrequests/42"
	if requests[0].URL.Path != wantGetPath {
		t.Errorf("first request path = %s, want %s", requests[0].URL.Path, wantGetPath)
	}
	wantMethod := http.MethodDelete
	if spec.post {
		wantMethod = http.MethodPost
	}
	if requests[1].Method != wantMethod {
		t.Errorf("method = %s, want %s", requests[1].Method, wantMethod)
	}
	if requests[1].URL.Path != wantPath {
		t.Errorf("path = %s, want %s", requests[1].URL.Path, wantPath)
	}

	if spec.post {
		var participant user.Participant
		if err := json.Unmarshal([]byte(stdout), &participant); err != nil {
			t.Fatalf("cannot unmarshal printed output %q: %v", stdout, err)
		}
		if participant.User.Name != "Ada Lovelace" {
			t.Errorf("printed participant user name = %q, want %q", participant.User.Name, "Ada Lovelace")
		}
	}
}

// testRunActionPreflightError proves a failure of the preflight existence GET (the pull request
// does not exist) surfaces as the "cannot <verb> pull request" wrap, without ever reaching the
// write.
func testRunActionPreflightError(t *testing.T, spec actionSpec) {
	t.Helper()

	var requests []*http.Request
	cmd := setupTest(t, func(w http.ResponseWriter, r *http.Request) {
		requests = append(requests, r)
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusNotFound)
		_, _ = w.Write([]byte(`{"type":"error","error":{"message":"pull request not found"}}`))
	}, false)

	err := runAction(cmd, []string{"42"}, spec)
	if err == nil {
		t.Fatal("runAction() expected an error, got nil")
	}
	wantSubstring := fmt.Sprintf("cannot %s pull request", spec.errVerb)
	if !strings.Contains(err.Error(), wantSubstring) {
		t.Errorf("error = %q, want it to contain %q", err.Error(), wantSubstring)
	}
	if !strings.Contains(err.Error(), "pull request not found") {
		t.Errorf("error = %q, want it to contain the BitBucket error message", err.Error())
	}
	if len(requests) != 1 {
		t.Errorf("expected exactly 1 request (the failed preflight GET, no write attempted), got %d", len(requests))
	}
}

// testRunActionWriteError proves a failure of the write itself (POST/DELETE), as opposed to the
// preflight existence GET, surfaces as the "failed to <verb> pull request 42" wrap -- the
// preflight GET must succeed first, or this assertion would be exercising the preflight error
// path instead.
func testRunActionWriteError(t *testing.T, spec actionSpec) {
	t.Helper()

	var requests []*http.Request
	cmd := setupTest(t, func(w http.ResponseWriter, r *http.Request) {
		requests = append(requests, r)
		w.Header().Set("Content-Type", "application/json")
		if r.Method == http.MethodGet {
			_, _ = w.Write([]byte(`{"id":42}`))
			return
		}
		w.WriteHeader(http.StatusBadRequest)
		_, _ = w.Write([]byte(`{"type":"error","error":{"message":"pull request state is not open"}}`))
	}, false)

	err := runAction(cmd, []string{"42"}, spec)
	if err == nil {
		t.Fatal("runAction() expected an error, got nil")
	}
	wantSubstring := fmt.Sprintf("failed to %s pull request 42", spec.errVerb)
	if !strings.Contains(err.Error(), wantSubstring) {
		t.Errorf("error = %q, want it to contain %q", err.Error(), wantSubstring)
	}
	if !strings.Contains(err.Error(), "pull request state is not open") {
		t.Errorf("error = %q, want it to contain the BitBucket error message", err.Error())
	}
	if len(requests) != 2 {
		t.Fatalf("expected exactly 2 requests (preflight GET, write), got %d", len(requests))
	}
	if requests[0].Method != http.MethodGet {
		t.Errorf("first request method = %s, want GET (preflight)", requests[0].Method)
	}
}

func testRunActionDryRun(t *testing.T, spec actionSpec) {
	t.Helper()

	var requests []*http.Request
	cmd := setupTest(t, func(w http.ResponseWriter, r *http.Request) {
		requests = append(requests, r)
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"id":42}`))
	}, true)

	if err := runAction(cmd, []string{"42"}, spec); err != nil {
		t.Fatalf("runAction() error = %v", err)
	}
	if len(requests) != 1 || requests[0].Method != http.MethodGet {
		t.Errorf("requests = %v, want exactly one preflight GET and no write in dry-run mode", requests)
	}
}

func testRunActionInvalidID(t *testing.T, spec actionSpec) {
	t.Helper()

	var requestCount int
	cmd := setupTest(t, func(http.ResponseWriter, *http.Request) { requestCount++ }, false)

	err := runAction(cmd, []string{"not-a-number"}, spec)
	if err == nil {
		t.Fatal("runAction() expected an error for an invalid pullrequest id, got nil")
	}
	if !strings.Contains(err.Error(), "argument pullrequest-id is invalid") {
		t.Errorf("error = %q, want it to mention the invalid pullrequest-id argument", err.Error())
	}
	if requestCount != 0 {
		t.Errorf("expected no HTTP request for an invalid pullrequest id, got %d", requestCount)
	}
}

func TestOpenPullRequestIDsCompletion(t *testing.T) {
	t.Run("returns no completions when an argument is already provided", func(t *testing.T) {
		cmd := setupTest(t, func(http.ResponseWriter, *http.Request) {
			t.Error("no HTTP request expected")
		}, false)

		ids, directive := openPullRequestIDsCompletion(cmd, []string{"42"}, "")
		if ids != nil {
			t.Errorf("ids = %v, want nil", ids)
		}
		if directive != cobra.ShellCompDirectiveNoFileComp {
			t.Errorf("directive = %v, want %v", directive, cobra.ShellCompDirectiveNoFileComp)
		}
	})

	t.Run("lists open pullrequest ids", func(t *testing.T) {
		var requests []*http.Request
		cmd := setupTest(t, func(w http.ResponseWriter, r *http.Request) {
			requests = append(requests, r)
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write([]byte(`{"values":[{"id":42},{"id":7}]}`))
		}, false)

		ids, directive := openPullRequestIDsCompletion(cmd, nil, "")
		if directive != cobra.ShellCompDirectiveNoFileComp {
			t.Errorf("directive = %v, want %v", directive, cobra.ShellCompDirectiveNoFileComp)
		}
		want := []string{"42", "7"}
		if !slices.Equal(ids, want) {
			t.Errorf("ids = %v, want %v", ids, want)
		}
		if len(requests) != 1 {
			t.Fatalf("expected exactly 1 request, got %d", len(requests))
		}
		wantPath := "/2.0/repositories/" + testutil.FixtureRepositoryFlag + "/pullrequests"
		if requests[0].URL.Path != wantPath || requests[0].URL.Query().Get("state") != "OPEN" {
			t.Errorf("request = %s?%s, want path %s with state=OPEN", requests[0].URL.Path, requests[0].URL.RawQuery, wantPath)
		}
	})
}

// mergeProcess/mergeValidArgs read the package-level mergeOptions var (bound to mergeCmd's
// flags), so tests mutate and restore it directly rather than going through cmd.Flags().
func withMergeOptions(t *testing.T, mutate func()) {
	t.Helper()
	old := mergeOptions
	t.Cleanup(func() { mergeOptions = old })
	mutate()
}

func TestMergeValidArgsDelegatesToSharedCompletion(t *testing.T) {
	cmd := setupTest(t, func(http.ResponseWriter, *http.Request) {
		t.Error("no HTTP request expected")
	}, false)

	ids, directive := mergeValidArgs(cmd, []string{"42"}, "")
	if ids != nil {
		t.Errorf("ids = %v, want nil", ids)
	}
	if directive != cobra.ShellCompDirectiveNoFileComp {
		t.Errorf("directive = %v, want %v", directive, cobra.ShellCompDirectiveNoFileComp)
	}
}

func TestMergeProcessSyncSuccess(t *testing.T) {
	withMergeOptions(t, func() {
		mergeOptions.Async = false
		mergeOptions.Message = ""
		mergeOptions.CloseSourceBranch = false
	})

	var requests []*http.Request
	cmd := setupTest(t, func(w http.ResponseWriter, r *http.Request) {
		requests = append(requests, r)
		w.Header().Set("Content-Type", "application/json")
		if r.Method == http.MethodGet {
			_, _ = w.Write([]byte(`{"id":42}`))
			return
		}
		_, _ = w.Write([]byte(`{"id":42,"title":"Add feature"}`))
	}, false)
	cmd.SetIn(strings.NewReader("y\n"))

	stdout := testutil.CaptureStdout(t, func() {
		if err := mergeProcess(cmd, []string{"42"}); err != nil {
			t.Fatalf("mergeProcess() error = %v", err)
		}
	})

	if len(requests) != 2 {
		t.Fatalf("expected exactly 2 requests (preflight GET, merge POST), got %d", len(requests))
	}
	if requests[0].Method != http.MethodGet {
		t.Errorf("first request method = %s, want GET (preflight existence check)", requests[0].Method)
	}
	if requests[1].Method != http.MethodPost {
		t.Errorf("method = %s, want POST", requests[1].Method)
	}
	wantPath := "/2.0/repositories/" + testutil.FixtureRepositoryFlag + "/pullrequests/42/merge"
	if requests[1].URL.Path != wantPath {
		t.Errorf("path = %s, want %s", requests[1].URL.Path, wantPath)
	}

	var pr PullRequest
	if err := json.Unmarshal([]byte(stdout), &pr); err != nil {
		t.Fatalf("cannot unmarshal printed output %q: %v", stdout, err)
	}
	if pr.Title != "Add feature" {
		t.Errorf("printed pullrequest title = %q, want %q", pr.Title, "Add feature")
	}
}

func TestMergeProcessAsyncSuccessUsesLocationHeader(t *testing.T) {
	withMergeOptions(t, func() {
		mergeOptions.Async = true
		mergeOptions.Message = ""
		mergeOptions.CloseSourceBranch = false
	})

	var requests []*http.Request
	cmd := setupTest(t, func(w http.ResponseWriter, r *http.Request) {
		requests = append(requests, r)
		if r.Method == http.MethodGet {
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write([]byte(`{"id":42}`))
			return
		}
		w.Header().Set(
			"Location",
			"https://api.bitbucket.org/2.0/repositories/acme/widgets/pullrequests/42/merge/task-status/abc123",
		)
		w.WriteHeader(http.StatusAccepted)
	}, false)
	cmd.SetIn(strings.NewReader("y\n"))

	stdout := testutil.CaptureStdout(t, func() {
		if err := mergeProcess(cmd, []string{"42"}); err != nil {
			t.Fatalf("mergeProcess() error = %v", err)
		}
	})

	if len(requests) != 2 {
		t.Fatalf("expected exactly 2 requests (preflight GET, async merge POST), got %d", len(requests))
	}
	if requests[1].URL.RawQuery != "async=true" {
		t.Errorf("query = %s, want async=true", requests[1].URL.RawQuery)
	}

	var status PullRequestMergeStatus
	if err := json.Unmarshal([]byte(stdout), &status); err != nil {
		t.Fatalf("cannot unmarshal printed output %q: %v", stdout, err)
	}
	if status.ID != "abc123" {
		t.Errorf("merge status id = %q, want %q", status.ID, "abc123")
	}
	if status.PullRequest.ID != 42 {
		t.Errorf("merge status pullrequest id = %d, want 42", status.PullRequest.ID)
	}
}

func TestMergeProcessAPIError(t *testing.T) {
	withMergeOptions(t, func() {
		mergeOptions.Async = false
		mergeOptions.Message = ""
		mergeOptions.CloseSourceBranch = false
	})

	cmd := setupTest(t, func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		if r.Method == http.MethodGet {
			_, _ = w.Write([]byte(`{"id":42}`))
			return
		}
		w.WriteHeader(http.StatusConflict)
		_, _ = w.Write([]byte(`{"type":"error","error":{"message":"merge conflict"}}`))
	}, false)
	cmd.SetIn(strings.NewReader("y\n"))

	err := mergeProcess(cmd, []string{"42"})
	if err == nil {
		t.Fatal("mergeProcess() expected an error, got nil")
	}
	if !strings.Contains(err.Error(), "failed to merge pull request 42") {
		t.Errorf("error = %q, want it to mention the failed merge", err.Error())
	}
	if !strings.Contains(err.Error(), "merge conflict") {
		t.Errorf("error = %q, want it to contain the BitBucket error message", err.Error())
	}
}

// TestMergeProcessPreflightAPIError verifies that a failure in the preflight existence GET (e.g.
// pull request 999999 does not exist) surfaces before any merge POST is attempted -- the same
// error a real merge against that id would eventually produce, but now caught during preflight
// instead of only by the (skipped) write under --dry-run.
func TestMergeProcessPreflightAPIError(t *testing.T) {
	withMergeOptions(t, func() {
		mergeOptions.Async = false
		mergeOptions.Message = ""
		mergeOptions.CloseSourceBranch = false
	})

	var requests []*http.Request
	cmd := setupTest(t, func(w http.ResponseWriter, r *http.Request) {
		requests = append(requests, r)
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusNotFound)
		_, _ = w.Write([]byte(`{"type":"error","error":{"message":"pull request not found"}}`))
	}, false)

	err := mergeProcess(cmd, []string{"999999"})
	if err == nil {
		t.Fatal("mergeProcess() expected an error, got nil")
	}
	if !strings.Contains(err.Error(), "failed to get pullrequest 999999") {
		t.Errorf("error = %q, want it to mention the failed preflight get", err.Error())
	}
	if !strings.Contains(err.Error(), "pull request not found") {
		t.Errorf("error = %q, want it to contain the BitBucket error message", err.Error())
	}
	if len(requests) != 1 || requests[0].Method != http.MethodGet {
		t.Errorf("requests = %v, want exactly one preflight GET and no merge POST", requests)
	}
}

// TestMergeProcessDryRunNonexistentPRFails is the FR-6 headline scenario: a nonexistent pull
// request (999999) must fail under --dry-run with the same error a real merge against that id
// would produce, instead of the pre-fix behavior of reporting a fixed, input-independent dry-run
// line at exit 0 regardless of whether the id exists.
func TestMergeProcessDryRunNonexistentPRFails(t *testing.T) {
	withMergeOptions(t, func() {
		mergeOptions.Async = false
		mergeOptions.Message = ""
		mergeOptions.CloseSourceBranch = false
	})

	var requests []*http.Request
	cmd := setupTest(t, func(w http.ResponseWriter, r *http.Request) {
		requests = append(requests, r)
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusNotFound)
		_, _ = w.Write([]byte(`{"type":"error","error":{"message":"pull request not found"}}`))
	}, true)

	err := mergeProcess(cmd, []string{"999999"})
	if err == nil {
		t.Fatal("mergeProcess() expected an error for a nonexistent pull request under --dry-run, got nil")
	}
	if !strings.Contains(err.Error(), "failed to get pullrequest 999999") {
		t.Errorf("error = %q, want it to mention the failed preflight get", err.Error())
	}
	if !strings.Contains(err.Error(), "pull request not found") {
		t.Errorf("error = %q, want it to contain the BitBucket error message", err.Error())
	}
	if len(requests) != 1 || requests[0].Method != http.MethodGet {
		t.Errorf("requests = %v, want exactly one preflight GET and no merge POST", requests)
	}
}

func TestMergeProcessDryRun(t *testing.T) {
	withMergeOptions(t, func() {
		mergeOptions.Async = false
		mergeOptions.Message = ""
		mergeOptions.CloseSourceBranch = false
	})

	var requests []*http.Request
	cmd := setupTest(t, func(w http.ResponseWriter, r *http.Request) {
		requests = append(requests, r)
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"id":42}`))
	}, true)

	if err := mergeProcess(cmd, []string{"42"}); err != nil {
		t.Fatalf("mergeProcess() error = %v", err)
	}
	if len(requests) != 1 || requests[0].Method != http.MethodGet {
		t.Errorf("requests = %v, want exactly one preflight GET and no merge POST in dry-run mode", requests)
	}
}

func TestMergeProcessInvalidID(t *testing.T) {
	withMergeOptions(t, func() {})

	var requestCount int
	cmd := setupTest(t, func(http.ResponseWriter, *http.Request) { requestCount++ }, false)

	err := mergeProcess(cmd, []string{"not-a-number"})
	if err == nil {
		t.Fatal("mergeProcess() expected an error for an invalid pullrequest id, got nil")
	}
	if !strings.Contains(err.Error(), "argument pullrequest-id is invalid") {
		t.Errorf("error = %q, want it to mention the invalid pullrequest-id argument", err.Error())
	}
	if requestCount != 0 {
		t.Errorf("expected no HTTP request for an invalid pullrequest id, got %d", requestCount)
	}
}
