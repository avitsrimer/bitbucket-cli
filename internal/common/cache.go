package common

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"time"

	"github.com/gildas/go-core"
)

// Cache is a persistent, TTL-based, on-disk cache.
//
// Entries are mirrored to folder/sha256(key) as JSON. A corrupt or unreadable cache file is
// treated as a cache miss, never as an error.
type Cache[T any] struct {
	expiration time.Duration
	folder     string
}

type cacheEntry[T any] struct {
	Item      T
	ExpiresAt int64 // UnixNano; zero means "never expires"
}

// NewCache creates the process-wide persistent cache used for repository/user/workspace
// lookups, rooted at os.UserCacheDir()/bitbucket and honoring BITBUCKET_CLI_CACHE_DURATION
// (default 5m).
func NewCache[T any]() *Cache[T] {
	folder, _ := os.UserCacheDir()
	expiration := core.GetEnvAsDuration("BITBUCKET_CLI_CACHE_DURATION", 5*time.Minute)
	return NewCacheAt[T](filepath.Join(folder, "bitbucket"), expiration)
}

// NewCacheAt creates a persistent Cache rooted at the given folder with the given default TTL.
//
// Tests use this to point a cache at a temporary directory instead of the real
// os.UserCacheDir().
func NewCacheAt[T any](folder string, expiration time.Duration) *Cache[T] {
	return &Cache[T]{folder: folder, expiration: expiration}
}

// Set stores item in the cache under key, honoring the cache's default expiration.
func (cache *Cache[T]) Set(key string, item T) error {
	var expiresAt int64
	if cache.expiration > 0 {
		expiresAt = time.Now().Add(cache.expiration).UnixNano()
	}
	entry := cacheEntry[T]{Item: item, ExpiresAt: expiresAt}

	data, err := json.Marshal(entry)
	if err != nil {
		return fmt.Errorf("cannot marshal cache entry: %w", err)
	}
	if err := os.MkdirAll(cache.folder, 0o700); err != nil {
		return fmt.Errorf("cannot create cache folder: %w", err)
	}
	if err := os.WriteFile(cache.filename(key), data, 0o600); err != nil {
		return fmt.Errorf("cannot write cache entry: %w", err)
	}
	return nil
}

// Get returns the item stored under key, or an error if it is missing, expired, or the
// cached value cannot be decoded.
func (cache *Cache[T]) Get(key string) (*T, error) {
	entry, err := cache.readFromDisk(key)
	if err != nil {
		return nil, fmt.Errorf("cache entry %q not found", key)
	}
	if entry.expired() {
		_ = os.Remove(cache.filename(key))
		return nil, fmt.Errorf("cache entry %q not found", key)
	}
	return &entry.Item, nil
}

func (entry cacheEntry[T]) expired() bool {
	return entry.ExpiresAt > 0 && time.Now().UnixNano() > entry.ExpiresAt
}

func (cache *Cache[T]) filename(key string) string {
	sum := sha256.Sum256([]byte(key))
	return filepath.Join(cache.folder, hex.EncodeToString(sum[:]))
}

// readFromDisk reads and decodes a cache entry. Any failure (missing file, corrupt JSON) is
// reported as a plain error so the caller treats it as a cache miss.
func (cache *Cache[T]) readFromDisk(key string) (cacheEntry[T], error) {
	var entry cacheEntry[T]

	data, err := os.ReadFile(cache.filename(key))
	if err != nil {
		return entry, fmt.Errorf("cannot read cache entry: %w", err)
	}
	if err = json.Unmarshal(data, &entry); err != nil {
		return entry, fmt.Errorf("cannot unmarshal cache entry: %w", err)
	}
	return entry, nil
}
