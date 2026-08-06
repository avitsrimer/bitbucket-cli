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
	if dryRun {
		_ = cmd.Flags().Set("dry-run", "true")
	}
	return cmd
}

func TestRunActionSimpleActions(t *testing.T) {
	specs := []actionSpec{approveSpec, unapproveSpec, declineSpec, requestChangesSpec, removeRequestChangesSpec}

	for _, spec := range specs {
		t.Run(spec.name+"/success", func(t *testing.T) { testRunActionSuccess(t, spec) })
		t.Run(spec.name+"/api_error", func(t *testing.T) { testRunActionAPIError(t, spec) })
		t.Run(spec.name+"/dry_run", func(t *testing.T) { testRunActionDryRun(t, spec) })
		t.Run(spec.name+"/invalid_id", func(t *testing.T) { testRunActionInvalidID(t, spec) })
	}
}

func testRunActionSuccess(t *testing.T, spec actionSpec) {
	t.Helper()

	var requests []*http.Request
	responseBody := []byte(`{"role":"REVIEWER","approved":true,"state":"approved","user":{"display_name":"Ada Lovelace"}}`)

	cmd := setupTest(t, func(w http.ResponseWriter, r *http.Request) {
		requests = append(requests, r)
		w.Header().Set("Content-Type", "application/json")
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

	if len(requests) != 1 {
		t.Fatalf("expected exactly 1 request, got %d", len(requests))
	}
	wantMethod := http.MethodDelete
	if spec.post {
		wantMethod = http.MethodPost
	}
	if requests[0].Method != wantMethod {
		t.Errorf("method = %s, want %s", requests[0].Method, wantMethod)
	}
	wantPath := "/2.0/repositories/" + testutil.FixtureRepositoryFlag + "/pullrequests/42/" + spec.endpoint
	if requests[0].URL.Path != wantPath {
		t.Errorf("path = %s, want %s", requests[0].URL.Path, wantPath)
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

func testRunActionAPIError(t *testing.T, spec actionSpec) {
	t.Helper()

	cmd := setupTest(t, func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
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
}

func testRunActionDryRun(t *testing.T, spec actionSpec) {
	t.Helper()

	var requestCount int
	cmd := setupTest(t, func(http.ResponseWriter, *http.Request) { requestCount++ }, true)

	if err := runAction(cmd, []string{"42"}, spec); err != nil {
		t.Fatalf("runAction() error = %v", err)
	}
	if requestCount != 0 {
		t.Errorf("expected no HTTP request in dry-run mode, got %d", requestCount)
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
		_, _ = w.Write([]byte(`{"id":42,"title":"Add feature"}`))
	}, false)

	stdout := testutil.CaptureStdout(t, func() {
		if err := mergeProcess(cmd, []string{"42"}); err != nil {
			t.Fatalf("mergeProcess() error = %v", err)
		}
	})

	if len(requests) != 1 {
		t.Fatalf("expected exactly 1 request, got %d", len(requests))
	}
	if requests[0].Method != http.MethodPost {
		t.Errorf("method = %s, want POST", requests[0].Method)
	}
	wantPath := "/2.0/repositories/" + testutil.FixtureRepositoryFlag + "/pullrequests/42/merge"
	if requests[0].URL.Path != wantPath {
		t.Errorf("path = %s, want %s", requests[0].URL.Path, wantPath)
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
		w.Header().Set(
			"Location",
			"https://api.bitbucket.org/2.0/repositories/acme/widgets/pullrequests/42/merge/task-status/abc123",
		)
		w.WriteHeader(http.StatusAccepted)
	}, false)

	stdout := testutil.CaptureStdout(t, func() {
		if err := mergeProcess(cmd, []string{"42"}); err != nil {
			t.Fatalf("mergeProcess() error = %v", err)
		}
	})

	if len(requests) != 1 {
		t.Fatalf("expected exactly 1 request, got %d", len(requests))
	}
	if requests[0].URL.RawQuery != "async=true" {
		t.Errorf("query = %s, want async=true", requests[0].URL.RawQuery)
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
		w.WriteHeader(http.StatusConflict)
		_, _ = w.Write([]byte(`{"type":"error","error":{"message":"merge conflict"}}`))
	}, false)

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

func TestMergeProcessDryRun(t *testing.T) {
	withMergeOptions(t, func() {
		mergeOptions.Async = false
		mergeOptions.Message = ""
		mergeOptions.CloseSourceBranch = false
	})

	var requestCount int
	cmd := setupTest(t, func(http.ResponseWriter, *http.Request) { requestCount++ }, true)

	if err := mergeProcess(cmd, []string{"42"}); err != nil {
		t.Fatalf("mergeProcess() error = %v", err)
	}
	if requestCount != 0 {
		t.Errorf("expected no HTTP request in dry-run mode, got %d", requestCount)
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
