package testutil

import (
	"net/http"
	"strings"
	"testing"
)

// WorkspaceScopeDeniedHandler wraps next, intercepting any request whose path contains
// "/workspaces/" with the exact 403 shape BitBucket returns for a token that lacks
// read:workspace, and passing every other request through to next unchanged. Callers that need
// to assert no such request was ever attempted should record requests in their own wrapper around
// the handler this returns, before delegating to it -- recording only inside next would never see
// a request this handler itself answers.
func WorkspaceScopeDeniedHandler(t *testing.T, next http.HandlerFunc) http.HandlerFunc {
	t.Helper()
	return func(w http.ResponseWriter, r *http.Request) {
		if strings.Contains(r.URL.Path, "/workspaces/") {
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusForbidden)
			_, _ = w.Write([]byte(`{"type":"error","error":{"message":"Your credentials lack one or more required privilege scopes. (required: read:workspace:bitbucket)"}}`))
			return
		}
		next(w, r)
	}
}
