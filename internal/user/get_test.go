package user

import (
	"encoding/json"
	"net/http"
	"strings"
	"testing"
)

func TestGetProcessSuccess(t *testing.T) {
	const profileName = "user-get-success"
	const targetID = "11111111-1111-1111-1111-111111111111"

	var requests []*http.Request
	cmd := setupTest(t, profileName, func(w http.ResponseWriter, r *http.Request) {
		requests = append(requests, r)
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"uuid":"{` + targetID + `}","display_name":"Jane Doe","account_id":"abc-123"}`))
	}, false)

	stdout := captureStdout(t, func() {
		if err := getProcess(cmd, []string{targetID}); err != nil {
			t.Fatalf("getProcess() error = %v", err)
		}
	})

	if len(requests) != 1 {
		t.Fatalf("expected exactly 1 request, got %d", len(requests))
	}
	if requests[0].Method != http.MethodGet {
		t.Errorf("method = %s, want GET", requests[0].Method)
	}

	var got User
	if err := json.Unmarshal([]byte(stdout), &got); err != nil {
		t.Fatalf("cannot unmarshal printed output %q: %v", stdout, err)
	}
	if got.Name != "Jane Doe" {
		t.Errorf("printed user display name = %q, want %q", got.Name, "Jane Doe")
	}
}

func TestGetProcessAPIError(t *testing.T) {
	const profileName = "user-get-api-error"
	const targetID = "22222222-2222-2222-2222-222222222222"

	cmd := setupTest(t, profileName, func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusNotFound)
		_, _ = w.Write([]byte(`{"type":"error","error":{"message":"user not found"}}`))
	}, false)

	err := getProcess(cmd, []string{targetID})
	if err == nil {
		t.Fatal("getProcess() expected an error, got nil")
	}
	if !strings.Contains(err.Error(), "user not found") {
		t.Errorf("error = %q, want it to contain the BitBucket error message", err.Error())
	}
}

func TestGetProcessRendersTableOutput(t *testing.T) {
	const profileName = "user-get-table"
	const targetID = "33333333-3333-3333-3333-333333333333"

	cmd := setupTest(t, profileName, func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"uuid":"{` + targetID + `}","display_name":"Jane Doe","username":"jdoe"}`))
	}, false)
	_ = cmd.Flags().Set("output", "table")

	stdout := captureStdout(t, func() {
		if err := getProcess(cmd, []string{targetID}); err != nil {
			t.Fatalf("getProcess() error = %v", err)
		}
	})

	if !strings.Contains(stdout, "Jane Doe") || !strings.Contains(stdout, "jdoe") {
		t.Errorf("table output = %q, want it to contain the user's name and username", stdout)
	}
}

func TestGetProcessDryRun(t *testing.T) {
	const profileName = "user-get-dry-run"

	var requestCount int
	cmd := setupTest(t, profileName, func(http.ResponseWriter, *http.Request) { requestCount++ }, true)

	if err := getProcess(cmd, []string{"44444444-4444-4444-4444-444444444444"}); err != nil {
		t.Fatalf("getProcess() error = %v", err)
	}
	if requestCount != 0 {
		t.Errorf("expected no HTTP request in dry-run mode, got %d", requestCount)
	}
}
