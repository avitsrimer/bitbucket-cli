package common

import (
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// newTestCache builds a Cache rooted at a fresh temp directory so entries never touch the real
// os.UserCacheDir().
func newTestCache[T any](t *testing.T, expiration time.Duration) *Cache[T] {
	t.Helper()
	return NewCacheAt[T](filepath.Join(t.TempDir(), "bitbucket-cli-test"), expiration)
}

func TestCacheSetAndGet(t *testing.T) {
	t.Run("round-trips a value under its key", func(t *testing.T) {
		cache := newTestCache[string](t, time.Minute)

		require.NoError(t, cache.Set("greeting", "hello"))

		value, err := cache.Get("greeting")
		require.NoError(t, err)
		require.NotNil(t, value)
		assert.Equal(t, "hello", *value)
	})

	t.Run("persists to disk and survives a fresh Cache instance", func(t *testing.T) {
		folder := filepath.Join(t.TempDir(), "bitbucket-cli-test")
		cache := NewCacheAt[string](folder, time.Minute)
		require.NoError(t, cache.Set("greeting", "hello"))

		reloaded := NewCacheAt[string](folder, time.Minute)

		value, err := reloaded.Get("greeting")
		require.NoError(t, err)
		require.NotNil(t, value)
		assert.Equal(t, "hello", *value)
	})

	t.Run("returns an error for a key that was never set", func(t *testing.T) {
		cache := newTestCache[string](t, time.Minute)

		value, err := cache.Get("missing")

		require.Error(t, err)
		assert.Nil(t, value)
	})

	t.Run("never expires when the cache has a zero expiration", func(t *testing.T) {
		cache := newTestCache[string](t, 0)
		require.NoError(t, cache.Set("greeting", "hello"))

		value, err := cache.Get("greeting")

		require.NoError(t, err)
		assert.Equal(t, "hello", *value)
	})
}

func TestCacheExpiry(t *testing.T) {
	t.Run("returns a miss once the entry's TTL has elapsed", func(t *testing.T) {
		cache := newTestCache[string](t, time.Nanosecond)
		require.NoError(t, cache.Set("greeting", "hello"))
		time.Sleep(time.Millisecond)

		value, err := cache.Get("greeting")

		require.Error(t, err)
		assert.Nil(t, value)
	})

	t.Run("removes the on-disk file for an expired entry", func(t *testing.T) {
		cache := newTestCache[string](t, time.Nanosecond)
		require.NoError(t, cache.Set("greeting", "hello"))
		time.Sleep(time.Millisecond)

		_, err := cache.Get("greeting")
		require.Error(t, err)

		_, statErr := os.Stat(cache.filename("greeting"))
		assert.True(t, os.IsNotExist(statErr))
	})
}

func TestCacheCorruptFile(t *testing.T) {
	t.Run("treats a corrupt cache file as a miss, not a crash", func(t *testing.T) {
		cache := newTestCache[string](t, time.Minute)
		require.NoError(t, os.MkdirAll(cache.folder, 0o700))
		require.NoError(t, os.WriteFile(cache.filename("greeting"), []byte("not valid json"), 0o600))

		value, err := cache.Get("greeting")

		require.Error(t, err)
		assert.Nil(t, value)
	})
}

func TestNewCacheUsesUserCacheDir(t *testing.T) {
	t.Setenv("BITBUCKET_CLI_CACHE_DURATION", "")
	wantCacheDir, err := os.UserCacheDir()
	require.NoError(t, err)

	cache := NewCache[string]()

	assert.Equal(t, filepath.Join(wantCacheDir, "bitbucket"), cache.folder)
	// NewCache resolves BITBUCKET_CLI_CACHE_DURATION lazily on each Set (see resolveExpiration),
	// not once at construction time, so its expiration is read through that method rather than
	// the raw field.
	assert.Equal(t, 5*time.Minute, cache.resolveExpiration())
}

// TestNewCacheResolvesCacheDurationLazily reproduces the .env-loading-order regression: process
// vars like RepositoryCache are constructed via NewCache at package-init time -- before main() has
// had a chance to load a .env file (see cmd/bb/main.go) -- so BITBUCKET_CLI_CACHE_DURATION set
// only via .env must still take effect once a command actually runs and calls Set, not be
// permanently missed because it wasn't yet in the environment when NewCache ran.
func TestNewCacheResolvesCacheDurationLazily(t *testing.T) {
	cache := NewCache[string]()

	t.Setenv("BITBUCKET_CLI_CACHE_DURATION", "30s")

	assert.Equal(t, 30*time.Second, cache.resolveExpiration())
}

// TestUserCacheDirFallsBackWhenUserCacheDirFails reproduces the container/CI regression where
// os.UserCacheDir()'s error was silently discarded, leaving NewCache to create a relative
// "bitbucket" directory inside the process's current working directory instead of falling back
// the way ConfigPath already does for the analogous os.UserConfigDir() case.
func TestUserCacheDirFallsBackWhenUserCacheDirFails(t *testing.T) {
	t.Setenv("HOME", "")
	t.Setenv("XDG_CACHE_HOME", "")
	if _, err := os.UserCacheDir(); err == nil {
		t.Skip("os.UserCacheDir() still resolves on this platform without $HOME/$XDG_CACHE_HOME; nothing to fall back from")
	}

	dir := userCacheDir()

	assert.NotEmpty(t, dir)
	assert.NotEqual(t, "bitbucket", dir)
	assert.True(t, filepath.IsAbs(dir), "fallback cache dir %q must be absolute, never a relative path rooted at the process's cwd", dir)
}

func TestNewCacheAt(t *testing.T) {
	folder := filepath.Join(t.TempDir(), "custom")
	cache := NewCacheAt[string](folder, 30*time.Second)

	assert.Equal(t, folder, cache.folder)
	assert.Equal(t, 30*time.Second, cache.expiration)
}
