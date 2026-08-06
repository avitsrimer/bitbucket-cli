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

func TestListProcessSuccessPreservesAPIOrderWithoutSortFlag(t *testing.T) {
	setRoleForTest(t, "owner")

	var requests []*http.Request
	cmd := setupTest(t, "repository-list-success", func(w http.ResponseWriter, r *http.Request) {
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
	if len(repositories) != 2 || repositories[0].Slug != "zeta" || repositories[1].Slug != "acme-repo" {
		t.Errorf("repositories = %+v, want API order preserved (zeta, acme-repo) since --sort was not set", repositories)
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

// TestListProcessSortFlagChangedSorts proves the sort-guard (rule 3): core.Sort only runs when
// cmd's "sort" flag is Changed, never unconditionally against an untouched default.
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
