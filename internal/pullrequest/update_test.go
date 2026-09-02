package pullrequest

import (
	"encoding/json"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	"github.com/avitsrimer/bitbucket-cli/internal/common"
	"github.com/avitsrimer/bitbucket-cli/internal/profile"
	"github.com/avitsrimer/bitbucket-cli/internal/repository"
	"github.com/avitsrimer/bitbucket-cli/internal/testutil"
	"github.com/avitsrimer/bitbucket-cli/internal/user"
	"github.com/avitsrimer/bitbucket-cli/internal/workspace"
	"github.com/spf13/cobra"
)

// withUpdateOptions saves/restores the package-level updateOptions (bound to updateCmd's flags
// at init) so tests can set the values they need without leaking state across other tests.
func withUpdateOptions(t *testing.T, mutate func()) {
	t.Helper()
	oldTitle, oldDescription, oldDescriptionFile := updateOptions.Title, updateOptions.Description, updateOptions.DescriptionFile
	oldDestinationValue := updateOptions.Destination.Value
	oldAddReviewers, oldRemoveReviewers := updateOptions.AddReviewers, updateOptions.RemoveReviewers
	oldCloseSourceBranch := updateOptions.CloseSourceBranch
	t.Cleanup(func() {
		updateOptions.Title = oldTitle
		updateOptions.Description = oldDescription
		updateOptions.DescriptionFile = oldDescriptionFile
		updateOptions.Destination.Value = oldDestinationValue
		updateOptions.AddReviewers = oldAddReviewers
		updateOptions.RemoveReviewers = oldRemoveReviewers
		updateOptions.CloseSourceBranch = oldCloseSourceBranch
	})
	mutate()
}

// registerUpdateFlags adds the update-specific flags updateProcess/applySimpleFieldUpdates/
// removeRequestedReviewers/addRequestedReviewers check via cmd.Flag(name).Changed; setupTestNamed's
// cmd only carries the flags common to every action (profile/repository/output/dry-run).
//
// add-reviewer/remove-reviewer are registered as real string slices (their production type) with
// storage local to this test command, not bound to the package-level
// updateOptions: tests that exercise removeRequestedReviewers/addRequestedReviewers still set
// updateOptions.AddReviewers/RemoveReviewers directly and use these flags only to mark
// cmd.Flag(name).Changed the way real flag parsing would.
func registerUpdateFlags(cmd *cobra.Command) {
	cmd.Flags().String("title", "", "")
	cmd.Flags().String("description", "", "")
	cmd.Flags().String("description-file", "", "")
	cmd.Flags().String("destination", "", "")
	cmd.Flags().Bool("close-source-branch", false, "")
	cmd.Flags().StringSlice("add-reviewer", nil, "")
	cmd.Flags().StringSlice("remove-reviewer", nil, "")
	// --ready/--draft go through the real registration: applySimpleFieldUpdates reads them off
	// cmd, and cmd.Flag(name) is nil (so .Changed panics) for any flag not registered here.
	registerDraftStateFlags(cmd)
	// Mirrors RootCmd's persistent stop-on-error/warn-on-error/ignore-errors flags (see
	// internal/cmd/root.go): Profile.ShouldStopOnError/ShouldWarnOnError/ShouldIgnoreErrors read
	// them via cmd.Flag(name).Changed unconditionally. Guarded against double-registration since
	// some callers pass a cmd that setupTestNamed already equipped with these same flags.
	if cmd.Flags().Lookup("stop-on-error") == nil {
		cmd.Flags().Bool("stop-on-error", false, "")
		cmd.Flags().Bool("warn-on-error", false, "")
		cmd.Flags().Bool("ignore-errors", false, "")
	}
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
	tests := []struct {
		name        string
		flags       map[string]string
		initial     PullRequest
		wantChanged bool
		want        PullRequest
	}{
		{
			name:        "title and description changed",
			flags:       map[string]string{"title": "New title", "description": "New description"},
			wantChanged: true,
			want:        PullRequest{Title: "New title", Description: "New description", Summary: common.RenderedText{Raw: "New description"}},
		},
		{
			name:        "no flags changed",
			initial:     PullRequest{Title: "Old title", Draft: true},
			wantChanged: false,
			want:        PullRequest{Title: "Old title", Draft: true},
		},
		{
			name:        "--ready on a draft clears Draft",
			flags:       map[string]string{"ready": "true"},
			initial:     PullRequest{Title: "T", Draft: true},
			wantChanged: true,
			want:        PullRequest{Title: "T", Draft: false},
		},
		{
			name:        "--draft on a non-draft sets Draft",
			flags:       map[string]string{"draft": "true"},
			initial:     PullRequest{Title: "T", Draft: false},
			wantChanged: true,
			want:        PullRequest{Title: "T", Draft: true},
		},
		{
			name:        "--ready alone on an already-ready pullrequest still reports a change",
			flags:       map[string]string{"ready": "true"},
			initial:     PullRequest{Title: "T", Draft: false},
			wantChanged: true,
			want:        PullRequest{Title: "T", Draft: false},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			withUpdateOptions(t, func() {
				updateOptions.Title = "New title"
				updateOptions.Description = "New description"
			})

			cmd := &cobra.Command{}
			registerUpdateFlags(cmd)
			for name, value := range tt.flags {
				if err := cmd.Flags().Set(name, value); err != nil {
					t.Fatalf("cannot set %s flag: %v", name, err)
				}
			}

			pr := tt.initial
			changed, err := applySimpleFieldUpdates(cmd, &pr)
			if err != nil {
				t.Fatalf("applySimpleFieldUpdates() error = %v", err)
			}
			if changed != tt.wantChanged {
				t.Errorf("applySimpleFieldUpdates() = %v, want %v", changed, tt.wantChanged)
			}
			if pr.Title != tt.want.Title {
				t.Errorf("Title = %q, want %q", pr.Title, tt.want.Title)
			}
			if pr.Description != tt.want.Description || pr.Summary.Raw != tt.want.Summary.Raw {
				t.Errorf("Description/Summary.Raw = %q/%q, want %q/%q", pr.Description, pr.Summary.Raw, tt.want.Description, tt.want.Summary.Raw)
			}
			if pr.Draft != tt.want.Draft {
				t.Errorf("Draft = %v, want %v", pr.Draft, tt.want.Draft)
			}
		})
	}
}

func TestRemoveRequestedReviewers(t *testing.T) {
	tests := []struct {
		name           string
		setFlag        bool
		reviewers      []user.User
		wantChanged    bool
		wantNicknames  []string
		wantSameLength int
	}{
		{
			name:          "removes a matching reviewer",
			setFlag:       true,
			reviewers:     []user.User{{Nickname: "jdoe", Name: "Jane Doe"}, {Nickname: "other"}},
			wantChanged:   true,
			wantNicknames: []string{"other"},
		},
		{
			name:           "no-op when --remove-reviewer was not passed",
			setFlag:        false,
			reviewers:      []user.User{{Nickname: "jdoe"}},
			wantChanged:    false,
			wantSameLength: 1,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			withUpdateOptions(t, func() {
				updateOptions.RemoveReviewers = []string{"jdoe"}
			})
			cmd := &cobra.Command{}
			registerUpdateFlags(cmd)
			if tt.setFlag {
				if err := cmd.Flags().Set("remove-reviewer", "true"); err != nil {
					t.Fatalf("cannot set remove-reviewer flag: %v", err)
				}
			}

			pr := &PullRequest{Reviewers: tt.reviewers}

			changed, err := removeRequestedReviewers(cmd.Context(), cmd, &profile.Profile{}, pr)
			if err != nil {
				t.Fatalf("removeRequestedReviewers() error = %v", err)
			}
			if changed != tt.wantChanged {
				t.Errorf("removeRequestedReviewers() = %v, want %v", changed, tt.wantChanged)
			}
			if tt.wantNicknames != nil {
				if len(pr.Reviewers) != len(tt.wantNicknames) {
					t.Fatalf("Reviewers = %+v, want %v", pr.Reviewers, tt.wantNicknames)
				}
				for i, nickname := range tt.wantNicknames {
					if pr.Reviewers[i].Nickname != nickname {
						t.Errorf("Reviewers[%d].Nickname = %q, want %q", i, pr.Reviewers[i].Nickname, nickname)
					}
				}
			}
			if tt.wantSameLength != 0 && len(pr.Reviewers) != tt.wantSameLength {
				t.Errorf("Reviewers = %+v, want unchanged (len %d)", pr.Reviewers, tt.wantSameLength)
			}
		})
	}
}

// TestResolveDefaultReviewersNilSourceRepository verifies that a pullrequest payload without a
// source repository returns a clear error instead of panicking.
func TestResolveDefaultReviewersNilSourceRepository(t *testing.T) {
	withUpdateOptions(t, func() {
		updateOptions.AddReviewers = []string{"default"}
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

// TestResolveDefaultReviewersDoesNotMutateSharedAddReviewersValues verifies that
// resolveDefaultReviewers never writes to updateOptions.AddReviewers, the package-level singleton
// every command invocation shares, so calling it repeatedly is idempotent.
func TestResolveDefaultReviewersDoesNotMutateSharedAddReviewersValues(t *testing.T) {
	withUpdateOptions(t, func() {
		updateOptions.AddReviewers = []string{"default", "extra-reviewer"}
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
		ID: id, Name: "Widgets", FullName: testutil.FixtureRepositoryFlag, Slug: testutil.FixtureRepositorySlug,
		Workspace: &workspace.Workspace{Slug: testutil.FixtureWorkspaceSlug},
	}}}

	before := append([]string(nil), updateOptions.AddReviewers...)

	resolved1, err := resolveDefaultReviewers(t.Context(), cmd, pr)
	if err != nil {
		t.Fatalf("resolveDefaultReviewers() error = %v", err)
	}
	if !slices.Equal(updateOptions.AddReviewers, before) {
		t.Errorf("updateOptions.AddReviewers = %v after call, want unchanged %v", updateOptions.AddReviewers, before)
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

// TestResolveDefaultReviewersRealFixtureSourceRepositoryHasNoWorkspace verifies that a
// pullrequest's source.repository, as BitBucket actually sends it (see testdata/pullrequest.json)
// with no "workspace" field, still resolves its effective default reviewers by going through
// Repository.GetWorkspaceSlug's cached/FullName fallback chain.
func TestResolveDefaultReviewersRealFixtureSourceRepositoryHasNoWorkspace(t *testing.T) {
	withUpdateOptions(t, func() {
		updateOptions.AddReviewers = []string{"default"}
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

// TestResolveDefaultReviewersUsesFullNameWhenSlugWasBackfilledFromName verifies that the
// effective-default-reviewers path is built from the repository's FullName, not its Slug: this
// fixture's Name differs from the repository's real slug (Validate backfills Slug from Name when
// BitBucket omits "slug" on a pullrequest's source/destination repository) and leaves
// WorkspaceCache empty for its workspace, so building the path from Slug instead of FullName would
// hit the wrong path; the server's 403 on any /workspaces/ request additionally guards against a
// regression that reintroduces a live workspace fetch anywhere in that resolution.
func TestResolveDefaultReviewersUsesFullNameWhenSlugWasBackfilledFromName(t *testing.T) {
	withUpdateOptions(t, func() {
		updateOptions.AddReviewers = []string{"default"}
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

	stdout := testutil.CaptureStdout(t, func() {
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

// runUpdateDraftState drives updateProcess against an httptest server whose GET returns getBody,
// with the given flags set on the command, and returns the requests seen, the decoded PUT body
// (nil when no PUT was sent) and the captured stdout. The PUT handler echoes the PUT body back as
// the response so the printed result reflects exactly what was sent.
func runUpdateDraftState(t *testing.T, profileName, getBody string, flags map[string]string, dryRun bool) (requests []*http.Request, putBody map[string]any, stdout, stderr string) {
	t.Helper()
	cmd := setupTestNamed(t, profileName, func(w http.ResponseWriter, r *http.Request) {
		requests = append(requests, r)
		w.Header().Set("Content-Type", "application/json")
		switch r.Method {
		case http.MethodGet:
			_, _ = w.Write([]byte(getBody))
		case http.MethodPut:
			raw, err := io.ReadAll(r.Body)
			if err != nil {
				t.Errorf("cannot read PUT body: %v", err)
			}
			if err := json.Unmarshal(raw, &putBody); err != nil {
				t.Errorf("cannot decode PUT body %s: %v", raw, err)
			}
			_, _ = w.Write(raw)
		}
	}, dryRun)
	registerUpdateFlags(cmd)
	for name, value := range flags {
		if err := cmd.Flags().Set(name, value); err != nil {
			t.Fatalf("cannot set %s flag: %v", name, err)
		}
	}

	stderr = testutil.CaptureStderr(t, func() {
		stdout = testutil.CaptureStdout(t, func() {
			if err := updateProcess(cmd, []string{"42"}); err != nil {
				t.Errorf("updateProcess() error = %v", err)
			}
		})
	})
	return requests, putBody, stdout, stderr
}

// assertGetThenPut fails unless requests is exactly a GET followed by a PUT.
func assertGetThenPut(t *testing.T, requests []*http.Request) {
	t.Helper()
	if len(requests) != 2 {
		t.Fatalf("expected exactly 2 requests (get, put), got %d", len(requests))
	}
	if requests[0].Method != http.MethodGet || requests[1].Method != http.MethodPut {
		t.Errorf("methods = %s, %s, want GET, PUT", requests[0].Method, requests[1].Method)
	}
}

// TestUpdateProcessReadyClearsDraftInPutBody verifies --ready on a draft pullrequest sends one
// PUT whose body carries "draft": false -- the key present and false, never omitted -- and that
// the printed result round-trips the same state.
func TestUpdateProcessReadyClearsDraftInPutBody(t *testing.T) {
	withUpdateOptions(t, func() {})

	requests, putBody, stdout, _ := runUpdateDraftState(t, "update-ready", `{"id":42,"title":"T","draft":true}`, map[string]string{"ready": "true"}, false)

	assertGetThenPut(t, requests)
	draft, ok := putBody["draft"]
	if !ok {
		t.Fatalf("PUT body = %v, want a draft key", putBody)
	}
	if draft != false {
		t.Errorf("PUT body draft = %v, want false", draft)
	}
	if putBody["title"] != "T" {
		t.Errorf("PUT body title = %v, want %q (untouched fields are echoed back)", putBody["title"], "T")
	}
	var printed map[string]any
	if err := json.Unmarshal([]byte(stdout), &printed); err != nil {
		t.Fatalf("cannot unmarshal printed output %q: %v", stdout, err)
	}
	if printed["draft"] != false {
		t.Errorf("printed draft = %v, want false", printed["draft"])
	}
}

// TestUpdateProcessDraftSetsDraftInPutBody is the symmetric case: --draft on a non-draft
// pullrequest sends one PUT with "draft": true.
func TestUpdateProcessDraftSetsDraftInPutBody(t *testing.T) {
	withUpdateOptions(t, func() {})

	requests, putBody, stdout, _ := runUpdateDraftState(t, "update-draft", `{"id":42,"title":"T","draft":false}`, map[string]string{"draft": "true"}, false)

	assertGetThenPut(t, requests)
	if putBody["draft"] != true {
		t.Errorf("PUT body draft = %v, want true", putBody["draft"])
	}
	var printed map[string]any
	if err := json.Unmarshal([]byte(stdout), &printed); err != nil {
		t.Fatalf("cannot unmarshal printed output %q: %v", stdout, err)
	}
	if printed["draft"] != true {
		t.Errorf("printed draft = %v, want true", printed["draft"])
	}
}

// TestUpdateProcessReadyCombinesWithTitle proves --ready and --title land in the same, single
// PUT: the body carries both "draft": false and the new title.
func TestUpdateProcessReadyCombinesWithTitle(t *testing.T) {
	withUpdateOptions(t, func() {
		updateOptions.Title = "New"
	})

	requests, putBody, _, _ := runUpdateDraftState(t, "update-ready-title", `{"id":42,"title":"Old","draft":true}`, map[string]string{"ready": "true", "title": "New"}, false)

	assertGetThenPut(t, requests)
	if putBody["draft"] != false {
		t.Errorf("PUT body draft = %v, want false", putBody["draft"])
	}
	if putBody["title"] != "New" {
		t.Errorf("PUT body title = %v, want %q", putBody["title"], "New")
	}
}

// TestUpdateProcessUntouchedDraftIsEchoedUnchanged verifies an update that does not pass
// --ready/--draft echoes the GET's draft state back verbatim: a --title-only update never
// promotes a draft by accident.
func TestUpdateProcessUntouchedDraftIsEchoedUnchanged(t *testing.T) {
	withUpdateOptions(t, func() {
		updateOptions.Title = "New"
	})

	requests, putBody, _, _ := runUpdateDraftState(t, "update-untouched-draft", `{"id":42,"title":"Old","draft":true}`, map[string]string{"title": "New"}, false)

	assertGetThenPut(t, requests)
	if putBody["draft"] != true {
		t.Errorf("PUT body draft = %v, want true (untouched draft state must be echoed unchanged)", putBody["draft"])
	}
	if putBody["title"] != "New" {
		t.Errorf("PUT body title = %v, want %q", putBody["title"], "New")
	}
}

// TestUpdateProcessReadyDryRun verifies --dry-run with --ready still performs the resolving GET,
// sends no PUT, and echoes the resolved payload -- including "draft": false -- to stderr.
func TestUpdateProcessReadyDryRun(t *testing.T) {
	withUpdateOptions(t, func() {})

	requests, putBody, _, stderr := runUpdateDraftState(t, "update-ready-dry-run", `{"id":42,"title":"T","draft":true}`, map[string]string{"ready": "true"}, true)

	if len(requests) != 1 || requests[0].Method != http.MethodGet {
		t.Errorf("requests = %v, want exactly one GET and no PUT in dry-run mode", requests)
	}
	if putBody != nil {
		t.Errorf("PUT body = %v, want no PUT in dry-run mode", putBody)
	}
	if !strings.Contains(stderr, `"draft": false`) {
		t.Errorf("stderr = %q, want the dry-run payload echo to contain %q", stderr, `"draft": false`)
	}
}

// TestUpdateProcessReadyCombinesWithSimpleFields proves --ready lands in the same, single PUT as
// the remaining simple-field flags: description, destination and close-source-branch all arrive in
// one body next to "draft": false, so a promotion never needs a second request.
func TestUpdateProcessReadyCombinesWithSimpleFields(t *testing.T) {
	withUpdateOptions(t, func() {
		updateOptions.Description = "New description"
		updateOptions.Destination.Value = "main"
		updateOptions.CloseSourceBranch = true
	})

	requests, putBody, _, _ := runUpdateDraftState(t, "update-ready-simple-fields",
		`{"id":42,"title":"T","draft":true,"close_source_branch":false,"destination":{"branch":{"name":"develop"}}}`,
		map[string]string{"ready": "true", "description": "New description", "destination": "main", "close-source-branch": "true"}, false)

	assertGetThenPut(t, requests)
	if putBody["draft"] != false {
		t.Errorf("PUT body draft = %v, want false", putBody["draft"])
	}
	if putBody["description"] != "New description" {
		t.Errorf("PUT body description = %v, want %q", putBody["description"], "New description")
	}
	if putBody["close_source_branch"] != true {
		t.Errorf("PUT body close_source_branch = %v, want true", putBody["close_source_branch"])
	}
	destination, _ := putBody["destination"].(map[string]any)
	branch, _ := destination["branch"].(map[string]any)
	if branch["name"] != "main" {
		t.Errorf("PUT body destination.branch.name = %v, want %q (full destination: %v)", branch["name"], "main", putBody["destination"])
	}
}

// TestUpdateProcessReadyNonexistentPullRequest proves a --ready update of a pullrequest that does
// not exist fails the same way with and without --dry-run: the resolving GET runs first in both
// modes, its 404 surfaces as the same "failed to get pullrequest" error, and no PUT is ever sent --
// a dry run never fabricates a success for a target the real invocation could not find.
func TestUpdateProcessReadyNonexistentPullRequest(t *testing.T) {
	tests := []struct {
		name   string
		dryRun bool
	}{
		{name: "real", dryRun: false},
		{name: "dry-run", dryRun: true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var requests []*http.Request
			cmd := setupTestNamed(t, "update-ready-missing-"+tt.name, func(w http.ResponseWriter, r *http.Request) {
				requests = append(requests, r)
				w.Header().Set("Content-Type", "application/json")
				w.WriteHeader(http.StatusNotFound)
				_, _ = w.Write([]byte(`{"type":"error","error":{"message":"pull request not found"}}`))
			}, tt.dryRun)
			registerUpdateFlags(cmd)
			if err := cmd.Flags().Set("ready", "true"); err != nil {
				t.Fatalf("cannot set ready flag: %v", err)
			}

			err := updateProcess(cmd, []string{"42"})
			if err == nil {
				t.Fatal("updateProcess() expected an error for a nonexistent pullrequest, got nil")
			}
			for _, want := range []string{"failed to get pullrequest 42", "pull request not found"} {
				if !strings.Contains(err.Error(), want) {
					t.Errorf("error = %q, want it to contain %q", err.Error(), want)
				}
			}
			if len(requests) != 1 || requests[0].Method != http.MethodGet {
				t.Errorf("requests = %v, want exactly one GET and no PUT for a nonexistent pullrequest", requests)
			}
		})
	}
}

// TestUpdateProcessStripsParticipantsFromPutBody proves the Participants field the initial GET
// populated (a server-owned, read-only record of each reviewer's approval state) is never echoed
// back in the PUT payload, mirroring the existing Summary.Type/Markup/HTML stripping.
func TestUpdateProcessStripsParticipantsFromPutBody(t *testing.T) {
	withUpdateOptions(t, func() {
		updateOptions.Title = "Updated title"
	})

	var putBodyRaw []byte
	cmd := setupTestNamed(t, "update-strips-participants", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		switch r.Method {
		case http.MethodGet:
			_, _ = w.Write([]byte(`{"id":42,"title":"Old title","participants":[{"role":"REVIEWER","approved":true,"state":"approved","user":{"display_name":"Ada Lovelace"}}]}`))
		case http.MethodPut:
			var err error
			putBodyRaw, err = io.ReadAll(r.Body)
			if err != nil {
				t.Errorf("cannot read PUT body: %v", err)
			}
			_, _ = w.Write([]byte(`{"id":42,"title":"Updated title"}`))
		}
	}, false)
	registerUpdateFlags(cmd)
	if err := cmd.Flags().Set("title", "Updated title"); err != nil {
		t.Fatalf("cannot set title flag: %v", err)
	}

	testutil.CaptureStdout(t, func() {
		if err := updateProcess(cmd, []string{"42"}); err != nil {
			t.Fatalf("updateProcess() error = %v", err)
		}
	})

	var putBody PullRequest
	if err := json.Unmarshal(putBodyRaw, &putBody); err != nil {
		t.Fatalf("cannot unmarshal PUT body: %v", err)
	}
	if len(putBody.Participants) != 0 {
		t.Errorf("PUT body participants = %+v, want none (server-owned, read-only)", putBody.Participants)
	}
	if strings.Contains(string(putBodyRaw), "Ada Lovelace") {
		t.Errorf("PUT body = %s, must not echo back the participants the GET populated", putBodyRaw)
	}
}

// TestUpdateProcessDescriptionFileSuccess verifies that --description-file's content lands in
// the PUT body verbatim, including backticks and $(), and that setting only --description-file
// (not --description) still marks the update as wanted.
func TestUpdateProcessDescriptionFileSuccess(t *testing.T) {
	body := "Now also fixes `go vet` warnings; verified with $(go build ./...).\n"
	path := filepath.Join(t.TempDir(), "description.md")
	if err := os.WriteFile(path, []byte(body), 0o600); err != nil {
		t.Fatalf("cannot write fixture file: %v", err)
	}

	withUpdateOptions(t, func() {
		updateOptions.DescriptionFile = path
	})

	var requests []*http.Request
	var putBody PullRequest
	cmd := setupTestNamed(t, "update-description-file", func(w http.ResponseWriter, r *http.Request) {
		requests = append(requests, r)
		w.Header().Set("Content-Type", "application/json")
		switch r.Method {
		case http.MethodGet:
			_, _ = w.Write([]byte(`{"id":42,"title":"Old title"}`))
		case http.MethodPut:
			if err := json.NewDecoder(r.Body).Decode(&putBody); err != nil {
				t.Errorf("cannot decode PUT body: %v", err)
			}
			_, _ = w.Write([]byte(`{"id":42,"title":"Old title"}`))
		}
	}, false)
	registerUpdateFlags(cmd)
	if err := cmd.Flags().Set("description-file", path); err != nil {
		t.Fatalf("cannot set description-file flag: %v", err)
	}

	testutil.CaptureStdout(t, func() {
		if err := updateProcess(cmd, []string{"42"}); err != nil {
			t.Fatalf("updateProcess() error = %v", err)
		}
	})

	if len(requests) != 2 || requests[1].Method != http.MethodPut {
		t.Fatalf("requests = %v, want exactly a GET followed by a PUT", requests)
	}
	if putBody.Description != body {
		t.Errorf("PUT body description = %q, want %q (verbatim)", putBody.Description, body)
	}
}

// TestUpdateProcessEmptyDescriptionFileBodyErrors verifies that a --description-file pointing at
// an empty file is rejected before any PUT, consistent with FR-6's empty-body rule.
func TestUpdateProcessEmptyDescriptionFileBodyErrors(t *testing.T) {
	path := filepath.Join(t.TempDir(), "empty.md")
	if err := os.WriteFile(path, []byte("\n"), 0o600); err != nil {
		t.Fatalf("cannot write fixture file: %v", err)
	}

	withUpdateOptions(t, func() {
		updateOptions.DescriptionFile = path
	})

	var requests []*http.Request
	cmd := setupTestNamed(t, "update-description-file-empty", func(w http.ResponseWriter, r *http.Request) {
		requests = append(requests, r)
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"id":42,"title":"Old title"}`))
	}, false)
	registerUpdateFlags(cmd)
	if err := cmd.Flags().Set("description-file", path); err != nil {
		t.Fatalf("cannot set description-file flag: %v", err)
	}

	err := updateProcess(cmd, []string{"42"})
	if err == nil {
		t.Fatal("updateProcess() expected an error, got nil")
	}
	if !strings.Contains(err.Error(), "description body is empty") {
		t.Errorf("error = %q, want it to mention the empty description body", err.Error())
	}
	if len(requests) != 1 || requests[0].Method != http.MethodGet {
		t.Errorf("requests = %v, want exactly one GET and no PUT", requests)
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

// TestAddReviewerFlagAcceptsDefaultSentinelThroughRealFlagParsing verifies that the documented
// `default` sentinel for --add-reviewer is accepted by real flag parsing: it drives
// cmd.Flags().Set("add-reviewer", "default") through the exact flag wiring updateCmd's init()
// uses (a StringSliceVar bound to updateOptions.AddReviewers), then runs addRequestedReviewers so
// the resolved default reviewer must actually be added, proving the sentinel reaches
// resolveDefaultReviewers end-to-end.
func TestAddReviewerFlagAcceptsDefaultSentinelThroughRealFlagParsing(t *testing.T) {
	withUpdateOptions(t, func() {})

	const defaultReviewerUUID = "{11111111-1111-1111-1111-111111111111}"
	cmd := setupTestNamed(t, "add-reviewer-default-sentinel", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		switch {
		case strings.HasSuffix(r.URL.Path, "/effective-default-reviewers"):
			_, _ = w.Write([]byte(`{"values":[{"user":{"uuid":"` + defaultReviewerUUID + `","display_name":"Default Reviewer"}}]}`))
		case strings.HasSuffix(r.URL.Path, "/user"):
			w.WriteHeader(http.StatusForbidden) // simulate a RAT client without /user access
		case strings.HasSuffix(r.URL.Path, "/members"):
			_, _ = w.Write([]byte(`{"values":[{"user":{"uuid":"` + defaultReviewerUUID + `","display_name":"Default Reviewer","nickname":"defaultreviewer"}}]}`))
		}
	}, false)

	// Register add-reviewer exactly as updateCmd's init() does: a real StringSlice flag bound
	// directly to the package-level updateOptions.AddReviewers, not a stand-in Bool/unbound flag.
	cmd.Flags().StringSliceVar(&updateOptions.AddReviewers, "add-reviewer", nil, "")

	if err := cmd.Flags().Set("add-reviewer", "default"); err != nil {
		t.Fatalf(`cmd.Flags().Set("add-reviewer", "default") error = %v, want nil: the "default" sentinel must be accepted by real flag parsing`, err)
	}
	if !slices.Equal(updateOptions.AddReviewers, []string{"default"}) {
		t.Fatalf("updateOptions.AddReviewers = %v, want [default]", updateOptions.AddReviewers)
	}

	id, err := common.ParseUUID("{22222222-2222-2222-2222-222222222222}")
	if err != nil {
		t.Fatalf("cannot parse fixture uuid: %v", err)
	}
	pr := &PullRequest{Source: Endpoint{Repository: &repository.Repository{
		ID: id, Name: "Widgets", FullName: testutil.FixtureRepositoryFlag, Slug: testutil.FixtureRepositorySlug,
		Workspace: &workspace.Workspace{Slug: testutil.FixtureWorkspaceSlug},
	}}}
	added, err := addRequestedReviewers(cmd.Context(), cmd, profile.Current, pr, testutil.FixtureWorkspaceSlug)
	if err != nil {
		t.Fatalf("addRequestedReviewers() error = %v", err)
	}
	if !added {
		t.Fatal("addRequestedReviewers() = false, want true: the resolved default reviewer must be added")
	}
	if len(pr.Reviewers) != 1 || pr.Reviewers[0].ID.String() != defaultReviewerUUID {
		t.Errorf("Reviewers = %+v, want exactly the resolved default reviewer %s", pr.Reviewers, defaultReviewerUUID)
	}
}

// TestAddReviewerFlagAcceptsNonNicknameIdentifierThroughRealFlagParsing verifies that a
// non-nickname identifier (UUID, Account ID, or display name) -- all documented as valid
// --add-reviewer values -- is accepted by real flag parsing, the same as the `default` sentinel.
func TestAddReviewerFlagAcceptsNonNicknameIdentifierThroughRealFlagParsing(t *testing.T) {
	withUpdateOptions(t, func() {})

	cmd := setupTestNamed(t, "add-reviewer-non-nickname", func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusForbidden)
	}, false)
	cmd.Flags().StringSliceVar(&updateOptions.AddReviewers, "add-reviewer", nil, "")

	const accountID = "5f8d3b2c1e9a4b0012345678"
	if err := cmd.Flags().Set("add-reviewer", accountID); err != nil {
		t.Fatalf(`cmd.Flags().Set("add-reviewer", %q) error = %v, want nil: a non-nickname identifier must be accepted by real flag parsing`, accountID, err)
	}
	if !slices.Equal(updateOptions.AddReviewers, []string{accountID}) {
		t.Errorf("updateOptions.AddReviewers = %v, want [%s]", updateOptions.AddReviewers, accountID)
	}
}

// TestUpdateProcessAddReviewerTypoErrorsBeforePut verifies that an unresolvable --add-reviewer
// value aborts the update with an error naming the offending value, and sends no PUT.
func TestUpdateProcessAddReviewerTypoErrorsBeforePut(t *testing.T) {
	withUpdateOptions(t, func() {
		updateOptions.AddReviewers = []string{"jdoe-typo"}
	})

	var putCount int
	cmd := setupTestNamed(t, "update-add-reviewer-typo", func(w http.ResponseWriter, r *http.Request) {
		switch {
		case strings.HasSuffix(r.URL.Path, "/members"):
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write([]byte(`{"values":[{"user":{"uuid":"{33333333-3333-3333-3333-333333333333}","nickname":"alice"}}]}`))
		case r.Method == http.MethodPut:
			putCount++
		default:
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write([]byte(`{"id":42,"title":"Old title"}`))
		}
	}, false)
	registerUpdateFlags(cmd)
	if err := cmd.Flags().Set("add-reviewer", "jdoe-typo"); err != nil {
		t.Fatalf("cannot set add-reviewer flag: %v", err)
	}

	err := updateProcess(cmd, []string{"42"})
	if err == nil {
		t.Fatal("updateProcess() expected an error, got nil")
	}
	if !strings.Contains(err.Error(), "jdoe-typo") {
		t.Errorf("error = %q, want it to name the unresolved reviewer jdoe-typo", err.Error())
	}
	if putCount != 0 {
		t.Errorf("expected no PUT request, got %d", putCount)
	}
}

// TestUpdateProcessRemoveReviewerNobodyErrors verifies that a --remove-reviewer value matching
// none of the pullrequest's current reviewers aborts the update with an error naming the
// offending value, and sends no PUT.
func TestUpdateProcessRemoveReviewerNobodyErrors(t *testing.T) {
	withUpdateOptions(t, func() {
		updateOptions.RemoveReviewers = []string{"nobody"}
	})

	var putCount int
	cmd := setupTestNamed(t, "update-remove-reviewer-nobody", func(w http.ResponseWriter, r *http.Request) {
		switch {
		case strings.HasSuffix(r.URL.Path, "/members"):
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write([]byte(`{"values":[]}`))
		case r.Method == http.MethodPut:
			putCount++
		default:
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write([]byte(`{"id":42,"title":"Old title","reviewers":[{"nickname":"alice"}]}`))
		}
	}, false)
	registerUpdateFlags(cmd)
	if err := cmd.Flags().Set("remove-reviewer", "nobody"); err != nil {
		t.Fatalf("cannot set remove-reviewer flag: %v", err)
	}

	err := updateProcess(cmd, []string{"42"})
	if err == nil {
		t.Fatal("updateProcess() expected an error, got nil")
	}
	if !strings.Contains(err.Error(), "nobody") {
		t.Errorf("error = %q, want it to name the unresolved reviewer nobody", err.Error())
	}
	if putCount != 0 {
		t.Errorf("expected no PUT request, got %d", putCount)
	}
}

// TestUpdateProcessAddReviewerAllExpandsToEveryMember is the --add-reviewer counterpart of
// TestCreateProcessReviewerAllExpandsToEveryMember: the "all" sentinel expands to every workspace
// member here too.
func TestUpdateProcessAddReviewerAllExpandsToEveryMember(t *testing.T) {
	withUpdateOptions(t, func() {
		updateOptions.AddReviewers = []string{"all"}
	})

	var putBody PullRequest
	var putCount int
	cmd := setupTestNamed(t, "update-add-reviewer-all", func(w http.ResponseWriter, r *http.Request) {
		switch {
		case strings.HasSuffix(r.URL.Path, "/members"):
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write([]byte(`{"values":[` +
				`{"user":{"uuid":"{33333333-3333-3333-3333-333333333333}","nickname":"alice"}},` +
				`{"user":{"uuid":"{44444444-4444-4444-4444-444444444444}","nickname":"bob"}}` +
				`]}`))
		case r.Method == http.MethodPut:
			putCount++
			if err := json.NewDecoder(r.Body).Decode(&putBody); err != nil {
				t.Errorf("cannot decode PUT body: %v", err)
			}
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write([]byte(`{"id":42,"title":"Old title"}`))
		default:
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write([]byte(`{"id":42,"title":"Old title"}`))
		}
	}, false)
	registerUpdateFlags(cmd)
	if err := cmd.Flags().Set("add-reviewer", "all"); err != nil {
		t.Fatalf("cannot set add-reviewer flag: %v", err)
	}

	if err := updateProcess(cmd, []string{"42"}); err != nil {
		t.Fatalf("updateProcess() error = %v", err)
	}
	if putCount != 1 {
		t.Fatalf("expected exactly one PUT request, got %d", putCount)
	}
	if len(putBody.Reviewers) != 2 {
		t.Fatalf("PUT body reviewers = %+v, want every workspace member (2)", putBody.Reviewers)
	}
	nicknames := map[string]bool{}
	for _, reviewer := range putBody.Reviewers {
		nicknames[reviewer.Nickname] = true
	}
	if !nicknames["alice"] || !nicknames["bob"] {
		t.Errorf("PUT body reviewer nicknames = %v, want both alice and bob", nicknames)
	}
}

// TestUpdateProcessAddReviewerAllExcludesAuthor is the --add-reviewer counterpart of
// TestCreateProcessReviewerAllExcludesAuthor: "all" must exclude the current user (the caller
// updating the pullrequest, standing in for the author here) just like the "default" sentinel.
func TestUpdateProcessAddReviewerAllExcludesAuthor(t *testing.T) {
	withUpdateOptions(t, func() {
		updateOptions.AddReviewers = []string{"all"}
	})

	var putBody PullRequest
	var putCount int
	cmd := setupTestNamed(t, "update-add-reviewer-all-excludes-author", func(w http.ResponseWriter, r *http.Request) {
		switch {
		case strings.HasSuffix(r.URL.Path, "/members"):
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write([]byte(`{"values":[` +
				`{"user":{"uuid":"{33333333-3333-3333-3333-333333333333}","nickname":"alice"}},` +
				`{"user":{"uuid":"{44444444-4444-4444-4444-444444444444}","nickname":"bob"}}` +
				`]}`))
		case r.URL.Path == "/2.0/user":
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write([]byte(`{"uuid":"{33333333-3333-3333-3333-333333333333}","nickname":"alice"}`))
		case r.Method == http.MethodPut:
			putCount++
			if err := json.NewDecoder(r.Body).Decode(&putBody); err != nil {
				t.Errorf("cannot decode PUT body: %v", err)
			}
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write([]byte(`{"id":42,"title":"Old title"}`))
		default:
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write([]byte(`{"id":42,"title":"Old title"}`))
		}
	}, false)
	registerUpdateFlags(cmd)
	if err := cmd.Flags().Set("add-reviewer", "all"); err != nil {
		t.Fatalf("cannot set add-reviewer flag: %v", err)
	}

	if err := updateProcess(cmd, []string{"42"}); err != nil {
		t.Fatalf("updateProcess() error = %v", err)
	}
	if putCount != 1 {
		t.Fatalf("expected exactly one PUT request, got %d", putCount)
	}
	if len(putBody.Reviewers) != 1 {
		t.Fatalf("PUT body reviewers = %+v, want exactly bob (author alice excluded)", putBody.Reviewers)
	}
	if putBody.Reviewers[0].Nickname != "bob" {
		t.Errorf("PUT body reviewer = %+v, want bob, not the author alice", putBody.Reviewers[0])
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

// TestUpdateProcessRejectsInvalidPullRequestID proves the <pullrequest-id> positional is
// validated via common.ValidatePathIdentifier before any request is sent: `bb pullrequest
// update ../..` must never reach repository.GetPath("pullrequests", "../..").
func TestUpdateProcessRejectsInvalidPullRequestID(t *testing.T) {
	var requestCount int
	cmd := setupTestNamed(t, "update-invalid-id", func(http.ResponseWriter, *http.Request) { requestCount++ }, false)
	registerUpdateFlags(cmd)

	err := updateProcess(cmd, []string{"../.."})
	if err == nil {
		t.Fatal("updateProcess() expected an error for '../..', got nil")
	}
	if !strings.Contains(err.Error(), "pullrequest-id") {
		t.Errorf("error = %q, want it to name pullrequest-id", err.Error())
	}
	if requestCount != 0 {
		t.Errorf("expected no HTTP request for an invalid pullrequest-id, got %d", requestCount)
	}
}
