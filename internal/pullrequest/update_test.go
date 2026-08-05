package pullrequest

import (
	"encoding/json"
	"net/http"
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
