package profile

import (
	"context"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strconv"
	"testing"

	"github.com/spf13/cobra"
)

// These tests live in package profile (not profile_test) specifically to reach
// resolvePageLengthAndLimit, nextPageURL, basicAuthorization, and sendOAuthTokenRequest, none of
// which are exported: profile_client_test.go's external package cannot call them directly.

func TestResolvePageLengthAndLimit(t *testing.T) {
	// resolvePageLengthAndLimit reads --page-length/--limit as ints via cmd.Flags().GetInt.
	newIntCmd := func(pageLength, limit int, pageLengthSet, limitSet bool) *cobra.Command {
		cmd := &cobra.Command{}
		cmd.Flags().Int("page-length", 0, "")
		cmd.Flags().Int("limit", 0, "")
		if pageLengthSet {
			_ = cmd.Flags().Set("page-length", strconv.Itoa(pageLength))
		}
		if limitSet {
			_ = cmd.Flags().Set("limit", strconv.Itoa(limit))
		}
		return cmd
	}

	t.Run("uses the default page length when nothing is set", func(t *testing.T) {
		cmd := newIntCmd(0, 0, false, false)
		pageLength, limit := resolvePageLengthAndLimit(cmd, 50)
		if pageLength != 50 || limit != 0 {
			t.Errorf("pageLength, limit = %d, %d, want 50, 0", pageLength, limit)
		}
	})

	t.Run("uses the page-length flag when set", func(t *testing.T) {
		cmd := newIntCmd(10, 0, true, false)
		pageLength, limit := resolvePageLengthAndLimit(cmd, 50)
		if pageLength != 10 || limit != 0 {
			t.Errorf("pageLength, limit = %d, %d, want 10, 0", pageLength, limit)
		}
	})

	t.Run("shrinks page length to limit when limit is smaller", func(t *testing.T) {
		cmd := newIntCmd(0, 3, false, true)
		pageLength, limit := resolvePageLengthAndLimit(cmd, 50)
		if pageLength != 3 || limit != 3 {
			t.Errorf("pageLength, limit = %d, %d, want 3, 3", pageLength, limit)
		}
	})

	t.Run("leaves page length alone when limit is larger", func(t *testing.T) {
		cmd := newIntCmd(10, 100, true, true)
		pageLength, limit := resolvePageLengthAndLimit(cmd, 50)
		if pageLength != 10 || limit != 100 {
			t.Errorf("pageLength, limit = %d, %d, want 10, 100", pageLength, limit)
		}
	})

	t.Run("handles a nil command", func(t *testing.T) {
		pageLength, limit := resolvePageLengthAndLimit(nil, 50)
		if pageLength != 50 || limit != 0 {
			t.Errorf("pageLength, limit = %d, %d, want 50, 0", pageLength, limit)
		}
	})
}

func TestNextPageURL(t *testing.T) {
	t.Run("preserves original query parameters missing from next", func(t *testing.T) {
		original := url.Values{"q": {`state="OPEN"`}}
		got, err := nextPageURL("https://api.example.com/x?page=2", original, 0, 0, 10)
		if err != nil {
			t.Fatalf("nextPageURL() error = %v", err)
		}
		parsed, _ := url.Parse(got)
		if parsed.Query().Get("q") != `state="OPEN"` {
			t.Errorf("q = %q, want the original filter preserved", parsed.Query().Get("q"))
		}
	})

	t.Run("does not overwrite a query parameter already present in next", func(t *testing.T) {
		original := url.Values{"q": {"original"}}
		got, err := nextPageURL("https://api.example.com/x?page=2&q=fromnext", original, 0, 0, 10)
		if err != nil {
			t.Fatalf("nextPageURL() error = %v", err)
		}
		parsed, _ := url.Parse(got)
		if parsed.Query().Get("q") != "fromnext" {
			t.Errorf("q = %q, want the next URL's own value preserved", parsed.Query().Get("q"))
		}
	})

	t.Run("shrinks pagelen once the limit is close to being reached", func(t *testing.T) {
		got, err := nextPageURL("https://api.example.com/x?page=2&pagelen=10", url.Values{}, 12, 8, 10)
		if err != nil {
			t.Fatalf("nextPageURL() error = %v", err)
		}
		parsed, _ := url.Parse(got)
		if parsed.Query().Get("pagelen") != "4" {
			t.Errorf("pagelen = %q, want \"4\" (limit 12 - resourceCount 8)", parsed.Query().Get("pagelen"))
		}
	})

	t.Run("leaves pagelen alone when there is no limit", func(t *testing.T) {
		got, err := nextPageURL("https://api.example.com/x?page=2&pagelen=10", url.Values{}, 0, 8, 10)
		if err != nil {
			t.Fatalf("nextPageURL() error = %v", err)
		}
		parsed, _ := url.Parse(got)
		if parsed.Query().Get("pagelen") != "10" {
			t.Errorf("pagelen = %q, want unchanged \"10\"", parsed.Query().Get("pagelen"))
		}
	})

	t.Run("returns an error for an unparsable next URL", func(t *testing.T) {
		_, err := nextPageURL("://not-a-url", url.Values{}, 0, 0, 10)
		if err == nil {
			t.Fatal("nextPageURL() expected an error for an invalid URL, got nil")
		}
	})
}

func TestBasicAuthorization(t *testing.T) {
	got := basicAuthorization("alice", "s3cr3t")
	if got != "Basic YWxpY2U6czNjcjN0" {
		t.Errorf("basicAuthorization() = %q, want %q", got, "Basic YWxpY2U6czNjcjN0")
	}
}

// TestResolveAuthorizationUsesBasicAuthForUserPassword proves resolveAuthorization sends Basic
// auth (not the OAuth path) when the profile has a plain user/password, by asserting on the
// actual header a real request carries.
func TestResolveAuthorizationUsesBasicAuthForUserPassword(t *testing.T) {
	var gotAuthorization string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotAuthorization = r.Header.Get("Authorization")
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{}`))
	}))
	defer server.Close()

	apiRoot, err := url.Parse(server.URL)
	if err != nil {
		t.Fatalf("cannot parse test server URL: %v", err)
	}
	target := &Profile{APIRoot: apiRoot, User: "alice", Password: "s3cr3t"}

	if err := target.Get(context.Background(), "/repo", &struct{}{}); err != nil {
		t.Fatalf("Get() error = %v", err)
	}

	want := basicAuthorization("alice", "s3cr3t")
	if gotAuthorization != want {
		t.Errorf("Authorization header = %q, want %q", gotAuthorization, want)
	}
}

// TestSendOAuthTokenRequestSendsFormEncodedBody is a regression test for the OAuth2 token
// endpoint: it requires application/x-www-form-urlencoded (not JSON, which go-request picked
// automatically for a map[string]string payload before the net/http rewrite made this explicit).
func TestSendOAuthTokenRequestSendsFormEncodedBody(t *testing.T) {
	var gotContentType, gotAuthorization string
	var gotForm url.Values
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotContentType = r.Header.Get("Content-Type")
		gotAuthorization = r.Header.Get("Authorization")
		if err := r.ParseForm(); err != nil {
			t.Errorf("cannot parse posted form: %v", err)
		}
		gotForm = r.PostForm
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"access_token":"tok","token_type":"bearer","expires_in":3600}`))
	}))
	defer server.Close()

	old := oauthTokenURL
	oauthTokenURL = server.URL
	defer func() { oauthTokenURL = old }()

	result, err := sendOAuthTokenRequest(context.Background(), "client-id", "client-secret", map[string]string{
		"grant_type":    "refresh_token",
		"refresh_token": "the-refresh-token",
	})
	if err != nil {
		t.Fatalf("sendOAuthTokenRequest() error = %v", err)
	}
	if result.StatusCode != http.StatusOK {
		t.Errorf("StatusCode = %d, want 200", result.StatusCode)
	}
	if gotContentType != "application/x-www-form-urlencoded" {
		t.Errorf("Content-Type = %q, want application/x-www-form-urlencoded", gotContentType)
	}
	wantAuth := basicAuthorization("client-id", "client-secret")
	if gotAuthorization != wantAuth {
		t.Errorf("Authorization = %q, want %q", gotAuthorization, wantAuth)
	}
	if gotForm.Get("grant_type") != "refresh_token" || gotForm.Get("refresh_token") != "the-refresh-token" {
		t.Errorf("posted form = %v, want grant_type=refresh_token and refresh_token=the-refresh-token", gotForm)
	}
}
