package profile

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/avitsrimer/bitbucket-cli/internal/common"
	"github.com/spf13/cobra"
	"github.com/zalando/go-keyring"
)

// newIsolatedDeleteCmd builds a throwaway *cobra.Command mirroring deleteCmd's real flags,
// following newIsolatedCreateCmd/newIsolatedUpdateCmd's rationale.
func newIsolatedDeleteCmd() *cobra.Command {
	cmd := &cobra.Command{Use: "delete", Args: cobra.MinimumNArgs(1), RunE: deleteProcess, SilenceUsage: true, SilenceErrors: true}
	cmd.Flags().BoolVar(&deleteOptions.All, "all", false, "")
	cmd.Flags().BoolVar(&deleteOptions.StopOnError, "stop-on-error", false, "")
	cmd.Flags().BoolVar(&deleteOptions.WarnOnError, "warn-on-error", false, "")
	cmd.Flags().BoolVar(&deleteOptions.IgnoreErrors, "ignore-errors", false, "")
	cmd.Flags().String("profile", "", "")
	cmd.Flags().String("config", "", "")
	cmd.Flags().Bool("dry-run", false, "")
	return cmd
}

// TestDeleteProcessDoesNotPurgeVaultCredentialWhenSaveFails reproduces major finding #12:
// deleteProcess purged the vault credential of every profile being deleted *before* calling
// saveProfilesConfig, so a save failure left the profile itself still present in the config file
// (Profiles.Delete's in-memory removal was never persisted) but its credential already destroyed
// in the vault -- unrecoverable. The purge must only happen once the removal has actually been
// saved.
func TestDeleteProcessDoesNotPurgeVaultCredentialWhenSaveFails(t *testing.T) {
	keyring.MockInit()

	oldProfiles, oldCurrent := Profiles, Current
	oldConfig := common.CurrentConfig()
	oldDeleteOptions := deleteOptions
	t.Cleanup(func() {
		Profiles = oldProfiles
		Current = oldCurrent
		common.SetCurrentConfig(oldConfig)
		deleteOptions = oldDeleteOptions
	})
	Profiles = make(profiles, 0, 1)
	Current = nil
	deleteOptions = struct {
		All          bool
		StopOnError  bool
		WarnOnError  bool
		IgnoreErrors bool
	}{}

	// Force saveProfilesConfig to fail deterministically: config.Path's parent directory can
	// never be created because a plain file already occupies that path segment.
	blocker := filepath.Join(t.TempDir(), "not-a-directory")
	if err := os.WriteFile(blocker, []byte("x"), 0o600); err != nil {
		t.Fatalf("cannot create blocking file: %v", err)
	}
	badConfigPath := filepath.Join(blocker, "subdir", "config-cli.yml")
	common.SetCurrentConfig(&common.Config{Path: badConfigPath, Data: map[string]any{}})

	const (
		profileName = "delete-fail-test"
		vaultKey    = "bitbucket-cli-test-delete-fail"
		vaultToken  = "s3cr3t-must-survive-failed-save"
	)
	if err := (Profile{}).SetCredentialInVault(vaultKey, profileName, vaultToken); err != nil {
		t.Fatalf("cannot seed the fake vault: %v", err)
	}
	target := &Profile{Name: profileName, VaultKey: vaultKey, AccessToken: vaultToken}
	Profiles = append(Profiles, target)

	cmd := newIsolatedDeleteCmd()
	cmd.SetArgs([]string{profileName})
	cmd.SetContext(context.Background())

	if err := cmd.Execute(); err == nil {
		t.Fatal("expected an error from the forced config save failure, got nil")
	}

	// The finding's specific concern: a failed save must not have already destroyed the vault
	// credential, which -- unlike the in-memory Profiles slice Profiles.Delete mutated regardless
	// of the save outcome -- can never be reconstructed once purged.
	if _, err := (Profile{}).GetCredentialFromVault(vaultKey, profileName); err != nil {
		t.Errorf("vault credential was purged despite the config save failing: %v", err)
	}
}
