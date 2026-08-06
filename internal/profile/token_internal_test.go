package profile

import (
	"path/filepath"
	"strings"
	"testing"
)

// TestAccessTokenCacheFilenameSanitizesProfileName reproduces the path-traversal regression: the
// on-disk access-token cache filename used to embed the profile name verbatim
// ("access-token-"+profile.Name), so a profile imported or hand-edited into the config file with
// a name like "../../../etc/cron.d/evil" could make loadAccessToken/saveAccessToken read or write
// a file outside the cache directory entirely. The filename must now be a pure hash of the name,
// containing none of its characters and no path separators.
func TestAccessTokenCacheFilenameSanitizesProfileName(t *testing.T) {
	malicious := "../../../etc/cron.d/evil"

	filename := accessTokenCacheFilename(malicious)

	if strings.ContainsAny(filename, `/\`) {
		t.Fatalf("accessTokenCacheFilename(%q) = %q, must not contain a path separator", malicious, filename)
	}
	if strings.Contains(filename, "..") {
		t.Fatalf("accessTokenCacheFilename(%q) = %q, must not contain a path-traversal segment", malicious, filename)
	}
	if filepath.Base(filename) != filename {
		t.Fatalf("accessTokenCacheFilename(%q) = %q, must be a single path element", malicious, filename)
	}
}

// TestAccessTokenCacheFilenameIsDeterministicAndDistinct proves the filename is stable for a given
// profile name (so a profile's cache round-trips across process runs) and differs between
// distinct names (so two profiles never collide on the same cache file).
func TestAccessTokenCacheFilenameIsDeterministicAndDistinct(t *testing.T) {
	first := accessTokenCacheFilename("alpha")
	again := accessTokenCacheFilename("alpha")
	other := accessTokenCacheFilename("beta")

	if first != again {
		t.Fatalf("accessTokenCacheFilename(\"alpha\") is not deterministic: %q != %q", first, again)
	}
	if first == other {
		t.Fatalf("accessTokenCacheFilename(\"alpha\") and accessTokenCacheFilename(\"beta\") collide: %q", first)
	}
}
