package workspace

import (
	"encoding/json"
	"net/http"
	"strings"
	"testing"
)

func TestListProcessSuccessPreservesAPIOrderWithoutSortFlag(t *testing.T) {
	var requests []*http.Request
	cmd := setupTest(t, "workspace-list-success", func(w http.ResponseWriter, r *http.Request) {
		requests = append(requests, r)
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"values":[` +
			`{"type":"workspace_access","administrator":false,"workspace":{"type":"workspace_base","slug":"zeta"}},` +
			`{"type":"workspace_access","administrator":false,"workspace":{"type":"workspace_base","slug":"acme"}}` +
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
	if len(workspaces) != 2 || workspaces[0].Slug != "zeta" || workspaces[1].Slug != "acme" {
		t.Errorf("workspaces = %+v, want API order preserved (zeta, acme) since --sort was not set", workspaces)
	}
}

// TestListProcessSortFlagChangedSorts proves the sort-guard (rule 3): core.Sort only runs when
// cmd's "sort" flag is Changed, never unconditionally against an untouched default.
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
