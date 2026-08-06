package workspace

import (
	"encoding/json"
	"net/http"
	"strings"
	"testing"
)

func TestMembersProcessSuccessPreservesAPIOrderWithoutSortFlag(t *testing.T) {
	const slug = "acme-members-success"
	if err := WorkspaceCache.Set(slug, Workspace{Slug: slug, Name: "Acme Corp"}); err != nil {
		t.Fatalf("cannot prime workspace cache: %v", err)
	}

	var requests []*http.Request
	cmd := setupTest(t, "workspace-members-success", func(w http.ResponseWriter, r *http.Request) {
		requests = append(requests, r)
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"values":[` +
			`{"type":"workspace_membership","user":{"display_name":"Zed"},"workspace":{"type":"workspace","slug":"` + slug + `"}},` +
			`{"type":"workspace_membership","user":{"display_name":"Ada"},"workspace":{"type":"workspace","slug":"` + slug + `"}}` +
			`]}`))
	}, false)

	stdout := captureStdout(t, func() {
		if err := membersProcess(cmd, []string{slug}); err != nil {
			t.Fatalf("membersProcess() error = %v", err)
		}
	})

	if len(requests) != 1 {
		t.Fatalf("expected exactly 1 request, got %d", len(requests))
	}
	wantPath := "/2.0/workspaces/" + slug + "/members"
	if requests[0].URL.Path != wantPath {
		t.Errorf("path = %s, want %s", requests[0].URL.Path, wantPath)
	}

	var members []Member
	if err := json.Unmarshal([]byte(stdout), &members); err != nil {
		t.Fatalf("cannot unmarshal printed output %q: %v", stdout, err)
	}
	if len(members) != 2 || members[0].User.Name != "Zed" || members[1].User.Name != "Ada" {
		t.Errorf("members = %+v, want API order preserved (Zed, Ada) since --sort was not set", members)
	}
}

// TestMembersProcessSortFlagChangedSorts proves the sort-guard (rule 3): core.Sort only runs when
// cmd's "sort" flag is Changed, never unconditionally against an untouched default.
func TestMembersProcessSortFlagChangedSorts(t *testing.T) {
	const slug = "acme-members-sorted"
	if err := WorkspaceCache.Set(slug, Workspace{Slug: slug, Name: "Acme Corp"}); err != nil {
		t.Fatalf("cannot prime workspace cache: %v", err)
	}

	cmd := setupTest(t, "workspace-members-sorted", func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"values":[` +
			`{"type":"workspace_membership","user":{"display_name":"Zed"},"workspace":{"type":"workspace","slug":"` + slug + `"}},` +
			`{"type":"workspace_membership","user":{"display_name":"Ada"},"workspace":{"type":"workspace","slug":"` + slug + `"}}` +
			`]}`))
	}, false)
	if err := cmd.Flags().Set("sort", "name"); err != nil {
		t.Fatalf("cannot set sort flag: %v", err)
	}

	stdout := captureStdout(t, func() {
		if err := membersProcess(cmd, []string{slug}); err != nil {
			t.Fatalf("membersProcess() error = %v", err)
		}
	})

	var members []Member
	if err := json.Unmarshal([]byte(stdout), &members); err != nil {
		t.Fatalf("cannot unmarshal printed output %q: %v", stdout, err)
	}
	if len(members) != 2 || members[0].User.Name != "Ada" || members[1].User.Name != "Zed" {
		t.Errorf("members = %+v, want sorted by name ascending (Ada, Zed) once --sort is Changed", members)
	}
}

func TestMembersProcessNoResults(t *testing.T) {
	const slug = "acme-members-empty"
	if err := WorkspaceCache.Set(slug, Workspace{Slug: slug, Name: "Acme Corp"}); err != nil {
		t.Fatalf("cannot prime workspace cache: %v", err)
	}

	var requests []*http.Request
	cmd := setupTest(t, "workspace-members-empty", func(w http.ResponseWriter, r *http.Request) {
		requests = append(requests, r)
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"values":[]}`))
	}, false)

	stdout := captureStdout(t, func() {
		if err := membersProcess(cmd, []string{slug}); err != nil {
			t.Fatalf("membersProcess() error = %v", err)
		}
	})

	if len(requests) != 1 {
		t.Fatalf("expected exactly 1 request, got %d", len(requests))
	}
	if strings.TrimSpace(stdout) != "No member found" {
		t.Errorf("stdout = %q, want %q printed on stdout", stdout, "No member found")
	}
}

func TestMembersProcessAPIError(t *testing.T) {
	const slug = "acme-members-error"
	if err := WorkspaceCache.Set(slug, Workspace{Slug: slug, Name: "Acme Corp"}); err != nil {
		t.Fatalf("cannot prime workspace cache: %v", err)
	}

	cmd := setupTest(t, "workspace-members-api-error", func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusForbidden)
		_, _ = w.Write([]byte(`{"type":"error","error":{"message":"access denied"}}`))
	}, false)

	err := membersProcess(cmd, []string{slug})
	if err == nil {
		t.Fatal("membersProcess() expected an error, got nil")
	}
	if !strings.Contains(err.Error(), "access denied") {
		t.Errorf("error = %q, want it to contain the BitBucket error message", err.Error())
	}
}

func TestMembersProcessDryRun(t *testing.T) {
	const slug = "acme-members-dry-run"
	if err := WorkspaceCache.Set(slug, Workspace{Slug: slug, Name: "Acme Corp"}); err != nil {
		t.Fatalf("cannot prime workspace cache: %v", err)
	}

	var requestCount int
	cmd := setupTest(t, "workspace-members-dry-run", func(http.ResponseWriter, *http.Request) { requestCount++ }, true)

	if err := membersProcess(cmd, []string{slug}); err != nil {
		t.Fatalf("membersProcess() error = %v", err)
	}
	if requestCount != 0 {
		t.Errorf("expected no HTTP request in dry-run mode, got %d", requestCount)
	}
}

func TestMembersProcessCurrentWorkspace(t *testing.T) {
	const slug = "acme-members-current"
	if err := WorkspaceCache.Set(slug, Workspace{Slug: slug, Name: "Acme Corp"}); err != nil {
		t.Fatalf("cannot prime workspace cache: %v", err)
	}

	var requests []*http.Request
	cmd := setupTest(t, "workspace-members-current", func(w http.ResponseWriter, r *http.Request) {
		requests = append(requests, r)
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"values":[{"type":"workspace_membership","user":{"display_name":"Ada"},"workspace":{"type":"workspace","slug":"` + slug + `"}}]}`))
	}, false)
	if err := cmd.Flags().Set("workspace", slug); err != nil {
		t.Fatalf("cannot set workspace flag: %v", err)
	}

	stdout := captureStdout(t, func() {
		if err := membersProcess(cmd, nil); err != nil {
			t.Fatalf("membersProcess() error = %v", err)
		}
	})

	if len(requests) != 1 {
		t.Fatalf("expected exactly 1 request, got %d", len(requests))
	}
	wantPath := "/2.0/workspaces/" + slug + "/members"
	if requests[0].URL.Path != wantPath {
		t.Errorf("path = %s, want %s", requests[0].URL.Path, wantPath)
	}

	var members []Member
	if err := json.Unmarshal([]byte(stdout), &members); err != nil {
		t.Fatalf("cannot unmarshal printed output %q: %v", stdout, err)
	}
	if len(members) != 1 || members[0].User.Name != "Ada" {
		t.Errorf("members = %+v, want a single member named Ada", members)
	}
}
