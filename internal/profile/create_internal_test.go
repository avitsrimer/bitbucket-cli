package profile

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/avitsrimer/bitbucket-cli/internal/common"
	"github.com/spf13/cobra"
	"github.com/zalando/go-keyring"
)

// newIsolatedCreateCmd builds a throwaway *cobra.Command carrying its own FlagSet bound to the
// same package-level createOptions fields createCmd's real flags are bound to (so createProcess
// reads exactly what the test sets), and the same MarkFlagsRequiredTogether/
// MarkFlagsMutuallyExclusive validation createCmd's init() registers -- reproducing the exact
// cobra validation this test needs without touching the actual singleton createCmd. Sharing
// createCmd (or a command tree rooted at it) between tests would leak each Set() call's Changed
// bit on the *pflag.Flag itself across every other test that ever runs `profile create` in the
// same test binary; a fresh FlagSet has no such memory, so the goroutine-independent Go values
// (createOptions) are the only cross-test state left to save and restore, and this test's caller
// already does exactly that.
func newIsolatedCreateCmd() *cobra.Command {
	cmd := &cobra.Command{Use: "create", Args: cobra.NoArgs, RunE: createProcess}
	cmd.Flags().StringVarP(&createOptions.Name, "name", "n", "", "")
	cmd.Flags().StringVar(&createOptions.ClientID, "client-id", "", "")
	cmd.Flags().StringVar(&createOptions.ClientSecret, "client-secret", "", "")
	cmd.Flags().StringVar(&createOptions.VaultKey, "vault-key", "bitbucket-cli", "")
	cmd.Flags().StringVar(&createOptions.User, "user", "", "")
	cmd.Flags().StringVar(&createOptions.Password, "password", "", "")
	cmd.Flags().Bool("password-stdin", false, "")
	cmd.Flags().StringVar(&createOptions.AccessToken, "access-token", "", "")
	cmd.Flags().Bool("access-token-stdin", false, "")
	cmd.Flags().String("profile", "", "")
	cmd.Flags().String("config", "", "")
	cmd.Flags().Bool("dry-run", false, "")
	cmd.MarkFlagsRequiredTogether("client-id", "client-secret")
	cmd.MarkFlagsMutuallyExclusive("user", "client-id", "access-token", "access-token-stdin")
	cmd.MarkFlagsMutuallyExclusive("password", "password-stdin")
	cmd.MarkFlagsMutuallyExclusive("access-token", "access-token-stdin")
	cmd.MarkFlagsMutuallyExclusive("password-stdin", "access-token-stdin")
	return cmd
}

// TestCreateProcessNeverPersistsVaultLoadedClientSecret proves `bb profile create -n foo
// --client-id abc --client-secret ""` never writes a vault-loaded client secret to the config file
// in plain text: MarkFlagsRequiredTogether only checks Changed, not the value, so this command
// line reaches resolveVaultSecret's load-from-vault branch, and resolveCreateSecrets must set
// createOptions.vault.clientSecret so Profile.forSave knows to blank the secret before saving. The
// vault itself is a keyring.MockInit in-memory store, not the real OS keychain, so this test is
// hermetic.
func TestCreateProcessNeverPersistsVaultLoadedClientSecret(t *testing.T) {
	keyring.MockInit()

	oldProfiles, oldCurrent := Profiles, Current
	oldConfig := common.CurrentConfig()
	oldOptions := createOptions
	t.Cleanup(func() {
		Profiles = oldProfiles
		Current = oldCurrent
		common.SetCurrentConfig(oldConfig)
		createOptions = oldOptions
	})
	Profiles = nil
	Current = nil

	configPath := filepath.Join(t.TempDir(), "config-cli.yml")
	common.SetCurrentConfig(&common.Config{Path: configPath, Data: map[string]any{}})

	const (
		profileName = "vault-leak-test"
		vaultKey    = "bitbucket-cli-test-vault-leak-key"
		clientID    = "client-id-vault-leak-test"
		vaultSecret = "s3cr3t-client-secret-from-vault-must-not-leak"
	)

	// Pre-populate the (mocked) vault, simulating a secret stored by an earlier `profile create`
	// or `profile update --client-secret`.
	if err := (Profile{}).SetCredentialInVault(vaultKey, clientID, vaultSecret); err != nil {
		t.Fatalf("cannot seed the fake vault: %v", err)
	}

	cmd := newIsolatedCreateCmd()
	cmd.SetArgs([]string{
		"--name", profileName,
		"--client-id", clientID,
		"--client-secret", "", // Changed=true, value empty: the reachable path per the finding
		"--vault-key", vaultKey,
	})
	cmd.SetContext(context.Background())

	if err := cmd.Execute(); err != nil {
		t.Fatalf("profile create error = %v", err)
	}

	raw, err := os.ReadFile(configPath)
	if err != nil {
		t.Fatalf("cannot read saved config: %v", err)
	}
	if strings.Contains(string(raw), vaultSecret) {
		t.Fatalf("saved config = %q, must NOT contain the vault-loaded client secret in plain text", raw)
	}

	created, found := Profiles.Find(profileName)
	if !found {
		t.Fatalf("profile %q was not created", profileName)
	}
	if !created.vault.clientSecret {
		t.Error("created profile's vault.clientSecret provenance bit = false, want true: the secret came from the vault")
	}
	if created.ClientSecret != vaultSecret {
		t.Errorf("created profile's in-memory ClientSecret = %q, want the vault-loaded value %q so the rest of this process can still use it", created.ClientSecret, vaultSecret)
	}
}
