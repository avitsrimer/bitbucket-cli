package workspace

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"strings"
	"testing"
)

// TestMembersProcessDefaultSortsByName proves the real command's documented default ("--sort
// string   Column to sort by (default \"name\")" per --help) actually applies when --sort is not
// passed: memberColumns marks "name" as its DefaultSorter, and common.SortFlagValue resolves that
// default from the flag itself, so this must always sort ascending by name -- not merely preserve
// whatever order the API happened to return. The fixture's API order (Zed, then Ada) is
// deliberately reversed from the expected sorted order, so the two orders can never be confused.
func TestMembersProcessDefaultSortsByName(t *testing.T) {
	const slug = "acme-members-default-sort"
	if err := WorkspaceCache.Set(slug, Workspace{Slug: slug, Name: "Acme Corp"}); err != nil {
		t.Fatalf("cannot prime workspace cache: %v", err)
	}

	var requests []*http.Request
	cmd := setupTest(t, "workspace-members-default-sort", func(w http.ResponseWriter, r *http.Request) {
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
	if len(members) != 2 || members[0].User.Name != "Ada" || members[1].User.Name != "Zed" {
		t.Errorf("members = %+v, want sorted by name ascending (Ada, Zed) by default, not the API's raw order (Zed, Ada)", members)
	}
}

// TestMembersProcessSortFlagChangedSorts proves --sort actually selects the comparator
// common.Sort runs: reading it via common.SortFlagValue(cmd) (cmd's own --sort flag, not a
// package-level SortBy.Value binding that is only ever populated on the real command) means
// the process sorts identically whether cmd is the real command or, as here, a standalone
// test command carrying its own --sort flag.
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

// TestGetMembersUsesItsOwnContextParameterNotCmdContext proves GetMembers actually issues its
// request with the ctx parameter it was called with, rather than silently substituting
// cmd.Context() -- distinguished here by handing it an already-canceled context while cmd itself
// carries a perfectly valid one, so the call can only fail if ctx was really the one used.
func TestGetMembersUsesItsOwnContextParameterNotCmdContext(t *testing.T) {
	const slug = "acme-members-ctx"

	var requests int
	cmd := setupTest(t, "workspace-members-ctx", func(http.ResponseWriter, *http.Request) {
		requests++
	}, false)

	canceledCtx, cancel := context.WithCancel(context.Background())
	cancel()

	_, err := GetMembers(canceledCtx, cmd, slug)
	if err == nil {
		t.Fatal("GetMembers() expected an error from an already-canceled context, got nil")
	}
	if !errors.Is(err, context.Canceled) {
		t.Errorf("GetMembers() error = %v, want it to wrap context.Canceled (proving ctx, not cmd.Context(), was used)", err)
	}
	if requests != 0 {
		t.Errorf("expected no HTTP request to complete with an already-canceled context, got %d", requests)
	}
}

// TestMembersProcessRendersTableOutput proves the columns -> GetHeaders -> GetRow wiring
// actually reaches profile.Print for --output table, not just the JSON path every other
// test in this file drives.
func TestMembersProcessRendersTableOutput(t *testing.T) {
	const slug = "acme-members-table"
	if err := WorkspaceCache.Set(slug, Workspace{Slug: slug, Name: "Acme Corp"}); err != nil {
		t.Fatalf("cannot prime workspace cache: %v", err)
	}

	cmd := setupTest(t, "workspace-members-table", func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"values":[{"type":"workspace_membership","user":{"display_name":"Jane Doe"},"workspace":{"type":"workspace","slug":"` + slug + `"}}]}`))
	}, false)
	if err := cmd.Flags().Set("output", "table"); err != nil {
		t.Fatalf("cannot set output flag: %v", err)
	}

	stdout := captureStdout(t, func() {
		if err := membersProcess(cmd, []string{slug}); err != nil {
			t.Fatalf("membersProcess() error = %v", err)
		}
	})

	if !strings.Contains(stdout, "Jane Doe") {
		t.Errorf("table output = %q, want it to contain the member's name", stdout)
	}
	if !strings.Contains(stdout, "+--") {
		t.Errorf("table output = %q, want the table renderer's box-drawing border", stdout)
	}
	var probe any
	if err := json.Unmarshal([]byte(stdout), &probe); err == nil {
		t.Errorf("table output = %q, want it not to parse as JSON", stdout)
	}
}
