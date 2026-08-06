package pullrequest

import (
	"encoding/json"
	"net/http"
	"os"
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
	oldTitle, oldDescription := updateOptions.Title, updateOptions.Description
	oldDestinationValue := updateOptions.Destination.Value
	oldAddReviewers, oldRemoveReviewers := updateOptions.AddReviewers, updateOptions.RemoveReviewers
	oldCloseSourceBranch := updateOptions.CloseSourceBranch
	t.Cleanup(func() {
		updateOptions.Title = oldTitle
		updateOptions.Description = oldDescription
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
	cmd.Flags().String("destination", "", "")
	cmd.Flags().Bool("close-source-branch", false, "")
	cmd.Flags().StringSlice("add-reviewer", nil, "")
	cmd.Flags().StringSlice("remove-reviewer", nil, "")
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
		setFlags    bool
		wantChanged bool
	}{
		{name: "title and description changed", setFlags: true, wantChanged: true},
		{name: "no flags changed", setFlags: false, wantChanged: false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			withUpdateOptions(t, func() {
				updateOptions.Title = "New title"
				updateOptions.Description = "New description"
			})

			cmd := &cobra.Command{}
			registerUpdateFlags(cmd)
			if tt.setFlags {
				if err := cmd.Flags().Set("title", "New title"); err != nil {
					t.Fatalf("cannot set title flag: %v", err)
				}
				if err := cmd.Flags().Set("description", "New description"); err != nil {
					t.Fatalf("cannot set description flag: %v", err)
				}
			}

			var pr PullRequest
			if changed := applySimpleFieldUpdates(cmd, &pr); changed != tt.wantChanged {
				t.Errorf("applySimpleFieldUpdates() = %v, want %v", changed, tt.wantChanged)
			}
			if !tt.setFlags {
				return
			}
			if pr.Title != "New title" {
				t.Errorf("Title = %q, want %q", pr.Title, "New title")
			}
			if pr.Description != "New description" || pr.Summary.Raw != "New description" {
				t.Errorf("Description/Summary.Raw = %q/%q, want %q", pr.Description, pr.Summary.Raw, "New description")
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
// Repository.GetWorkspace's cached/FullName fallback chain.
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
// WorkspaceCache empty for its workspace, so building the path from Slug instead of FullName, or
// falling back to a live GetWorkspace call, would either hit the wrong path or the
// RAT-simulating 403 this test's server returns for any /workspaces/ request.
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
	pullrequestWorkspace := &workspace.Workspace{Slug: testutil.FixtureWorkspaceSlug}

	added, err := addRequestedReviewers(cmd.Context(), cmd, profile.Current, pr, pullrequestWorkspace)
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
