package repository

import (
	"encoding/json"
	"os"
	"strings"
	"testing"

	"github.com/avitsrimer/bitbucket-cli/internal/common"
	"github.com/avitsrimer/bitbucket-cli/internal/workspace"
	"github.com/spf13/cobra"
)

func TestRepositoryStringPrefersSlug(t *testing.T) {
	target := Repository{Slug: "bb", Name: "bitbucket-cli"}
	if got := target.String(); got != "bb" {
		t.Errorf("String() = %q, want %q", got, "bb")
	}
}

func TestRepositoryStringFallsBackToName(t *testing.T) {
	target := Repository{Name: "bitbucket-cli"}
	if got := target.String(); got != "bitbucket-cli" {
		t.Errorf("String() = %q, want %q", got, "bitbucket-cli")
	}
}

func TestRepositoryGetPath(t *testing.T) {
	target := Repository{Slug: "bb", Workspace: &workspace.Workspace{Slug: "acme"}}
	if got := target.GetPath("pullrequests", "42"); got != "/repositories/acme/bb/pullrequests/42" {
		t.Errorf("GetPath() = %q, want %q", got, "/repositories/acme/bb/pullrequests/42")
	}
}

// TestRepositoryGetPathNeverPanicsOnNilWorkspace proves GetPath does not dereference a nil
// Workspace: BitBucket omits "workspace" on a trimmed nested Repository payload (e.g. a pull
// request's source/destination repository), and GetPath is reached by nearly every
// repository-scoped command, so a panic here is a regression that reaches almost the entire CLI.
func TestRepositoryGetPathNeverPanicsOnNilWorkspace(t *testing.T) {
	t.Run("falls back to FullName's workspace segment when Workspace is nil", func(t *testing.T) {
		target := Repository{Slug: "bb", FullName: "acme/bb"}
		if got := target.GetPath("pullrequests", "42"); got != "/repositories/acme/bb/pullrequests/42" {
			t.Errorf("GetPath() = %q, want %q", got, "/repositories/acme/bb/pullrequests/42")
		}
	})

	t.Run("degrades to an empty workspace segment instead of panicking when nothing resolves it", func(t *testing.T) {
		target := Repository{Slug: "bb"}
		if got := target.GetPath("pullrequests"); got != "/repositories/bb/pullrequests" {
			t.Errorf("GetPath() = %q, want %q", got, "/repositories/bb/pullrequests")
		}
	})
}

// TestRepositoryGetWorkspaceSlugNeverCallsTheNetwork proves each of GetWorkspaceSlug's three
// resolution branches (embedded Workspace, FullName split, --workspace flag fallback) never
// reaches out to the network: cmd here carries no profile at all, so any code path that tried to
// call the BitBucket API would panic or error instead of returning cleanly, structurally proving
// a repository's workspace slug is resolved without a Workspace fetch.
func TestRepositoryGetWorkspaceSlugNeverCallsTheNetwork(t *testing.T) {
	newBareCmd := func() *cobra.Command {
		cmd := &cobra.Command{Use: "test"}
		cmd.SetContext(t.Context())
		cmd.Flags().String("workspace", "", "")
		return cmd
	}

	t.Run("embedded workspace wins over full name", func(t *testing.T) {
		target := Repository{Slug: "bb", FullName: "ignored/bb", Workspace: &workspace.Workspace{Slug: "embedded-ws"}}
		got, err := target.GetWorkspaceSlug(t.Context(), newBareCmd())
		if err != nil {
			t.Fatalf("GetWorkspaceSlug() error = %v", err)
		}
		if got != "embedded-ws" {
			t.Errorf("GetWorkspaceSlug() = %q, want %q", got, "embedded-ws")
		}
	})

	t.Run("full name split when workspace missing", func(t *testing.T) {
		target := Repository{Slug: "bb", FullName: "acme/bb"}
		got, err := target.GetWorkspaceSlug(t.Context(), newBareCmd())
		if err != nil {
			t.Fatalf("GetWorkspaceSlug() error = %v", err)
		}
		if got != "acme" {
			t.Errorf("GetWorkspaceSlug() = %q, want %q", got, "acme")
		}
	})

	t.Run("falls back to workspace flag when neither is available", func(t *testing.T) {
		target := Repository{Slug: "bb"}
		cmd := newBareCmd()
		if err := cmd.Flags().Set("workspace", "flag-ws"); err != nil {
			t.Fatalf("cannot set workspace flag: %v", err)
		}
		got, err := target.GetWorkspaceSlug(t.Context(), cmd)
		if err != nil {
			t.Fatalf("GetWorkspaceSlug() error = %v", err)
		}
		if got != "flag-ws" {
			t.Errorf("GetWorkspaceSlug() = %q, want %q", got, "flag-ws")
		}
	})

	t.Run("error when nothing resolves the workspace", func(t *testing.T) {
		target := Repository{Slug: "bb"}
		if _, err := target.GetWorkspaceSlug(t.Context(), newBareCmd()); err == nil {
			t.Fatal("GetWorkspaceSlug() expected an error when no source provides a workspace, got nil")
		}
	})
}

func TestRepositoryValidateAccumulatesErrors(t *testing.T) {
	target := &Repository{}
	err := target.Validate()
	if err == nil {
		t.Fatal("Validate() expected an error for an empty repository, got nil")
	}
	for _, want := range []string{"uuid is missing", "name is missing", "full_name is missing"} {
		if !strings.Contains(err.Error(), want) {
			t.Errorf("Validate() error = %q, want it to contain %q", err.Error(), want)
		}
	}
}

func TestRepositoryValidateDefaultsSlugToName(t *testing.T) {
	id, err := common.ParseUUID("{11111111-1111-1111-1111-111111111111}")
	if err != nil {
		t.Fatalf("cannot parse fixture uuid: %v", err)
	}
	target := &Repository{ID: id, Name: "bitbucket-cli", FullName: "acme/bitbucket-cli"}
	if err := target.Validate(); err != nil {
		t.Fatalf("Validate() error = %v", err)
	}
	if target.Slug != "bitbucket-cli" {
		t.Errorf("Slug = %q, want %q", target.Slug, "bitbucket-cli")
	}
}

func TestRepositoryUnmarshalJSONFromTestdata(t *testing.T) {
	data, err := os.ReadFile("../../testdata/repository.json")
	if err != nil {
		t.Fatalf("cannot read testdata: %v", err)
	}

	var repo Repository
	if err := json.Unmarshal(data, &repo); err != nil {
		t.Fatalf("Unmarshal() error = %v", err)
	}
	if repo.Name != "bb" {
		t.Errorf("repo.Name = %q, want %q", repo.Name, "bb")
	}
	if repo.FullName != "gildas_cherruel/bb" {
		t.Errorf("repo.FullName = %q, want %q", repo.FullName, "gildas_cherruel/bb")
	}
	if repo.MainBranch != "master" {
		t.Errorf("repo.MainBranch = %q, want %q", repo.MainBranch, "master")
	}
	if repo.Language != "go" {
		t.Errorf("repo.Language = %q, want %q", repo.Language, "go")
	}
}

func TestRepositoryUnmarshalJSONRejectsWrongType(t *testing.T) {
	var repo Repository
	err := json.Unmarshal([]byte(`{"type":"not-a-repository"}`), &repo)
	if err == nil {
		t.Fatal("Unmarshal() expected an error for the wrong type, got nil")
	}
}

func TestRepositoryMarshalJSONRoundTrip(t *testing.T) {
	id, err := common.ParseUUID("{11111111-1111-1111-1111-111111111111}")
	if err != nil {
		t.Fatalf("cannot parse fixture uuid: %v", err)
	}
	original := Repository{
		ID:         id,
		Name:       "bb",
		FullName:   "acme/bb",
		Slug:       "bb",
		MainBranch: "master",
		HasIssues:  true,
	}

	data, err := json.Marshal(original)
	if err != nil {
		t.Fatalf("Marshal() error = %v", err)
	}

	var decoded Repository
	if err := json.Unmarshal(data, &decoded); err != nil {
		t.Fatalf("Unmarshal() error = %v", err)
	}
	if decoded.Name != original.Name || decoded.FullName != original.FullName {
		t.Errorf("decoded = %+v, want name/full_name to match original %+v", decoded, original)
	}
	if decoded.MainBranch != "master" {
		t.Errorf("decoded.MainBranch = %q, want %q", decoded.MainBranch, "master")
	}
	if !decoded.HasIssues {
		t.Error("decoded.HasIssues = false, want true")
	}
}
