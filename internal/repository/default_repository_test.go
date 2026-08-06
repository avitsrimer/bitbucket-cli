package repository

import (
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/avitsrimer/bitbucket-cli/internal/profile"
	"github.com/spf13/cobra"
)

// chdirToFakeGitConfig writes a hand-rolled .git/config file (no real git binary required) whose
// content is config, then chdirs the process into that directory for the duration of the calling
// test, restoring the original working directory via t.Cleanup. This is enough to steer
// remote.GetRemote (via common.OpenGitConfig, which just walks up from cwd looking for a
// ".git/config" file) without needing a real git checkout.
func chdirToFakeGitConfig(t *testing.T, config string) {
	t.Helper()

	dir := t.TempDir()
	gitDir := filepath.Join(dir, ".git")
	if err := os.Mkdir(gitDir, 0o750); err != nil {
		t.Fatalf("cannot create fake .git dir: %v", err)
	}
	if config != "" {
		if err := os.WriteFile(filepath.Join(gitDir, "config"), []byte(config), 0o600); err != nil {
			t.Fatalf("cannot write fake .git/config: %v", err)
		}
	}

	original, err := os.Getwd()
	if err != nil {
		t.Fatalf("cannot get working directory: %v", err)
	}
	if err := os.Chdir(dir); err != nil {
		t.Fatalf("cannot chdir to %s: %v", dir, err)
	}
	t.Cleanup(func() {
		if err := os.Chdir(original); err != nil {
			t.Fatalf("cannot restore working directory: %v", err)
		}
	})
}

// newRepositoryNameCmd returns a standalone command carrying just the flags GetRepositoryName
// reads (repository, git-remote), with profile.Current reset to nil and restored on cleanup so
// each test starts from a clean slate regardless of what an earlier test left behind.
func newRepositoryNameCmd(t *testing.T) *cobra.Command {
	t.Helper()
	oldCurrent := profile.Current
	profile.Current = nil
	t.Cleanup(func() { profile.Current = oldCurrent })

	cmd := &cobra.Command{Use: "test"}
	cmd.Flags().String("repository", "", "")
	return cmd
}

// TestGetRepositoryNameFlagWins proves the --repository flag is the first rung: it must win
// without even inspecting the git config or the profile, regardless of what either holds.
func TestGetRepositoryNameFlagWins(t *testing.T) {
	cmd := newRepositoryNameCmd(t)
	if err := cmd.Flags().Set("repository", "explicit/repo"); err != nil {
		t.Fatalf("cannot set repository flag: %v", err)
	}
	profile.Current = &profile.Profile{Name: "p", DefaultRepository: "should-not-be-used"}

	name, err := GetRepositoryName(t.Context(), cmd)
	if err != nil {
		t.Fatalf("GetRepositoryName() error = %v", err)
	}
	if name != "explicit/repo" {
		t.Errorf("GetRepositoryName() = %q, want %q", name, "explicit/repo")
	}
}

// TestGetRepositoryNameFallsBackToBitbucketRemote proves the second rung: with no --repository
// flag, a Bitbucket git remote in the current checkout wins over any profile default.
func TestGetRepositoryNameFallsBackToBitbucketRemote(t *testing.T) {
	chdirToFakeGitConfig(t, "[remote \"origin\"]\n\turl = git@bitbucket.org:acme/from-remote.git\n\tfetch = +refs/heads/*:refs/remotes/origin/*\n")
	cmd := newRepositoryNameCmd(t)
	profile.Current = &profile.Profile{Name: "p", DefaultRepository: "should-not-be-used"}

	name, err := GetRepositoryName(t.Context(), cmd)
	if err != nil {
		t.Fatalf("GetRepositoryName() error = %v", err)
	}
	if name != "acme/from-remote" {
		t.Errorf("GetRepositoryName() = %q, want %q", name, "acme/from-remote")
	}
}

// TestGetRepositoryNameGitHubRemoteFallsThroughToProfileDefault reproduces FR-12's core scenario:
// the current checkout's remote is GitHub (or any non-bitbucket.org host), which
// remote.GetRemote already rejects -- this proves that rejection is treated as "no remote", not
// an error the caller must handle specially, and the chain falls through cleanly to the profile
// default instead of erroring out.
func TestGetRepositoryNameGitHubRemoteFallsThroughToProfileDefault(t *testing.T) {
	chdirToFakeGitConfig(t, "[remote \"origin\"]\n\turl = git@github.com:someorg/somerepo.git\n\tfetch = +refs/heads/*:refs/remotes/origin/*\n")
	cmd := newRepositoryNameCmd(t)
	profile.Current = &profile.Profile{Name: "p", DefaultRepository: "acme/from-profile"}

	name, err := GetRepositoryName(t.Context(), cmd)
	if err != nil {
		t.Fatalf("GetRepositoryName() error = %v, want it to fall through to the profile default", err)
	}
	if name != "acme/from-profile" {
		t.Errorf("GetRepositoryName() = %q, want %q", name, "acme/from-profile")
	}
}

// TestGetRepositoryNameNoRemoteAtAllFallsBackToProfileDefault covers the other way the second
// rung can come up empty: no git remote configured at all (as opposed to one that is present but
// not a Bitbucket host, covered above). Both must fall through identically to the profile default.
func TestGetRepositoryNameNoRemoteAtAllFallsBackToProfileDefault(t *testing.T) {
	chdirToFakeGitConfig(t, "[core]\n\tbare = false\n")
	cmd := newRepositoryNameCmd(t)
	profile.Current = &profile.Profile{Name: "p", DefaultRepository: "acme/from-profile"}

	name, err := GetRepositoryName(t.Context(), cmd)
	if err != nil {
		t.Fatalf("GetRepositoryName() error = %v, want it to fall through to the profile default", err)
	}
	if name != "acme/from-profile" {
		t.Errorf("GetRepositoryName() = %q, want %q", name, "acme/from-profile")
	}
}

// TestGetRepositoryNameErrorNamesAllThreeWaysWhenEveryRungFails proves that when the flag, the
// git remote, and the profile default all come up empty, the error names every way to supply the
// value instead of a bare "argument repository is missing".
func TestGetRepositoryNameErrorNamesAllThreeWaysWhenEveryRungFails(t *testing.T) {
	chdirToFakeGitConfig(t, "[core]\n\tbare = false\n")
	cmd := newRepositoryNameCmd(t)
	profile.Current = &profile.Profile{Name: "p"}

	_, err := GetRepositoryName(t.Context(), cmd)
	if err == nil {
		t.Fatal("GetRepositoryName() expected an error when every rung is empty, got nil")
	}
	for _, want := range []string{"--repository", "git", "default-repository"} {
		if !strings.Contains(err.Error(), want) {
			t.Errorf("GetRepositoryName() error = %q, want it to mention %q", err.Error(), want)
		}
	}
}

// TestGetRepositoryNameLogsWarnOnProfileLoadError proves that when the flag and the git remote
// both come up empty and resolving the profile itself fails (an invalid --profile value here), the
// underlying profile error is logged as a [WARN] rather than silently discarded, so the real cause
// is still visible even though GetRepositoryName still returns its own generic
// "argument repository is missing" error.
func TestGetRepositoryNameLogsWarnOnProfileLoadError(t *testing.T) {
	chdirToFakeGitConfig(t, "[core]\n\tbare = false\n")

	oldCurrent, oldProfiles := profile.Current, profile.Profiles
	profile.Current = nil
	profile.Profiles = nil
	t.Cleanup(func() {
		profile.Current = oldCurrent
		profile.Profiles = oldProfiles
	})

	cmd := &cobra.Command{Use: "test"}
	cmd.Flags().String("repository", "", "")
	cmd.Flags().String("profile", "bogus-profile", "")

	logs := captureLog(t)

	_, err := GetRepositoryName(t.Context(), cmd)
	if err == nil {
		t.Fatal("GetRepositoryName() expected an error, got nil")
	}
	if !strings.Contains(err.Error(), "argument repository is missing") {
		t.Errorf("GetRepositoryName() error = %q, want the generic missing-argument message", err.Error())
	}
	if !strings.Contains(logs.String(), "WARN") || !strings.Contains(logs.String(), "argument profile is invalid") {
		t.Errorf("logs = %q, want a [WARN] line surfacing the profile load error", logs.String())
	}
}

// TestGetRepositoryBothDefaultsSetAndNoFlagsNeverErrorsWithArgumentMissing drives GetRepository
// end to end (GetRepositoryName -> GetRepositoryBySlugOrID -> workspace.GetWorkspaceName): with
// both profile.DefaultWorkspace and profile.DefaultRepository set, no --workspace/--repository
// flags, and no Bitbucket git remote in the current checkout, resolving the repository must
// succeed with zero "argument ... is missing" errors, proving the two independent precedence
// chains compose.
func TestGetRepositoryBothDefaultsSetAndNoFlagsNeverErrorsWithArgumentMissing(t *testing.T) {
	chdirToFakeGitConfig(t, "[core]\n\tbare = false\n")

	const workspaceSlug = "fr12-both-defaults-ws"
	const repositorySlug = "fr12-both-defaults-repo"
	repositoryPath := "/2.0/repositories/" + workspaceSlug + "/" + repositorySlug

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		switch r.URL.Path {
		case repositoryPath:
			_, _ = w.Write([]byte(`{"type":"repository","uuid":"{66666666-6666-6666-6666-666666666666}","name":"FR12","full_name":"` + workspaceSlug + `/` + repositorySlug + `","slug":"` + repositorySlug + `"}`))
		default:
			t.Errorf("unexpected request to %s", r.URL.Path)
		}
	}))
	t.Cleanup(server.Close)

	apiRoot, err := url.Parse(server.URL)
	if err != nil {
		t.Fatalf("cannot parse test server URL: %v", err)
	}

	testProfile := &profile.Profile{
		Name:              "fr12-both-defaults",
		APIRoot:           apiRoot,
		AccessToken:       "dummy-token",
		OutputFormat:      "json",
		DefaultWorkspace:  workspaceSlug,
		DefaultRepository: repositorySlug,
	}
	oldProfiles, oldCurrent := profile.Profiles, profile.Current
	profile.Profiles = append(profile.Profiles, testProfile)
	profile.Current = testProfile
	t.Cleanup(func() {
		profile.Profiles = oldProfiles
		profile.Current = oldCurrent
	})

	cmd := &cobra.Command{Use: "test"}
	cmd.SetContext(t.Context())
	cmd.Flags().String("profile", "", "")
	cmd.Flags().String("output", "", "")
	cmd.Flags().Bool("dry-run", false, "")
	cmd.Flags().String("repository", "", "")
	cmd.Flags().String("workspace", "", "")

	repo, err := GetRepository(t.Context(), cmd)
	if err != nil {
		t.Fatalf("GetRepository() error = %v, want no error with both defaults set and no flags", err)
	}
	if repo.Slug != repositorySlug {
		t.Errorf("repo.Slug = %q, want %q", repo.Slug, repositorySlug)
	}
}
