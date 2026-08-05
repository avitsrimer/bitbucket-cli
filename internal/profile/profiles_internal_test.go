package profile

import (
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/avitsrimer/bitbucket-cli/internal/common"
	"gopkg.in/yaml.v3"
)

// TestSaveProfilesConfigNeverPersistsVaultLoadedAccessToken is a regression test: a Profile whose
// AccessToken was populated at runtime by loadAccessToken's vault fallback (e.g. authorizing the
// workspace-completion request an EnumFlag issues against the current profile while running
// `profile update <name> --default-workspace ...`) must never have that value written to the
// config file in plain text -- that would defeat the vault silently, since accessToken's yaml tag
// is only "omitempty", not "-". Profile.MarshalYAML omits it whenever accessTokenFromVault is set,
// so the in-memory value is never mutated (there is nothing to blank or restore) and the rest of
// this process can keep using it.
func TestSaveProfilesConfigNeverPersistsVaultLoadedAccessToken(t *testing.T) {
	oldProfiles, oldCurrent := Profiles, Current
	oldConfig := common.CurrentConfig()
	t.Cleanup(func() {
		Profiles = oldProfiles
		Current = oldCurrent
		common.SetCurrentConfig(oldConfig)
	})

	configPath := filepath.Join(t.TempDir(), "config-cli.yml")
	common.SetCurrentConfig(&common.Config{Path: configPath, Data: map[string]any{}})

	target := &Profile{Name: "runtime-vault-token-test", ClientID: "client-id"}
	target.AccessToken = "s3cr3t-from-vault"
	target.accessTokenFromVault = true
	Profiles = profiles{target}

	if err := saveProfilesConfig(); err != nil {
		t.Fatalf("saveProfilesConfig() error = %v", err)
	}

	raw, err := os.ReadFile(configPath)
	if err != nil {
		t.Fatalf("cannot read saved config: %v", err)
	}
	if strings.Contains(string(raw), "s3cr3t-from-vault") {
		t.Errorf("saved config = %q, must not contain the vault-loaded access token in plain text", raw)
	}

	// The in-memory value must still be usable by the rest of this process after the save.
	if target.AccessToken != "s3cr3t-from-vault" {
		t.Errorf("AccessToken after save = %q, want the in-memory value restored", target.AccessToken)
	}
}

// TestSaveProfilesConfigStillPersistsExplicitAccessToken proves the fix does not overreach: an
// AccessToken the user actually configured (not vault-loaded at runtime) must still be saved.
func TestSaveProfilesConfigStillPersistsExplicitAccessToken(t *testing.T) {
	oldProfiles, oldCurrent := Profiles, Current
	oldConfig := common.CurrentConfig()
	t.Cleanup(func() {
		Profiles = oldProfiles
		Current = oldCurrent
		common.SetCurrentConfig(oldConfig)
	})

	configPath := filepath.Join(t.TempDir(), "config-cli.yml")
	common.SetCurrentConfig(&common.Config{Path: configPath, Data: map[string]any{}})

	target := &Profile{Name: "explicit-access-token-test", AccessToken: "user-provided-token"}
	Profiles = profiles{target}

	if err := saveProfilesConfig(); err != nil {
		t.Fatalf("saveProfilesConfig() error = %v", err)
	}

	raw, err := os.ReadFile(configPath)
	if err != nil {
		t.Fatalf("cannot read saved config: %v", err)
	}
	if !strings.Contains(string(raw), "user-provided-token") {
		t.Errorf("saved config = %q, want the explicitly configured access token persisted", raw)
	}
}

// TestConfigSaveNeverPersistsVaultLoadedAccessTokenViaGenericSave is a regression test proving the
// vault-token invariant holds "by construction" for every path that serializes Profiles, not just
// saveProfilesConfig. It drives a bare Config.SetSection/Config.Save call directly -- the shape
// any future caller (a new command, a different save helper) would use -- entirely bypassing
// saveProfilesConfig and the package-level Profiles/Current globals, so it exercises
// Profile.MarshalYAML itself rather than a caller-side blank-and-restore convention that could be
// forgotten (as the removed clearVaultLoadedAccessTokens dance was).
func TestConfigSaveNeverPersistsVaultLoadedAccessTokenViaGenericSave(t *testing.T) {
	configPath := filepath.Join(t.TempDir(), "config-cli.yml")
	config := &common.Config{Path: configPath, Data: map[string]any{}}

	target := &Profile{Name: "generic-save-vault-token-test", ClientID: "client-id"}
	target.AccessToken = "s3cr3t-from-vault-generic"
	target.accessTokenFromVault = true

	if err := config.SetSection("profiles", profiles{target}); err != nil {
		t.Fatalf("Config.SetSection() error = %v", err)
	}

	raw, err := os.ReadFile(configPath)
	if err != nil {
		t.Fatalf("cannot read saved config: %v", err)
	}
	if strings.Contains(string(raw), "s3cr3t-from-vault-generic") {
		t.Errorf("saved config = %q, must not contain the vault-loaded access token in plain text", raw)
	}
	if target.AccessToken != "s3cr3t-from-vault-generic" {
		t.Errorf("AccessToken after save = %q, want the in-memory value untouched", target.AccessToken)
	}
}

// TestProfileMarshalYAMLRoundTripsAPIRootUserinfo is a regression test for the write side of the
// same bug UnmarshalYAML fixes on read: Profile's default yaml struct encoding marshals APIRoot
// through url.URL's field-by-field mapping form, which cannot represent url.URL.User (a
// *url.Userinfo whose fields are all unexported) and so silently drops credentials on save.
// MarshalYAML must rewrite apiRoot to its plain string form instead, so a profile saved with a
// userinfo-bearing apiRoot reads back identically.
func TestProfileMarshalYAMLRoundTripsAPIRootUserinfo(t *testing.T) {
	apiRoot, err := url.Parse("https://alice:s3cr3t@api.bitbucket.org/userinfo")
	if err != nil {
		t.Fatalf("cannot parse fixture apiRoot: %v", err)
	}
	original := &Profile{Name: "marshal-userinfo-test", APIRoot: apiRoot}

	data, err := yaml.Marshal(original)
	if err != nil {
		t.Fatalf("yaml.Marshal() error = %v", err)
	}
	if !strings.Contains(string(data), "s3cr3t") {
		t.Fatalf("marshaled yaml = %q, want it to contain the apiRoot password in string form", data)
	}

	var decoded Profile
	if err := yaml.Unmarshal(data, &decoded); err != nil {
		t.Fatalf("yaml.Unmarshal() error = %v", err)
	}
	if decoded.APIRoot == nil {
		t.Fatal("decoded.APIRoot = nil, want the round-tripped URL")
	}
	if got := decoded.APIRoot.String(); got != apiRoot.String() {
		t.Errorf("decoded.APIRoot = %q, want %q", got, apiRoot.String())
	}
}

// TestProfileMarshalYAMLOmitsVaultLoadedAccessToken is a focused unit test on Profile.MarshalYAML
// itself (as opposed to the Config.Save-level tests above): a Profile with accessTokenFromVault
// set must never emit an accesstoken key at all.
func TestProfileMarshalYAMLOmitsVaultLoadedAccessToken(t *testing.T) {
	target := &Profile{Name: "marshal-vault-test", AccessToken: "s3cr3t-from-vault"}
	target.accessTokenFromVault = true

	data, err := yaml.Marshal(target)
	if err != nil {
		t.Fatalf("yaml.Marshal() error = %v", err)
	}
	if strings.Contains(string(data), "s3cr3t-from-vault") {
		t.Errorf("marshaled yaml = %q, must not contain the vault-loaded access token", data)
	}
	if strings.Contains(string(data), "accesstoken") {
		t.Errorf("marshaled yaml = %q, must not contain an accesstoken key at all", data)
	}
}
