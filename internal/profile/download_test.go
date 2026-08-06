package profile_test

import (
	"bytes"
	"net/http"
	"net/http/httptest"
	"net/url"

	"github.com/avitsrimer/bitbucket-cli/internal/profile"
)

func (suite *ProfileSuite) TestDownload_CopiesBodyToDestination() {
	const content = "artifact-bytes"

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		suite.Equal("/2.0/downloads/build.log", r.URL.Path)
		suite.Equal("Bearer dummy-token", r.Header.Get("Authorization"))
		_, _ = w.Write([]byte(content))
	}))
	defer server.Close()

	apiRoot, err := url.Parse(server.URL)
	suite.Require().NoError(err)
	target := &profile.Profile{APIRoot: apiRoot, AccessToken: "dummy-token"}

	var dest bytes.Buffer
	written, err := target.Download(suite.Context, "/downloads/build.log", &dest)
	suite.Require().NoError(err)
	suite.Equal(int64(len(content)), written)
	suite.Equal(content, dest.String())
}

func (suite *ProfileSuite) TestDownload_APIErrorLeavesDestinationUntouched() {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusNotFound)
		_, _ = w.Write([]byte(`{"type":"error","error":{"message":"artifact not found"}}`))
	}))
	defer server.Close()

	apiRoot, err := url.Parse(server.URL)
	suite.Require().NoError(err)
	target := &profile.Profile{APIRoot: apiRoot, AccessToken: "dummy-token"}

	var dest bytes.Buffer
	written, err := target.Download(suite.Context, "/downloads/missing.log", &dest)
	suite.Require().Error(err)
	suite.Contains(err.Error(), "artifact not found")
	suite.Equal(int64(0), written)
	suite.Equal(0, dest.Len())
}

// TestDownload_DoesNotRetryOnRetryableStatus is a regression test: Download must not go through
// doRequestWithRetry (the buffering that retry logic depends on is exactly what Download exists to
// avoid), so a retryable status code like 503 must still result in exactly one request.
func (suite *ProfileSuite) TestDownload_DoesNotRetryOnRetryableStatus() {
	var requestCount int
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		requestCount++
		w.WriteHeader(http.StatusServiceUnavailable)
	}))
	defer server.Close()

	apiRoot, err := url.Parse(server.URL)
	suite.Require().NoError(err)
	target := &profile.Profile{APIRoot: apiRoot, AccessToken: "dummy-token"}

	var dest bytes.Buffer
	_, err = target.Download(suite.Context, "/downloads/build.log", &dest)
	suite.Require().Error(err)
	suite.Equal(1, requestCount)
}
