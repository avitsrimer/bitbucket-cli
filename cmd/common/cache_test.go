package common

import (
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// newTestCache builds a Cache rooted at a fresh temp directory (via a stubbed
// os.UserCacheDir) so entries never touch the real user cache.
func newTestCache[T any](t *testing.T, expiration time.Duration, encryptionKey string) *Cache[T] {
	t.Helper()
	cache := newCache[T]("bitbucket-cli-test")
	cache.folder = filepath.Join(t.TempDir(), "bitbucket-cli-test")
	cache.WithExpiration(expiration)
	cache.WithEncryptionKey([]byte(encryptionKey))
	return cache
}

func TestCacheSetAndGet(t *testing.T) {
	t.Run("round-trips a value under its key", func(t *testing.T) {
		cache := newTestCache[string](t, time.Minute, "")

		require.NoError(t, cache.Set("hello", "greeting"))

		value, err := cache.Get("greeting")
		require.NoError(t, err)
		require.NotNil(t, value)
		assert.Equal(t, "hello", *value)
	})

	t.Run("persists to disk and survives a fresh Cache instance (in-memory miss)", func(t *testing.T) {
		folder := filepath.Join(t.TempDir(), "bitbucket-cli-test")
		cache := newCache[string]("bitbucket-cli-test")
		cache.folder = folder
		cache.WithExpiration(time.Minute)
		require.NoError(t, cache.Set("hello", "greeting"))

		reloaded := newCache[string]("bitbucket-cli-test")
		reloaded.folder = folder
		reloaded.WithExpiration(time.Minute)

		value, err := reloaded.Get("greeting")
		require.NoError(t, err)
		require.NotNil(t, value)
		assert.Equal(t, "hello", *value)
	})

	t.Run("returns an error for a key that was never set", func(t *testing.T) {
		cache := newTestCache[string](t, time.Minute, "")

		value, err := cache.Get("missing")

		require.Error(t, err)
		assert.Nil(t, value)
	})

	t.Run("never expires when the cache has a zero expiration", func(t *testing.T) {
		cache := newTestCache[string](t, 0, "")
		require.NoError(t, cache.Set("hello", "greeting"))

		value, err := cache.Get("greeting")

		require.NoError(t, err)
		assert.Equal(t, "hello", *value)
	})
}

func TestCacheExpiry(t *testing.T) {
	t.Run("returns a miss once the entry's TTL has elapsed", func(t *testing.T) {
		cache := newTestCache[string](t, time.Nanosecond, "")
		require.NoError(t, cache.Set("hello", "greeting"))
		time.Sleep(time.Millisecond)

		value, err := cache.Get("greeting")

		require.Error(t, err)
		assert.Nil(t, value)
	})

	t.Run("removes the on-disk file for an expired entry", func(t *testing.T) {
		cache := newTestCache[string](t, time.Nanosecond, "")
		require.NoError(t, cache.Set("hello", "greeting"))
		time.Sleep(time.Millisecond)

		_, err := cache.Get("greeting")
		require.Error(t, err)

		_, statErr := os.Stat(cache.filename("greeting"))
		assert.True(t, os.IsNotExist(statErr))
	})
}

func TestCacheEncryption(t *testing.T) {
	t.Run("round-trips a value when an encryption key is set", func(t *testing.T) {
		cache := newTestCache[string](t, time.Minute, "0123456789abcdef0123456789abcdef")
		require.NoError(t, cache.Set("secret", "token"))

		value, err := cache.Get("token")

		require.NoError(t, err)
		assert.Equal(t, "secret", *value)
	})

	t.Run("stores encrypted entries as non-plaintext on disk", func(t *testing.T) {
		cache := newTestCache[string](t, time.Minute, "0123456789abcdef0123456789abcdef")
		require.NoError(t, cache.Set("secret-value", "token"))

		data, err := os.ReadFile(cache.filename("token"))
		require.NoError(t, err)
		assert.NotContains(t, string(data), "secret-value")
	})

	t.Run("a fresh instance with the same key reads a previously encrypted entry", func(t *testing.T) {
		folder := filepath.Join(t.TempDir(), "bitbucket-cli-test")
		key := "0123456789abcdef0123456789abcdef"
		cache := newCache[string]("bitbucket-cli-test")
		cache.folder = folder
		cache.WithExpiration(time.Minute)
		cache.WithEncryptionKey([]byte(key))
		require.NoError(t, cache.Set("secret", "token"))

		reloaded := newCache[string]("bitbucket-cli-test")
		reloaded.folder = folder
		reloaded.WithExpiration(time.Minute)
		reloaded.WithEncryptionKey([]byte(key))

		value, err := reloaded.Get("token")
		require.NoError(t, err)
		assert.Equal(t, "secret", *value)
	})
}

func TestCacheCorruptFile(t *testing.T) {
	t.Run("treats a corrupt cache file as a miss, not a crash", func(t *testing.T) {
		cache := newTestCache[string](t, time.Minute, "")
		require.NoError(t, os.MkdirAll(cache.folder, 0o700))
		require.NoError(t, os.WriteFile(cache.filename("greeting"), []byte("not valid json"), 0o600))

		value, err := cache.Get("greeting")

		require.Error(t, err)
		assert.Nil(t, value)
	})

	t.Run("treats an encrypted cache read with the wrong key as a miss, not a crash", func(t *testing.T) {
		cache := newTestCache[string](t, time.Minute, "0123456789abcdef0123456789abcdef")
		require.NoError(t, cache.Set("secret", "token"))

		wrongKey := newCache[string]("bitbucket-cli-test")
		wrongKey.folder = cache.folder
		wrongKey.WithExpiration(time.Minute)
		wrongKey.WithEncryptionKey([]byte("fedcba9876543210fedcba9876543210"))

		value, err := wrongKey.Get("token")

		require.Error(t, err)
		assert.Nil(t, value)
	})
}

func TestNewCacheUsesUserCacheDir(t *testing.T) {
	t.Setenv("BITBUCKET_CLI_CACHE_DURATION", "")
	t.Setenv("BITBUCKET_CLI_CACHE_ENCRYPTIONKEY", "")
	userCacheDir, err := os.UserCacheDir()
	require.NoError(t, err)

	cache := NewCache[string]()

	assert.Equal(t, filepath.Join(userCacheDir, "bitbucket"), cache.folder)
	assert.Equal(t, 5*time.Minute, cache.expiration)
}
