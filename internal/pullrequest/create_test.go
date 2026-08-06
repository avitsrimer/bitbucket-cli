package pullrequest

import (
	"encoding/json"
	"net/http"
	"os"
	"strings"
	"testing"
)

// withCreateOptions saves/restores the package-level createOptions (bound to createCmd's flags
// at init) so tests can set the values they need without leaking state across other tests.
func withCreateOptions(t *testing.T, mutate func()) {
	t.Helper()
	oldTitle, oldDescription := createOptions.Title, createOptions.Description
	oldSourceValue, oldDestinationValue := createOptions.Source.Value, createOptions.Destination.Value
	oldReviewerValues := createOptions.Reviewers
	oldCloseSourceBranch, oldDraft := createOptions.CloseSourceBranch, createOptions.Draft
	t.Cleanup(func() {
		createOptions.Title = oldTitle
		createOptions.Description = oldDescription
		createOptions.Source.Value = oldSourceValue
		createOptions.Destination.Value = oldDestinationValue
		createOptions.Reviewers = oldReviewerValues
		createOptions.CloseSourceBranch = oldCloseSourceBranch
		createOptions.Draft = oldDraft
	})
	mutate()
}

func TestCreateProcessSuccessWithDefaultReviewers(t *testing.T) {
	withCreateOptions(t, func() {
		createOptions.Title = "Add feature"
		createOptions.Description = "some description"
		createOptions.Source.Value = "feature"
		createOptions.Destination.Value = ""
		createOptions.Reviewers = nil
	})

	var requests []*http.Request
	mux := http.NewServeMux()
	mux.HandleFunc("/2.0/user", func(w http.ResponseWriter, r *http.Request) {
		requests = append(requests, r)
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"uuid":"{11111111-1111-1111-1111-111111111111}","display_name":"Current User"}`))
	})
	mux.HandleFunc("/2.0/repositories/"+fixtureRepositoryFlag+"/effective-default-reviewers", func(w http.ResponseWriter, r *http.Request) {
		requests = append(requests, r)
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"values":[]}`))
	})
	mux.HandleFunc("/2.0/repositories/"+fixtureRepositoryFlag+"/pullrequests", func(w http.ResponseWriter, r *http.Request) {
		requests = append(requests, r)
		if r.Method != http.MethodPost {
			t.Errorf("method = %s, want POST", r.Method)
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"id":99,"title":"Add feature"}`))
	})

	const profileName = "create-success"
	cmd := setupTestNamed(t, profileName, mux.ServeHTTP, false)

	stdout := captureStdout(t, func() {
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
		createOptions.Reviewers = nil
	})

	var pullrequestRequests int
	mux := http.NewServeMux()
	mux.HandleFunc("/2.0/user", func(w http.ResponseWriter, r *http.Request) {
		// simulate a repo-scoped token without access to /user; createProcess only warns on this
		w.WriteHeader(http.StatusForbidden)
	})
	mux.HandleFunc("/2.0/repositories/"+fixtureRepositoryFlag+"/effective-default-reviewers", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusInternalServerError)
		_, _ = w.Write([]byte(`{"type":"error","error":{"message":"internal error"}}`))
	})
	mux.HandleFunc("/2.0/repositories/"+fixtureRepositoryFlag+"/pullrequests", func(w http.ResponseWriter, r *http.Request) {
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

func TestCreateProcessPostAPIError(t *testing.T) {
	withCreateOptions(t, func() {
		createOptions.Title = "Add feature"
		createOptions.Source.Value = "dummy"
		createOptions.Destination.Value = ""
		createOptions.Reviewers = nil
	})

	fixture, err := os.ReadFile("../../testdata/error-badrequest-nobranch.json")
	if err != nil {
		t.Fatalf("cannot read testdata: %v", err)
	}

	mux := http.NewServeMux()
	mux.HandleFunc("/2.0/repositories/"+fixtureRepositoryFlag+"/effective-default-reviewers", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"values":[]}`))
	})
	mux.HandleFunc("/2.0/repositories/"+fixtureRepositoryFlag+"/pullrequests", func(w http.ResponseWriter, r *http.Request) {
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

// TestCreateProcessTypoReviewerErrorsBeforePost is a regression test for review-iter5 finding 2:
// an unresolvable --reviewer value used to be silently dropped (printed to stderr, no error
// returned), so `pullrequest create --reviewer jdoe-typo` created a pullrequest with the reviewer
// omitted and exited 0. It must now abort with an error naming the offending value, and the
// pullrequest must never be created.
func TestCreateProcessTypoReviewerErrorsBeforePost(t *testing.T) {
	withCreateOptions(t, func() {
		createOptions.Title = "Add feature"
		createOptions.Source.Value = "feature"
		createOptions.Destination.Value = ""
		createOptions.Reviewers = []string{"jdoe-typo"}
	})

	var pullrequestRequests int
	mux := http.NewServeMux()
	mux.HandleFunc("/2.0/workspaces/"+fixtureWorkspaceSlug+"/members", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"values":[{"user":{"uuid":"{33333333-3333-3333-3333-333333333333}","nickname":"alice"}}]}`))
	})
	mux.HandleFunc("/2.0/repositories/"+fixtureRepositoryFlag+"/pullrequests", func(w http.ResponseWriter, r *http.Request) {
		pullrequestRequests++
	})

	const profileName = "create-reviewer-typo"
	cmd := setupTestNamed(t, profileName, mux.ServeHTTP, false)

	err := createProcess(cmd, nil)
	if err == nil {
		t.Fatal("createProcess() expected an error, got nil")
	}
	if !strings.Contains(err.Error(), "jdoe-typo") {
		t.Errorf("error = %q, want it to name the unresolved reviewer jdoe-typo", err.Error())
	}
	if pullrequestRequests != 0 {
		t.Errorf("expected no pullrequest creation request, got %d", pullrequestRequests)
	}
}

// TestCreateProcessMixedValidInvalidReviewersNamesInvalidOne proves a --reviewer list combining a
// resolvable and an unresolvable value fails naming only the invalid one, and still creates
// nothing.
func TestCreateProcessMixedValidInvalidReviewersNamesInvalidOne(t *testing.T) {
	withCreateOptions(t, func() {
		createOptions.Title = "Add feature"
		createOptions.Source.Value = "feature"
		createOptions.Destination.Value = ""
		createOptions.Reviewers = []string{"alice", "bobb"}
	})

	var pullrequestRequests int
	mux := http.NewServeMux()
	mux.HandleFunc("/2.0/workspaces/"+fixtureWorkspaceSlug+"/members", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"values":[{"user":{"uuid":"{33333333-3333-3333-3333-333333333333}","nickname":"alice"}}]}`))
	})
	mux.HandleFunc("/2.0/repositories/"+fixtureRepositoryFlag+"/pullrequests", func(w http.ResponseWriter, r *http.Request) {
		pullrequestRequests++
	})

	const profileName = "create-reviewer-mixed"
	cmd := setupTestNamed(t, profileName, mux.ServeHTTP, false)

	err := createProcess(cmd, nil)
	if err == nil {
		t.Fatal("createProcess() expected an error, got nil")
	}
	if !strings.Contains(err.Error(), "bobb") {
		t.Errorf("error = %q, want it to name the unresolved reviewer bobb", err.Error())
	}
	if strings.Contains(err.Error(), "alice is not a member") {
		t.Errorf("error = %q, must not also complain about the valid reviewer alice", err.Error())
	}
	if pullrequestRequests != 0 {
		t.Errorf("expected no pullrequest creation request, got %d", pullrequestRequests)
	}
}

// TestCreateProcessReviewerAllExpandsToEveryMember is a regression test for review-iter5 finding
// 3: --reviewer used to be a common.EnumSliceFlag with AllAllowed, expanding the literal value
// "all" to every workspace member's nickname at flag-parse time; switching to a plain StringSlice
// (review-iter4 finding 3) dropped that expansion entirely, turning "all" into a literal (and
// unresolvable) reviewer name. expandAllReviewers restores it at resolution time instead.
func TestCreateProcessReviewerAllExpandsToEveryMember(t *testing.T) {
	withCreateOptions(t, func() {
		createOptions.Title = "Add feature"
		createOptions.Source.Value = "feature"
		createOptions.Destination.Value = ""
		createOptions.Reviewers = []string{"all"}
	})

	var postBody PullRequestCreator
	mux := http.NewServeMux()
	mux.HandleFunc("/2.0/workspaces/"+fixtureWorkspaceSlug+"/members", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"values":[` +
			`{"user":{"uuid":"{33333333-3333-3333-3333-333333333333}","nickname":"alice"}},` +
			`{"user":{"uuid":"{44444444-4444-4444-4444-444444444444}","nickname":"bob"}}` +
			`]}`))
	})
	mux.HandleFunc("/2.0/repositories/"+fixtureRepositoryFlag+"/pullrequests", func(w http.ResponseWriter, r *http.Request) {
		if err := json.NewDecoder(r.Body).Decode(&postBody); err != nil {
			t.Errorf("cannot decode POST body: %v", err)
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"id":99,"title":"Add feature"}`))
	})

	const profileName = "create-reviewer-all"
	cmd := setupTestNamed(t, profileName, mux.ServeHTTP, false)

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

func TestCreateProcessDryRun(t *testing.T) {
	withCreateOptions(t, func() {
		createOptions.Title = "Add feature"
		createOptions.Source.Value = "feature"
		createOptions.Destination.Value = ""
		createOptions.Reviewers = nil
	})

	var pullrequestRequests int
	mux := http.NewServeMux()
	mux.HandleFunc("/2.0/user", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"uuid":"{22222222-2222-2222-2222-222222222222}","display_name":"Current User"}`))
	})
	mux.HandleFunc("/2.0/repositories/"+fixtureRepositoryFlag+"/effective-default-reviewers", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"values":[]}`))
	})
	mux.HandleFunc("/2.0/repositories/"+fixtureRepositoryFlag+"/pullrequests", func(w http.ResponseWriter, r *http.Request) {
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
