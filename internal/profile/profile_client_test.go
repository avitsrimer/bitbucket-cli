package profile_test

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"time"

	"github.com/avitsrimer/bitbucket-cli/internal/profile"
	"github.com/spf13/cobra"
)

type testItem struct {
	ID string `json:"id"`
}

func (suite *ProfileSuite) TestGetAll_OriginalQueryIsPreservedForNextMissingParams() {
	oldCurrent := profile.Current
	defer func() { profile.Current = oldCurrent }()

	const filter = `target.ref_name="my-branch"`
	var server *httptest.Server
	server = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		q := r.URL.Query().Get("q")
		if r.URL.Path == "/pipelines" {
			if r.URL.Query().Get("page") == "" {
				suite.Equal(filter, q, "initial request should include original q")
				resp := map[string]any{
					"values": []map[string]string{{"id": "1"}},
					"next":   server.URL + "/pipelines?page=2&pagelen=1",
				}
				_ = json.NewEncoder(w).Encode(resp)
				return
			}
			suite.Equal(filter, q, "second request should include original q even when next omits it")
			resp := map[string]any{
				"values": []map[string]string{{"id": "2"}},
			}
			_ = json.NewEncoder(w).Encode(resp)
			return
		}
		w.WriteHeader(http.StatusNotFound)
	}))
	defer server.Close()

	apiRoot, err := url.Parse(server.URL)
	suite.Require().NoError(err)
	profile.Current = &profile.Profile{APIRoot: apiRoot, DefaultPageLength: 0, AccessToken: "dummy-token"}

	cmd := &cobra.Command{}
	cmd.Flags().String("profile", "", "")
	cmd.Flags().Int("page-length", 0, "")
	items, err := profile.GetAll[testItem](suite.Context, cmd, server.URL+"/pipelines?pagelen=1&q="+url.QueryEscape(filter))
	suite.Require().NoError(err)
	suite.Require().Len(items, 2)
	suite.Require().Equal("1", items[0].ID)
	suite.Require().Equal("2", items[1].ID)
}

// TestGetAll_PageLengthNotDroppedWhenQueryTextContainsPagelenSubstring pins that getAll's
// pagelen-present guard checks the parsed query's actual "pagelen" KEY, not a substring test over
// the whole request path: every list command builds user-controlled q= into the uripath passed to
// GetAll (pullrequest/comment/task/branch/pipeline/artifact/commit list), so a query whose text
// happens to contain the word "pagelen" (e.g. a branch search for "feature/pagelen-tuning") must
// not suppress --page-length/DefaultPageLength being appended, even though no pagelen= parameter
// was ever actually present in the query itself.
func (suite *ProfileSuite) TestGetAll_PageLengthNotDroppedWhenQueryTextContainsPagelenSubstring() {
	oldCurrent := profile.Current
	defer func() { profile.Current = oldCurrent }()

	const filter = `source.branch.name="feature/pagelen-tuning"`
	var gotPagelen string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotPagelen = r.URL.Query().Get("pagelen")
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]any{
			"values": []map[string]string{{"id": "1"}},
		})
	}))
	defer server.Close()

	apiRoot, err := url.Parse(server.URL)
	suite.Require().NoError(err)
	profile.Current = &profile.Profile{APIRoot: apiRoot, DefaultPageLength: 25, AccessToken: "dummy-token"}

	cmd := &cobra.Command{}
	cmd.Flags().String("profile", "", "")
	cmd.Flags().Int("page-length", 0, "")
	_, err = profile.GetAll[testItem](suite.Context, cmd, server.URL+"/pullrequests?q="+url.QueryEscape(filter))
	suite.Require().NoError(err)
	suite.Equal("25", gotPagelen, "pagelen must be appended from DefaultPageLength even though q= contains the substring \"pagelen\"")
}

// TestGetAllUnbounded_IgnoresLimitFlag is a regression test: GetAll sniffs --limit off the
// ambient cmd, which is correct for a command's own output query but wrong for an internal
// id-resolution query sharing that same cmd (e.g. resolving an omitted pullrequest-id before `pr
// commits --limit 1` fetches commits). GetAllUnbounded must return every item regardless of a
// --limit flag set on cmd.
func (suite *ProfileSuite) TestGetAllUnbounded_IgnoresLimitFlag() {
	oldCurrent := profile.Current
	defer func() { profile.Current = oldCurrent }()

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]any{
			"values": []map[string]string{{"id": "1"}, {"id": "2"}, {"id": "3"}, {"id": "4"}},
		})
	}))
	defer server.Close()

	apiRoot, err := url.Parse(server.URL)
	suite.Require().NoError(err)
	profile.Current = &profile.Profile{APIRoot: apiRoot, DefaultPageLength: 0, AccessToken: "dummy-token"}

	cmd := &cobra.Command{}
	cmd.Flags().String("profile", "", "")
	cmd.Flags().Int("page-length", 0, "")
	cmd.Flags().Int("limit", 0, "")
	suite.Require().NoError(cmd.Flags().Set("limit", "1"))

	bounded, err := profile.GetAll[testItem](suite.Context, cmd, server.URL+"/pipelines")
	suite.Require().NoError(err)
	suite.Require().Len(bounded, 1, "GetAll must honor the ambient --limit flag")

	unbounded, err := profile.GetAllUnbounded[testItem](suite.Context, cmd, server.URL+"/pipelines")
	suite.Require().NoError(err)
	suite.Require().Len(unbounded, 4, "GetAllUnbounded must ignore the ambient --limit flag")
}

func (suite *ProfileSuite) TestGetAll_DoesNotOverwriteExistingNextParams() {
	oldCurrent := profile.Current
	defer func() { profile.Current = oldCurrent }()

	const originalFilter = `target.ref_name="original"`
	const nextFilter = `target.ref_name="different"`
	var server *httptest.Server
	server = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		q := r.URL.Query().Get("q")
		if r.URL.Path == "/pipelines" {
			if r.URL.Query().Get("page") == "" {
				suite.Equal(originalFilter, q, "initial request should include original q")
				resp := map[string]any{
					"values": []map[string]string{{"id": "1"}},
					"next":   server.URL + "/pipelines?page=2&pagelen=1&q=" + url.QueryEscape(nextFilter),
				}
				_ = json.NewEncoder(w).Encode(resp)
				return
			}
			suite.Equal(nextFilter, q, "existing q on next URL must not be overwritten")
			resp := map[string]any{
				"values": []map[string]string{{"id": "2"}},
			}
			_ = json.NewEncoder(w).Encode(resp)
			return
		}
		w.WriteHeader(http.StatusNotFound)
	}))
	defer server.Close()

	apiRoot, err := url.Parse(server.URL)
	suite.Require().NoError(err)
	profile.Current = &profile.Profile{APIRoot: apiRoot, DefaultPageLength: 0, AccessToken: "dummy-token"}

	cmd := &cobra.Command{}
	cmd.Flags().String("profile", "", "")
	cmd.Flags().Int("page-length", 0, "")
	items, err := profile.GetAll[testItem](suite.Context, cmd, server.URL+"/pipelines?pagelen=1&q="+url.QueryEscape(originalFilter))
	suite.Require().NoError(err)
	suite.Require().Len(items, 2)
	suite.Require().Equal("1", items[0].ID)
	suite.Require().Equal("2", items[1].ID)
}

func (suite *ProfileSuite) TestGetSendsAuthHeaderAndUnmarshalsSuccessResponse() {
	var gotAuthorization, gotAccept string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotAuthorization = r.Header.Get("Authorization")
		gotAccept = r.Header.Get("Accept")
		suite.Equal(http.MethodGet, r.Method)
		suite.Equal("/2.0/repo", r.URL.Path)
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(testItem{ID: "42"})
	}))
	defer server.Close()

	apiRoot, err := url.Parse(server.URL)
	suite.Require().NoError(err)
	target := &profile.Profile{APIRoot: apiRoot, AccessToken: "dummy-token"}

	var item testItem
	err = target.Get(suite.Context, "/repo", &item)
	suite.Require().NoError(err)
	suite.Equal("42", item.ID)
	suite.Equal("Bearer dummy-token", gotAuthorization, "the bearer token from the profile should be sent as the Authorization header")
	suite.Equal("application/json", gotAccept, "requests with an unmarshal target should ask for JSON")
}

func (suite *ProfileSuite) TestPostSendsJSONPayloadAndUnmarshalsResponse() {
	var gotContentType string
	var gotBody testItem
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotContentType = r.Header.Get("Content-Type")
		suite.Equal(http.MethodPost, r.Method)
		suite.NoError(json.NewDecoder(r.Body).Decode(&gotBody))
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(testItem{ID: "created-" + gotBody.ID})
	}))
	defer server.Close()

	apiRoot, err := url.Parse(server.URL)
	suite.Require().NoError(err)
	target := &profile.Profile{APIRoot: apiRoot, AccessToken: "dummy-token"}

	var result testItem
	err = target.Post(suite.Context, "/repo", testItem{ID: "1"}, &result)
	suite.Require().NoError(err)
	suite.Equal("created-1", result.ID)
	suite.Equal("application/json", gotContentType, "a JSON payload should be sent with an application/json Content-Type")
}

func (suite *ProfileSuite) TestGetMapsBitBucketErrorBody() {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusNotFound)
		_, _ = w.Write(suite.LoadTestData("error-noapi.json"))
	}))
	defer server.Close()

	apiRoot, err := url.Parse(server.URL)
	suite.Require().NoError(err)
	target := &profile.Profile{APIRoot: apiRoot, AccessToken: "dummy-token"}

	var item testItem
	err = target.Get(suite.Context, "/repo", &item)
	suite.Require().Error(err)
	var bberr *profile.BitBucketError
	suite.Require().ErrorAs(err, &bberr, "a BitBucket-shaped error body should be mapped to a *BitBucketError")
	suite.Equal("Resource not found", bberr.Message)
	suite.Equal("There is no API hosted at this URL", bberr.Detail)
}

// TestIsNotFoundRecognizesBothErrorShapes pins profile.IsNotFound against both errors a non-2xx
// response can map to -- a *BitBucketError built from the API's own error payload and the bare
// status error used when the body carries none -- and against a non-404 of each shape. Callers use
// it to attach guidance to a specific status (see pullrequest list's --author 404 message), which
// only works if the status survives the mapping instead of only appearing inside the message text.
func (suite *ProfileSuite) TestIsNotFoundRecognizesBothErrorShapes() {
	tests := []struct {
		name        string
		status      int
		body        string
		contentType string
		want        bool
	}{
		{name: "404 with bitbucket error body", status: http.StatusNotFound, body: `{"type":"error","error":{"message":"No such user"}}`, contentType: "application/json", want: true},
		{name: "404 without json body", status: http.StatusNotFound, body: "not found, not json", want: true},
		{name: "403 with bitbucket error body", status: http.StatusForbidden, body: `{"type":"error","error":{"message":"forbidden"}}`, contentType: "application/json"},
		{name: "500 without json body", status: http.StatusInternalServerError, body: "boom"},
	}

	for _, tt := range tests {
		suite.Run(tt.name, func() {
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
				if tt.contentType != "" {
					w.Header().Set("Content-Type", tt.contentType)
				}
				w.WriteHeader(tt.status)
				_, _ = w.Write([]byte(tt.body))
			}))
			defer server.Close()

			apiRoot, err := url.Parse(server.URL)
			suite.Require().NoError(err)
			target := &profile.Profile{APIRoot: apiRoot, AccessToken: "dummy-token"}

			var item testItem
			err = target.Get(suite.Context, "/repo", &item)
			suite.Require().Error(err)
			suite.Equal(tt.want, profile.IsNotFound(err))
		})
	}
}

func (suite *ProfileSuite) TestGetNon2xxWithoutJSONBodyReturnsGenericError() {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
		_, _ = w.Write([]byte("internal server error, not json"))
	}))
	defer server.Close()

	apiRoot, err := url.Parse(server.URL)
	suite.Require().NoError(err)
	target := &profile.Profile{APIRoot: apiRoot, AccessToken: "dummy-token"}

	var item testItem
	err = target.Get(suite.Context, "/repo", &item)
	suite.Require().Error(err)
	var bberr *profile.BitBucketError
	suite.Require().NotErrorAs(err, &bberr, "a non-JSON error body should not be mapped to a BitBucketError")
	suite.Contains(err.Error(), "500")
}

// TestGetNon2xxWithUnrelatedJSONBodyReturnsGenericError proves a JSON body that isn't shaped like
// a BitBucket error (e.g. a proxy's {"message":"..."} shape) is not mapped to a blank
// *BitBucketError: BitBucketError.UnmarshalJSON's last-resort fallback succeeds for any valid JSON
// object, so mapErrorResponse only trusts it when Message/Detail/Fields actually carry something,
// falling back to a generic status-text error instead of a completely empty message.
func (suite *ProfileSuite) TestGetNon2xxWithUnrelatedJSONBodyReturnsGenericError() {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusBadRequest)
		_, _ = w.Write([]byte(`{"message":"upstream unavailable"}`))
	}))
	defer server.Close()

	apiRoot, err := url.Parse(server.URL)
	suite.Require().NoError(err)
	target := &profile.Profile{APIRoot: apiRoot, AccessToken: "dummy-token"}

	var item testItem
	err = target.Get(suite.Context, "/repo", &item)
	suite.Require().Error(err)
	var bberr *profile.BitBucketError
	suite.Require().NotErrorAs(err, &bberr, "a JSON body not shaped like a BitBucket error should not be mapped to a blank BitBucketError")
	suite.Contains(err.Error(), "400")
	suite.NotEmpty(err.Error())
}

func (suite *ProfileSuite) TestGetRawUsesWildcardAcceptAndReturnsRawBody() {
	var gotAccept string
	const rawBody = "diff --git a/x b/x\n"
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotAccept = r.Header.Get("Accept")
		w.Header().Set("Content-Type", "text/x-diff")
		_, _ = w.Write([]byte(rawBody))
	}))
	defer server.Close()

	apiRoot, err := url.Parse(server.URL)
	suite.Require().NoError(err)
	target := &profile.Profile{APIRoot: apiRoot, AccessToken: "dummy-token"}

	reader, err := target.GetRaw(suite.Context, "/repo/diff")
	suite.Require().NoError(err)
	data, err := io.ReadAll(reader)
	suite.Require().NoError(err)
	suite.Equal(rawBody, string(data))
	suite.Equal("*/*", gotAccept)
}

// TestGetRetriesAfter429ThenSucceeds proves a 429 response (with a short Retry-After) is retried
// and the eventual 200 is returned, rather than hard-failing the command on the first rate-limit
// response. Retry-After is "1", not "0": a zero value is treated as "no header" (use the computed
// backoff) rather than "retry instantly", so this exercises the header actually being honored.
func (suite *ProfileSuite) TestGetRetriesAfter429ThenSucceeds() {
	var attempts int
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		attempts++
		if attempts == 1 {
			w.Header().Set("Retry-After", "1")
			w.WriteHeader(http.StatusTooManyRequests)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(testItem{ID: "42"})
	}))
	defer server.Close()

	apiRoot, err := url.Parse(server.URL)
	suite.Require().NoError(err)
	target := &profile.Profile{APIRoot: apiRoot, AccessToken: "dummy-token"}

	var item testItem
	err = target.Get(suite.Context, "/repo", &item)
	suite.Require().NoError(err)
	suite.Equal("42", item.ID)
	suite.Equal(2, attempts, "the 429 response should have been retried exactly once")
}

// TestGetGivesUpAfterExhaustingRetriesOn429 proves the retry loop terminates: a server that
// always returns 429 must not hang the CLI forever, and the final BitBucket error should still
// surface to the caller.
func (suite *ProfileSuite) TestGetGivesUpAfterExhaustingRetriesOn429() {
	var attempts int
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		attempts++
		w.Header().Set("Retry-After", "1")
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusTooManyRequests)
		_, _ = w.Write([]byte(`{"error":{"message":"rate limited"}}`))
	}))
	defer server.Close()

	apiRoot, err := url.Parse(server.URL)
	suite.Require().NoError(err)
	target := &profile.Profile{APIRoot: apiRoot, AccessToken: "dummy-token"}

	var item testItem
	err = target.Get(suite.Context, "/repo", &item)
	suite.Require().Error(err)
	suite.Equal(5, attempts, "should attempt exactly maxRequestAttempts times before giving up")
}

// TestGetAbortsRetryLoopWhenContextIsCanceled proves a canceled context stops the retry loop
// immediately instead of waiting out the full backoff schedule.
func (suite *ProfileSuite) TestGetAbortsRetryLoopWhenContextIsCanceled() {
	var attempts int
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		attempts++
		w.WriteHeader(http.StatusServiceUnavailable)
	}))
	defer server.Close()

	apiRoot, err := url.Parse(server.URL)
	suite.Require().NoError(err)
	target := &profile.Profile{APIRoot: apiRoot, AccessToken: "dummy-token"}

	ctx, cancel := context.WithCancel(suite.Context)
	go func() {
		<-time.After(50 * time.Millisecond)
		cancel()
	}()

	var item testItem
	err = target.Get(ctx, "/repo", &item)
	suite.Require().Error(err)
	suite.Require().ErrorIs(err, context.Canceled)
	suite.Less(attempts, 5, "the retry loop should have aborted before exhausting all attempts")
}

// TestGetAllRejectsNextPageURLFromDifferentHost is a regression test for the Authorization
// header being attached to whatever host a "next" pagination URL claims: a compromised or
// misconfigured API response could otherwise exfiltrate the profile's token by pointing "next"
// at an attacker-controlled host.
func (suite *ProfileSuite) TestGetAllRejectsNextPageURLFromDifferentHost() {
	oldCurrent := profile.Current
	defer func() { profile.Current = oldCurrent }()

	var attackerReceivedAuth bool
	attacker := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		attackerReceivedAuth = r.Header.Get("Authorization") != ""
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]any{"values": []map[string]string{{"id": "2"}}})
	}))
	defer attacker.Close()

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]any{
			"values": []map[string]string{{"id": "1"}},
			"next":   attacker.URL + "/pipelines?page=2",
		})
	}))
	defer server.Close()

	apiRoot, err := url.Parse(server.URL)
	suite.Require().NoError(err)
	profile.Current = &profile.Profile{APIRoot: apiRoot, DefaultPageLength: 0, AccessToken: "dummy-token"}

	cmd := &cobra.Command{}
	cmd.Flags().String("profile", "", "")
	cmd.Flags().Int("page-length", 0, "")
	items, err := profile.GetAll[testItem](suite.Context, cmd, server.URL+"/pipelines?pagelen=1")

	suite.Require().Error(err, "a next URL pointing at a different host must be rejected")
	suite.Contains(err.Error(), "does not match")
	suite.Empty(items)
	suite.False(attackerReceivedAuth, "the attacker server should never have been reached with an Authorization header")
}

// TestCodeGrantCallbackDoesNotBlockOnDuplicateRequest proves a second callback request (browser
// reload/prefetch) arriving after the first result was already delivered returns immediately
// instead of blocking its handler goroutine forever on resultchan's send, which would otherwise
// make authorizeProcess's server.Shutdown hang waiting on that in-flight request.
func (suite *ProfileSuite) TestCodeGrantCallbackDoesNotBlockOnDuplicateRequest() {
	testProfile := &profile.Profile{Name: "callback-test", ClientID: "client-id", VaultKey: "bitbucket-cli-test-nonexistent-vault-key"}
	resultchan := make(chan error, 1)
	handler := testProfile.CodeGrantCallback(resultchan)

	req1 := httptest.NewRequest(http.MethodGet, "/callback?code=abc", http.NoBody)
	handler.ServeHTTP(httptest.NewRecorder(), req1)

	select {
	case err := <-resultchan:
		suite.Require().Error(err, "a nonexistent vault entry should make GetClientSecret fail")
	case <-time.After(5 * time.Second):
		suite.FailNow("expected a result from the first callback request")
	}

	// Nothing drains resultchan anymore; a second callback request must still return instead of
	// blocking its handler goroutine on the channel send.
	done := make(chan struct{})
	go func() {
		req2 := httptest.NewRequest(http.MethodGet, "/callback?code=abc", http.NoBody)
		handler.ServeHTTP(httptest.NewRecorder(), req2)
		close(done)
	}()

	select {
	case <-done:
	case <-time.After(5 * time.Second):
		suite.FailNow("second callback request blocked instead of using a non-blocking send")
	}
}

func (suite *ProfileSuite) TestPostWithResultExposesResponseHeaders() {
	var server *httptest.Server
	server = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Location", server.URL+"/task-status/123")
		w.WriteHeader(http.StatusAccepted)
	}))
	defer server.Close()

	apiRoot, err := url.Parse(server.URL)
	suite.Require().NoError(err)
	target := &profile.Profile{APIRoot: apiRoot, AccessToken: "dummy-token"}

	result, err := target.PostWithResult(suite.Context, "/repo/merge", nil)
	suite.Require().NoError(err)
	suite.Require().NotNil(result)
	suite.Equal(server.URL+"/task-status/123", result.Headers.Get("Location"))
}
