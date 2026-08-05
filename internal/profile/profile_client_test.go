package profile_test

import (
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"

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
	err = target.Get(suite.Context, &cobra.Command{}, "/repo", &item)
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
	err = target.Post(suite.Context, &cobra.Command{}, "/repo", testItem{ID: "1"}, &result)
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
	err = target.Get(suite.Context, &cobra.Command{}, "/repo", &item)
	suite.Require().Error(err)
	var bberr *profile.BitBucketError
	suite.Require().ErrorAs(err, &bberr, "a BitBucket-shaped error body should be mapped to a *BitBucketError")
	suite.Equal("Resource not found", bberr.Message)
	suite.Equal("There is no API hosted at this URL", bberr.Detail)
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
	err = target.Get(suite.Context, &cobra.Command{}, "/repo", &item)
	suite.Require().Error(err)
	var bberr *profile.BitBucketError
	suite.Require().NotErrorAs(err, &bberr, "a non-JSON error body should not be mapped to a BitBucketError")
	suite.Contains(err.Error(), "500")
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

	reader, err := target.GetRaw(suite.Context, &cobra.Command{}, "/repo/diff")
	suite.Require().NoError(err)
	data, err := io.ReadAll(reader)
	suite.Require().NoError(err)
	suite.Equal(rawBody, string(data))
	suite.Equal("*/*", gotAccept)
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

	result, err := target.PostWithResult(suite.Context, &cobra.Command{}, "/repo/merge", nil)
	suite.Require().NoError(err)
	suite.Require().NotNil(result)
	suite.Equal(server.URL+"/task-status/123", result.Headers.Get("Location"))
}
