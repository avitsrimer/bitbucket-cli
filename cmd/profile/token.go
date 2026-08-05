package profile

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/gildas/go-core"
	"github.com/go-pkgz/lgr"
)

// ErrUnmarshalJSON is returned when the access-token JSON payload from BitBucket, or the local
// access-token cache file, fails to unmarshal.
var ErrUnmarshalJSON = errors.New("cannot unmarshal json")

type Token struct {
	TokenType    string         `json:"token_type"`
	AccessToken  string         `json:"access_token"`
	RefreshToken string         `json:"refresh_token"`
	ExpiresOn    core.Timestamp `json:"expires_on"`
	Scope        string         `json:"scope"`
}

// loadAccessToken loads the access token from the cache
func (profile *Profile) loadAccessToken(_ context.Context) (err error) {
	if profile.token != nil {
		lgr.Printf("[DEBUG] access token already loaded in memory for profile %s", profile.Name)
		return nil
	}

	if profile.AccessToken != "" {
		lgr.Printf("[DEBUG] repository/project/workspace access token for profile %s", profile.Name)
		profile.token = &Token{
			AccessToken: profile.AccessToken,
			ExpiresOn:   core.Timestamp(time.Now().Add(100 * 365 * 24 * time.Hour)), // Loaded Access Tokens never expire
		}
		return nil
	}

	// then load the access token from the file cache
	cacheDir, err := os.UserCacheDir()
	if err == nil {
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
			lgr.Printf("[ERROR] failed to get access token for profile %s: %v", profile.Name, vaultErr)
			return nil // We don't return an error if the token is not found, so the authorization process can continue
		}
		profile.AccessToken = credential.Password
		lgr.Printf("[DEBUG] loaded repository/project/workspace access token for profile %s from the vault", profile.Name)
		profile.token = &Token{
			AccessToken: profile.AccessToken,
			ExpiresOn:   core.Timestamp(time.Now().Add(100 * 365 * 24 * time.Hour)), // Loaded Access Tokens never expire
		}
		return nil
	}
	return fmt.Errorf("cannot determine cache directory: %w", err)
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

// GetScopes returns the scopes of the token
func (token *Token) GetScopes() []string {
	return strings.Split(token.Scope, " ")
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
