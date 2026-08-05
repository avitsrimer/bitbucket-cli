package profile

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"

	"github.com/go-pkgz/lgr"
	"github.com/spf13/cobra"
)

// requestTimeout is the deadline applied to every single attempt of a request sent to the
// BitBucket API or the OAuth2 token endpoint; retries each get a fresh deadline.
const requestTimeout = 30 * time.Second

// maxRequestAttempts is the total number of attempts (the initial try plus retries) made for a
// request that keeps failing with a transport error or a retryable status code.
const maxRequestAttempts = 5

// initialRetryBackoff and maxRetryBackoff bound the exponential backoff used between retries
// when the response carries no Retry-After header.
const (
	initialRetryBackoff = 500 * time.Millisecond
	maxRetryBackoff     = 8 * time.Second
)

// maxRetryAfterBackoff caps how long a server-supplied Retry-After header is honored for. A
// header can legitimately ask for minutes or hours (a sustained rate-limit window); honoring it
// verbatim would otherwise freeze an interactive CLI command for that entire duration with only a
// single [WARN] line to show for it. Past this cap, doRequestWithRetry's own maxRetryBudget and
// maxRequestAttempts end the request instead.
const maxRetryAfterBackoff = 30 * time.Second

// maxRetryBudget is the overall wall-clock budget for a single request's attempts and backoff
// delays combined, independent of what any individual Retry-After header asks for. It is a var
// (not a const) solely so tests can shrink it to exercise the budget-exceeded path quickly;
// production code never reassigns it.
var maxRetryBudget = 2 * time.Minute

// userAgent is sent with every outgoing request.
const userAgent = "bitbucket-cli"

// oauthTokenURL is BitBucket's OAuth2 token endpoint. It is a var rather than a const solely so
// internal tests can point it at an httptest server; production code never reassigns it.
var oauthTokenURL = "https://bitbucket.org/site/oauth2/access_token" //nolint:gosec // endpoint URL, not a credential

// httpClient is the shared HTTP client used for every request; per-request deadlines come
// from the context passed to send/sendOAuthTokenRequest, so the client itself carries no
// default timeout.
var httpClient = &http.Client{}

// Response carries the raw result of a request: status, headers, and body.
type Response struct {
	StatusCode int
	StatusText string
	Headers    http.Header
	Body       []byte
}

// requestOptions describes a single request to the BitBucket API
type requestOptions struct {
	Method  string
	Payload any
	Accept  string
}

// PaginatedResources is a single page of a BitBucket paginated list response. Only the fields
// GetAll actually reads (Values, Next, Previous) are kept; page/pagelen/size are tracked by this
// package itself via resolvePageLengthAndLimit/nextPageURL instead.
type PaginatedResources[T any] struct {
	Values   []T    `json:"values"`
	Next     string `json:"next"`
	Previous string `json:"previous"`
}

// Post posts a resource
func (profile *Profile) Post(ctx context.Context, uripath string, body, response any) (err error) {
	_, err = profile.send(ctx, &requestOptions{Method: http.MethodPost, Payload: body}, uripath, response)
	return
}

// PostWithResult posts a resource and returns the raw result
func (profile *Profile) PostWithResult(ctx context.Context, uripath string, body any) (result *Response, err error) {
	return profile.send(ctx, &requestOptions{Method: http.MethodPost, Payload: body}, uripath, nil)
}

// Get gets a resource
func (profile *Profile) Get(ctx context.Context, uripath string, response any) (err error) {
	_, err = profile.send(ctx, &requestOptions{Method: http.MethodGet}, uripath, response)
	return
}

// GetRaw gets a resource without unmarshaling it
func (profile *Profile) GetRaw(ctx context.Context, uripath string) (raw io.Reader, err error) {
	result, err := profile.send(ctx, &requestOptions{Method: http.MethodGet, Accept: "*/*"}, uripath, nil)
	if result == nil {
		return nil, err
	}
	return bytes.NewReader(result.Body), err
}

// Put puts/updates a resource
func (profile *Profile) Put(ctx context.Context, uripath string, body, response any) (err error) {
	_, err = profile.send(ctx, &requestOptions{Method: http.MethodPut, Payload: body}, uripath, response)
	return
}

// Delete deletes a resource
func (profile *Profile) Delete(ctx context.Context, uripath string, response any) (err error) {
	_, err = profile.send(ctx, &requestOptions{Method: http.MethodDelete}, uripath, response)
	return
}

// Patch patches a resource
func (profile *Profile) Patch(ctx context.Context, uripath string, body, response any) (err error) {
	_, err = profile.send(ctx, &requestOptions{Method: http.MethodPatch, Payload: body}, uripath, response)
	return
}

// resolvePageLengthAndLimit reads the --page-length flag, falling back to defaultPageLength, and
// (when honorLimit is true) the --limit flag; honorLimit is false for GetAllUnbounded, so a
// --limit flag belonging to the command's own, unrelated output query neither truncates nor
// shrinks the page size of an internal id-resolution query run against the same cmd.
func resolvePageLengthAndLimit(cmd *cobra.Command, defaultPageLength int, honorLimit bool) (pageLength, limit int) {
	pageLength = defaultPageLength
	if cmd != nil && cmd.Flag("page-length") != nil && cmd.Flag("page-length").Changed {
		if length, err := cmd.Flags().GetInt("page-length"); err == nil && length > 0 {
			pageLength = length
			lgr.Printf("[DEBUG] using page length of %d from the command line flags", pageLength)
		}
	}
	if honorLimit && cmd != nil && cmd.Flag("limit") != nil && cmd.Flag("limit").Changed {
		if l, err := cmd.Flags().GetInt("limit"); err == nil && l > 0 {
			limit = l
			lgr.Printf("[DEBUG] using limit of %d from the command line flags", limit)
		}
	}
	if limit > 0 && (pageLength == 0 || limit < pageLength) {
		pageLength = limit
	}
	return pageLength, limit
}

// nextPageURL builds the URL for the next page of resources, preserving the original query
// parameters and trimming pagelen once limit is close to being reached
func nextPageURL(next string, originalQuery url.Values, limit, resourceCount, pageLength int) (string, error) {
	nextURL, err := url.Parse(next)
	if err != nil {
		return "", fmt.Errorf("cannot parse next page url: %w", err)
	}
	nextQuery := nextURL.Query()
	for key, values := range originalQuery {
		if _, exists := nextQuery[key]; !exists {
			for _, value := range values {
				nextQuery.Add(key, value)
			}
		}
	}
	if limit > 0 {
		remaining := limit - resourceCount
		if remaining < pageLength {
			// Adjust pagelen on the next URL to only fetch what we still need
			nextQuery.Set("pagelen", strconv.Itoa(remaining))
		}
	}
	nextURL.RawQuery = nextQuery.Encode()
	return nextURL.String(), nil
}

// GetAll gets all resources of the given type, honoring cmd's own --page-length and --limit
// flags (if registered) as the caller's requested bound on the *output* of this specific query.
//
// Do not use this for an internal id-resolution query that happens to run against the same cmd as
// an unrelated output query of its own (e.g. resolving the single open pull request id `pr
// commits`/`pr activities` operate on, when no id was given on the command line): cmd's --limit
// flag is ambient to the whole command, so it would truncate that resolution query exactly as
// much as the real output query, silently making an id-resolution "how many are there" check
// pass when it shouldn't. Use GetAllUnbounded for those instead.
func GetAll[T any](ctx context.Context, cmd *cobra.Command, uripath string) (resources []T, err error) {
	return getAll[T](ctx, cmd, uripath, true)
}

// GetAllUnbounded gets all resources of the given type like GetAll, but always ignores any
// --limit flag on cmd. Use this for internal id-resolution queries that must enumerate every
// matching resource to make a correct decision (e.g. detecting "more than one open pull request"),
// regardless of a --limit flag belonging to the command's own, unrelated output query.
func GetAllUnbounded[T any](ctx context.Context, cmd *cobra.Command, uripath string) (resources []T, err error) {
	return getAll[T](ctx, cmd, uripath, false)
}

func getAll[T any](ctx context.Context, cmd *cobra.Command, uripath string, honorLimit bool) (resources []T, err error) {
	profile, err := GetProfileFromCommand(ctx, cmd)
	if err != nil {
		lgr.Printf("[ERROR] failed to get profile: %v", err)
		return nil, err
	}
	Current = profile // Make sure the current profile is set

	pageLength, limit := resolvePageLengthAndLimit(cmd, Current.DefaultPageLength, honorLimit)

	if !strings.Contains(uripath, "pagelen") && pageLength > 0 {
		if strings.Contains(uripath, "?") {
			uripath = fmt.Sprintf("%s&pagelen=%d", uripath, pageLength)
		} else {
			uripath = fmt.Sprintf("%s?pagelen=%d", uripath, pageLength)
		}
	}

	originalQuery := url.Values{}
	if parsed, parseErr := url.Parse(uripath); parseErr == nil {
		originalQuery = parsed.Query()
	}

	if limit > 0 {
		lgr.Printf("[DEBUG] getting up to %d resources for profile %s (%d at a time)", limit, profile.Name, pageLength)
	} else {
		lgr.Printf("[DEBUG] getting all resources for profile %s (%d at a time)", profile.Name, pageLength)
	}
	for {
		var paginated PaginatedResources[T]

		err = profile.Get(
			ctx,
			uripath,
			&paginated,
		)
		if err != nil {
			return nil, err
		}
		resources = append(resources, paginated.Values...)
		if limit > 0 && len(resources) >= limit {
			resources = resources[:limit]
			break
		}
		lgr.Printf("[DEBUG] got %d resources (total: %d)", len(paginated.Values), len(resources))
		lgr.Printf("[DEBUG] next page:     %s", paginated.Next)
		lgr.Printf("[DEBUG] previous page: %s", paginated.Previous)
		if paginated.Next == "" {
			break
		}

		uripath, err = nextPageURL(paginated.Next, originalQuery, limit, len(resources), pageLength)
		if err != nil {
			return nil, err
		}
	}
	return resources, nil
}

// sendResult delivers err to resultchan without blocking, so a second callback request (browser
// reload/prefetch) arriving after the first result was already delivered cannot hang forever
// waiting for a receiver that has already moved on.
func sendResult(resultchan chan error, err error) {
	select {
	case resultchan <- err:
	default:
	}
}

func (profile *Profile) CodeGrantCallback(resultchan chan error) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet {
			http.Error(w, "Method Not Allowed", http.StatusMethodNotAllowed)
			return
		}
		if r.URL.Path == "/favicon.ico" {
			_, _ = w.Write([]byte{})
			return
		}

		lgr.Printf("[DEBUG] received callback from BitBucket for profile %s", profile.Name)
		code := r.URL.Query().Get("code")
		if code == "" {
			lgr.Printf("[ERROR] no code in the callback")
			http.Error(w, "No code in the callback", http.StatusBadRequest)
			return
		}
		lgr.Printf("[DEBUG] received code %s", code)

		// Get the client secret from the vault if it is empty
		clientSecret, err := profile.GetClientSecret(r.Context())
		if err != nil {
			lgr.Printf("[ERROR] failed to get client secret for profile %s: %v", profile.Name, err)
			http.Error(w, "Failed to get client secret for profile "+profile.Name+": "+err.Error(), http.StatusUnauthorized)
			sendResult(resultchan, err)
			return
		}

		lgr.Printf("[DEBUG] requesting authorization token for profile %s", profile.Name)
		result, err := sendOAuthTokenRequest(r.Context(), profile.ClientID, clientSecret, map[string]string{
			"grant_type": "authorization_code",
			"code":       code,
		})
		if err != nil {
			writeAuthorizationErrorResponse(w, err, result)
			sendResult(resultchan, err)
			return
		}
		if _, err := profile.saveAccessToken(r.Context(), result.Body); err != nil {
			lgr.Printf("[ERROR] failed to save access token for profile %s: %v", profile.Name, err)
			if errors.Is(err, ErrUnmarshalJSON) {
				http.Error(w, "Failed to parse access token response from BitBucket: "+err.Error(), http.StatusBadRequest)
			} else {
				http.Error(w, "Failed to save access token for profile "+profile.Name+": "+err.Error(), http.StatusInternalServerError)
			}
			sendResult(resultchan, err)
			return
		}
		_, _ = w.Write([]byte("Authorization Code received. You can close this window."))
		sendResult(resultchan, nil)
	})
}

// writeAuthorizationErrorResponse writes the appropriate HTTP error response for a failed
// authorization token request
func writeAuthorizationErrorResponse(w http.ResponseWriter, err error, result *Response) {
	if result == nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	var errorResponse struct {
		Error            string `json:"error"`
		ErrorDescription string `json:"error_description"`
	}
	if jerr := json.Unmarshal(result.Body, &errorResponse); jerr != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	status := result.StatusCode
	if status == 0 {
		status = http.StatusInternalServerError
	}
	http.Error(w, errorResponse.ErrorDescription, status)
}

func (profile *Profile) authorize(ctx context.Context) (authorization string, err error) {
	// loadAccessToken's contract guarantees profile.token is non-nil whenever it returns a nil
	// error, so the nil-safe isTokenExpired() and the token.GetExpiresOn() dereference below are
	// only reached once we know we actually have a token.
	if loadErr := profile.loadAccessToken(ctx); loadErr == nil && !profile.isTokenExpired() {
		lgr.Printf("[DEBUG] using access token for profile %s", profile.Name)
		lgr.Printf("[DEBUG] token expires on %s in %s", profile.token.GetExpiresOn().Format(time.RFC3339), profile.token.GetExpiresIn())
		return bearerAuthorization(profile.token.AccessToken), nil
	}

	payload := map[string]string{}
	if profile.token != nil && profile.token.RefreshToken != "" {
		lgr.Printf("[WARN] access token for profile %s expired %s ago and we have a refresh token", profile.Name, profile.token.GetExpiredSince())
		payload["grant_type"] = "refresh_token"
		payload["refresh_token"] = profile.token.RefreshToken
	} else {
		if profile.token != nil {
			lgr.Printf("[WARN] access token for profile %s expired %s ago but we don't have a refresh token", profile.Name, profile.token.GetExpiredSince())
		} else {
			lgr.Printf("[WARN] no access token found for profile %s, we need to authorize the profile", profile.Name)
		}
		payload["grant_type"] = "client_credentials"
	}

	// Get the client secret from the vault if it is empty
	clientSecret, err := profile.GetClientSecret(ctx)
	if err != nil {
		return "", err
	}
	lgr.Printf("[DEBUG] authorizing profile %s", profile.Name)
	result, err := sendOAuthTokenRequest(ctx, profile.ClientID, clientSecret, payload)
	if err != nil {
		return "", bitbucketAuthError(err, result)
	}
	accessToken, err := profile.saveAccessToken(ctx, result.Body)
	if err != nil {
		return "", err
	}
	return bearerAuthorization(accessToken), err
}

// bitbucketAuthError turns a failed authorization request into an error carrying BitBucket's
// own error payload, when one is available
func bitbucketAuthError(err error, result *Response) error {
	if result == nil {
		return err
	}
	var errorResponse struct {
		Error            string `json:"error"`
		ErrorDescription string `json:"error_description"`
	}
	if jerr := json.Unmarshal(result.Body, &errorResponse); jerr != nil {
		return err
	}
	status := result.StatusCode
	if status == 0 {
		status = http.StatusInternalServerError
	}
	return &oauthError{StatusCode: status, Code: errorResponse.Error, Description: errorResponse.ErrorDescription}
}

// oauthError represents an error response from BitBucket's OAuth2 token endpoint.
type oauthError struct {
	StatusCode  int
	Code        string
	Description string
}

func (e *oauthError) Error() string {
	if e.Description != "" {
		return e.Description
	}
	if e.Code != "" {
		return e.Code
	}
	return "oauth error"
}

// basicAuthorization builds a Basic authorization header value
func basicAuthorization(user, password string) string {
	return "Basic " + base64.StdEncoding.EncodeToString([]byte(user+":"+password))
}

// bearerAuthorization builds a Bearer authorization header value
func bearerAuthorization(token string) string {
	return "Bearer " + token
}

// sendOAuthTokenRequest sends a form-encoded request to BitBucket's OAuth2 token endpoint,
// as required by that endpoint (it does not accept JSON payloads)
func sendOAuthTokenRequest(ctx context.Context, clientID, clientSecret string, payload map[string]string) (*Response, error) {
	form := url.Values{}
	for key, value := range payload {
		form.Set(key, value)
	}
	encoded := form.Encode()

	newReq := func(reqCtx context.Context) (*http.Request, error) {
		req, err := http.NewRequestWithContext(reqCtx, http.MethodPost, oauthTokenURL, strings.NewReader(encoded))
		if err != nil {
			return nil, fmt.Errorf("cannot build oauth token request: %w", err)
		}
		req.Header.Set("Authorization", basicAuthorization(clientID, clientSecret))
		req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
		req.Header.Set("User-Agent", userAgent)
		return req, nil
	}

	result, err := doRequestWithRetry(ctx, http.MethodPost, newReq)
	if err != nil {
		return nil, err
	}
	if result.StatusCode >= http.StatusBadRequest {
		return result, fmt.Errorf("oauth token request failed: %s", result.StatusText)
	}
	return result, nil
}

// isIdempotentMethod reports whether method is safe to retry after an ambiguous failure: repeating
// it cannot itself create a duplicate side effect on the server, because performing it more than
// once has the same effect as performing it once.
func isIdempotentMethod(method string) bool {
	switch method {
	case http.MethodGet, http.MethodHead, http.MethodOptions, http.MethodPut, http.MethodDelete:
		return true
	default:
		return false
	}
}

// isRetryableStatus reports whether a response's status code warrants an automatic retry for the
// given request method. Rate-limiting (429) is always safe to retry: BitBucket rejected the
// request outright, so it was never processed. The upstream-gateway statuses (502/503/504) are
// ambiguous - the request may have reached and been applied by BitBucket before the gateway lost
// the response - so they are only retried for idempotent methods; retrying a non-idempotent
// POST/PATCH on one of these could create a duplicate pull request, comment, task, or repeat a
// merge/approve/decline.
func isRetryableStatus(method string, statusCode int) bool {
	switch statusCode {
	case http.StatusTooManyRequests:
		return true
	case http.StatusBadGateway, http.StatusServiceUnavailable, http.StatusGatewayTimeout:
		return isIdempotentMethod(method)
	default:
		return false
	}
}

// isPreSendConnectionError reports whether err represents a failure establishing the connection
// itself (DNS resolution, TCP dial, TLS handshake) rather than one that could have occurred after
// the request was already written to the wire. Such failures are safe to retry even for a
// non-idempotent method, since the server never received the request.
func isPreSendConnectionError(err error) bool {
	var opErr *net.OpError
	if errors.As(err, &opErr) {
		switch opErr.Op {
		case "dial", "resolve":
			return true
		}
	}
	var dnsErr *net.DNSError
	return errors.As(err, &dnsErr)
}

// isRetryableError reports whether a transport-level error (no response received at all) warrants
// an automatic retry for the given request method: always for idempotent methods, and for a
// non-idempotent method only when the failure demonstrably happened before the request could have
// reached the server.
func isRetryableError(method string, err error) bool {
	return isIdempotentMethod(method) || isPreSendConnectionError(err)
}

// retryDelay computes how long to wait before the next attempt: the response's Retry-After header
// when present and strictly positive (seconds, per RFC 9110), capped at maxRetryAfterBackoff;
// otherwise exponential backoff capped at maxRetryBackoff. A "Retry-After: 0" (or a negative/
// unparsable value) is treated the same as no header at all - use the computed backoff - rather
// than as "retry with no delay", which would otherwise hammer a server that just told us it is
// rate-limiting us.
func retryDelay(attempt int, headers http.Header) time.Duration {
	if headers != nil {
		if retryAfter := headers.Get("Retry-After"); retryAfter != "" {
			if seconds, err := strconv.Atoi(retryAfter); err == nil && seconds > 0 {
				return min(time.Duration(seconds)*time.Second, maxRetryAfterBackoff)
			}
		}
	}
	return min(initialRetryBackoff*time.Duration(1<<attempt), maxRetryBackoff)
}

// doRequest performs a single attempt of the request built by newReq (called with a context
// bound to requestTimeout) and reads its body fully into the returned Response.
func doRequest(ctx context.Context, newReq func(context.Context) (*http.Request, error)) (*Response, error) {
	attemptCtx, cancel := context.WithTimeout(ctx, requestTimeout)
	defer cancel()

	req, err := newReq(attemptCtx)
	if err != nil {
		return nil, err
	}

	res, err := httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("cannot send request: %w", err)
	}
	defer func() { _ = res.Body.Close() }()

	data, err := io.ReadAll(res.Body)
	if err != nil {
		return nil, fmt.Errorf("cannot read response body: %w", err)
	}
	return &Response{StatusCode: res.StatusCode, StatusText: res.Status, Headers: res.Header, Body: data}, nil
}

// doRequestWithRetry retries doRequest up to maxRequestAttempts times, bounded overall by
// maxRetryBudget, on transport errors and on the retryable status codes reported by
// isRetryableStatus (both method-aware: see isRetryableError/isRetryableStatus), waiting
// retryDelay between attempts (honoring the response's Retry-After header when present, capped at
// maxRetryAfterBackoff) and aborting immediately if ctx is done. newReq is invoked once per
// attempt so a request body can be rebuilt from scratch each time (an *http.Request's body can
// only be read once).
func doRequestWithRetry(ctx context.Context, method string, newReq func(context.Context) (*http.Request, error)) (*Response, error) {
	budgetCtx, cancel := context.WithTimeout(ctx, maxRetryBudget)
	defer cancel()

	var lastErr error
	for attempt := range maxRequestAttempts {
		result, err := doRequest(budgetCtx, newReq)
		if err == nil && !isRetryableStatus(method, result.StatusCode) {
			return result, nil
		}
		if err != nil && !isRetryableError(method, err) {
			return nil, err
		}
		lastErr = err
		if attempt == maxRequestAttempts-1 {
			if err != nil {
				return nil, err
			}
			return result, nil
		}

		var delay time.Duration
		if err != nil {
			delay = retryDelay(attempt, nil)
			lgr.Printf("[WARN] request failed (%v), retrying in %s (attempt %d/%d)", err, delay, attempt+2, maxRequestAttempts)
		} else {
			delay = retryDelay(attempt, result.Headers)
			lgr.Printf("[WARN] request returned %s, retrying in %s (attempt %d/%d)", result.StatusText, delay, attempt+2, maxRequestAttempts)
		}
		select {
		case <-budgetCtx.Done():
			return nil, fmt.Errorf("request retry budget exceeded: %w", budgetCtx.Err())
		case <-time.After(delay):
		}
	}
	return nil, lastErr
}

// resolveAuthorization computes the Authorization header value for a request: Basic auth from
// the profile's User/password when set, otherwise the OAuth2 bearer token (refreshing it first
// if needed)
func (profile *Profile) resolveAuthorization(ctx context.Context) (string, error) {
	if profile.User != "" {
		password, err := profile.GetPassword(ctx)
		if err != nil {
			return "", err
		}
		return basicAuthorization(profile.User, password), nil
	}
	return profile.authorize(ctx)
}

// resolveRequestURL turns a uripath (either an absolute URL or a path relative to the profile's
// API root) into the final request URL
func resolveRequestURL(apiRoot *url.URL, uripath string) (*url.URL, error) {
	if apiRoot == nil {
		apiRoot = &url.URL{Scheme: "https", Host: "api.bitbucket.org"}
	}
	if !strings.HasPrefix(uripath, "/") {
		reqURL, err := url.Parse(uripath)
		if err != nil {
			return nil, fmt.Errorf("cannot parse url: %w", err)
		}
		// uripath is absolute here, which only happens for a paginated "next" URL taken
		// verbatim from a BitBucket response (see GetAll/nextPageURL). That URL is
		// server-controlled, so reject it unless its scheme and host still match the
		// profile's own API root - otherwise the Authorization header send() attaches next
		// would be sent to whatever host the response claimed.
		if !strings.EqualFold(reqURL.Scheme, apiRoot.Scheme) || !strings.EqualFold(reqURL.Host, apiRoot.Host) {
			return nil, fmt.Errorf("refusing to send request to %s://%s: does not match the profile's API root %s://%s", reqURL.Scheme, reqURL.Host, apiRoot.Scheme, apiRoot.Host)
		}
		return reqURL, nil
	}
	components := strings.Split(uripath, "?")
	reqURL := apiRoot.JoinPath("2.0", components[0])
	if len(components) > 1 {
		reqURL.RawQuery = components[1]
	}
	return reqURL, nil
}

// mapErrorResponse turns a non-2xx response into an error: BitBucket's own error payload when
// the body carries one, a generic status error otherwise
func mapErrorResponse(result *Response) error {
	var bberr BitBucketError
	// BitBucketError.UnmarshalJSON's last-resort fallback succeeds for any valid JSON object
	// (e.g. a proxy's {"message":"..."} shape, or "{}"), leaving Message/Detail/Fields all
	// empty; only trust it when it actually carried something, otherwise fall through to the
	// generic status-text error so failures are never reported as a completely blank message.
	if jerr := json.Unmarshal(result.Body, &bberr); jerr == nil && (bberr.Message != "" || bberr.Detail != "" || len(bberr.Fields) > 0) {
		lgr.Printf("[WARN] we have a BitBucketError: %#+v", bberr)
		return &bberr
	}
	return fmt.Errorf("cannot send request: %s", result.StatusText)
}

func (profile *Profile) send(ctx context.Context, options *requestOptions, uripath string, response any) (result *Response, err error) {
	authorization, err := profile.resolveAuthorization(ctx)
	if err != nil {
		return nil, err
	}

	reqURL, err := resolveRequestURL(profile.APIRoot, uripath)
	if err != nil {
		return nil, err
	}

	var payload []byte
	if options.Payload != nil {
		payload, err = json.Marshal(options.Payload)
		if err != nil {
			return nil, fmt.Errorf("cannot marshal payload: %w", err)
		}
	}

	accept := options.Accept
	if accept == "" {
		accept = "*/*"
		if response != nil {
			accept = "application/json"
		}
	}

	newReq := func(reqCtx context.Context) (*http.Request, error) {
		var body io.Reader
		if payload != nil {
			body = bytes.NewReader(payload)
		}
		req, reqErr := http.NewRequestWithContext(reqCtx, options.Method, reqURL.String(), body)
		if reqErr != nil {
			return nil, fmt.Errorf("cannot build request: %w", reqErr)
		}
		req.Header.Set("Authorization", authorization)
		req.Header.Set("User-Agent", userAgent)
		req.Header.Set("Accept", accept)
		if payload != nil {
			req.Header.Set("Content-Type", "application/json")
		}
		return req, nil
	}

	lgr.Printf("[DEBUG] sending %s request to %s", options.Method, reqURL)
	result, err = doRequestWithRetry(ctx, options.Method, newReq)
	if err != nil {
		return nil, err
	}
	lgr.Printf("[DEBUG] received %s for %s %s", result.StatusText, options.Method, reqURL)

	if result.StatusCode >= http.StatusBadRequest {
		return result, mapErrorResponse(result)
	}

	if response != nil && len(result.Body) > 0 {
		if jerr := json.Unmarshal(result.Body, response); jerr != nil {
			return result, fmt.Errorf("cannot unmarshal response: %w", jerr)
		}
	}
	return result, nil
}
