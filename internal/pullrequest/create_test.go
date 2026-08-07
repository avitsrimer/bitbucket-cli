package pullrequest

import (
	"encoding/json"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/avitsrimer/bitbucket-cli/internal/profile"
	"github.com/avitsrimer/bitbucket-cli/internal/repository"
	"github.com/avitsrimer/bitbucket-cli/internal/testutil"
	"github.com/spf13/cobra"
)

// withCreateOptions saves/restores the package-level createOptions (bound to createCmd's flags
// at init) so tests can set the values they need without leaking state across other tests.
func withCreateOptions(t *testing.T, mutate func()) {
	t.Helper()
	oldTitle, oldDescription, oldDescriptionFile := createOptions.Title, createOptions.Description, createOptions.DescriptionFile
	oldSourceValue, oldDestinationValue := createOptions.Source.Value, createOptions.Destination.Value
	oldCloseSourceBranch, oldDraft := createOptions.CloseSourceBranch, createOptions.Draft
	t.Cleanup(func() {
		createOptions.Title = oldTitle
		createOptions.Description = oldDescription
		createOptions.DescriptionFile = oldDescriptionFile
		createOptions.Source.Value = oldSourceValue
		createOptions.Destination.Value = oldDestinationValue
		createOptions.CloseSourceBranch = oldCloseSourceBranch
		createOptions.Draft = oldDraft
	})
	mutate()
}

// setReviewerFlag sets cmd's --reviewer flag to values, calling Set once per element so a test
// can distinguish "--reviewer a --reviewer b" (repeated flag, append semantics) from a single
// comma-separated element such as "a,b" (one Set call, CSV-split) -- both are real invocation
// shapes createProcess must treat identically. createProcess reads this flag directly off cmd
// (never the package-level createOptions), so it must be set here rather than via
// withCreateOptions.
func setReviewerFlag(t *testing.T, cmd *cobra.Command, values ...string) {
	t.Helper()
	for _, value := range values {
		if err := cmd.Flags().Set("reviewer", value); err != nil {
			t.Fatalf("cannot set --reviewer=%s: %v", value, err)
		}
	}
}

func TestCreateProcessSuccessWithDefaultReviewers(t *testing.T) {
	withCreateOptions(t, func() {
		createOptions.Title = "Add feature"
		createOptions.Description = "some description"
		createOptions.Source.Value = "feature"
		createOptions.Destination.Value = ""
	})

	var requests []*http.Request
	mux := http.NewServeMux()
	mux.HandleFunc("/2.0/user", func(w http.ResponseWriter, r *http.Request) {
		requests = append(requests, r)
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"uuid":"{11111111-1111-1111-1111-111111111111}","display_name":"Current User"}`))
	})
	mux.HandleFunc("/2.0/repositories/"+testutil.FixtureRepositoryFlag+"/effective-default-reviewers", func(w http.ResponseWriter, r *http.Request) {
		requests = append(requests, r)
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"values":[]}`))
	})
	mux.HandleFunc("/2.0/repositories/"+testutil.FixtureRepositoryFlag+"/pullrequests", func(w http.ResponseWriter, r *http.Request) {
		requests = append(requests, r)
		if r.Method != http.MethodPost {
			t.Errorf("method = %s, want POST", r.Method)
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"id":99,"title":"Add feature"}`))
	})

	const profileName = "create-success"
	cmd := setupTestNamed(t, profileName, mux.ServeHTTP, false)

	stdout := testutil.CaptureStdout(t, func() {
		if err := createProcess(cmd, nil); err != nil {
			t.Fatalf("createProcess() error = %v", err)
		}
	})

	if len(requests) != 3 {
		t.Fatalf("expected exactly 3 requests (user, effective-default-reviewers, pullrequests), got %d", len(requests))
	}

	var pr PullRequest
	if err := json.Unmarshal([]byte(stdout), &pr); err != nil {
		t.Fatalf("cannot unmarshal printed output %q: %v", stdout, err)
	}
	if pr.ID != 99 || pr.Title != "Add feature" {
		t.Errorf("printed pullrequest = %+v, want id=99 title=%q", pr, "Add feature")
	}
}

func TestCreateProcessMissingTitle(t *testing.T) {
	withCreateOptions(t, func() {
		createOptions.Title = ""
	})

	var requestCount int
	cmd := setupTest(t, func(http.ResponseWriter, *http.Request) { requestCount++ }, false)

	err := createProcess(cmd, nil)
	if err == nil {
		t.Fatal("createProcess() expected an error, got nil")
	}
	if !strings.Contains(err.Error(), "argument title is missing") {
		t.Errorf("error = %q, want it to mention the missing title argument", err.Error())
	}
	if requestCount != 0 {
		t.Errorf("expected no HTTP request when title is missing, got %d", requestCount)
	}
}

func TestCreateProcessDefaultReviewersAPIError(t *testing.T) {
	withCreateOptions(t, func() {
		createOptions.Title = "Add feature"
		createOptions.Source.Value = "feature"
		createOptions.Destination.Value = ""
	})

	var pullrequestRequests int
	mux := http.NewServeMux()
	mux.HandleFunc("/2.0/user", func(w http.ResponseWriter, r *http.Request) {
		// simulate a repo-scoped token without access to /user; createProcess only warns on this
		w.WriteHeader(http.StatusForbidden)
	})
	mux.HandleFunc("/2.0/repositories/"+testutil.FixtureRepositoryFlag+"/effective-default-reviewers", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusInternalServerError)
		_, _ = w.Write([]byte(`{"type":"error","error":{"message":"internal error"}}`))
	})
	mux.HandleFunc("/2.0/repositories/"+testutil.FixtureRepositoryFlag+"/pullrequests", func(w http.ResponseWriter, r *http.Request) {
		pullrequestRequests++
	})

	const profileName = "create-reviewers-error"
	cmd := setupTestNamed(t, profileName, mux.ServeHTTP, false)

	err := createProcess(cmd, nil)
	if err == nil {
		t.Fatal("createProcess() expected an error, got nil")
	}
	if !strings.Contains(err.Error(), "failed to get the default reviewers") {
		t.Errorf("error = %q, want it to mention the failed default reviewers lookup", err.Error())
	}
	if !strings.Contains(err.Error(), "internal error") {
		t.Errorf("error = %q, want it to contain the BitBucket error message", err.Error())
	}
	if pullrequestRequests != 0 {
		t.Errorf("expected no pullrequest creation request, got %d", pullrequestRequests)
	}
}

// TestCreateProcessDefaultReviewersAPIErrorWarnOnErrorProceedsWithoutReviewers proves the
// error-tolerance matrix now applies to the *implicit* default-reviewers lookup (no --reviewer
// flag at all): before the fix, resolveCreateDefaultReviewers always returned the lookup failure
// as a hard error regardless of --warn-on-error/--ignore-errors, aborting pullrequest creation the
// caller never asked reviewers for in the first place. With the fix, --warn-on-error must let the
// pullrequest be created anyway, warning on stderr and simply omitting the reviewers.
func TestCreateProcessDefaultReviewersAPIErrorWarnOnErrorProceedsWithoutReviewers(t *testing.T) {
	withCreateOptions(t, func() {
		createOptions.Title = "Add feature"
		createOptions.Source.Value = "feature"
		createOptions.Destination.Value = ""
	})

	var postBody PullRequestCreator
	var pullrequestRequests int
	mux := http.NewServeMux()
	mux.HandleFunc("/2.0/user", func(w http.ResponseWriter, r *http.Request) {
		// simulate a repo-scoped token without access to /user; createProcess only warns on this
		w.WriteHeader(http.StatusForbidden)
	})
	mux.HandleFunc("/2.0/repositories/"+testutil.FixtureRepositoryFlag+"/effective-default-reviewers", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusInternalServerError)
		_, _ = w.Write([]byte(`{"type":"error","error":{"message":"internal error"}}`))
	})
	mux.HandleFunc("/2.0/repositories/"+testutil.FixtureRepositoryFlag+"/pullrequests", func(w http.ResponseWriter, r *http.Request) {
		pullrequestRequests++
		if err := json.NewDecoder(r.Body).Decode(&postBody); err != nil {
			t.Errorf("cannot decode POST body: %v", err)
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"id":99,"title":"Add feature"}`))
	})

	cmd := setupTestNamed(t, "create-default-reviewers-warn-on-error", mux.ServeHTTP, false)
	if err := cmd.Flags().Set("warn-on-error", "true"); err != nil {
		t.Fatalf("cannot set --warn-on-error: %v", err)
	}

	var createErr error
	stderr := testutil.CaptureStderr(t, func() {
		_ = testutil.CaptureStdout(t, func() {
			createErr = createProcess(cmd, nil)
		})
	})

	if createErr != nil {
		t.Fatalf("createProcess() error = %v, want nil: --warn-on-error must tolerate the failed default-reviewers lookup", createErr)
	}
	if pullrequestRequests != 1 {
		t.Fatalf("expected exactly 1 pullrequest creation request, got %d", pullrequestRequests)
	}
	if !strings.Contains(stderr, "internal error") {
		t.Errorf("stderr = %q, want it to warn about the failed default-reviewers lookup", stderr)
	}
	if len(postBody.Reviewers) != 0 {
		t.Errorf("posted reviewers = %+v, want none: the default-reviewers lookup failed", postBody.Reviewers)
	}
}

// TestCreateProcessExplicitDefaultReviewersAPIErrorIgnoresWarnOnError verifies that an EXPLICIT
// `--reviewer default` still hard-fails on a default-reviewers lookup failure even with
// --warn-on-error set: unlike the implicit lookup (no --reviewer flag at all), the caller here
// explicitly asked for the default reviewers, so a failure to resolve them must not be silently
// tolerated into a pullrequest with none of the reviewers the caller asked for.
func TestCreateProcessExplicitDefaultReviewersAPIErrorIgnoresWarnOnError(t *testing.T) {
	withCreateOptions(t, func() {
		createOptions.Title = "Add feature"
		createOptions.Source.Value = "feature"
		createOptions.Destination.Value = ""
	})

	var pullrequestRequests int
	mux := http.NewServeMux()
	mux.HandleFunc("/2.0/user", func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusForbidden)
	})
	mux.HandleFunc("/2.0/repositories/"+testutil.FixtureRepositoryFlag+"/effective-default-reviewers", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusInternalServerError)
		_, _ = w.Write([]byte(`{"type":"error","error":{"message":"internal error"}}`))
	})
	mux.HandleFunc("/2.0/repositories/"+testutil.FixtureRepositoryFlag+"/pullrequests", func(w http.ResponseWriter, r *http.Request) {
		pullrequestRequests++
	})

	cmd := setupTestNamed(t, "create-explicit-default-reviewers-warn-on-error", mux.ServeHTTP, false)
	setReviewerFlag(t, cmd, "default")
	if err := cmd.Flags().Set("warn-on-error", "true"); err != nil {
		t.Fatalf("cannot set --warn-on-error: %v", err)
	}

	err := createProcess(cmd, nil)
	if err == nil {
		t.Fatal("createProcess() expected an error, got nil: an explicit --reviewer default must not tolerate a failed lookup")
	}
	if !strings.Contains(err.Error(), "failed to get the default reviewers") {
		t.Errorf("error = %q, want it to mention the failed default reviewers lookup", err.Error())
	}
	if pullrequestRequests != 0 {
		t.Errorf("expected no pullrequest creation request, got %d", pullrequestRequests)
	}
}

func TestCreateProcessPostAPIError(t *testing.T) {
	withCreateOptions(t, func() {
		createOptions.Title = "Add feature"
		createOptions.Source.Value = "dummy"
		createOptions.Destination.Value = ""
	})

	fixture, err := os.ReadFile("../../testdata/error-badrequest-nobranch.json")
	if err != nil {
		t.Fatalf("cannot read testdata: %v", err)
	}

	mux := http.NewServeMux()
	mux.HandleFunc("/2.0/repositories/"+testutil.FixtureRepositoryFlag+"/effective-default-reviewers", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"values":[]}`))
	})
	mux.HandleFunc("/2.0/repositories/"+testutil.FixtureRepositoryFlag+"/pullrequests", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusBadRequest)
		_, _ = w.Write(fixture)
	})

	const profileName = "create-post-error"
	cmd := setupTestNamed(t, profileName, mux.ServeHTTP, false)

	err = createProcess(cmd, nil)
	if err == nil {
		t.Fatal("createProcess() expected an error, got nil")
	}
	if !strings.Contains(err.Error(), "failed to create pullrequest") {
		t.Errorf("error = %q, want it to mention the failed create", err.Error())
	}
	if !strings.Contains(err.Error(), "branch not found: dummy") {
		t.Errorf("error = %q, want it to contain the BitBucket error message", err.Error())
	}
}

// TestCreateProcessUnresolvedReviewerErrorsBeforePost verifies that a --reviewer list containing
// an unresolvable value aborts pullrequest creation with an error naming only the offending
// value(s), and creates no pullrequest.
func TestCreateProcessUnresolvedReviewerErrorsBeforePost(t *testing.T) {
	tests := []struct {
		name          string
		reviewers     []string
		profileName   string
		wantErrSubstr string
		wantNotErr    string
	}{
		{
			name:          "single typo reviewer",
			reviewers:     []string{"jdoe-typo"},
			profileName:   "create-reviewer-typo",
			wantErrSubstr: "jdoe-typo",
		},
		{
			name:          "mixed valid and invalid reviewers names only the invalid one",
			reviewers:     []string{"alice", "bobb"},
			profileName:   "create-reviewer-mixed",
			wantErrSubstr: "bobb",
			wantNotErr:    "alice is not a member",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			withCreateOptions(t, func() {
				createOptions.Title = "Add feature"
				createOptions.Source.Value = "feature"
				createOptions.Destination.Value = ""
			})

			var pullrequestRequests int
			mux := http.NewServeMux()
			mux.HandleFunc("/2.0/workspaces/"+testutil.FixtureWorkspaceSlug+"/members", func(w http.ResponseWriter, r *http.Request) {
				w.Header().Set("Content-Type", "application/json")
				_, _ = w.Write([]byte(`{"values":[{"user":{"uuid":"{33333333-3333-3333-3333-333333333333}","nickname":"alice"}}]}`))
			})
			mux.HandleFunc("/2.0/repositories/"+testutil.FixtureRepositoryFlag+"/pullrequests", func(w http.ResponseWriter, r *http.Request) {
				pullrequestRequests++
			})

			cmd := setupTestNamed(t, tt.profileName, mux.ServeHTTP, false)
			setReviewerFlag(t, cmd, tt.reviewers...)

			err := createProcess(cmd, nil)
			if err == nil {
				t.Fatal("createProcess() expected an error, got nil")
			}
			if !strings.Contains(err.Error(), tt.wantErrSubstr) {
				t.Errorf("error = %q, want it to name the unresolved reviewer %q", err.Error(), tt.wantErrSubstr)
			}
			if tt.wantNotErr != "" && strings.Contains(err.Error(), tt.wantNotErr) {
				t.Errorf("error = %q, must not also contain %q", err.Error(), tt.wantNotErr)
			}
			if pullrequestRequests != 0 {
				t.Errorf("expected no pullrequest creation request, got %d", pullrequestRequests)
			}
		})
	}
}

// TestCreateProcessWarnOnErrorProceedsWithUnresolvedReviewer reproduces critical finding #1: before
// the fix, ShouldStopOnError's fallback checked only profile.ErrorProcessing == StopOnError (its
// zero value), so an explicit --warn-on-error still hard-failed here -- the same as the default
// stop-on-error behavior in TestCreateProcessUnresolvedReviewerErrorsBeforePost -- because a test
// profile has no ErrorProcessing configured at all. With the fix, --warn-on-error must let the
// pullrequest be created anyway, warning on stderr and simply omitting the unresolved reviewer.
func TestCreateProcessWarnOnErrorProceedsWithUnresolvedReviewer(t *testing.T) {
	withCreateOptions(t, func() {
		createOptions.Title = "Add feature"
		createOptions.Source.Value = "feature"
		createOptions.Destination.Value = ""
	})

	var postBody PullRequestCreator
	var pullrequestRequests int
	mux := http.NewServeMux()
	mux.HandleFunc("/2.0/workspaces/"+testutil.FixtureWorkspaceSlug+"/members", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"values":[{"user":{"uuid":"{33333333-3333-3333-3333-333333333333}","nickname":"alice"}}]}`))
	})
	mux.HandleFunc("/2.0/repositories/"+testutil.FixtureRepositoryFlag+"/pullrequests", func(w http.ResponseWriter, r *http.Request) {
		pullrequestRequests++
		if err := json.NewDecoder(r.Body).Decode(&postBody); err != nil {
			t.Errorf("cannot decode POST body: %v", err)
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"id":99,"title":"Add feature"}`))
	})

	cmd := setupTestNamed(t, "create-reviewer-warn-on-error", mux.ServeHTTP, false)
	setReviewerFlag(t, cmd, "jdoe-typo")
	if err := cmd.Flags().Set("warn-on-error", "true"); err != nil {
		t.Fatalf("cannot set --warn-on-error: %v", err)
	}

	var createErr error
	stderr := testutil.CaptureStderr(t, func() {
		_ = testutil.CaptureStdout(t, func() {
			createErr = createProcess(cmd, nil)
		})
	})

	if createErr != nil {
		t.Fatalf("createProcess() error = %v, want nil: --warn-on-error must tolerate the unresolved reviewer", createErr)
	}
	if pullrequestRequests != 1 {
		t.Fatalf("expected exactly 1 pullrequest creation request, got %d", pullrequestRequests)
	}
	if !strings.Contains(stderr, "jdoe-typo") {
		t.Errorf("stderr = %q, want it to warn about the unresolved reviewer jdoe-typo", stderr)
	}
	if len(postBody.Reviewers) != 0 {
		t.Errorf("posted reviewers = %+v, want none: the only requested reviewer could not be resolved", postBody.Reviewers)
	}
}

// TestCreateProcessReviewerAllExpandsToEveryMember verifies that a --reviewer value of exactly
// "all" expands to every workspace member's nickname via expandAllReviewers at resolution time.
func TestCreateProcessReviewerAllExpandsToEveryMember(t *testing.T) {
	withCreateOptions(t, func() {
		createOptions.Title = "Add feature"
		createOptions.Source.Value = "feature"
		createOptions.Destination.Value = ""
	})

	var postBody PullRequestCreator
	mux := http.NewServeMux()
	mux.HandleFunc("/2.0/workspaces/"+testutil.FixtureWorkspaceSlug+"/members", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"values":[` +
			`{"user":{"uuid":"{33333333-3333-3333-3333-333333333333}","nickname":"alice"}},` +
			`{"user":{"uuid":"{44444444-4444-4444-4444-444444444444}","nickname":"bob"}}` +
			`]}`))
	})
	mux.HandleFunc("/2.0/repositories/"+testutil.FixtureRepositoryFlag+"/pullrequests", func(w http.ResponseWriter, r *http.Request) {
		if err := json.NewDecoder(r.Body).Decode(&postBody); err != nil {
			t.Errorf("cannot decode POST body: %v", err)
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"id":99,"title":"Add feature"}`))
	})

	const profileName = "create-reviewer-all"
	cmd := setupTestNamed(t, profileName, mux.ServeHTTP, false)
	setReviewerFlag(t, cmd, "all")

	if err := createProcess(cmd, nil); err != nil {
		t.Fatalf("createProcess() error = %v", err)
	}

	if len(postBody.Reviewers) != 2 {
		t.Fatalf("payload reviewers = %+v, want every workspace member (2)", postBody.Reviewers)
	}
	nicknames := map[string]bool{}
	for _, reviewer := range postBody.Reviewers {
		nicknames[reviewer.Nickname] = true
	}
	if !nicknames["alice"] || !nicknames["bob"] {
		t.Errorf("payload reviewer nicknames = %v, want both alice and bob", nicknames)
	}
}

// TestCreateProcessReviewerAllErrorsWhenMembersCannotBeListed reproduces major finding #1's first
// defect: both create.go and update.go used to discard the member-listing error
// (`members, _ := ...GetMembers(...)`), so a --reviewer all whose workspace/repo-scoped token
// cannot read /workspaces/{slug}/members (a common restriction) expanded "all" against a nil
// member list, resolved to zero reviewers, and still created the pullrequest at exit 0 -- the
// exact silent no-op the ShouldStopOnError/ShouldWarnOnError/ShouldIgnoreErrors tolerance was
// introduced to eliminate. expandAllReviewers must instead surface that error as a hard failure,
// since there is nothing to expand "all" to without the member list.
func TestCreateProcessReviewerAllErrorsWhenMembersCannotBeListed(t *testing.T) {
	withCreateOptions(t, func() {
		createOptions.Title = "Add feature"
		createOptions.Source.Value = "feature"
		createOptions.Destination.Value = ""
	})

	var pullrequestRequests int
	mux := http.NewServeMux()
	mux.HandleFunc("/2.0/workspaces/"+testutil.FixtureWorkspaceSlug+"/members", func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusForbidden)
	})
	mux.HandleFunc("/2.0/repositories/"+testutil.FixtureRepositoryFlag+"/pullrequests", func(w http.ResponseWriter, r *http.Request) {
		pullrequestRequests++
	})

	cmd := setupTestNamed(t, "create-reviewer-all-members-error", mux.ServeHTTP, false)
	setReviewerFlag(t, cmd, "all")

	err := createProcess(cmd, nil)
	if err == nil {
		t.Fatal("createProcess() expected an error, got nil: a --reviewer all that cannot list members must not silently create a pullrequest with zero reviewers")
	}
	if pullrequestRequests != 0 {
		t.Errorf("expected no pullrequest creation request, got %d", pullrequestRequests)
	}
}

// TestCreateProcessReviewerAllDoesNotDuplicateAMatchingReviewer reproduces major finding #1's
// second defect in resolveExplicitReviewers (create.go): matchesMember previously compared against
// member.User.Nickname, which Bitbucket leaves empty for many accounts; an empty expanded value
// then matched every nickname-less member via EqualFold("", "") and matches[0] always won, so
// resolveExplicitReviewers (unlike update.go's addRequestedReviewers) had no dedupe and appended
// the same user twice whenever "all" expanded to more than one nickname-less member.
// expandAllReviewers must expand to each member's UUID (never empty) instead of their nickname,
// and resolveExplicitReviewers must dedupe like update.go already does.
func TestCreateProcessReviewerAllDoesNotDuplicateAMatchingReviewer(t *testing.T) {
	withCreateOptions(t, func() {
		createOptions.Title = "Add feature"
		createOptions.Source.Value = "feature"
		createOptions.Destination.Value = ""
	})

	var postBody PullRequestCreator
	mux := http.NewServeMux()
	mux.HandleFunc("/2.0/workspaces/"+testutil.FixtureWorkspaceSlug+"/members", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		// Two members with no nickname at all, the case Bitbucket allows and the regression
		// depended on: an empty expanded reviewer value used to match both of them.
		_, _ = w.Write([]byte(`{"values":[` +
			`{"user":{"uuid":"{33333333-3333-3333-3333-333333333333}"}},` +
			`{"user":{"uuid":"{44444444-4444-4444-4444-444444444444}"}}` +
			`]}`))
	})
	mux.HandleFunc("/2.0/repositories/"+testutil.FixtureRepositoryFlag+"/pullrequests", func(w http.ResponseWriter, r *http.Request) {
		if err := json.NewDecoder(r.Body).Decode(&postBody); err != nil {
			t.Errorf("cannot decode POST body: %v", err)
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"id":99,"title":"Add feature"}`))
	})

	cmd := setupTestNamed(t, "create-reviewer-all-no-nickname", mux.ServeHTTP, false)
	setReviewerFlag(t, cmd, "all")

	if err := createProcess(cmd, nil); err != nil {
		t.Fatalf("createProcess() error = %v", err)
	}

	if len(postBody.Reviewers) != 2 {
		t.Fatalf("payload reviewers = %+v, want exactly 2 (one per member, no duplicates)", postBody.Reviewers)
	}
	ids := map[string]bool{}
	for _, reviewer := range postBody.Reviewers {
		ids[reviewer.ID.String()] = true
	}
	if len(ids) != 2 {
		t.Errorf("payload reviewers = %+v, want 2 distinct members, got %d distinct IDs (%v): the same member must not be appended twice", postBody.Reviewers, len(ids), ids)
	}
}

// TestCreateProcessReviewerNoneSkipsAllResolution verifies that a --reviewer value of exactly
// "none" creates the pullrequest with no "reviewers" key in the posted payload at all (nil slice,
// omitempty) and issues zero reviewer-resolution requests: no /user, no
// effective-default-reviewers, no workspace members lookup.
func TestCreateProcessReviewerNoneSkipsAllResolution(t *testing.T) {
	withCreateOptions(t, func() {
		createOptions.Title = "Add feature"
		createOptions.Source.Value = "feature"
		createOptions.Destination.Value = ""
	})

	var postBody string
	mux := http.NewServeMux()
	mux.HandleFunc("/2.0/user", testutil.FailIfCalled(t, "a --reviewer none create"))
	mux.HandleFunc("/2.0/repositories/"+testutil.FixtureRepositoryFlag+"/effective-default-reviewers", testutil.FailIfCalled(t, "a --reviewer none create"))
	mux.HandleFunc("/2.0/workspaces/"+testutil.FixtureWorkspaceSlug+"/members", testutil.FailIfCalled(t, "a --reviewer none create"))
	mux.HandleFunc("/2.0/repositories/"+testutil.FixtureRepositoryFlag+"/pullrequests", func(w http.ResponseWriter, r *http.Request) {
		body, err := io.ReadAll(r.Body)
		if err != nil {
			t.Fatalf("cannot read POST body: %v", err)
		}
		postBody = string(body)
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"id":99,"title":"Add feature"}`))
	})

	cmd := setupTestNamed(t, "create-reviewer-none", mux.ServeHTTP, false)
	setReviewerFlag(t, cmd, "none")

	testutil.CaptureStdout(t, func() {
		if err := createProcess(cmd, nil); err != nil {
			t.Fatalf("createProcess() error = %v", err)
		}
	})

	if strings.Contains(postBody, `"reviewers"`) {
		t.Errorf("posted payload = %s, want no \"reviewers\" key at all", postBody)
	}
}

// TestCreateProcessReviewerNoneCombinedWithOthersErrorsBeforeAnyWrite verifies that "none"
// appearing anywhere alongside another --reviewer value is rejected with the pinned error message
// before any write -- and, in this fixture, before any HTTP request at all, per
// testutil.FailIfCalled below -- regardless of whether the values arrived as repeated flags or a
// single comma-separated list, and regardless of what the other value is (a plain nickname or the
// "all" sentinel).
func TestCreateProcessReviewerNoneCombinedWithOthersErrorsBeforeAnyWrite(t *testing.T) {
	tests := []struct {
		name        string
		profileName string
		reviewers   []string
	}{
		{name: "none then alice as repeated flags", profileName: "create-reviewer-none-alice-repeated", reviewers: []string{"none", "alice"}},
		{name: "none,alice as one comma-separated value", profileName: "create-reviewer-none-alice-csv", reviewers: []string{"none,alice"}},
		{name: "none then all as repeated flags", profileName: "create-reviewer-none-all-repeated", reviewers: []string{"none", "all"}},
	}

	const wantErr = `cannot combine reviewer "none" with other reviewers`

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			withCreateOptions(t, func() {
				createOptions.Title = "Add feature"
				createOptions.Source.Value = "feature"
				createOptions.Destination.Value = ""
			})

			cmd := setupTestNamed(t, tt.profileName, testutil.FailIfCalled(t, "a --reviewer none combined with other reviewers"), false)
			setReviewerFlag(t, cmd, tt.reviewers...)

			err := createProcess(cmd, nil)
			if err == nil {
				t.Fatal("createProcess() expected an error, got nil")
			}
			if err.Error() != wantErr {
				t.Errorf("error = %q, want %q", err.Error(), wantErr)
			}
		})
	}
}

// TestCreateProcessExplicitDefaultReviewersSuccess verifies that --reviewer default still resolves
// and posts the repository's effective default reviewers, unchanged by the none-sentinel addition.
func TestCreateProcessExplicitDefaultReviewersSuccess(t *testing.T) {
	withCreateOptions(t, func() {
		createOptions.Title = "Add feature"
		createOptions.Source.Value = "feature"
		createOptions.Destination.Value = ""
	})

	var postBody PullRequestCreator
	mux := http.NewServeMux()
	mux.HandleFunc("/2.0/user", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"uuid":"{55555555-5555-5555-5555-555555555555}","display_name":"Current User"}`))
	})
	mux.HandleFunc("/2.0/repositories/"+testutil.FixtureRepositoryFlag+"/effective-default-reviewers", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"values":[{"user":{"uuid":"{33333333-3333-3333-3333-333333333333}","nickname":"alice"}}]}`))
	})
	mux.HandleFunc("/2.0/repositories/"+testutil.FixtureRepositoryFlag+"/pullrequests", func(w http.ResponseWriter, r *http.Request) {
		if err := json.NewDecoder(r.Body).Decode(&postBody); err != nil {
			t.Errorf("cannot decode POST body: %v", err)
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"id":99,"title":"Add feature"}`))
	})

	cmd := setupTestNamed(t, "create-reviewer-default-success", mux.ServeHTTP, false)
	setReviewerFlag(t, cmd, "default")

	testutil.CaptureStdout(t, func() {
		if err := createProcess(cmd, nil); err != nil {
			t.Fatalf("createProcess() error = %v", err)
		}
	})

	if len(postBody.Reviewers) != 1 || postBody.Reviewers[0].Nickname != "alice" {
		t.Errorf("posted reviewers = %+v, want exactly the effective default reviewer alice", postBody.Reviewers)
	}
}

// TestCreateProcessDryRunReviewerNoneOmitsReviewersKey verifies that --reviewer none's dry-run
// echo shows no "reviewers" key, matching what a real invocation would post -- FR-6's dry-run
// symmetry, since both a dry run and a real run skip reviewer resolution identically under "none".
func TestCreateProcessDryRunReviewerNoneOmitsReviewersKey(t *testing.T) {
	withCreateOptions(t, func() {
		createOptions.Title = "Add feature"
		createOptions.Source.Value = "feature"
		createOptions.Destination.Value = ""
	})

	cmd := setupTestNamed(t, "create-reviewer-none-dry-run", testutil.FailIfCalled(t, "a --reviewer none dry run"), true)
	setReviewerFlag(t, cmd, "none")

	stderr := testutil.CaptureStderr(t, func() {
		if err := createProcess(cmd, nil); err != nil {
			t.Fatalf("createProcess() error = %v", err)
		}
	})

	if strings.Contains(stderr, `"reviewers"`) {
		t.Errorf("dry-run echo = %s, want no \"reviewers\" key", stderr)
	}
}

func TestCreateProcessDryRun(t *testing.T) {
	withCreateOptions(t, func() {
		createOptions.Title = "Add feature"
		createOptions.Source.Value = "feature"
		createOptions.Destination.Value = ""
	})

	var pullrequestRequests int
	mux := http.NewServeMux()
	mux.HandleFunc("/2.0/user", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"uuid":"{22222222-2222-2222-2222-222222222222}","display_name":"Current User"}`))
	})
	mux.HandleFunc("/2.0/repositories/"+testutil.FixtureRepositoryFlag+"/effective-default-reviewers", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"values":[]}`))
	})
	mux.HandleFunc("/2.0/repositories/"+testutil.FixtureRepositoryFlag+"/pullrequests", func(w http.ResponseWriter, r *http.Request) {
		pullrequestRequests++
	})

	const profileName = "create-dry-run"
	cmd := setupTestNamed(t, profileName, mux.ServeHTTP, true)

	if err := createProcess(cmd, nil); err != nil {
		t.Fatalf("createProcess() error = %v", err)
	}
	if pullrequestRequests != 0 {
		t.Errorf("expected no pullrequest creation request in dry-run mode, got %d", pullrequestRequests)
	}
}

// TestCreateProcessDescriptionFromFileVerbatim verifies that --description-file's content lands
// in the POSTed description verbatim, including the shell-quoting hazard class (backticks and
// $()) that --description-file exists to route around.
func TestCreateProcessDescriptionFromFileVerbatim(t *testing.T) {
	body := "Fixes the flaky test by running `go test -race ./...` and checking $(git diff) first.\n"
	path := filepath.Join(t.TempDir(), "description.md")
	if err := os.WriteFile(path, []byte(body), 0o600); err != nil {
		t.Fatalf("cannot write fixture file: %v", err)
	}

	withCreateOptions(t, func() {
		createOptions.Title = "Add feature"
		createOptions.Description = ""
		createOptions.DescriptionFile = path
		createOptions.Source.Value = "feature"
		createOptions.Destination.Value = ""
	})

	var postBody PullRequestCreator
	mux := http.NewServeMux()
	mux.HandleFunc("/2.0/repositories/"+testutil.FixtureRepositoryFlag+"/effective-default-reviewers", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"values":[]}`))
	})
	mux.HandleFunc("/2.0/repositories/"+testutil.FixtureRepositoryFlag+"/pullrequests", func(w http.ResponseWriter, r *http.Request) {
		if err := json.NewDecoder(r.Body).Decode(&postBody); err != nil {
			t.Errorf("cannot decode POST body: %v", err)
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"id":99,"title":"Add feature"}`))
	})

	cmd := setupTestNamed(t, "create-description-file", mux.ServeHTTP, false)

	testutil.CaptureStdout(t, func() {
		if err := createProcess(cmd, nil); err != nil {
			t.Fatalf("createProcess() error = %v", err)
		}
	})

	if postBody.Description != body {
		t.Errorf("posted description = %q, want %q (verbatim)", postBody.Description, body)
	}
}

// TestCreateProcessEmptyDescriptionFileBodyErrors verifies that a --description-file pointing at
// an empty (or whitespace-only) file is rejected before any HTTP request, consistent with FR-6's
// empty-body rule.
func TestCreateProcessEmptyDescriptionFileBodyErrors(t *testing.T) {
	path := filepath.Join(t.TempDir(), "empty.md")
	if err := os.WriteFile(path, []byte("  \n"), 0o600); err != nil {
		t.Fatalf("cannot write fixture file: %v", err)
	}

	withCreateOptions(t, func() {
		createOptions.Title = "Add feature"
		createOptions.Description = ""
		createOptions.DescriptionFile = path
		createOptions.Source.Value = "feature"
	})

	var requestCount int
	cmd := setupTest(t, func(http.ResponseWriter, *http.Request) { requestCount++ }, false)

	err := createProcess(cmd, nil)
	if err == nil {
		t.Fatal("createProcess() expected an error, got nil")
	}
	if !strings.Contains(err.Error(), "description body is empty") {
		t.Errorf("error = %q, want it to mention the empty description body", err.Error())
	}
	if requestCount != 0 {
		t.Errorf("expected no HTTP request for an empty description-file body, got %d", requestCount)
	}
}

// TestResolveExplicitReviewersNeverPanicsOnNilWorkspace proves resolveExplicitReviewers does not
// dereference repository.Workspace directly: BitBucket omits "workspace" on a trimmed nested
// Repository payload, and update.go's addRequestedReviewers already resolves the workspace slug
// through repository.GetWorkspaceSlug (which falls back to FullName) instead of reading
// Workspace.Slug directly for exactly this reason -- this proves create.go's own reviewer
// resolution follows the same safe pattern.
func TestResolveExplicitReviewersNeverPanicsOnNilWorkspace(t *testing.T) {
	repo := &repository.Repository{Slug: testutil.FixtureRepositorySlug, FullName: testutil.FixtureWorkspaceSlug + "/" + testutil.FixtureRepositorySlug}

	var requests []*http.Request
	mux := http.NewServeMux()
	mux.HandleFunc("/2.0/workspaces/"+testutil.FixtureWorkspaceSlug+"/members", func(w http.ResponseWriter, r *http.Request) {
		requests = append(requests, r)
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"values":[{"user":{"uuid":"{33333333-3333-3333-3333-333333333333}","nickname":"alice"}}]}`))
	})

	cmd := setupTestNamed(t, "create-nil-workspace-reviewer", mux.ServeHTTP, false)
	currentProfile, err := profile.GetProfileFromCommand(cmd.Context(), cmd)
	if err != nil {
		t.Fatalf("cannot get profile: %v", err)
	}

	reviewers, err := resolveExplicitReviewers(cmd.Context(), cmd, currentProfile, repo, []string{"alice"})
	if err != nil {
		t.Fatalf("resolveExplicitReviewers() error = %v, want it to resolve the workspace from FullName without panicking", err)
	}
	if len(reviewers) != 1 || reviewers[0].Nickname != "alice" {
		t.Errorf("reviewers = %+v, want exactly one reviewer named %q", reviewers, "alice")
	}
	if len(requests) != 1 {
		t.Fatalf("expected exactly 1 request to the members endpoint, got %d", len(requests))
	}
}
