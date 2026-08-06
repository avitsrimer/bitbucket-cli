package repository

import (
	"net/http"
	"strings"
	"testing"
)

func TestGetRepositorySlugsSuccess(t *testing.T) {
	var requests []*http.Request
	cmd := setupTest(t, "repository-slugs-success", func(w http.ResponseWriter, r *http.Request) {
		requests = append(requests, r)
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"values":[` +
			`{"type":"repository","uuid":"{11111111-1111-1111-1111-111111111111}","name":"Zeta","full_name":"acme/zeta","slug":"zeta"},` +
			`{"type":"repository","uuid":"{22222222-2222-2222-2222-222222222222}","name":"Acme","full_name":"acme/acme-repo","slug":"acme-repo"}` +
			`]}`))
	}, false)

	slugs, err := GetRepositorySlugs(t.Context(), cmd)
	if err != nil {
		t.Fatalf("GetRepositorySlugs() error = %v", err)
	}
	want := []string{"acme-repo", "zeta"}
	if len(slugs) != len(want) || slugs[0] != want[0] || slugs[1] != want[1] {
		t.Errorf("slugs = %v, want %v (sorted case-insensitively)", slugs, want)
	}
	if len(requests) != 1 {
		t.Fatalf("expected exactly 1 request, got %d", len(requests))
	}
	wantPath := "/2.0/repositories/" + testWorkspaceSlug
	if requests[0].URL.Path != wantPath {
		t.Errorf("path = %s, want %s", requests[0].URL.Path, wantPath)
	}
}

func TestGetRepositorySlugsAPIError(t *testing.T) {
	cmd := setupTest(t, "repository-slugs-api-error", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusInternalServerError)
		_, _ = w.Write([]byte(`{"type":"error","error":{"message":"server exploded"}}`))
	}, false)

	_, err := GetRepositorySlugs(t.Context(), cmd)
	if err == nil {
		t.Fatal("GetRepositorySlugs() expected an error, got nil")
	}
	if !strings.Contains(err.Error(), "server exploded") {
		t.Errorf("error = %q, want it to contain the BitBucket error message", err.Error())
	}
}
