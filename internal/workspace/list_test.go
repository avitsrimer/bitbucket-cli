package workspace

import (
	"encoding/json"
	"net/http"
	"strings"
	"testing"
)

// TestListProcessDefaultSortsByName proves the real command's documented default ("--sort string
// Column to sort by (default \"name\")") actually applies when --sort is not passed: columns
// marks "name" as its DefaultSorter, and common.SortFlagValue resolves that default from the flag
// itself, so this must always sort ascending by name -- not merely preserve whatever order the
// API happened to return. Both fixture entries carry a distinct "name" (the column Compare
// actually sorts by): omitting it, as an earlier version of this fixture did, makes every
// comparison equal and the sort a no-op regardless of whether sorting ran at all. The fixture's
// API order (Zeta Corp, then Acme Corp) is deliberately reversed from the expected sorted order,
// so the two orders can never be confused.
func TestListProcessDefaultSortsByName(t *testing.T) {
	var requests []*http.Request
	cmd := setupTest(t, "workspace-list-default-sort", func(w http.ResponseWriter, r *http.Request) {
		requests = append(requests, r)
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"values":[` +
			`{"type":"workspace_access","administrator":false,"workspace":{"type":"workspace_base","slug":"zeta","name":"Zeta Corp"}},` +
			`{"type":"workspace_access","administrator":false,"workspace":{"type":"workspace_base","slug":"acme","name":"Acme Corp"}}` +
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
	if requests[0].URL.Path != "/2.0/user/workspaces" {
		t.Errorf("path = %s, want /2.0/user/workspaces", requests[0].URL.Path)
	}

	var workspaces []Workspace
	if err := json.Unmarshal([]byte(stdout), &workspaces); err != nil {
		t.Fatalf("cannot unmarshal printed output %q: %v", stdout, err)
	}
	if len(workspaces) != 2 || workspaces[0].Slug != "acme" || workspaces[1].Slug != "zeta" {
		t.Errorf("workspaces = %+v, want sorted by name ascending (Acme Corp, Zeta Corp) by default, not the API's raw order (Zeta Corp, Acme Corp)", workspaces)
	}
}

// TestListProcessSortFlagChangedSorts proves --sort actually selects the comparator
// core.Sort runs: reading it via common.SortFlagValue(cmd) (cmd's own --sort flag, not a
// package-level SortBy.Value binding that is only ever populated on the real command) means
// the process sorts identically whether cmd is the real command or, as here, a standalone
// test command carrying its own --sort flag.
func TestListProcessSortFlagChangedSorts(t *testing.T) {
	cmd := setupTest(t, "workspace-list-sorted", func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"values":[` +
			`{"type":"workspace_access","administrator":false,"workspace":{"type":"workspace_base","name":"Zeta Corp","slug":"zeta"}},` +
			`{"type":"workspace_access","administrator":false,"workspace":{"type":"workspace_base","name":"Acme Corp","slug":"acme"}}` +
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

	var workspaces []Workspace
	if err := json.Unmarshal([]byte(stdout), &workspaces); err != nil {
		t.Fatalf("cannot unmarshal printed output %q: %v", stdout, err)
	}
	if len(workspaces) != 2 || workspaces[0].Slug != "acme" || workspaces[1].Slug != "zeta" {
		t.Errorf("workspaces = %+v, want sorted by name ascending (acme, zeta) once --sort is Changed", workspaces)
	}
}

func TestListProcessNoResults(t *testing.T) {
	var requests []*http.Request
	cmd := setupTest(t, "workspace-list-empty", func(w http.ResponseWriter, r *http.Request) {
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
	if strings.TrimSpace(stdout) != "No workspace found" {
		t.Errorf("stdout = %q, want %q printed on stdout", stdout, "No workspace found")
	}
}

func TestListProcessAPIError(t *testing.T) {
	cmd := setupTest(t, "workspace-list-api-error", func(w http.ResponseWriter, _ *http.Request) {
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
	var requestCount int
	cmd := setupTest(t, "workspace-list-dry-run", func(http.ResponseWriter, *http.Request) { requestCount++ }, true)

	if err := listProcess(cmd, nil); err != nil {
		t.Fatalf("listProcess() error = %v", err)
	}
	if requestCount != 0 {
		t.Errorf("expected no HTTP request in dry-run mode, got %d", requestCount)
	}
}

// TestListProcessRendersTableOutput proves the columns -> GetHeaders -> GetRow wiring rule 1
// covers actually reaches profile.Print for --output table, not just the JSON path every other
// test in this file drives.
func TestListProcessRendersTableOutput(t *testing.T) {
	cmd := setupTest(t, "workspace-list-table", func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"values":[{"type":"workspace_access","administrator":false,"workspace":{"type":"workspace_base","slug":"acme","name":"Acme Corp"}}]}`))
	}, false)
	if err := cmd.Flags().Set("output", "table"); err != nil {
		t.Fatalf("cannot set output flag: %v", err)
	}

	stdout := captureStdout(t, func() {
		if err := listProcess(cmd, nil); err != nil {
			t.Fatalf("listProcess() error = %v", err)
		}
	})

	if !strings.Contains(stdout, "Acme Corp") {
		t.Errorf("table output = %q, want it to contain the workspace name", stdout)
	}
	if !strings.Contains(stdout, "+--") {
		t.Errorf("table output = %q, want tablewriter's box-drawing border", stdout)
	}
	var probe any
	if err := json.Unmarshal([]byte(stdout), &probe); err == nil {
		t.Errorf("table output = %q, want it not to parse as JSON", stdout)
	}
}
