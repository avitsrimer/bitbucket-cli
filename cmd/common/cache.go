package common

import (
	"crypto/aes"
	"crypto/cipher"
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sync"
	"time"

	"github.com/gildas/go-core"
)

// Cache is a persistent, TTL-based, in-memory-and-on-disk cache.
//
// Entries are kept in memory for the lifetime of the process and mirrored to
// os.UserCacheDir()/<name>/<sha256(key)> as JSON, optionally AES-GCM encrypted when an
// encryption key is set. A corrupt or unreadable cache file is treated as a cache miss,
// never as an error.
type Cache[T any] struct {
	expiration    time.Duration
	encryptionKey []byte
	folder        string
	items         sync.Map
}

type cacheEntry[T any] struct {
	Item      T
	ExpiresAt int64 // UnixNano; zero means "never expires"
}

// newCache creates a new persistent Cache rooted at os.UserCacheDir()/name.
func newCache[T any](name string) *Cache[T] {
	folder, _ := os.UserCacheDir()
	return &Cache[T]{folder: filepath.Join(folder, name)}
}

// WithExpiration sets the default TTL applied to entries stored with Set.
func (cache *Cache[T]) WithExpiration(expiration time.Duration) *Cache[T] {
	cache.expiration = expiration
	return cache
}

// WithEncryptionKey sets the AES-GCM key used to encrypt entries on disk.
//
// An empty key disables encryption; entries are then stored as plain JSON.
func (cache *Cache[T]) WithEncryptionKey(key []byte) *Cache[T] {
	cache.encryptionKey = key
	return cache
}

// Set stores item in the cache under key, honoring the cache's default expiration.
func (cache *Cache[T]) Set(item T, key ...string) error {
	var expiresAt int64
	if cache.expiration > 0 {
		expiresAt = time.Now().Add(cache.expiration).UnixNano()
	}
	entry := cacheEntry[T]{Item: item, ExpiresAt: expiresAt}

	var errs []error
	for _, k := range key {
		cache.items.Store(k, entry)
		if err := cache.writeToDisk(k, entry); err != nil {
			errs = append(errs, err)
		}
	}
	return errors.Join(errs...)
}

// Get returns the item stored under key, or an error if it is missing, expired, or the
// cached value cannot be decoded.
func (cache *Cache[T]) Get(key string) (*T, error) {
	if value, found := cache.items.Load(key); found {
		entry, _ := value.(cacheEntry[T])
		if entry.expired() {
			cache.items.Delete(key)
			_ = os.Remove(cache.filename(key))
			return nil, fmt.Errorf("cache entry %q not found", key)
		}
		return &entry.Item, nil
	}

	entry, err := cache.readFromDisk(key)
	if err != nil {
		return nil, fmt.Errorf("cache entry %q not found", key)
	}
	if entry.expired() {
		_ = os.Remove(cache.filename(key))
		return nil, fmt.Errorf("cache entry %q not found", key)
	}
	cache.items.Store(key, entry)
	return &entry.Item, nil
}

func (entry cacheEntry[T]) expired() bool {
	return entry.ExpiresAt > 0 && time.Now().UnixNano() > entry.ExpiresAt
}

func (cache *Cache[T]) filename(key string) string {
	sum := sha256.Sum256([]byte(key))
	return filepath.Join(cache.folder, hex.EncodeToString(sum[:]))
}

func (cache *Cache[T]) writeToDisk(key string, entry cacheEntry[T]) error {
	data, err := json.Marshal(entry)
	if err != nil {
		return fmt.Errorf("cannot marshal cache entry: %w", err)
	}
	if len(cache.encryptionKey) > 0 {
		if data, err = cache.encrypt(data); err != nil {
			return fmt.Errorf("cannot encrypt cache entry: %w", err)
		}
	}
	if err = os.MkdirAll(cache.folder, 0o700); err != nil {
		return fmt.Errorf("cannot create cache folder: %w", err)
	}
	if err = os.WriteFile(cache.filename(key), data, 0o600); err != nil {
		return fmt.Errorf("cannot write cache entry: %w", err)
	}
	return nil
}

// readFromDisk reads and decodes a cache entry. Any failure (missing file, corrupt JSON,
// bad ciphertext) is reported as a plain error so the caller treats it as a cache miss.
func (cache *Cache[T]) readFromDisk(key string) (cacheEntry[T], error) {
	var entry cacheEntry[T]

	data, err := os.ReadFile(cache.filename(key))
	if err != nil {
		return entry, fmt.Errorf("cannot read cache entry: %w", err)
	}
	if len(cache.encryptionKey) > 0 {
		if data, err = cache.decrypt(data); err != nil {
			return entry, fmt.Errorf("cannot decrypt cache entry: %w", err)
		}
	}
	if err = json.Unmarshal(data, &entry); err != nil {
		return entry, fmt.Errorf("cannot unmarshal cache entry: %w", err)
	}
	return entry, nil
}

func (cache *Cache[T]) encrypt(data []byte) ([]byte, error) {
	block, err := aes.NewCipher(cache.encryptionKey)
	if err != nil {
		return nil, fmt.Errorf("cannot create cipher: %w", err)
	}
	gcm, err := cipher.NewGCM(block)
	if err != nil {
		return nil, fmt.Errorf("cannot create GCM: %w", err)
	}
	nonce := make([]byte, gcm.NonceSize())
	if _, err = io.ReadFull(rand.Reader, nonce); err != nil {
		return nil, fmt.Errorf("cannot generate nonce: %w", err)
	}
	return gcm.Seal(nonce, nonce, data, nil), nil
}

func (cache *Cache[T]) decrypt(data []byte) ([]byte, error) {
	block, err := aes.NewCipher(cache.encryptionKey)
	if err != nil {
		return nil, fmt.Errorf("cannot create cipher: %w", err)
	}
	gcm, err := cipher.NewGCM(block)
	if err != nil {
		return nil, fmt.Errorf("cannot create GCM: %w", err)
	}
	nonceSize := gcm.NonceSize()
	if len(data) < nonceSize {
		return nil, errors.New("ciphertext too short")
	}
	nonce, ciphertext := data[:nonceSize], data[nonceSize:]
	decrypted, err := gcm.Open(nil, nonce, ciphertext, nil)
	if err != nil {
		return nil, fmt.Errorf("cannot decrypt: %w", err)
	}
	return decrypted, nil
}

// NewCache creates the process-wide persistent cache used for repository/user/workspace
// lookups, honoring BITBUCKET_CLI_CACHE_DURATION (default 5m) and
// BITBUCKET_CLI_CACHE_ENCRYPTIONKEY (AES-GCM encryption when set).
func NewCache[T any]() *Cache[T] {
	return newCache[T]("bitbucket").
		WithExpiration(core.GetEnvAsDuration("BITBUCKET_CLI_CACHE_DURATION", 5*time.Minute)).
		WithEncryptionKey([]byte(core.GetEnvAsString("BITBUCKET_CLI_CACHE_ENCRYPTIONKEY", "")))
}
