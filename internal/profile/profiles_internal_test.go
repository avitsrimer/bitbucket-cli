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

// TestSaveProfilesConfigNeverPersistsVaultLoadedSecrets is a regression test: a Profile whose
// AccessToken/ClientSecret/Password was populated at runtime by LoadSecrets'/loadAccessToken's
// vault fallback (e.g. authorizing the workspace-completion request an EnumFlag issues against the
// current profile while running `profile update <name> --default-workspace ...`) must never have
// that value written to the config file in plain text -- that would defeat the vault silently,
// since none of the three secrets' yaml tags are more than "omitempty". saveProfilesConfig
// converts every profile to its forSave view before handing it to Config.SetSection, which blanks
// out exactly the fields Profile.vault marks, so the in-memory value is never mutated (there is
// nothing to blank or restore) and the rest of this process can keep using it.
func TestSaveProfilesConfigNeverPersistsVaultLoadedSecrets(t *testing.T) {
	cases := []struct {
		name   string
		secret string
		build  func(secret string) *Profile
	}{
		{
			name:   "accessToken",
			secret: "s3cr3t-access-token-from-vault",
			build: func(secret string) *Profile {
				p := &Profile{Name: "runtime-vault-access-token-test"}
				p.AccessToken = secret
				p.vault.accessToken = true
				return p
			},
		},
		{
			name:   "clientSecret",
			secret: "s3cr3t-client-secret-from-vault",
			build: func(secret string) *Profile {
				p := &Profile{Name: "runtime-vault-client-secret-test", ClientID: "client-id"}
				p.ClientSecret = secret
				p.vault.clientSecret = true
				return p
			},
		},
		{
			name:   "password",
			secret: "s3cr3t-password-from-vault",
			build: func(secret string) *Profile {
				p := &Profile{Name: "runtime-vault-password-test", User: "alice"}
				p.Password = secret
				p.vault.password = true
				return p
			},
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			oldProfiles, oldCurrent := Profiles, Current
			oldConfig := common.CurrentConfig()
			t.Cleanup(func() {
				Profiles = oldProfiles
				Current = oldCurrent
				common.SetCurrentConfig(oldConfig)
			})

			configPath := filepath.Join(t.TempDir(), "config-cli.yml")
			common.SetCurrentConfig(&common.Config{Path: configPath, Data: map[string]any{}})

			target := tc.build(tc.secret)
			Profiles = profiles{target}

			if err := saveProfilesConfig(); err != nil {
				t.Fatalf("saveProfilesConfig() error = %v", err)
			}

			raw, err := os.ReadFile(configPath)
			if err != nil {
				t.Fatalf("cannot read saved config: %v", err)
			}
			if strings.Contains(string(raw), tc.secret) {
				t.Errorf("saved config = %q, must not contain the vault-loaded %s in plain text", raw, tc.name)
			}

			// The in-memory value must still be usable by the rest of this process after the save.
			switch tc.name {
			case "accessToken":
				if target.AccessToken != tc.secret {
					t.Errorf("AccessToken after save = %q, want the in-memory value restored", target.AccessToken)
				}
			case "clientSecret":
				if target.ClientSecret != tc.secret {
					t.Errorf("ClientSecret after save = %q, want the in-memory value restored", target.ClientSecret)
				}
			case "password":
				if target.Password != tc.secret {
					t.Errorf("Password after save = %q, want the in-memory value restored", target.Password)
				}
			}
		})
	}
}

// TestSaveProfilesConfigStillPersistsExplicitSecrets proves the fix does not overreach: a secret
// the user actually configured (not vault-loaded at runtime) must still be saved, for all three
// secret fields.
func TestSaveProfilesConfigStillPersistsExplicitSecrets(t *testing.T) {
	oldProfiles, oldCurrent := Profiles, Current
	oldConfig := common.CurrentConfig()
	t.Cleanup(func() {
		Profiles = oldProfiles
		Current = oldCurrent
		common.SetCurrentConfig(oldConfig)
	})

	configPath := filepath.Join(t.TempDir(), "config-cli.yml")
	common.SetCurrentConfig(&common.Config{Path: configPath, Data: map[string]any{}})

	target := &Profile{
		Name:         "explicit-secrets-test",
		AccessToken:  "user-provided-token",
		ClientID:     "client-id",
		ClientSecret: "user-provided-client-secret",
	}
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
	if !strings.Contains(string(raw), "user-provided-client-secret") {
		t.Errorf("saved config = %q, want the explicitly configured client secret persisted", raw)
	}
}

// TestConfigSaveNeverPersistsVaultLoadedSecretsWhenGivenForSaveProfiles proves the persistence
// invariant holds for any caller of Config.SetSection/Config.Save, not just saveProfilesConfig --
// provided the caller passes the persistence view (Profile.forSave) rather than a bare Profile.
// This is the documented contract going forward: unlike a bare Profile (which always marshals as
// a full, vault-secrets-included display view, see
// TestProfileMarshalYAMLIncludesVaultLoadedAccessTokenForDisplay below), profileForSave carries no
// vault secret at all once built, by construction, regardless of which generic method serializes
// it.
func TestConfigSaveNeverPersistsVaultLoadedSecretsWhenGivenForSaveProfiles(t *testing.T) {
	configPath := filepath.Join(t.TempDir(), "config-cli.yml")
	config := &common.Config{Path: configPath, Data: map[string]any{}}

	target := &Profile{Name: "generic-save-vault-token-test", ClientID: "client-id"}
	target.AccessToken = "s3cr3t-from-vault-generic"
	target.vault.accessToken = true

	if err := config.SetSection("profiles", []profileForSave{target.forSave()}); err != nil {
		t.Fatalf("Config.SetSection() error = %v", err)
	}

	raw, err := os.ReadFile(configPath)
	if err != nil {
		t.Fatalf("cannot read saved config: %v", err)
	}
	if strings.Contains(string(raw), "s3cr3t-from-vault-generic") {
		t.Errorf("saved config = %q, must not contain the vault-loaded access token in plain text", raw)
	}
	// forSave copies the profile; the original must be untouched.
	if target.AccessToken != "s3cr3t-from-vault-generic" {
		t.Errorf("AccessToken after forSave()/save = %q, want the original untouched", target.AccessToken)
	}
}

// TestProfileMarshalYAMLIncludesVaultLoadedAccessTokenForDisplay is a regression test for review-
// iter4 finding 1: `profile get`/`profile list -o yaml` call LoadSecrets specifically so a vault-
// loaded secret can be shown to the user, and reach this exact code path (yaml.Marshal on a live
// *Profile, via printYAML) to do it. MarshalYAML must not drop AccessToken just because it came
// from the vault -- that omission belongs solely to the persistence view (profileForSave), not to
// every yaml encoding of a Profile.
func TestProfileMarshalYAMLIncludesVaultLoadedAccessTokenForDisplay(t *testing.T) {
	target := &Profile{Name: "marshal-display-vault-test", AccessToken: "s3cr3t-from-vault"}
	target.vault.accessToken = true

	data, err := yaml.Marshal(target)
	if err != nil {
		t.Fatalf("yaml.Marshal() error = %v", err)
	}
	if !strings.Contains(string(data), "s3cr3t-from-vault") {
		t.Errorf("marshaled yaml = %q, want the vault-loaded access token shown for display", data)
	}
}

// TestProfileForSaveDoesNotMutateOriginal is a focused unit test on Profile.forSave: it must
// return a blanked copy without touching the receiver, for every one of the three secrets.
func TestProfileForSaveDoesNotMutateOriginal(t *testing.T) {
	target := &Profile{Name: "for-save-no-mutate-test", ClientID: "client-id", User: "alice"}
	target.AccessToken = "access-token-secret"
	target.ClientSecret = "client-secret-secret"
	target.Password = "password-secret"
	target.vault.accessToken = true
	target.vault.clientSecret = true
	target.vault.password = true

	saved := target.forSave()

	if saved.AccessToken != "" || saved.ClientSecret != "" || saved.Password != "" {
		t.Errorf("forSave() result = %+v, want all three vault-loaded secrets blanked", saved.Profile)
	}
	if target.AccessToken != "access-token-secret" || target.ClientSecret != "client-secret-secret" || target.Password != "password-secret" {
		t.Errorf("original profile after forSave() = %+v, want it untouched", *target)
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

// TestProfileUnmarshalYAMLDecodingSameNodeTwiceIsIdempotent is a regression test for review-iter4
// finding 5: UnmarshalYAML used to have extractAPIRootNode null the "apiroot" value node in place
// on the node it was handed, so a second Decode of that same *yaml.Node (e.g. a config layer that
// retained a parsed document tree, or any other code decoding the same node twice) silently lost
// apiRoot the second time. Decoding from the same node twice must yield identical, non-nil results
// both times.
func TestProfileUnmarshalYAMLDecodingSameNodeTwiceIsIdempotent(t *testing.T) {
	const document = "name: p\napiroot: https://alice:s3cr3t@api.bitbucket.org\n"

	var root yaml.Node
	if err := yaml.Unmarshal([]byte(document), &root); err != nil {
		t.Fatalf("cannot parse fixture document: %v", err)
	}
	if root.Kind != yaml.DocumentNode || len(root.Content) != 1 {
		t.Fatalf("root = %+v, want a document node wrapping a single mapping node", root)
	}
	mapping := root.Content[0]

	var first Profile
	if err := mapping.Decode(&first); err != nil {
		t.Fatalf("first Decode() error = %v", err)
	}
	if first.APIRoot == nil {
		t.Fatal("first.APIRoot = nil, want the parsed apiRoot")
	}

	var second Profile
	if err := mapping.Decode(&second); err != nil {
		t.Fatalf("second Decode() error = %v", err)
	}
	if second.APIRoot == nil {
		t.Fatal("second.APIRoot = nil, want decoding the same node twice to be idempotent")
	}
	if first.APIRoot.String() != second.APIRoot.String() {
		t.Errorf("first.APIRoot = %q, second.APIRoot = %q, want identical results from decoding the same node twice", first.APIRoot, second.APIRoot)
	}
}

// TestResolveProfileCredentialsPlainTextDetectionExcludesVaultLoadedSecrets is a regression test
// for review-iter4 finding 2: resolveProfileCredentials used to treat any non-empty AccessToken/
// ClientSecret/Password on the profile being updated as proof it stores credentials in plain text,
// forcing NoVault = true -- even when that value had just been populated at runtime by
// LoadSecrets'/loadAccessToken's vault fallback (e.g. authorizing the workspace/project lookup
// --default-workspace/--default-project triggers during flag parsing, before this function ever
// runs). That silently defeated a profile's own choice to keep its credentials in the vault:
// `profile update <name> --default-workspace ws --access-token NEWTOKEN` would skip
// SetCredentialInVault and end up writing the new token to config-cli.yml in plain text. On
// unfixed code this test fails because NoVault is forced true purely from the vault-loaded value's
// presence.
func TestResolveProfileCredentialsPlainTextDetectionExcludesVaultLoadedSecrets(t *testing.T) {
	cases := []struct {
		name  string
		build func(p *Profile)
	}{
		{"accessToken", func(p *Profile) {
			p.AccessToken = "s3cr3t-from-vault"
			p.vault.accessToken = true
		}},
		{"clientSecret", func(p *Profile) {
			p.ClientID = "client-id"
			p.ClientSecret = "s3cr3t-from-vault"
			p.vault.clientSecret = true
		}},
		{"password", func(p *Profile) {
			p.User = "alice"
			p.Password = "s3cr3t-from-vault"
			p.vault.password = true
		}},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			oldOptions := updateOptions
			t.Cleanup(func() { updateOptions = oldOptions })
			updateOptions.NoVault = false
			updateOptions.ToVault = false
			updateOptions.AccessToken = ""
			updateOptions.ClientSecret = ""
			updateOptions.Password = ""
			updateOptions.ClientID = ""

			target := &Profile{Name: "plaintext-detection-test-" + tc.name}
			tc.build(target)

			if err := resolveProfileCredentials(updateCmd, target); err != nil {
				t.Fatalf("resolveProfileCredentials() error = %v", err)
			}
			if updateOptions.NoVault {
				t.Errorf("updateOptions.NoVault = true, want false: a vault-loaded %s must not force NoVault", tc.name)
			}
		})
	}
}

// TestResolveProfileCredentialsPlainTextDetectionStillCatchesExplicitSecrets proves the fix does
// not overreach: a secret genuinely stored in plain text on the profile (not vault-loaded) must
// still force NoVault, so an existing plain-text profile is never silently pushed toward the
// vault.
func TestResolveProfileCredentialsPlainTextDetectionStillCatchesExplicitSecrets(t *testing.T) {
	oldOptions := updateOptions
	t.Cleanup(func() { updateOptions = oldOptions })
	updateOptions.NoVault = false
	updateOptions.ToVault = false
	updateOptions.AccessToken = ""

	target := &Profile{Name: "plaintext-detection-explicit-test", AccessToken: "plain-text-token"}

	if err := resolveProfileCredentials(updateCmd, target); err != nil {
		t.Fatalf("resolveProfileCredentials() error = %v", err)
	}
	if !updateOptions.NoVault {
		t.Error("updateOptions.NoVault = false, want true: a plain-text AccessToken must still force NoVault")
	}
}
