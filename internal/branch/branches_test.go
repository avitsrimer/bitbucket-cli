package branch

import (
	"net/http"
	"strings"
	"testing"
)

func TestGetBranchesSuccess(t *testing.T) {
	var requests []*http.Request
	cmd := setupTest(t, func(w http.ResponseWriter, r *http.Request) {
		requests = append(requests, r)
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"values":[{"type":"branch","name":"main"}]}`))
	}, false)

	branches, err := GetBranches(t.Context(), cmd)
	if err != nil {
		t.Fatalf("GetBranches() error = %v", err)
	}
	if len(branches) != 1 || branches[0].Name != "main" {
		t.Errorf("branches = %+v, want a single branch main", branches)
	}
	if len(requests) != 1 {
		t.Fatalf("expected exactly 1 request, got %d", len(requests))
	}
}

func TestGetBranchesAPIError(t *testing.T) {
	cmd := setupTest(t, func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusInternalServerError)
		_, _ = w.Write([]byte(`{"type":"error","error":{"message":"server exploded"}}`))
	}, false)

	_, err := GetBranches(t.Context(), cmd)
	if err == nil {
		t.Fatal("GetBranches() expected an error, got nil")
	}
	if !strings.Contains(err.Error(), "server exploded") {
		t.Errorf("error = %q, want it to contain the BitBucket error message", err.Error())
	}
}

func TestGetBranchNamesSuccessSortedCaseInsensitively(t *testing.T) {
	var requests []*http.Request
	cmd := setupTest(t, func(w http.ResponseWriter, r *http.Request) {
		requests = append(requests, r)
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"values":[` +
			`{"type":"branch","name":"Zeta"},` +
			`{"type":"branch","name":"alpha"}` +
			`]}`))
	}, false)

	names, err := GetBranchNames(t.Context(), cmd, nil, "")
	if err != nil {
		t.Fatalf("GetBranchNames() error = %v", err)
	}
	want := []string{"alpha", "Zeta"}
	if len(names) != len(want) || names[0] != want[0] || names[1] != want[1] {
		t.Errorf("names = %v, want %v (sorted case-insensitively)", names, want)
	}
	if len(requests) != 1 {
		t.Fatalf("expected exactly 1 request, got %d", len(requests))
	}
}

func TestGetBranchNamesAPIError(t *testing.T) {
	cmd := setupTest(t, func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusInternalServerError)
		_, _ = w.Write([]byte(`{"type":"error","error":{"message":"server exploded"}}`))
	}, false)

	_, err := GetBranchNames(t.Context(), cmd, nil, "")
	if err == nil {
		t.Fatal("GetBranchNames() expected an error, got nil")
	}
	if !strings.Contains(err.Error(), "server exploded") {
		t.Errorf("error = %q, want it to contain the BitBucket error message", err.Error())
	}
}

func TestBranchesTableables(t *testing.T) {
	target := Branches{
		{Name: "main"},
		{Name: "dev"},
	}

	if target.Size() != 2 {
		t.Errorf("Size() = %d, want 2", target.Size())
	}
	if row := target.GetRowAt(0, []string{"name"}); len(row) != 1 || row[0] != "main" {
		t.Errorf("GetRowAt(0, ...) = %v, want [\"main\"]", row)
	}
	if row := target.GetRowAt(1, []string{"name"}); len(row) != 1 || row[0] != "dev" {
		t.Errorf("GetRowAt(1, ...) = %v, want [\"dev\"]", row)
	}
	if row := target.GetRowAt(-1, []string{"name"}); len(row) != 0 {
		t.Errorf("GetRowAt(-1, ...) = %v, want empty", row)
	}
	if row := target.GetRowAt(5, []string{"name"}); len(row) != 0 {
		t.Errorf("GetRowAt(5, ...) = %v, want empty", row)
	}
}
