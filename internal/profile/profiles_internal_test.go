package profile

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/avitsrimer/bitbucket-cli/internal/common"
)

// TestSaveProfilesConfigNeverPersistsVaultLoadedAccessToken is a regression test: a Profile whose
// AccessToken was populated at runtime by loadAccessToken's vault fallback (e.g. authorizing the
// workspace-completion request an EnumFlag issues against the current profile while running
// `profile update <name> --default-workspace ...`) must never have that value written to the
// config file in plain text -- that would defeat the vault silently, since accessToken's yaml tag
// is only "omitempty", not "-". saveProfilesConfig must blank it for the duration of the save and
// restore it afterward, so the rest of this process can still use it.
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
