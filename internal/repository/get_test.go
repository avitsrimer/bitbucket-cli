package repository

import (
	"encoding/json"
	"net/http"
	"strings"
	"testing"

	"github.com/avitsrimer/bitbucket-cli/internal/common"
)

func TestGetProcessSuccessWithArg(t *testing.T) {
	const slug = "acme-get-success"

	var requests []*http.Request
	cmd := setupTest(t, "repository-get-success", func(w http.ResponseWriter, r *http.Request) {
		requests = append(requests, r)
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"type":"repository","uuid":"{11111111-1111-1111-1111-111111111111}","name":"BB","full_name":"` + testWorkspaceSlug + `/` + slug + `","slug":"` + slug + `"}`))
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
	wantPath := "/2.0/repositories/" + testWorkspaceSlug + "/" + slug
	if requests[0].URL.Path != wantPath {
		t.Errorf("path = %s, want %s", requests[0].URL.Path, wantPath)
	}

	var got Repository
	if err := json.Unmarshal([]byte(stdout), &got); err != nil {
		t.Fatalf("cannot unmarshal printed output %q: %v", stdout, err)
	}
	if got.Slug != slug {
		t.Errorf("printed repository slug = %q, want %q", got.Slug, slug)
	}
}

func TestGetProcessSuccessCurrentRepository(t *testing.T) {
	const slug = "acme-get-current"

	var requests []*http.Request
	cmd := setupTest(t, "repository-get-current", func(w http.ResponseWriter, r *http.Request) {
		requests = append(requests, r)
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"type":"repository","uuid":"{22222222-2222-2222-2222-222222222222}","name":"BB","full_name":"` + testWorkspaceSlug + `/` + slug + `","slug":"` + slug + `"}`))
	}, false)
	if err := cmd.Flags().Set("repository", slug); err != nil {
		t.Fatalf("cannot set repository flag: %v", err)
	}

	stdout := captureStdout(t, func() {
		if err := getProcess(cmd, nil); err != nil {
			t.Fatalf("getProcess() error = %v", err)
		}
	})

	if len(requests) != 1 {
		t.Fatalf("expected exactly 1 request, got %d", len(requests))
	}
	wantPath := "/2.0/repositories/" + testWorkspaceSlug + "/" + slug
	if requests[0].URL.Path != wantPath {
		t.Errorf("path = %s, want %s", requests[0].URL.Path, wantPath)
	}

	var got Repository
	if err := json.Unmarshal([]byte(stdout), &got); err != nil {
		t.Fatalf("cannot unmarshal printed output %q: %v", stdout, err)
	}
	if got.Slug != slug {
		t.Errorf("printed repository slug = %q, want %q", got.Slug, slug)
	}
}

func TestGetProcessAPIError(t *testing.T) {
	const slug = "acme-get-error"

	cmd := setupTest(t, "repository-get-api-error", func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusNotFound)
		_, _ = w.Write([]byte(`{"type":"error","error":{"message":"repository not found"}}`))
	}, false)

	err := getProcess(cmd, []string{slug})
	if err == nil {
		t.Fatal("getProcess() expected an error, got nil")
	}
	if !strings.Contains(err.Error(), "cannot get repository "+slug) {
		t.Errorf("error = %q, want it to mention the repository slug", err.Error())
	}
	if !strings.Contains(err.Error(), "repository not found") {
		t.Errorf("error = %q, want it to contain the BitBucket error message", err.Error())
	}
}

func TestGetProcessDryRun(t *testing.T) {
	const slug = "acme-get-dry-run"

	var requestCount int
	cmd := setupTest(t, "repository-get-dry-run", func(http.ResponseWriter, *http.Request) { requestCount++ }, true)

	// The cache round-trips through JSON (see Cache.Set/Get), and Repository.UnmarshalJSON's
	// Validate call requires ID/Name/FullName, so the fixture needs all three to actually survive
	// being read back rather than only ever resolving via an in-memory shortcut.
	id, err := common.ParseUUID("{33333333-3333-3333-3333-333333333333}")
	if err != nil {
		t.Fatalf("cannot parse fixture uuid: %v", err)
	}
	if err := RepositoryCache.Set(testWorkspaceSlug+"/"+slug, Repository{ID: id, Slug: slug, Name: "BB", FullName: testWorkspaceSlug + "/" + slug}); err != nil {
		t.Fatalf("cannot prime repository cache: %v", err)
	}

	stdout := captureStdout(t, func() {
		if err := getProcess(cmd, []string{slug}); err != nil {
			t.Fatalf("getProcess() error = %v", err)
		}
	})
	if requestCount != 0 {
		t.Errorf("expected no HTTP request in dry-run mode, got %d", requestCount)
	}
	// getProcess resolves the target from the cache before checking common.WhatIf: without this
	// assertion, deleting the WhatIf check entirely would still make zero requests (the cache hit
	// hides that), and this test would still pass while Print dumped the object to stdout.
	if stdout != "" {
		t.Errorf("expected no printed output in dry-run mode, got %q", stdout)
	}
}
