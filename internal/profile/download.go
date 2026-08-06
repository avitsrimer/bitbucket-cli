package profile

import (
	"context"
	"fmt"
	"io"
	"net/http"

	"github.com/go-pkgz/lgr"
)

// Download performs a single GET request for uripath and copies the response body directly to
// dest via io.Copy, without buffering it in memory the way send/GetRaw do: an artifact can be
// arbitrarily large binary content, and buffering the whole thing before it ever reaches disk
// would be wasteful at best and exhaust memory on a large enough download at worst.
//
// Unlike every other request this client makes, a download is not retried on a transport error or
// a retryable status code: doRequestWithRetry's retry logic depends on having buffered the full
// body already to decide whether to retry, which is exactly the buffering this method exists to
// avoid, and retrying a download that already copied part of its body to dest risks silently
// corrupting whatever dest is backed by. A failed download simply returns an error; the caller
// (the artifact download command) writes to a temporary file and only moves it into place on
// success, so a failed attempt here never corrupts an existing destination file.
//
// The shared httpClient follows redirects using the standard library's default policy, which
// strips the Authorization header whenever a redirect crosses to a different host. That is
// exactly the behavior wanted here: BitBucket's /downloads/<name> endpoint redirects to a signed,
// third-party storage URL (bbuseruploads) that must never see this client's own bearer/basic
// credentials.
func (profile *Profile) Download(ctx context.Context, uripath string, dest io.Writer) (written int64, err error) {
	authorization, err := profile.resolveAuthorization(ctx)
	if err != nil {
		return 0, err
	}

	reqURL, err := resolveRequestURL(profile.APIRoot, uripath)
	if err != nil {
		return 0, err
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, reqURL.String(), http.NoBody)
	if err != nil {
		return 0, fmt.Errorf("cannot build request: %w", err)
	}
	req.Header.Set("Authorization", authorization)
	req.Header.Set("User-Agent", userAgent)
	req.Header.Set("Accept", "*/*")

	lgr.Printf("[DEBUG] downloading %s", reqURL.Redacted())
	res, err := httpClient.Do(req)
	if err != nil {
		return 0, fmt.Errorf("cannot send request: %w", err)
	}
	defer func() { _ = res.Body.Close() }()

	if res.StatusCode >= http.StatusBadRequest {
		data, readErr := io.ReadAll(res.Body)
		if readErr != nil {
			return 0, fmt.Errorf("cannot read response body: %w", readErr)
		}
		return 0, mapErrorResponse(&Response{StatusCode: res.StatusCode, StatusText: res.Status, Headers: res.Header, Body: data})
	}

	written, err = io.Copy(dest, res.Body)
	if err != nil {
		return written, fmt.Errorf("cannot write response body: %w", err)
	}
	lgr.Printf("[DEBUG] downloaded %d bytes from %s", written, reqURL.Redacted())
	return written, nil
}
