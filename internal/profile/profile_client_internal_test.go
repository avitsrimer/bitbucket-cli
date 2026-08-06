package profile

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strconv"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/go-pkgz/lgr"
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
		pageLength, limit := resolvePageLengthAndLimit(cmd, 50, true)
		if pageLength != 50 || limit != 0 {
			t.Errorf("pageLength, limit = %d, %d, want 50, 0", pageLength, limit)
		}
	})

	t.Run("uses the page-length flag when set", func(t *testing.T) {
		cmd := newIntCmd(10, 0, true, false)
		pageLength, limit := resolvePageLengthAndLimit(cmd, 50, true)
		if pageLength != 10 || limit != 0 {
			t.Errorf("pageLength, limit = %d, %d, want 10, 0", pageLength, limit)
		}
	})

	t.Run("shrinks page length to limit when limit is smaller", func(t *testing.T) {
		cmd := newIntCmd(0, 3, false, true)
		pageLength, limit := resolvePageLengthAndLimit(cmd, 50, true)
		if pageLength != 3 || limit != 3 {
			t.Errorf("pageLength, limit = %d, %d, want 3, 3", pageLength, limit)
		}
	})

	t.Run("leaves page length alone when limit is larger", func(t *testing.T) {
		cmd := newIntCmd(10, 100, true, true)
		pageLength, limit := resolvePageLengthAndLimit(cmd, 50, true)
		if pageLength != 10 || limit != 100 {
			t.Errorf("pageLength, limit = %d, %d, want 10, 100", pageLength, limit)
		}
	})

	t.Run("handles a nil command", func(t *testing.T) {
		pageLength, limit := resolvePageLengthAndLimit(nil, 50, true)
		if pageLength != 50 || limit != 0 {
			t.Errorf("pageLength, limit = %d, %d, want 50, 0", pageLength, limit)
		}
	})

	// TestResolvePageLengthAndLimitIgnoresLimitWhenNotHonored is a regression test: GetAllUnbounded
	// (used for internal id-resolution queries, e.g. counting open pull requests to detect
	// ambiguity) must not have its page size shrunk by a --limit flag belonging to the command's
	// own, unrelated output query -- honorLimit=false must behave as if --limit were never set,
	// for both the returned limit and the page length derived from it.
	t.Run("ignores the limit flag entirely when honorLimit is false", func(t *testing.T) {
		cmd := newIntCmd(0, 1, false, true)
		pageLength, limit := resolvePageLengthAndLimit(cmd, 50, false)
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

// TestLoadAccessTokenReturnsErrNoAccessTokenWhenNothingCached pins loadAccessToken's contract: a
// nil error must mean profile.token is now non-nil. When nothing can be found (no in-memory
// token, no plain AccessToken, no cache file, no vault entry), it must return ErrNoAccessToken
// rather than silently returning nil while leaving profile.token nil.
func TestLoadAccessTokenReturnsErrNoAccessTokenWhenNothingCached(t *testing.T) {
	target := &Profile{Name: "load-access-token-nothing-cached-test", VaultKey: "bitbucket-cli-test-nonexistent-vault-key"}

	err := target.loadAccessToken(context.Background())
	if !errors.Is(err, ErrNoAccessToken) {
		t.Fatalf("loadAccessToken() error = %v, want ErrNoAccessToken", err)
	}
	if target.token != nil {
		t.Errorf("token = %+v, want nil when loadAccessToken could not find one", target.token)
	}
}

// TestAuthorizeDoesNotPanicWithNoCachedAccessToken proves authorize falls through to the OAuth2
// client-credentials flow, instead of dereferencing a nil profile.token, when loadAccessToken finds
// no cached token at all (e.g. an OAuth client-ID/secret profile before its first token exchange).
func TestAuthorizeDoesNotPanicWithNoCachedAccessToken(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"access_token":"fresh-token","token_type":"bearer","expires_in":3600}`))
	}))
	defer server.Close()

	old := oauthTokenURL
	oauthTokenURL = server.URL
	defer func() { oauthTokenURL = old }()

	target := &Profile{
		Name:         "authorize-no-cached-token-test",
		ClientID:     "client-id",
		ClientSecret: "client-secret",
		VaultKey:     "bitbucket-cli-test-nonexistent-vault-key",
	}

	authorization, err := target.authorize(context.Background())
	if err != nil {
		t.Fatalf("authorize() error = %v", err)
	}
	if authorization != "Bearer fresh-token" {
		t.Errorf("authorize() = %q, want %q", authorization, "Bearer fresh-token")
	}
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

// TestSendOAuthTokenRequestSendsFormEncodedBody proves the OAuth2 token endpoint request is sent
// as application/x-www-form-urlencoded, not JSON.
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

// TestRetryDelayCapsRetryAfterHeader is a regression test: a Retry-After header far beyond any
// reasonable interactive wait (e.g. an hour-long rate-limit window) must be clamped to
// maxRetryAfterBackoff rather than honored verbatim.
func TestRetryDelayCapsRetryAfterHeader(t *testing.T) {
	got := retryDelay(0, http.Header{"Retry-After": []string{"3600"}})
	if got != maxRetryAfterBackoff {
		t.Errorf("retryDelay() = %s, want capped at %s", got, maxRetryAfterBackoff)
	}
}

// TestRetryDelayHonorsRetryAfterUnderCap proves a Retry-After value under the cap is used as-is.
func TestRetryDelayHonorsRetryAfterUnderCap(t *testing.T) {
	got := retryDelay(0, http.Header{"Retry-After": []string{"5"}})
	if got != 5*time.Second {
		t.Errorf("retryDelay() = %s, want %s", got, 5*time.Second)
	}
}

// TestRetryDelayTreatsZeroRetryAfterAsUseComputedBackoff proves "Retry-After: 0" falls back to the
// computed exponential backoff, the same as when the header is absent entirely, rather than being
// honored as "retry with no delay" and hammering a server that is rate-limiting us.
func TestRetryDelayTreatsZeroRetryAfterAsUseComputedBackoff(t *testing.T) {
	const attempt = 2
	want := min(initialRetryBackoff*time.Duration(1<<attempt), maxRetryBackoff)

	got := retryDelay(attempt, http.Header{"Retry-After": []string{"0"}})
	if got != want {
		t.Errorf("retryDelay() = %s, want computed backoff %s", got, want)
	}

	gotNoHeader := retryDelay(attempt, nil)
	if gotNoHeader != want {
		t.Errorf("retryDelay() with no header = %s, want the same computed backoff %s", gotNoHeader, want)
	}
}

// TestRetryDelayTreatsNegativeRetryAfterAsUseComputedBackoff covers the other invalid value: a
// negative Retry-After should not be honored either.
func TestRetryDelayTreatsNegativeRetryAfterAsUseComputedBackoff(t *testing.T) {
	want := min(initialRetryBackoff*time.Duration(1<<0), maxRetryBackoff)
	got := retryDelay(0, http.Header{"Retry-After": []string{"-5"}})
	if got != want {
		t.Errorf("retryDelay() = %s, want computed backoff %s", got, want)
	}
}

// TestIsRetryableStatusRestrictsGatewayErrorsToIdempotentMethods proves a gateway error
// (502/503/504) is retryable only for idempotent methods: BitBucket may already have applied a
// POST/PATCH whose response was lost to the gateway, so retrying it could create a duplicate pull
// request, comment, task, or repeat a merge/approve/decline. 429 (the request was rejected
// outright, never processed) stays retryable for every method.
func TestIsRetryableStatusRestrictsGatewayErrorsToIdempotentMethods(t *testing.T) {
	gatewayStatuses := []int{http.StatusBadGateway, http.StatusServiceUnavailable, http.StatusGatewayTimeout}

	for _, method := range []string{http.MethodGet, http.MethodHead, http.MethodOptions, http.MethodPut, http.MethodDelete} {
		for _, status := range gatewayStatuses {
			if !isRetryableStatus(method, status) {
				t.Errorf("isRetryableStatus(%s, %d) = false, want true for an idempotent method", method, status)
			}
		}
		if !isRetryableStatus(method, http.StatusTooManyRequests) {
			t.Errorf("isRetryableStatus(%s, 429) = false, want true", method)
		}
	}

	for _, method := range []string{http.MethodPost, http.MethodPatch} {
		for _, status := range gatewayStatuses {
			if isRetryableStatus(method, status) {
				t.Errorf("isRetryableStatus(%s, %d) = true, want false for a non-idempotent method", method, status)
			}
		}
		if !isRetryableStatus(method, http.StatusTooManyRequests) {
			t.Errorf("isRetryableStatus(%s, 429) = false, want true even for a non-idempotent method", method)
		}
	}
}

// TestDoRequestWithRetryDoesNotRetryPostOnGatewayError is an end-to-end regression test for the
// same restriction: a POST that gets a 502 must be reported to the caller after a single attempt,
// not retried.
func TestDoRequestWithRetryDoesNotRetryPostOnGatewayError(t *testing.T) {
	var attempts atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		attempts.Add(1)
		w.WriteHeader(http.StatusBadGateway)
	}))
	defer server.Close()

	newReq := func(ctx context.Context) (*http.Request, error) {
		return http.NewRequestWithContext(ctx, http.MethodPost, server.URL, http.NoBody)
	}

	result, err := doRequestWithRetry(context.Background(), http.MethodPost, newReq)
	if err != nil {
		t.Fatalf("doRequestWithRetry() error = %v, want the 502 response returned without error", err)
	}
	if result.StatusCode != http.StatusBadGateway {
		t.Errorf("StatusCode = %d, want %d", result.StatusCode, http.StatusBadGateway)
	}
	if got := attempts.Load(); got != 1 {
		t.Errorf("attempts = %d, want exactly 1 (no retry for a non-idempotent method on a gateway error)", got)
	}
}

// TestDoRequestWithRetryRetriesGetOnGatewayError is the idempotent-method counterpart: a GET
// facing the same 502 must still be retried.
func TestDoRequestWithRetryRetriesGetOnGatewayError(t *testing.T) {
	var attempts atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if attempts.Add(1) == 1 {
			w.WriteHeader(http.StatusBadGateway)
			return
		}
		w.WriteHeader(http.StatusOK)
	}))
	defer server.Close()

	newReq := func(ctx context.Context) (*http.Request, error) {
		return http.NewRequestWithContext(ctx, http.MethodGet, server.URL, http.NoBody)
	}

	result, err := doRequestWithRetry(context.Background(), http.MethodGet, newReq)
	if err != nil {
		t.Fatalf("doRequestWithRetry() error = %v", err)
	}
	if result.StatusCode != http.StatusOK {
		t.Errorf("StatusCode = %d, want %d", result.StatusCode, http.StatusOK)
	}
	if got := attempts.Load(); got != 2 {
		t.Errorf("attempts = %d, want exactly 2 (the 502 should have been retried once)", got)
	}
}

// TestDoRequestWithRetryDoesNotRetryPostOnTransportErrorAfterConnected proves a non-pre-send
// transport error (the connection succeeded, then failed while writing/reading, e.g. the server
// hung up mid-response) is not retried for a non-idempotent method, since the request may already
// have reached and been applied by the server.
func TestDoRequestWithRetryDoesNotRetryPostOnTransportErrorAfterConnected(t *testing.T) {
	var attempts atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		attempts.Add(1)
		hijacker, ok := w.(http.Hijacker)
		if !ok {
			t.Fatal("test server does not support hijacking")
		}
		conn, _, err := hijacker.Hijack()
		if err != nil {
			t.Fatalf("cannot hijack connection: %v", err)
		}
		_ = conn.Close() // close without writing a response: a post-connect transport failure
	}))
	defer server.Close()

	newReq := func(ctx context.Context) (*http.Request, error) {
		return http.NewRequestWithContext(ctx, http.MethodPost, server.URL, http.NoBody)
	}

	_, err := doRequestWithRetry(context.Background(), http.MethodPost, newReq)
	if err == nil {
		t.Fatal("doRequestWithRetry() expected an error, got nil")
	}
	if got := attempts.Load(); got != 1 {
		t.Errorf("attempts = %d, want exactly 1 (no retry for a non-idempotent method on an ambiguous post-connect failure)", got)
	}
}

// TestDoRequestWithRetryRetriesPostOnPreSendConnectionError proves the counterpart: a failure to
// even establish the connection (nothing was ever sent) is safe to retry regardless of method.
func TestDoRequestWithRetryRetriesPostOnPreSendConnectionError(t *testing.T) {
	var attempts int
	newReq := func(ctx context.Context) (*http.Request, error) {
		attempts++
		if attempts == 1 {
			// Port 0 on an address with nothing listening reliably fails to dial.
			return http.NewRequestWithContext(ctx, http.MethodPost, "http://127.0.0.1:0", http.NoBody)
		}
		return nil, fmt.Errorf("stop after the first retry: %w", errStopTest)
	}

	_, err := doRequestWithRetry(context.Background(), http.MethodPost, newReq)
	if !errors.Is(err, errStopTest) {
		t.Fatalf("doRequestWithRetry() error = %v, want it to have retried past the dial failure", err)
	}
	if attempts != 2 {
		t.Errorf("attempts = %d, want exactly 2 (the pre-send dial failure should have been retried once)", attempts)
	}
}

var errStopTest = errors.New("stop test")

// TestSendRedactsURLUserinfoInDebugLogs proves send's debug log lines use reqURL.Redacted()
// rather than its plain String() form, so an APIRoot carrying userinfo credentials (preserved
// verbatim by MarshalYAML/UnmarshalYAML's string-form round trip) never leaks its password in
// plain text in the logs.
func TestSendRedactsURLUserinfoInDebugLogs(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{}`))
	}))
	defer server.Close()

	apiRoot, err := url.Parse(server.URL)
	if err != nil {
		t.Fatalf("cannot parse test server URL: %v", err)
	}
	apiRoot.User = url.UserPassword("alice", "s3cr3t-userinfo-password")

	var buf bytes.Buffer
	lgr.Setup(lgr.Out(&buf), lgr.Debug)
	defer lgr.Setup() // restore the package's zero-value default logger

	target := &Profile{APIRoot: apiRoot, AccessToken: "dummy-token"}
	if err := target.Get(context.Background(), "/repo", &struct{}{}); err != nil {
		t.Fatalf("Get() error = %v", err)
	}

	logged := buf.String()
	if strings.Contains(logged, "s3cr3t-userinfo-password") {
		t.Errorf("debug log = %q, must not contain the apiRoot userinfo password in plain text", logged)
	}
	if !strings.Contains(logged, "alice") {
		t.Errorf("debug log = %q, want the apiRoot username still visible for context", logged)
	}
	if !strings.Contains(logged, "sending") || !strings.Contains(logged, "received") {
		t.Errorf("debug log = %q, want both the \"sending\" and \"received\" lines present", logged)
	}
}

// TestCodeGrantCallbackRedactsAuthorizationCodeInDebugLogs reproduces critical finding #6:
// CodeGrantCallback logged the OAuth authorization code verbatim ("[DEBUG] received code %s"),
// the one secret in this package not passed through redactWithHash before hitting the log.
func TestCodeGrantCallbackRedactsAuthorizationCodeInDebugLogs(t *testing.T) {
	var buf bytes.Buffer
	lgr.Setup(lgr.Out(&buf), lgr.Debug)
	defer lgr.Setup() // restore the package's zero-value default logger

	target := &Profile{Name: "redact-code-test", ClientID: "client-id", VaultKey: "bitbucket-cli-test-nonexistent-vault-key"}
	resultchan := make(chan error, 1)
	handler := target.CodeGrantCallback(resultchan)

	const authCode = "s3cr3t-oauth-authorization-code"
	req := httptest.NewRequest(http.MethodGet, "/callback?code="+authCode, http.NoBody)
	handler.ServeHTTP(httptest.NewRecorder(), req)
	<-resultchan // drain: GetClientSecret fails against the nonexistent vault entry, but the log line under test already happened before that

	logged := buf.String()
	if strings.Contains(logged, authCode) {
		t.Errorf("debug log = %q, must not contain the OAuth authorization code in plain text", logged)
	}
	if !strings.Contains(logged, "REDACTED-") {
		t.Errorf("debug log = %q, want the redacted code's hash placeholder present", logged)
	}
}

// TestDoRequestWithRetryStopsAtOverallBudget is a regression test: honoring an uncapped
// Retry-After with no overall deadline could freeze a request for hours. This shrinks
// maxRetryBudget to prove the retry loop gives up once the overall budget is exceeded, even though
// individual per-attempt/backoff values would otherwise allow more attempts.
func TestDoRequestWithRetryStopsAtOverallBudget(t *testing.T) {
	oldBudget := maxRetryBudget
	maxRetryBudget = 200 * time.Millisecond
	defer func() { maxRetryBudget = oldBudget }()

	var attempts atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		attempts.Add(1)
		w.Header().Set("Retry-After", "5")
		w.WriteHeader(http.StatusTooManyRequests)
	}))
	defer server.Close()

	newReq := func(ctx context.Context) (*http.Request, error) {
		return http.NewRequestWithContext(ctx, http.MethodGet, server.URL, http.NoBody)
	}

	_, err := doRequestWithRetry(context.Background(), http.MethodGet, newReq)
	if err == nil {
		t.Fatal("doRequestWithRetry() expected an error once the retry budget was exceeded, got nil")
	}
	if got := attempts.Load(); got >= int32(maxRequestAttempts) {
		t.Errorf("attempts = %d, want fewer than maxRequestAttempts (%d): the budget should have cut the loop short", got, maxRequestAttempts)
	}
}

// TestResolveRequestURLRejectsMalformedPercentEscape proves resolveRequestURL now errors instead
// of silently returning apiRoot essentially unmodified when the path portion carries a percent
// sign that isn't part of a valid escape sequence -- url.URL.JoinPath treats each element as
// already percent-encoded, so an invalid "%" makes its internal setPath fail, and JoinPath itself
// swallows that error rather than surfacing it. Before this guard, a caller building uripath from
// an unescaped literal (e.g. an artifact name containing a bare "%") would unknowingly send an
// authenticated request to the bare API root instead of the intended path.
func TestResolveRequestURLRejectsMalformedPercentEscape(t *testing.T) {
	apiRoot, err := url.Parse("https://api.bitbucket.org")
	if err != nil {
		t.Fatalf("cannot parse api root: %v", err)
	}

	_, err = resolveRequestURL(apiRoot, "/repositories/acme/widgets/downloads/release (100%).zip")
	if err == nil {
		t.Fatal("resolveRequestURL() expected an error for a malformed percent escape, got nil")
	}
}

// TestResolveRequestURLAcceptsProperlyEscapedPath proves the companion fix: a caller that escapes
// its path segment first (url.PathEscape, as artifact download now does) round-trips through
// resolveRequestURL to exactly the intended path, including a segment that legitimately contains a
// literal "%" or "?".
func TestResolveRequestURLAcceptsProperlyEscapedPath(t *testing.T) {
	apiRoot, err := url.Parse("https://api.bitbucket.org")
	if err != nil {
		t.Fatalf("cannot parse api root: %v", err)
	}

	tests := []struct {
		name     string
		rawName  string
		wantPath string
	}{
		{name: "bare percent sign", rawName: "release (100%).zip", wantPath: "2.0/repositories/acme/widgets/downloads/release (100%).zip"},
		{name: "question mark", rawName: "a?b.zip", wantPath: "2.0/repositories/acme/widgets/downloads/a?b.zip"},
		{name: "literal percent-two-five", rawName: "50%25.zip", wantPath: "2.0/repositories/acme/widgets/downloads/50%25.zip"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			uripath := "/repositories/acme/widgets/downloads/" + url.PathEscape(tt.rawName)

			reqURL, err := resolveRequestURL(apiRoot, uripath)
			if err != nil {
				t.Fatalf("resolveRequestURL() unexpected error: %v", err)
			}
			if reqURL.Path != tt.wantPath {
				t.Errorf("resolveRequestURL().Path = %q, want %q", reqURL.Path, tt.wantPath)
			}
		})
	}
}
