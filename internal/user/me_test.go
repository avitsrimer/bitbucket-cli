package user

import (
	"encoding/json"
	"net/http"
	"strings"
	"testing"
)

func withMeOptions(t *testing.T, mutate func()) {
	t.Helper()
	old := meOptions.Emails
	t.Cleanup(func() { meOptions.Emails = old })
	mutate()
}

func TestMeProcessSuccess(t *testing.T) {
	withMeOptions(t, func() { meOptions.Emails = false })
	const profileName = "user-me-success"

	var requests []*http.Request
	cmd := setupTest(t, profileName, func(w http.ResponseWriter, r *http.Request) {
		requests = append(requests, r)
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"uuid":"{55555555-5555-5555-5555-555555555555}","display_name":"Current User"}`))
	}, false)
	t.Cleanup(func() { removeCacheEntry(profileName + ":me") })

	stdout := captureStdout(t, func() {
		if err := meProcess(cmd, nil); err != nil {
			t.Fatalf("meProcess() error = %v", err)
		}
	})

	if len(requests) != 1 {
		t.Fatalf("expected exactly 1 request, got %d", len(requests))
	}
	wantPath := "/2.0/user"
	if requests[0].URL.Path != wantPath {
		t.Errorf("path = %s, want %s", requests[0].URL.Path, wantPath)
	}

	var got User
	if err := json.Unmarshal([]byte(stdout), &got); err != nil {
		t.Fatalf("cannot unmarshal printed output %q: %v", stdout, err)
	}
	if got.Name != "Current User" {
		t.Errorf("printed user display name = %q, want %q", got.Name, "Current User")
	}
}

func TestMeProcessAPIError(t *testing.T) {
	withMeOptions(t, func() { meOptions.Emails = false })
	const profileName = "user-me-api-error"

	cmd := setupTest(t, profileName, func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusUnauthorized)
		_, _ = w.Write([]byte(`{"type":"error","error":{"message":"invalid credentials"}}`))
	}, false)
	t.Cleanup(func() { removeCacheEntry(profileName + ":me") })

	err := meProcess(cmd, nil)
	if err == nil {
		t.Fatal("meProcess() expected an error, got nil")
	}
	if !strings.Contains(err.Error(), "invalid credentials") {
		t.Errorf("error = %q, want it to contain the BitBucket error message", err.Error())
	}
}

func TestMeProcessEmailsSuccess(t *testing.T) {
	withMeOptions(t, func() { meOptions.Emails = true })
	const profileName = "user-me-emails-success"

	var requests []*http.Request
	cmd := setupTest(t, profileName, func(w http.ResponseWriter, r *http.Request) {
		requests = append(requests, r)
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"values":[{"type":"email","email":"jane@example.com","is_primary":true,"is_confirmed":true}]}`))
	}, false)

	stdout := captureStdout(t, func() {
		if err := meProcess(cmd, nil); err != nil {
			t.Fatalf("meProcess() error = %v", err)
		}
	})

	if len(requests) != 1 {
		t.Fatalf("expected exactly 1 request, got %d", len(requests))
	}
	wantPath := "/2.0/user/emails"
	if requests[0].URL.Path != wantPath {
		t.Errorf("path = %s, want %s", requests[0].URL.Path, wantPath)
	}

	var got []Email
	if err := json.Unmarshal([]byte(stdout), &got); err != nil {
		t.Fatalf("cannot unmarshal printed output %q: %v", stdout, err)
	}
	if len(got) != 1 || got[0].Email != "jane@example.com" {
		t.Errorf("printed emails = %+v, want a single jane@example.com entry", got)
	}
}

func TestMeProcessEmailsAPIError(t *testing.T) {
	withMeOptions(t, func() { meOptions.Emails = true })
	const profileName = "user-me-emails-api-error"

	cmd := setupTest(t, profileName, func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusInternalServerError)
		_, _ = w.Write([]byte(`{"type":"error","error":{"message":"server exploded"}}`))
	}, false)

	err := meProcess(cmd, nil)
	if err == nil {
		t.Fatal("meProcess() expected an error, got nil")
	}
	if !strings.Contains(err.Error(), "server exploded") {
		t.Errorf("error = %q, want it to contain the BitBucket error message", err.Error())
	}
}

func TestMeProcessRendersTableOutput(t *testing.T) {
	withMeOptions(t, func() { meOptions.Emails = false })
	const profileName = "user-me-table"

	cmd := setupTest(t, profileName, func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"uuid":"{66666666-6666-6666-6666-666666666666}","display_name":"Current User","username":"cuser"}`))
	}, false)
	t.Cleanup(func() { removeCacheEntry(profileName + ":me") })
	_ = cmd.Flags().Set("output", "table")

	stdout := captureStdout(t, func() {
		if err := meProcess(cmd, nil); err != nil {
			t.Fatalf("meProcess() error = %v", err)
		}
	})

	if !strings.Contains(stdout, "Current User") || !strings.Contains(stdout, "cuser") {
		t.Errorf("table output = %q, want it to contain the user's name and username", stdout)
	}
}
