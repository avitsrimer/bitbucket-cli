package pullrequest

import (
	"encoding/json"
	"net/http"
	"os"
	"slices"
	"strings"
	"testing"

	"github.com/avitsrimer/bitbucket-cli/internal/common"
	"github.com/avitsrimer/bitbucket-cli/internal/repository"
	"github.com/avitsrimer/bitbucket-cli/internal/user"
	"github.com/avitsrimer/bitbucket-cli/internal/workspace"
	"github.com/spf13/cobra"
)

// withUpdateOptions saves/restores the package-level updateOptions (bound to updateCmd's flags
// at init) so tests can set the values they need without leaking state across other tests.
func withUpdateOptions(t *testing.T, mutate func()) {
	t.Helper()
	oldTitle, oldDescription := updateOptions.Title, updateOptions.Description
	oldDestinationValue := updateOptions.Destination.Value
	oldAddReviewers, oldRemoveReviewers := updateOptions.AddReviewers.Values, updateOptions.RemoveReviewers.Values
	oldCloseSourceBranch := updateOptions.CloseSourceBranch
	t.Cleanup(func() {
		updateOptions.Title = oldTitle
		updateOptions.Description = oldDescription
		updateOptions.Destination.Value = oldDestinationValue
		updateOptions.AddReviewers.Values = oldAddReviewers
		updateOptions.RemoveReviewers.Values = oldRemoveReviewers
		updateOptions.CloseSourceBranch = oldCloseSourceBranch
	})
	mutate()
}

// registerUpdateFlags adds the update-specific flags updateProcess/applySimpleFieldUpdates/
// removeRequestedReviewers/addRequestedReviewers check via cmd.Flag(name).Changed; setupTestNamed's
// cmd only carries the flags common to every action (profile/repository/output/dry-run).
func registerUpdateFlags(cmd *cobra.Command) {
	cmd.Flags().String("title", "", "")
	cmd.Flags().String("description", "", "")
	cmd.Flags().String("destination", "", "")
	cmd.Flags().Bool("close-source-branch", false, "")
	cmd.Flags().Bool("add-reviewer", false, "")
	cmd.Flags().Bool("remove-reviewer", false, "")
}

func TestUpdateValidArgsListsAllPullRequestIDs(t *testing.T) {
	var requests []*http.Request
	cmd := setupTestNamed(t, "update-valid-args", func(w http.ResponseWriter, r *http.Request) {
		requests = append(requests, r)
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"values":[{"id":42},{"id":7}]}`))
	}, false)

	ids, directive := updateValidArgs(cmd, nil, "")
	if directive != cobra.ShellCompDirectiveNoFileComp {
		t.Errorf("directive = %v, want %v", directive, cobra.ShellCompDirectiveNoFileComp)
	}
	if !slices.Equal(ids, []string{"42", "7"}) {
		t.Errorf("ids = %v, want [42 7]", ids)
	}
	if len(requests) != 1 || requests[0].URL.Query().Get("state") != "ALL" {
		t.Errorf("expected exactly 1 request with state=ALL, got %v", requests)
	}
}

func TestUpdateValidArgsReturnsNoCompletionsWhenArgAlreadyProvided(t *testing.T) {
	cmd := setupTestNamed(t, "update-valid-args-arg-provided", func(http.ResponseWriter, *http.Request) {
		t.Error("no HTTP request expected")
	}, false)

	ids, directive := updateValidArgs(cmd, []string{"42"}, "")
	if ids != nil {
		t.Errorf("ids = %v, want nil", ids)
	}
	if directive != cobra.ShellCompDirectiveNoFileComp {
		t.Errorf("directive = %v, want %v", directive, cobra.ShellCompDirectiveNoFileComp)
	}
}

func TestApplySimpleFieldUpdates(t *testing.T) {
	withUpdateOptions(t, func() {
		updateOptions.Title = "New title"
		updateOptions.Description = "New description"
	})

	cmd := &cobra.Command{}
	registerUpdateFlags(cmd)
	if err := cmd.Flags().Set("title", "New title"); err != nil {
		t.Fatalf("cannot set title flag: %v", err)
	}
	if err := cmd.Flags().Set("description", "New description"); err != nil {
		t.Fatalf("cannot set description flag: %v", err)
	}

	var pr PullRequest
	if changed := applySimpleFieldUpdates(cmd, &pr); !changed {
		t.Error("applySimpleFieldUpdates() = false, want true when title/description changed")
	}
	if pr.Title != "New title" {
		t.Errorf("Title = %q, want %q", pr.Title, "New title")
	}
	if pr.Description != "New description" || pr.Summary.Raw != "New description" {
		t.Errorf("Description/Summary.Raw = %q/%q, want %q", pr.Description, pr.Summary.Raw, "New description")
	}
}

func TestApplySimpleFieldUpdatesNoFlagsChanged(t *testing.T) {
	cmd := &cobra.Command{}
	registerUpdateFlags(cmd)

	var pr PullRequest
	if changed := applySimpleFieldUpdates(cmd, &pr); changed {
		t.Error("applySimpleFieldUpdates() = true, want false when no flags changed")
	}
}

func TestRemoveRequestedReviewersRemovesMatch(t *testing.T) {
	withUpdateOptions(t, func() {
		updateOptions.RemoveReviewers.Values = []string{"jdoe"}
	})
	cmd := &cobra.Command{}
	registerUpdateFlags(cmd)
	if err := cmd.Flags().Set("remove-reviewer", "true"); err != nil {
		t.Fatalf("cannot set remove-reviewer flag: %v", err)
	}

	pr := &PullRequest{Reviewers: []user.User{{Nickname: "jdoe", Name: "Jane Doe"}, {Nickname: "other"}}}
	isMember := func(member workspace.Member, id string) bool {
		return strings.EqualFold(member.User.Nickname, id)
	}

	if changed := removeRequestedReviewers(cmd, pr, isMember); !changed {
		t.Error("removeRequestedReviewers() = false, want true when a matching reviewer is removed")
	}
	if len(pr.Reviewers) != 1 || pr.Reviewers[0].Nickname != "other" {
		t.Errorf("Reviewers = %+v, want only the non-matching reviewer left", pr.Reviewers)
	}
}

func TestRemoveRequestedReviewersNoOpWhenFlagNotChanged(t *testing.T) {
	withUpdateOptions(t, func() {
		updateOptions.RemoveReviewers.Values = []string{"jdoe"}
	})
	cmd := &cobra.Command{}
	registerUpdateFlags(cmd)

	pr := &PullRequest{Reviewers: []user.User{{Nickname: "jdoe"}}}
	isMember := func(member workspace.Member, id string) bool { return strings.EqualFold(member.User.Nickname, id) }

	if changed := removeRequestedReviewers(cmd, pr, isMember); changed {
		t.Error("removeRequestedReviewers() = true, want false when --remove-reviewer was not passed")
	}
	if len(pr.Reviewers) != 1 {
		t.Errorf("Reviewers = %+v, want unchanged", pr.Reviewers)
	}
}

// TestResolveDefaultReviewersNilSourceRepository is a regression test: a pullrequest payload
// without a source repository used to panic dereferencing pullrequest.Source.Repository; it must
// now return a clear error instead.
func TestResolveDefaultReviewersNilSourceRepository(t *testing.T) {
	withUpdateOptions(t, func() {
		updateOptions.AddReviewers.Values = []string{"default"}
	})
	cmd := &cobra.Command{}
	pr := &PullRequest{}

	_, err := resolveDefaultReviewers(t.Context(), cmd, pr)
	if err == nil {
		t.Fatal("resolveDefaultReviewers() expected an error for a nil source repository, got nil")
	}
	if !strings.Contains(err.Error(), "no source repository") {
		t.Errorf("error = %q, want it to mention the missing source repository", err.Error())
	}
}

// TestResolveDefaultReviewersDoesNotMutateSharedAddReviewersValues is a regression test: this
// function used to write updateOptions.AddReviewers.Values = append(...) in place, mutating the
// package-level singleton every command invocation shares, so calling it twice (or reusing the
// singleton across tests) produced different results the second time.
func TestResolveDefaultReviewersDoesNotMutateSharedAddReviewersValues(t *testing.T) {
	withUpdateOptions(t, func() {
		updateOptions.AddReviewers.Values = []string{"default", "extra-reviewer"}
	})

	var requestCount int
	cmd := setupTestNamed(t, "resolve-default-reviewers", func(w http.ResponseWriter, r *http.Request) {
		requestCount++
		w.Header().Set("Content-Type", "application/json")
		switch {
		case strings.HasSuffix(r.URL.Path, "/effective-default-reviewers"):
			_, _ = w.Write([]byte(`{"values":[{"user":{"uuid":"{11111111-1111-1111-1111-111111111111}","display_name":"Default Reviewer"}}]}`))
		case strings.HasSuffix(r.URL.Path, "/user"):
			w.WriteHeader(http.StatusForbidden) // simulate a RAT client without /user access
		}
	}, false)

	id, err := common.ParseUUID("{22222222-2222-2222-2222-222222222222}")
	if err != nil {
		t.Fatalf("cannot parse fixture uuid: %v", err)
	}
	pr := &PullRequest{Source: Endpoint{Repository: &repository.Repository{
		ID: id, Name: "Widgets", FullName: fixtureRepositoryFlag, Slug: fixtureRepositorySlug,
		Workspace: &workspace.Workspace{Slug: fixtureWorkspaceSlug},
	}}}

	before := append([]string(nil), updateOptions.AddReviewers.Values...)

	resolved1, err := resolveDefaultReviewers(t.Context(), cmd, pr)
	if err != nil {
		t.Fatalf("resolveDefaultReviewers() error = %v", err)
	}
	if !slices.Equal(updateOptions.AddReviewers.Values, before) {
		t.Errorf("updateOptions.AddReviewers.Values = %v after call, want unchanged %v", updateOptions.AddReviewers.Values, before)
	}

	resolved2, err := resolveDefaultReviewers(t.Context(), cmd, pr)
	if err != nil {
		t.Fatalf("resolveDefaultReviewers() second call error = %v", err)
	}
	if !slices.Equal(resolved1, resolved2) {
		t.Errorf("resolveDefaultReviewers() is not idempotent: first = %v, second = %v", resolved1, resolved2)
	}
	want := []string{"{11111111-1111-1111-1111-111111111111}", "extra-reviewer"}
	if !slices.Equal(resolved1, want) {
		t.Errorf("resolved reviewers = %v, want %v", resolved1, want)
	}
}

// TestResolveDefaultReviewersRealFixtureSourceRepositoryHasNoWorkspace is a regression test: a
// pullrequest's source.repository, as BitBucket actually sends it (see testdata/pullrequest.json),
// never carries a "workspace" field. GetEffectiveDefaultReviewers used to dereference
// repository.Workspace.Slug directly and nil-panic for exactly this shape; it must now resolve the
// workspace through Repository.GetWorkspace's cached/FullName fallback chain instead.
func TestResolveDefaultReviewersRealFixtureSourceRepositoryHasNoWorkspace(t *testing.T) {
	withUpdateOptions(t, func() {
		updateOptions.AddReviewers.Values = []string{"default"}
	})

	data, err := os.ReadFile("../../testdata/pullrequest.json")
	if err != nil {
		t.Fatalf("cannot read testdata: %v", err)
	}
	var pr PullRequest
	if unmarshalErr := json.Unmarshal(data, &pr); unmarshalErr != nil {
		t.Fatalf("cannot unmarshal testdata: %v", unmarshalErr)
	}
	if pr.Source.Repository == nil {
		t.Fatal("fixture source.repository is nil; fixture no longer matches this test's premise")
	}
	if pr.Source.Repository.Workspace != nil {
		t.Fatal("fixture source.repository now carries a workspace; this test's premise (no workspace) no longer holds")
	}

	const workspaceSlug = "gildas_cherruel"
	if cacheErr := workspace.WorkspaceCache.Set(workspaceSlug, workspace.Workspace{Slug: workspaceSlug}); cacheErr != nil {
		t.Fatalf("cannot prime workspace cache: %v", cacheErr)
	}

	var requests []*http.Request
	cmd := setupTestNamed(t, "resolve-default-reviewers-fixture", func(w http.ResponseWriter, r *http.Request) {
		requests = append(requests, r)
		w.Header().Set("Content-Type", "application/json")
		switch {
		case strings.HasSuffix(r.URL.Path, "/effective-default-reviewers"):
			_, _ = w.Write([]byte(`{"values":[{"user":{"uuid":"{11111111-1111-1111-1111-111111111111}","display_name":"Default Reviewer"}}]}`))
		case strings.HasSuffix(r.URL.Path, "/user"):
			w.WriteHeader(http.StatusForbidden) // simulate a RAT client without /user access
		}
	}, false)

	resolved, err := resolveDefaultReviewers(t.Context(), cmd, &pr)
	if err != nil {
		t.Fatalf("resolveDefaultReviewers() error = %v", err)
	}
	want := []string{"{11111111-1111-1111-1111-111111111111}"}
	if !slices.Equal(resolved, want) {
		t.Errorf("resolved reviewers = %v, want %v", resolved, want)
	}

	wantPath := "/2.0/repositories/gildas_cherruel/gitflow-pr-sandbox/effective-default-reviewers"
	var found bool
	for _, req := range requests {
		if req.URL.Path == wantPath {
			found = true
		}
	}
	if !found {
		t.Errorf("requests = %v, want one to %s", requests, wantPath)
	}
}

// TestResolveDefaultReviewersUsesFullNameWhenSlugWasBackfilledFromName is a regression test: the
// two existing "resolve default reviewers" fixtures above both happen to have Name == Slug (the
// literal fixture in the first, and testdata/pullrequest.json's "gitflow-pr-sandbox" repository
// in the second), and the second also primes WorkspaceCache for "gildas_cherruel" -- so neither
// would have caught building the effective-default-reviewers path from repository.Slug once
// Validate backfills it from Name (BitBucket omits "slug" on a pullrequest's source/destination
// repository). This fixture uses a Name that differs from the repository's real slug and leaves
// WorkspaceCache empty for its workspace, so a regression that falls back to a live GetWorkspace
// call would either 403 (this test's server rejects any /workspaces/ request, simulating a RAT
// client) or build the wrong path from repository.Slug ("My Repo") instead of FullName's
// "my-repo-slug".
func TestResolveDefaultReviewersUsesFullNameWhenSlugWasBackfilledFromName(t *testing.T) {
	withUpdateOptions(t, func() {
		updateOptions.AddReviewers.Values = []string{"default"}
	})

	id, err := common.ParseUUID("{33333333-3333-3333-3333-333333333333}")
	if err != nil {
		t.Fatalf("cannot parse fixture uuid: %v", err)
	}
	repo := &repository.Repository{
		ID:       id,
		Name:     "My Repo",
		FullName: "other-workspace/my-repo-slug",
		// Slug is set the way Repository.Validate backfills it when BitBucket's response omits
		// "slug" (as it does for a pullrequest's source/destination repository): equal to Name,
		// not the real slug FullName carries.
		Slug: "My Repo",
	}

	var requests []*http.Request
	cmd := setupTestNamed(t, "resolve-default-reviewers-name-neq-slug", func(w http.ResponseWriter, r *http.Request) {
		requests = append(requests, r)
		w.Header().Set("Content-Type", "application/json")
		switch {
		case strings.HasSuffix(r.URL.Path, "/effective-default-reviewers"):
			_, _ = w.Write([]byte(`{"values":[{"user":{"uuid":"{11111111-1111-1111-1111-111111111111}","display_name":"Default Reviewer"}}]}`))
		case strings.HasSuffix(r.URL.Path, "/user"):
			w.WriteHeader(http.StatusForbidden) // simulate a RAT client without /user access
		case strings.Contains(r.URL.Path, "/workspaces/"):
			// A live workspace lookup here means the fix fell back to GetWorkspace instead of
			// FullName; RAT/repo-scoped tokens typically cannot reach this endpoint.
			w.WriteHeader(http.StatusForbidden)
		}
	}, false)

	pr := &PullRequest{Source: Endpoint{Repository: repo}}

	resolved, err := resolveDefaultReviewers(t.Context(), cmd, pr)
	if err != nil {
		t.Fatalf("resolveDefaultReviewers() error = %v", err)
	}
	want := []string{"{11111111-1111-1111-1111-111111111111}"}
	if !slices.Equal(resolved, want) {
		t.Errorf("resolved reviewers = %v, want %v", resolved, want)
	}

	wantPath := "/2.0/repositories/other-workspace/my-repo-slug/effective-default-reviewers"
	var found bool
	for _, req := range requests {
		if req.URL.Path == wantPath {
			found = true
		}
		if strings.Contains(req.URL.Path, "/workspaces/") {
			t.Errorf("unexpected live workspace lookup: %s", req.URL.Path)
		}
	}
	if !found {
		t.Errorf("requests = %v, want one to %s", requests, wantPath)
	}
}

func TestUpdateProcessSimpleFieldsSuccess(t *testing.T) {
	withUpdateOptions(t, func() {
		updateOptions.Title = "Updated title"
	})

	var requests []*http.Request
	var putBody PullRequest
	cmd := setupTestNamed(t, "update-simple-fields", func(w http.ResponseWriter, r *http.Request) {
		requests = append(requests, r)
		w.Header().Set("Content-Type", "application/json")
		switch r.Method {
		case http.MethodGet:
			_, _ = w.Write([]byte(`{"id":42,"title":"Old title"}`))
		case http.MethodPut:
			if err := json.NewDecoder(r.Body).Decode(&putBody); err != nil {
				t.Errorf("cannot decode PUT body: %v", err)
			}
			_, _ = w.Write([]byte(`{"id":42,"title":"Updated title"}`))
		}
	}, false)
	registerUpdateFlags(cmd)
	if err := cmd.Flags().Set("title", "Updated title"); err != nil {
		t.Fatalf("cannot set title flag: %v", err)
	}

	stdout := captureStdout(t, func() {
		if err := updateProcess(cmd, []string{"42"}); err != nil {
			t.Fatalf("updateProcess() error = %v", err)
		}
	})

	if len(requests) != 2 {
		t.Fatalf("expected exactly 2 requests (get, put), got %d", len(requests))
	}
	if requests[0].Method != http.MethodGet || requests[1].Method != http.MethodPut {
		t.Errorf("methods = %s, %s, want GET, PUT", requests[0].Method, requests[1].Method)
	}
	if putBody.Title != "Updated title" {
		t.Errorf("PUT body title = %q, want %q", putBody.Title, "Updated title")
	}
	var printed PullRequest
	if err := json.Unmarshal([]byte(stdout), &printed); err != nil {
		t.Fatalf("cannot unmarshal printed output %q: %v", stdout, err)
	}
	if printed.Title != "Updated title" {
		t.Errorf("printed title = %q, want %q", printed.Title, "Updated title")
	}
}

func TestUpdateProcessNoChangesSkipsPut(t *testing.T) {
	var requests []*http.Request
	cmd := setupTestNamed(t, "update-no-changes", func(w http.ResponseWriter, r *http.Request) {
		requests = append(requests, r)
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"id":42,"title":"Unchanged"}`))
	}, false)
	registerUpdateFlags(cmd)

	if err := updateProcess(cmd, []string{"42"}); err != nil {
		t.Fatalf("updateProcess() error = %v", err)
	}
	if len(requests) != 1 || requests[0].Method != http.MethodGet {
		t.Errorf("requests = %v, want exactly one GET and no PUT when nothing changed", requests)
	}
}

func TestUpdateProcessGetAPIError(t *testing.T) {
	cmd := setupTestNamed(t, "update-get-error", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusNotFound)
		_, _ = w.Write([]byte(`{"type":"error","error":{"message":"pull request not found"}}`))
	}, false)
	registerUpdateFlags(cmd)

	err := updateProcess(cmd, []string{"42"})
	if err == nil {
		t.Fatal("updateProcess() expected an error, got nil")
	}
	if !strings.Contains(err.Error(), "failed to get pullrequest") {
		t.Errorf("error = %q, want it to mention the failed get", err.Error())
	}
}

func TestUpdateProcessPutAPIError(t *testing.T) {
	withUpdateOptions(t, func() {
		updateOptions.Title = "Updated title"
	})

	cmd := setupTestNamed(t, "update-put-error", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		switch r.Method {
		case http.MethodGet:
			_, _ = w.Write([]byte(`{"id":42,"title":"Old title"}`))
		case http.MethodPut:
			w.WriteHeader(http.StatusBadRequest)
			_, _ = w.Write([]byte(`{"type":"error","error":{"message":"pull request is not open"}}`))
		}
	}, false)
	registerUpdateFlags(cmd)
	if err := cmd.Flags().Set("title", "Updated title"); err != nil {
		t.Fatalf("cannot set title flag: %v", err)
	}

	err := updateProcess(cmd, []string{"42"})
	if err == nil {
		t.Fatal("updateProcess() expected an error, got nil")
	}
	if !strings.Contains(err.Error(), "failed to update pullrequest") {
		t.Errorf("error = %q, want it to mention the failed update", err.Error())
	}
	if !strings.Contains(err.Error(), "pull request is not open") {
		t.Errorf("error = %q, want it to contain the BitBucket error message", err.Error())
	}
}

func TestUpdateProcessDryRun(t *testing.T) {
	withUpdateOptions(t, func() {
		updateOptions.Title = "Updated title"
	})

	var requests []*http.Request
	cmd := setupTestNamed(t, "update-dry-run", func(w http.ResponseWriter, r *http.Request) {
		requests = append(requests, r)
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"id":42,"title":"Old title"}`))
	}, true)
	registerUpdateFlags(cmd)
	if err := cmd.Flags().Set("title", "Updated title"); err != nil {
		t.Fatalf("cannot set title flag: %v", err)
	}

	if err := updateProcess(cmd, []string{"42"}); err != nil {
		t.Fatalf("updateProcess() error = %v", err)
	}
	if len(requests) != 1 || requests[0].Method != http.MethodGet {
		t.Errorf("requests = %v, want exactly one GET and no PUT in dry-run mode", requests)
	}
}
