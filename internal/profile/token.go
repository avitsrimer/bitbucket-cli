package profile

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"time"

	"github.com/gildas/go-core"
	"github.com/go-pkgz/lgr"
)

// ErrUnmarshalJSON is returned when the access-token JSON payload from BitBucket, or the local
// access-token cache file, fails to unmarshal.
var ErrUnmarshalJSON = errors.New("cannot unmarshal json")

// ErrNoAccessToken is returned by loadAccessToken when no access token could be found (not set on
// the profile, no cache file, no vault entry). It is not itself a failure: it tells authorize
// there is nothing usable yet, so it can fall through to the OAuth2 flow, without authorize having
// to infer that from a nil profile.token.
var ErrNoAccessToken = errors.New("no access token available")

// nonExpiringTokenLifetime is the lifetime assigned to a loaded access token (a repository/
// project/workspace access token set directly on the profile or found in the vault): such tokens
// never expire, so this is set far enough in the future that isTokenExpired never reports one as
// expired.
const nonExpiringTokenLifetime = 100 * 365 * 24 * time.Hour

type Token struct {
	TokenType    string         `json:"token_type"`
	AccessToken  string         `json:"access_token"`
	RefreshToken string         `json:"refresh_token"`
	ExpiresOn    core.Timestamp `json:"expires_on"`
	Scope        string         `json:"scope"`
}

// loadAccessToken loads the access token from the cache.
//
// Its contract is explicit: a nil error return means profile.token is now non-nil (a usable token
// was found in memory, on the profile itself, in the local file cache, or in the vault). Any other
// outcome -- including simply not finding a cached token anywhere -- returns a non-nil error
// (ErrNoAccessToken for the "not found" case), so callers like authorize never have to guess
// whether profile.token is safe to dereference from a nil error alone.
func (profile *Profile) loadAccessToken(_ context.Context) (err error) {
	if profile.token != nil {
		lgr.Printf("[DEBUG] access token already loaded in memory for profile %s", profile.Name)
		return nil
	}

	if profile.AccessToken != "" {
		lgr.Printf("[DEBUG] repository/project/workspace access token for profile %s", profile.Name)
		profile.token = &Token{
			AccessToken: profile.AccessToken,
			ExpiresOn:   core.Timestamp(time.Now().Add(nonExpiringTokenLifetime)),
		}
		return nil
	}

	// then load the access token from the file cache
	cacheDir, err := os.UserCacheDir()
	if err != nil {
		return fmt.Errorf("cannot determine cache directory: %w", err)
	}

	accessTokenFile := filepath.Join(cacheDir, "bitbucket", "access-token-"+profile.Name)
	data, readErr := os.ReadFile(accessTokenFile) //nolint:gosec // accessTokenFile is built from the OS-provided cache dir and the profile's own name, not external input
	if readErr == nil {
		var token Token
		if readErr = json.Unmarshal(data, &token); readErr == nil {
			lgr.Printf("[DEBUG] loaded access token from cache for profile %s", profile.Name)
			// token.Redact() masks secrets before they hit the debug log
			lgr.Printf("[DEBUG] access token details for profile %s: %+v", profile.Name, token.Redact())
			profile.token = &token
			return nil
		}
	}
	// Load the access token from the vault in case this is an API Token
	lgr.Printf("[DEBUG] looking for access token in the vault for profile %s", profile.Name)
	credential, vaultErr := profile.GetCredentialFromVault(profile.VaultKey, profile.Name)
	if vaultErr != nil {
		lgr.Printf("[DEBUG] no cached access token found for profile %s: %v", profile.Name, vaultErr)
		return ErrNoAccessToken // Not found is not a fatal error: the caller can fall through to the OAuth2 flow
	}
	profile.AccessToken = credential.Password
	profile.vault.accessToken = true // must never be written back to the config file in plain text
	lgr.Printf("[DEBUG] loaded repository/project/workspace access token for profile %s from the vault", profile.Name)
	profile.token = &Token{
		AccessToken: profile.AccessToken,
		ExpiresOn:   core.Timestamp(time.Now().Add(nonExpiringTokenLifetime)),
	}
	return nil
}

// isTokenExpired tells if the token is expired
func (profile *Profile) isTokenExpired() bool {
	return profile.token != nil && profile.token.IsExpired()
}

// saveAccessToken saves the access token to the cache
func (profile *Profile) saveAccessToken(_ context.Context, data []byte) (accessToken string, err error) {
	profile.token, err = UnmarshalTokenFromBitbucketData(data)
	if err != nil {
		lgr.Printf("[ERROR] failed to unmarshal access token data for profile %s: %v", profile.Name, err)
		return "", err
	}

	if cacheDir, err := os.UserCacheDir(); err == nil {
		cachePath := filepath.Join(cacheDir, "bitbucket")
		if err = os.MkdirAll(cachePath, 0o700); err == nil {
			cacheFile := filepath.Join(cachePath, "access-token-"+profile.Name)
			payload, _ := json.Marshal(profile.token) //nolint:gosec // G117: caching the access token locally (0600) is the intended behavior here, not a leak
			if err = os.WriteFile(cacheFile, payload, 0o600); err != nil {
				lgr.Printf("[ERROR] failed to save access token to cache for profile %s: %v", profile.Name, err)
			}
		}
	}
	return profile.token.AccessToken, nil
}

// Redact redacts sensitive information from the token, for logging purposes
func (token Token) Redact() any {
	redacted := token
	if redacted.AccessToken != "" {
		redacted.AccessToken = redactWithHash(redacted.AccessToken)
	}
	if redacted.RefreshToken != "" {
		redacted.RefreshToken = redactWithHash(redacted.RefreshToken)
	}
	return redacted
}

// IsExpired tells if the token is expired
func (token *Token) IsExpired() bool {
	return time.Time(token.ExpiresOn).Before(time.Now())
}

// GetExpiresOn returns the expiration time of the token
func (token *Token) GetExpiresOn() time.Time {
	return time.Time(token.ExpiresOn)
}

// GetExpiresIn returns the duration until the token expires
func (token *Token) GetExpiresIn() time.Duration {
	return time.Until(time.Time(token.ExpiresOn))
}

// GetExpiredSince returns the duration since the token expired
func (token *Token) GetExpiredSince() time.Duration {
	if !token.IsExpired() {
		return 0
	}
	return time.Since(time.Time(token.ExpiresOn))
}

// UnmarshalTokenFromBitbucketData unmarshals the token data from the BitBucket response
func UnmarshalTokenFromBitbucketData(data []byte) (token *Token, err error) {
	var result struct {
		TokenType    string `json:"token_type"`
		State        string `json:"state"`
		AccessToken  string `json:"access_token"`
		RefreshToken string `json:"refresh_token"`
		ExpiresIn    int64  `json:"expires_in"`
		Scopes       string `json:"scopes"`
	}
	if err = json.Unmarshal(data, &result); err != nil {
		return nil, fmt.Errorf("%w: %w", ErrUnmarshalJSON, err)
	}
	token = &Token{
		TokenType:    result.TokenType,
		AccessToken:  result.AccessToken,
		RefreshToken: result.RefreshToken,
		ExpiresOn:    core.Timestamp(time.Now().Add(time.Duration(result.ExpiresIn) * time.Second)),
		Scope:        result.Scopes,
	}
	return token, nil
}
