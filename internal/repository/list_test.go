package repository

import (
	"encoding/json"
	"net/http"
	"strings"
	"testing"
)

// setRoleForTest overrides the package-level listOptions.Role.Value (the real --role flag is
// bound to listCmd, not to setupTest's standalone cmd) for the duration of the calling test,
// restoring the previous value afterward.
func setRoleForTest(t *testing.T, value string) {
	t.Helper()
	original := listOptions.Role.Value
	listOptions.Role.Value = value
	t.Cleanup(func() { listOptions.Role.Value = original })
}

// TestListProcessDefaultSortsByName proves the real command's documented default ("--sort string
// Column to sort by (default \"name\")") actually applies when --sort is not passed: columns
// marks "name" as its DefaultSorter, and common.SortFlagValue resolves that default from the flag
// itself, so this must always sort ascending by name -- not merely preserve whatever order the
// API happened to return. The fixture's API order (Zeta, then Acme) is deliberately reversed from
// the expected sorted order, so the two orders can never be confused.
func TestListProcessDefaultSortsByName(t *testing.T) {
	setRoleForTest(t, "owner")

	var requests []*http.Request
	cmd := setupTest(t, "repository-list-default-sort", func(w http.ResponseWriter, r *http.Request) {
		requests = append(requests, r)
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"values":[` +
			`{"type":"repository","uuid":"{11111111-1111-1111-1111-111111111111}","name":"Zeta","full_name":"acme/zeta","slug":"zeta"},` +
			`{"type":"repository","uuid":"{22222222-2222-2222-2222-222222222222}","name":"Acme","full_name":"acme/acme-repo","slug":"acme-repo"}` +
			`]}`))
	}, false)

	stdout := captureStdout(t, func() {
		if err := listProcess(cmd, nil); err != nil {
			t.Fatalf("listProcess() error = %v", err)
		}
	})

	if len(requests) != 1 {
		t.Fatalf("expected exactly 1 request, got %d", len(requests))
	}
	wantPath := "/2.0/repositories/" + testWorkspaceSlug
	if requests[0].URL.Path != wantPath {
		t.Errorf("path = %s, want %s", requests[0].URL.Path, wantPath)
	}
	if got := requests[0].URL.Query().Get("role"); got != "owner" {
		t.Errorf("role query = %q, want %q", got, "owner")
	}

	var repositories []Repository
	if err := json.Unmarshal([]byte(stdout), &repositories); err != nil {
		t.Fatalf("cannot unmarshal printed output %q: %v", stdout, err)
	}
	if len(repositories) != 2 || repositories[0].Slug != "acme-repo" || repositories[1].Slug != "zeta" {
		t.Errorf("repositories = %+v, want sorted by name ascending (Acme, Zeta) by default, not the API's raw order (Zeta, Acme)", repositories)
	}
}

func TestListProcessRoleAllOmitsQueryFilter(t *testing.T) {
	setRoleForTest(t, "all")

	var requests []*http.Request
	cmd := setupTest(t, "repository-list-role-all", func(w http.ResponseWriter, r *http.Request) {
		requests = append(requests, r)
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"values":[]}`))
	}, false)

	captureStdout(t, func() {
		if err := listProcess(cmd, nil); err != nil {
			t.Fatalf("listProcess() error = %v", err)
		}
	})

	if len(requests) != 1 {
		t.Fatalf("expected exactly 1 request, got %d", len(requests))
	}
	if got := requests[0].URL.Query().Get("role"); got != "" {
		t.Errorf("role query = %q, want no role filter when --role=all", got)
	}
}

// TestListProcessSortFlagChangedSorts proves --sort actually selects the comparator
// common.Sort runs: reading it via common.SortFlagValue(cmd) (cmd's own --sort flag, not a
// package-level SortBy.Value binding that is only ever populated on the real command) means
// the process sorts identically whether cmd is the real command or, as here, a standalone
// test command carrying its own --sort flag.
func TestListProcessSortFlagChangedSorts(t *testing.T) {
	setRoleForTest(t, "owner")

	cmd := setupTest(t, "repository-list-sorted", func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"values":[` +
			`{"type":"repository","uuid":"{11111111-1111-1111-1111-111111111111}","name":"Zeta","full_name":"acme/zeta","slug":"zeta"},` +
			`{"type":"repository","uuid":"{22222222-2222-2222-2222-222222222222}","name":"Acme","full_name":"acme/acme-repo","slug":"acme-repo"}` +
			`]}`))
	}, false)
	if err := cmd.Flags().Set("sort", "name"); err != nil {
		t.Fatalf("cannot set sort flag: %v", err)
	}

	stdout := captureStdout(t, func() {
		if err := listProcess(cmd, nil); err != nil {
			t.Fatalf("listProcess() error = %v", err)
		}
	})

	var repositories []Repository
	if err := json.Unmarshal([]byte(stdout), &repositories); err != nil {
		t.Fatalf("cannot unmarshal printed output %q: %v", stdout, err)
	}
	if len(repositories) != 2 || repositories[0].Name != "Acme" || repositories[1].Name != "Zeta" {
		t.Errorf("repositories = %+v, want sorted by name ascending (Acme, Zeta) once --sort is Changed", repositories)
	}
}

func TestListProcessNoResults(t *testing.T) {
	setRoleForTest(t, "owner")

	var requests []*http.Request
	cmd := setupTest(t, "repository-list-empty", func(w http.ResponseWriter, r *http.Request) {
		requests = append(requests, r)
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"values":[]}`))
	}, false)

	stdout := captureStdout(t, func() {
		if err := listProcess(cmd, nil); err != nil {
			t.Fatalf("listProcess() error = %v", err)
		}
	})

	if len(requests) != 1 {
		t.Fatalf("expected exactly 1 request, got %d", len(requests))
	}
	if strings.TrimSpace(stdout) != "No repository found" {
		t.Errorf("stdout = %q, want %q printed on stdout", stdout, "No repository found")
	}
}

func TestListProcessAPIError(t *testing.T) {
	setRoleForTest(t, "owner")

	cmd := setupTest(t, "repository-list-api-error", func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusInternalServerError)
		_, _ = w.Write([]byte(`{"type":"error","error":{"message":"server exploded"}}`))
	}, false)

	err := listProcess(cmd, nil)
	if err == nil {
		t.Fatal("listProcess() expected an error, got nil")
	}
	if !strings.Contains(err.Error(), "server exploded") {
		t.Errorf("error = %q, want it to contain the BitBucket error message", err.Error())
	}
}

func TestListProcessDryRun(t *testing.T) {
	setRoleForTest(t, "owner")

	var requestCount int
	cmd := setupTest(t, "repository-list-dry-run", func(http.ResponseWriter, *http.Request) { requestCount++ }, true)

	if err := listProcess(cmd, nil); err != nil {
		t.Fatalf("listProcess() error = %v", err)
	}
	if requestCount != 0 {
		t.Errorf("expected no HTTP request in dry-run mode, got %d", requestCount)
	}
}

// TestListProcessRendersTableOutput proves the columns -> GetHeaders -> GetRow wiring
// actually reaches profile.Print for --output table, not just the JSON path every other
// test in this file drives.
func TestListProcessRendersTableOutput(t *testing.T) {
	setRoleForTest(t, "owner")

	cmd := setupTest(t, "repository-list-table", func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"values":[{"type":"repository","uuid":"{11111111-1111-1111-1111-111111111111}","name":"Widgets","full_name":"acme/widgets","slug":"widgets"}]}`))
	}, false)
	if err := cmd.Flags().Set("output", "table"); err != nil {
		t.Fatalf("cannot set output flag: %v", err)
	}

	stdout := captureStdout(t, func() {
		if err := listProcess(cmd, nil); err != nil {
			t.Fatalf("listProcess() error = %v", err)
		}
	})

	if !strings.Contains(stdout, "Widgets") {
		t.Errorf("table output = %q, want it to contain the repository name", stdout)
	}
	if !strings.Contains(stdout, "+--") {
		t.Errorf("table output = %q, want the table renderer's box-drawing border", stdout)
	}
	var probe any
	if err := json.Unmarshal([]byte(stdout), &probe); err == nil {
		t.Errorf("table output = %q, want it not to parse as JSON", stdout)
	}
}

// TestRepositoryListRoleFlagDefaultsToMember reproduces the FINAL CRITICAL GATE's priority-5
// finding: --role defaulted to "owner", which returns nothing on a first-run `bb repo list`
// against a team workspace (every repository there is workspace-owned, not owned by any
// individual member) -- an empty result the caller has no reason to expect, since they can see
// (and are a member of) every one of those repositories. The default must be "member" instead,
// which always includes a personal-workspace user's own repositories too.
func TestRepositoryListRoleFlagDefaultsToMember(t *testing.T) {
	if got := listOptions.Role.String(); got != "member" {
		t.Errorf("--role default = %q, want %q", got, "member")
	}
}
