package workspace

import (
	"encoding/json"
	"net/http"
	"strings"
	"testing"
)

func TestGetProcessSuccessWithArg(t *testing.T) {
	const slug = "acme-get-success"

	var requests []*http.Request
	cmd := setupTest(t, "workspace-get-success", func(w http.ResponseWriter, r *http.Request) {
		requests = append(requests, r)
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"type":"workspace","uuid":"{11111111-1111-1111-1111-111111111111}","name":"Acme Corp","slug":"` + slug + `"}`))
	}, false)

	stdout := captureStdout(t, func() {
		if err := getProcess(cmd, []string{slug}); err != nil {
			t.Fatalf("getProcess() error = %v", err)
		}
	})

	if len(requests) != 1 {
		t.Fatalf("expected exactly 1 request, got %d", len(requests))
	}
	if requests[0].Method != http.MethodGet {
		t.Errorf("method = %s, want GET", requests[0].Method)
	}
	wantPath := "/2.0/workspaces/" + slug
	if requests[0].URL.Path != wantPath {
		t.Errorf("path = %s, want %s", requests[0].URL.Path, wantPath)
	}

	var got Workspace
	if err := json.Unmarshal([]byte(stdout), &got); err != nil {
		t.Fatalf("cannot unmarshal printed output %q: %v", stdout, err)
	}
	if got.Slug != slug {
		t.Errorf("printed workspace slug = %q, want %q", got.Slug, slug)
	}
}

func TestGetProcessSuccessCurrentWorkspace(t *testing.T) {
	const slug = "acme-get-current"

	var requests []*http.Request
	cmd := setupTest(t, "workspace-get-current", func(w http.ResponseWriter, r *http.Request) {
		requests = append(requests, r)
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"type":"workspace","uuid":"{22222222-2222-2222-2222-222222222222}","name":"Acme Corp","slug":"` + slug + `"}`))
	}, false)
	if err := cmd.Flags().Set("workspace", slug); err != nil {
		t.Fatalf("cannot set workspace flag: %v", err)
	}

	stdout := captureStdout(t, func() {
		if err := getProcess(cmd, nil); err != nil {
			t.Fatalf("getProcess() error = %v", err)
		}
	})

	if len(requests) != 1 {
		t.Fatalf("expected exactly 1 request, got %d", len(requests))
	}
	wantPath := "/2.0/workspaces/" + slug
	if requests[0].URL.Path != wantPath {
		t.Errorf("path = %s, want %s", requests[0].URL.Path, wantPath)
	}

	var got Workspace
	if err := json.Unmarshal([]byte(stdout), &got); err != nil {
		t.Fatalf("cannot unmarshal printed output %q: %v", stdout, err)
	}
	if got.Slug != slug {
		t.Errorf("printed workspace slug = %q, want %q", got.Slug, slug)
	}
}

func TestGetProcessAPIError(t *testing.T) {
	const slug = "acme-get-error"

	cmd := setupTest(t, "workspace-get-api-error", func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusNotFound)
		_, _ = w.Write([]byte(`{"type":"error","error":{"message":"workspace not found"}}`))
	}, false)

	err := getProcess(cmd, []string{slug})
	if err == nil {
		t.Fatal("getProcess() expected an error, got nil")
	}
	if !strings.Contains(err.Error(), "cannot get workspace "+slug) {
		t.Errorf("error = %q, want it to mention the workspace slug", err.Error())
	}
	if !strings.Contains(err.Error(), "workspace not found") {
		t.Errorf("error = %q, want it to contain the BitBucket error message", err.Error())
	}
}

func TestGetProcessDryRun(t *testing.T) {
	const slug = "acme-get-dry-run"

	var requestCount int
	cmd := setupTest(t, "workspace-get-dry-run", func(http.ResponseWriter, *http.Request) { requestCount++ }, true)

	if err := WorkspaceCache.Set(slug, Workspace{Slug: slug, Name: "Acme Corp"}); err != nil {
		t.Fatalf("cannot prime workspace cache: %v", err)
	}

	if err := getProcess(cmd, []string{slug}); err != nil {
		t.Fatalf("getProcess() error = %v", err)
	}
	if requestCount != 0 {
		t.Errorf("expected no HTTP request in dry-run mode, got %d", requestCount)
	}
}
