package profile

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"

	"github.com/gildas/go-core"
	"github.com/gildas/go-errors"
	"github.com/gildas/go-logger"
	"github.com/gildas/go-request"
	"github.com/spf13/cobra"
)

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
	options := &request.Options{Method: http.MethodPost, Payload: body}
	_, err = profile.send(ctx, options, uripath, response)
	return
}

// PostWithResult posts a resource and returns the raw result
func (profile *Profile) PostWithResult(ctx context.Context, cmd *cobra.Command, uripath string, body any) (result *request.Content, err error) {
	options := &request.Options{Method: http.MethodPost, Payload: body}
	return profile.send(ctx, options, uripath, nil)
}

// Get gets a resource
func (profile *Profile) Get(ctx context.Context, cmd *cobra.Command, uripath string, response any) (err error) {
	options := &request.Options{Method: http.MethodGet}
	_, err = profile.send(ctx, options, uripath, response)
	return
}

// GetRaw gets a resource without unmarshaling it
func (profile *Profile) GetRaw(ctx context.Context, cmd *cobra.Command, uripath string) (raw io.Reader, err error) {
	options := &request.Options{
		Method: http.MethodGet,
		Accept: "*/*",
	}
	result, err := profile.send(ctx, options, uripath, nil)
	return result.Reader(), err
}

// Put puts/updates a resource
func (profile *Profile) Put(ctx context.Context, cmd *cobra.Command, uripath string, body, response any) (err error) {
	options := &request.Options{Method: http.MethodPut, Payload: body}
	_, err = profile.send(ctx, options, uripath, response)
	return
}

// Delete deletes a resource
func (profile *Profile) Delete(ctx context.Context, cmd *cobra.Command, uripath string, response any) (err error) {
	options := &request.Options{Method: http.MethodDelete}
	_, err = profile.send(ctx, options, uripath, response)
	return
}

// Patch patches a resource
func (profile *Profile) Patch(ctx context.Context, cmd *cobra.Command, uripath string, body, response any) (err error) {
	options := &request.Options{Method: http.MethodPatch, Payload: body}
	_, err = profile.send(ctx, options, uripath, response)
	return
}

// GetAll gets all resources of the given type
//
// The Current profile will be set to the profile of the command
// resolvePageLengthAndLimit reads the --page-length and --limit flags, falling back to defaultPageLength
func resolvePageLengthAndLimit(log *logger.Logger, cmd *cobra.Command, defaultPageLength int) (pageLength, limit int) {
	pageLength = defaultPageLength
	if cmd != nil && cmd.Flag("page-length") != nil && cmd.Flag("page-length").Changed {
		if length, err := cmd.Flags().GetInt("page-length"); err == nil && length > 0 {
			pageLength = length
			log.Debugf("Using page length of %d from the command line flags", pageLength)
		}
	}
	if cmd != nil && cmd.Flag("limit") != nil && cmd.Flag("limit").Changed {
		if l, err := cmd.Flags().GetInt("limit"); err == nil && l > 0 {
			limit = l
			log.Debugf("Using limit of %d from the command line flags", limit)
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
		return "", err
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
	log := logger.Must(logger.FromContext(ctx)).Child(nil, "getall")

	profile, err := GetProfileFromCommand(ctx, cmd)
	if err != nil {
		log.Errorf("Failed to get profile.", err)
		return nil, err
	}
	Current = profile // Make sure the current profile is set

	pageLength, limit := resolvePageLengthAndLimit(log, cmd, Current.DefaultPageLength)

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
		log.Infof("Getting up to %d resources for profile %s (%d at a time)", limit, profile.Name, pageLength)
	} else {
		log.Infof("Getting all resources for profile %s (%d at a time)", profile.Name, pageLength)
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
		log.Debugf("Got %d resources (total: %d)", len(paginated.Values), len(resources))
		log.Debugf("Next page:     %s", paginated.Next)
		log.Debugf("Previous page: %s", paginated.Previous)
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
		log := logger.Must(logger.FromContext(r.Context())).Child(nil, nil, "profile", profile.Name)

		if r.Method != http.MethodGet {
			http.Error(w, "Method Not Allowed", http.StatusMethodNotAllowed)
			return
		}
		if r.URL.Path == "/favicon.ico" {
			_, _ = w.Write([]byte{})
			return
		}

		log.Infof("Received callback from BitBucket")
		code := r.URL.Query().Get("code")
		if code == "" {
			log.Errorf("No code in the callback")
			http.Error(w, "No code in the callback", http.StatusBadRequest)
			return
		}
		log.Infof("Received code %s", code)

		// Get the client secret from the vault if it is empty
		clientSecret, err := profile.GetClientSecret(r.Context())
		if err != nil {
			log.Errorf("Failed to get client secret for profile %s: %v", profile.Name, err)
			http.Error(w, "Failed to get client secret for profile "+profile.Name+": "+err.Error(), http.StatusUnauthorized)
			resultchan <- err
			return
		}

		log.Infof("Requesting authorization token for profile %s", profile.Name)
		result, err := request.Send(&request.Options{
			Method:        http.MethodPost,
			Authorization: request.BasicAuthorization(profile.ClientID, clientSecret),
			URL:           core.Must(url.Parse("https://bitbucket.org/site/oauth2/access_token")),
			Payload:       map[string]string{"grant_type": "authorization_code", "code": code},
			Timeout:       30 * time.Second,
			Logger:        log,
		}, nil)
		if err != nil {
			writeAuthorizationErrorResponse(w, err, result)
			resultchan <- err
			return
		}
		if _, err := profile.saveAccessToken(r.Context(), result.Data); err != nil {
			log.Errorf("Failed to save access token for profile %s: %v", profile.Name, err)
			if errors.Is(err, errors.JSONUnmarshalError) {
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
func writeAuthorizationErrorResponse(w http.ResponseWriter, err error, result *request.Content) {
	if result == nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	var errorResponse struct {
		Error            string `json:"error"`
		ErrorDescription string `json:"error_description"`
	}
	if jerr := result.UnmarshalContentJSON(&errorResponse); jerr != nil {
		return
	}
	var details *errors.Error
	status := http.StatusInternalServerError
	if errors.As(err, &details) {
		status = details.Code
	}
	http.Error(w, errorResponse.ErrorDescription, status)
}

func (profile *Profile) authorize(ctx context.Context) (authorization string, err error) {
	log := logger.Must(logger.FromContext(ctx)).Child("profile", "authorize")

	if loadErr := profile.loadAccessToken(ctx); loadErr == nil {
		if !profile.isTokenExpired() {
			log.Infof("Using access token for profile %s", profile.Name)
			log.Debugf("Token expires on %s in %s", profile.token.GetExpiresOn().Format(time.RFC3339), profile.token.GetExpiresIn())
			return request.BearerAuthorization(profile.token.AccessToken), nil
		}
	}

	payload := map[string]string{}
	if profile.token != nil && profile.token.RefreshToken != "" {
		log.Warnf("Access token for profile %s expired %s ago and we have a refresh token", profile.Name, profile.token.GetExpiredSince())
		payload["grant_type"] = "refresh_token"
		payload["refresh_token"] = profile.token.RefreshToken
	} else {
		if profile.token != nil {
			log.Warnf("Access token for profile %s expired %s ago but we don't have a refresh token", profile.Name, profile.token.GetExpiredSince())
		} else {
			log.Warnf("No access token found for profile %s, we need to authorize the profile", profile.Name)
		}
		payload["grant_type"] = "client_credentials"
	}

	// Get the client secret from the vault if it is empty
	clientSecret, err := profile.GetClientSecret(ctx)
	if err != nil {
		return "", err
	}
	log.Infof("Authorizing profile %s", profile.Name)
	result, err := request.Send(&request.Options{
		Method:        http.MethodPost,
		Authorization: request.BasicAuthorization(profile.ClientID, clientSecret),
		URL:           core.Must(url.Parse("https://bitbucket.org/site/oauth2/access_token")),
		Payload:       payload,
		Timeout:       30 * time.Second,
		Logger:        log,
	}, nil)
	if err != nil {
		return "", bitbucketAuthError(err, result)
	}
	accessToken, err := profile.saveAccessToken(ctx, result.Data)
	if err != nil {
		return "", err
	}
	return request.BearerAuthorization(accessToken), err
}

// bitbucketAuthError turns a failed authorization request into a sentinel error carrying
// BitBucket's own error payload, when one is available
func bitbucketAuthError(err error, result *request.Content) error {
	if result == nil {
		return err
	}
	var errorResponse struct {
		Error            string `json:"error"`
		ErrorDescription string `json:"error_description"`
	}
	if jerr := result.UnmarshalContentJSON(&errorResponse); jerr != nil {
		return err
	}
	var details *errors.Error
	if errors.As(err, &details) {
		return errors.NewSentinel(details.Code, errorResponse.Error, errorResponse.ErrorDescription)
	}
	return errors.NewSentinel(500, errorResponse.Error, errorResponse.ErrorDescription)
}

func (profile *Profile) send(ctx context.Context, options *request.Options, uripath string, response any) (result *request.Content, err error) {
	log := logger.Must(logger.FromContext(ctx)).Child(nil, strings.ToLower(options.Method))

	if profile.User != "" {
		password, passErr := profile.GetPassword(ctx)
		if passErr != nil {
			return nil, passErr
		}
		options.Authorization = request.BasicAuthorization(profile.User, password)
	} else if options.Authorization, err = profile.authorize(ctx); err != nil {
		return nil, err
	}

	apiRoot := profile.APIRoot
	if apiRoot == nil {
		apiRoot = &url.URL{Scheme: "https", Host: "api.bitbucket.org"}
	}

	if strings.HasPrefix(uripath, "/") {
		components := strings.Split(uripath, "?")
		options.URL = apiRoot.JoinPath("2.0", components[0])
		if len(components) > 1 {
			options.URL.RawQuery = components[1]
		}
	} else {
		if options.URL, err = url.Parse(uripath); err != nil {
			return nil, err
		}
	}

	if options.Timeout == 0 {
		options.Timeout = 30 * time.Second
	}
	if options.Logger == nil {
		options.Logger = log
	}
	if options.RequestBodyLogSize == 0 {
		options.RequestBodyLogSize = 16 * 1024
	}
	if options.ResponseBodyLogSize == 0 {
		options.ResponseBodyLogSize = 16 * 1024
	}
	if options.ProgressWriter != nil {
		log.Warnf("[B] We have a ProgressWriter for uploading content")
	}
	log.Infof("Sending %s request to %s", options.Method, options.URL)
	result, err = request.Send(options, response)
	if err != nil {
		if errors.Is(err, errors.JSONUnmarshalError) {
			return result, err
		}
		if result != nil {
			var bberr *BitBucketError
			jerr := result.UnmarshalContentJSON(&bberr)
			if jerr == nil {
				log.Warnf("We have a BitBucketError: %#+v", bberr)
				return result, bberr
			}
			log.Debugf("the Error %s is not a bitbucket error: %s", err.Error(), jerr.Error())
		}
	}
	return result, err
}
