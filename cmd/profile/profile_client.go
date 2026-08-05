package profile

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"

	"github.com/go-pkgz/lgr"
	"github.com/spf13/cobra"
)

// requestTimeout is the deadline applied to every request sent to the BitBucket API and to
// the OAuth2 token endpoint.
const requestTimeout = 30 * time.Second

// userAgent is sent with every outgoing request.
const userAgent = "bitbucket-cli"

// oauthTokenURL is BitBucket's OAuth2 token endpoint.
const oauthTokenURL = "https://bitbucket.org/site/oauth2/access_token" //nolint:gosec // endpoint URL, not a credential

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

type PaginatedResources[T any] struct {
	Values   []T    `json:"values"`
	Page     int    `json:"page"`
	PageSize int    `json:"pagelen"`
	Size     int    `json:"size"`
	Next     string `json:"next"`
	Previous string `json:"previous"`
}

// Post posts a resource
func (profile *Profile) Post(ctx context.Context, cmd *cobra.Command, uripath string, body, response any) (err error) {
	_, err = profile.send(ctx, &requestOptions{Method: http.MethodPost, Payload: body}, uripath, response)
	return
}

// PostWithResult posts a resource and returns the raw result
func (profile *Profile) PostWithResult(ctx context.Context, cmd *cobra.Command, uripath string, body any) (result *Response, err error) {
	return profile.send(ctx, &requestOptions{Method: http.MethodPost, Payload: body}, uripath, nil)
}

// Get gets a resource
func (profile *Profile) Get(ctx context.Context, cmd *cobra.Command, uripath string, response any) (err error) {
	_, err = profile.send(ctx, &requestOptions{Method: http.MethodGet}, uripath, response)
	return
}

// GetRaw gets a resource without unmarshaling it
func (profile *Profile) GetRaw(ctx context.Context, cmd *cobra.Command, uripath string) (raw io.Reader, err error) {
	result, err := profile.send(ctx, &requestOptions{Method: http.MethodGet, Accept: "*/*"}, uripath, nil)
	if result == nil {
		return nil, err
	}
	return bytes.NewReader(result.Body), err
}

// Put puts/updates a resource
func (profile *Profile) Put(ctx context.Context, cmd *cobra.Command, uripath string, body, response any) (err error) {
	_, err = profile.send(ctx, &requestOptions{Method: http.MethodPut, Payload: body}, uripath, response)
	return
}

// Delete deletes a resource
func (profile *Profile) Delete(ctx context.Context, cmd *cobra.Command, uripath string, response any) (err error) {
	_, err = profile.send(ctx, &requestOptions{Method: http.MethodDelete}, uripath, response)
	return
}

// Patch patches a resource
func (profile *Profile) Patch(ctx context.Context, cmd *cobra.Command, uripath string, body, response any) (err error) {
	_, err = profile.send(ctx, &requestOptions{Method: http.MethodPatch, Payload: body}, uripath, response)
	return
}

// GetAll gets all resources of the given type
//
// The Current profile will be set to the profile of the command
// resolvePageLengthAndLimit reads the --page-length and --limit flags, falling back to defaultPageLength
func resolvePageLengthAndLimit(cmd *cobra.Command, defaultPageLength int) (pageLength, limit int) {
	pageLength = defaultPageLength
	if cmd != nil && cmd.Flag("page-length") != nil && cmd.Flag("page-length").Changed {
		if length, err := cmd.Flags().GetInt("page-length"); err == nil && length > 0 {
			pageLength = length
			lgr.Printf("[DEBUG] using page length of %d from the command line flags", pageLength)
		}
	}
	if cmd != nil && cmd.Flag("limit") != nil && cmd.Flag("limit").Changed {
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

func GetAll[T any](ctx context.Context, cmd *cobra.Command, uripath string) (resources []T, err error) {
	profile, err := GetProfileFromCommand(ctx, cmd)
	if err != nil {
		lgr.Printf("[ERROR] failed to get profile: %v", err)
		return nil, err
	}
	Current = profile // Make sure the current profile is set

	pageLength, limit := resolvePageLengthAndLimit(cmd, Current.DefaultPageLength)

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
			cmd,
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
			resultchan <- err
			return
		}

		lgr.Printf("[DEBUG] requesting authorization token for profile %s", profile.Name)
		result, err := sendOAuthTokenRequest(r.Context(), profile.ClientID, clientSecret, map[string]string{
			"grant_type": "authorization_code",
			"code":       code,
		})
		if err != nil {
			writeAuthorizationErrorResponse(w, err, result)
			resultchan <- err
			return
		}
		if _, err := profile.saveAccessToken(r.Context(), result.Body); err != nil {
			lgr.Printf("[ERROR] failed to save access token for profile %s: %v", profile.Name, err)
			if errors.Is(err, ErrUnmarshalJSON) {
				http.Error(w, "Failed to parse access token response from BitBucket: "+err.Error(), http.StatusBadRequest)
			} else {
				http.Error(w, "Failed to save access token for profile "+profile.Name+": "+err.Error(), http.StatusInternalServerError)
			}
			resultchan <- err
			return
		}
		_, _ = w.Write([]byte("Authorization Code received. You can close this window."))
		resultchan <- nil
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
	if loadErr := profile.loadAccessToken(ctx); loadErr == nil {
		if !profile.isTokenExpired() {
			lgr.Printf("[DEBUG] using access token for profile %s", profile.Name)
			lgr.Printf("[DEBUG] token expires on %s in %s", profile.token.GetExpiresOn().Format(time.RFC3339), profile.token.GetExpiresIn())
			return bearerAuthorization(profile.token.AccessToken), nil
		}
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

	ctx, cancel := context.WithTimeout(ctx, requestTimeout)
	defer cancel()

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, oauthTokenURL, strings.NewReader(form.Encode()))
	if err != nil {
		return nil, fmt.Errorf("cannot build oauth token request: %w", err)
	}
	req.Header.Set("Authorization", basicAuthorization(clientID, clientSecret))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	req.Header.Set("User-Agent", userAgent)

	res, err := httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("cannot send oauth token request: %w", err)
	}
	defer func() { _ = res.Body.Close() }()

	data, err := io.ReadAll(res.Body)
	if err != nil {
		return nil, fmt.Errorf("cannot read oauth token response: %w", err)
	}

	result := &Response{StatusCode: res.StatusCode, StatusText: res.Status, Headers: res.Header, Body: data}
	if res.StatusCode >= http.StatusBadRequest {
		return result, fmt.Errorf("oauth token request failed: %s", res.Status)
	}
	return result, nil
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
	if jerr := json.Unmarshal(result.Body, &bberr); jerr == nil {
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

	var body io.Reader
	if options.Payload != nil {
		payload, jerr := json.Marshal(options.Payload)
		if jerr != nil {
			return nil, fmt.Errorf("cannot marshal payload: %w", jerr)
		}
		body = bytes.NewReader(payload)
	}

	requestCtx, cancel := context.WithTimeout(ctx, requestTimeout)
	defer cancel()

	req, err := http.NewRequestWithContext(requestCtx, options.Method, reqURL.String(), body)
	if err != nil {
		return nil, fmt.Errorf("cannot build request: %w", err)
	}
	req.Header.Set("Authorization", authorization)
	req.Header.Set("User-Agent", userAgent)
	accept := options.Accept
	if accept == "" {
		accept = "*/*"
		if response != nil {
			accept = "application/json"
		}
	}
	req.Header.Set("Accept", accept)
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}

	lgr.Printf("[DEBUG] sending %s request to %s", options.Method, reqURL)
	res, err := httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("cannot send request: %w", err)
	}
	defer func() { _ = res.Body.Close() }()

	data, err := io.ReadAll(res.Body)
	if err != nil {
		return nil, fmt.Errorf("cannot read response body: %w", err)
	}
	result = &Response{StatusCode: res.StatusCode, StatusText: res.Status, Headers: res.Header, Body: data}
	lgr.Printf("[DEBUG] received %s for %s %s", res.Status, options.Method, reqURL)

	if res.StatusCode >= http.StatusBadRequest {
		return result, mapErrorResponse(result)
	}

	if response != nil && len(data) > 0 {
		if jerr := json.Unmarshal(data, response); jerr != nil {
			return result, fmt.Errorf("cannot unmarshal response: %w", jerr)
		}
	}
	return result, nil
}
