package profile

import (
	"context"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/avitsrimer/bitbucket-cli/internal/common"
	"github.com/spf13/cobra"
	"github.com/zalando/go-keyring"
)

// captureStdout redirects os.Stdout for the duration of fn and returns what was written. This
// package cannot import internal/testutil's equivalent helper: testutil imports profile, so that
// would be an import cycle for an internal (package profile) test file.
func captureStdout(t *testing.T, fn func()) string {
	t.Helper()

	r, w, err := os.Pipe()
	if err != nil {
		t.Fatalf("cannot create pipe: %v", err)
	}
	original := os.Stdout
	os.Stdout = w
	defer func() { os.Stdout = original }()

	captured := make(chan string, 1)
	go func() {
		data, _ := io.ReadAll(r)
		captured <- string(data)
	}()

	fn()

	_ = w.Close()
	return <-captured
}

// newIsolatedUpdateCmd builds a throwaway *cobra.Command carrying its own FlagSet bound to the
// same package-level updateOptions fields updateCmd's real flags are bound to, mirroring
// newIsolatedCreateCmd's rationale: a fresh FlagSet avoids leaking each Set() call's Changed bit
// across other tests sharing the singleton updateCmd.
func newIsolatedUpdateCmd() *cobra.Command {
	cmd := &cobra.Command{Use: "update", Args: cobra.ExactArgs(1), RunE: updateProcess}
	cmd.Flags().StringVarP(&updateOptions.Name, "name", "n", "", "")
	cmd.Flags().StringVar(&updateOptions.Description, "description", "", "")
	cmd.Flags().BoolVar(&updateOptions.Default, "default", false, "")
	cmd.Flags().StringVar(&updateOptions.VaultKey, "vault-key", "", "")
	cmd.Flags().StringVarP(&updateOptions.User, "user", "u", "", "")
	cmd.Flags().StringVar(&updateOptions.Password, "password", "", "")
	cmd.Flags().StringVar(&updateOptions.ClientID, "client-id", "", "")
	cmd.Flags().StringVar(&updateOptions.ClientSecret, "client-secret", "", "")
	cmd.Flags().StringVar(&updateOptions.AccessToken, "access-token", "", "")
	cmd.Flags().BoolVar(&updateOptions.ToVault, "to-vault", false, "")
	cmd.Flags().BoolVar(&updateOptions.NoVault, "no-vault", false, "")
	cmd.Flags().IntVar(&updateOptions.DefaultPageLength, "default-page-length", 0, "")
	cmd.Flags().Var(&updateOptions.ErrorProcessing, "error-processing", "")
	cmd.Flags().BoolVar(&updateOptions.Progress, "progress", false, "")
	cmd.Flags().String("profile", "", "")
	cmd.Flags().String("config", "", "")
	cmd.Flags().String("output", "", "")
	cmd.Flags().Bool("dry-run", false, "")
	return cmd
}

// withUpdateOptions saves/restores the package-level updateOptions (bound to updateCmd's real
// flags at init), matching withCreateOptions's rationale in create_test.go.
func withUpdateOptions(t *testing.T) {
	t.Helper()
	old := updateOptions
	t.Cleanup(func() { updateOptions = old })
}

// withIsolatedProfilesConfig resets the package-global Profiles/Current/CurrentConfig singletons
// to a fresh, empty config file rooted at a temp directory, restoring the previous values on
// cleanup, matching TestCreateProcessNeverPersistsVaultLoadedClientSecret's rationale.
func withIsolatedProfilesConfig(t *testing.T) {
	t.Helper()
	oldProfiles, oldCurrent := Profiles, Current
	oldConfig := common.CurrentConfig()
	t.Cleanup(func() {
		Profiles = oldProfiles
		Current = oldCurrent
		common.SetCurrentConfig(oldConfig)
	})
	Profiles = nil
	Current = nil

	configPath := filepath.Join(t.TempDir(), "config-cli.yml")
	common.SetCurrentConfig(&common.Config{Path: configPath, Data: map[string]any{}})
}

// TestUpdateCommandDoesNotShadowRootOutputFlag reproduces major finding #13: createCmd/updateCmd
// used to register their own local "output" flag (setting the profile's default output format),
// which shadows the root persistent -o/--output flag of the same name (a local flag always wins
// over an inherited one), breaking -o on these two commands and making Profile.Print read the
// wrong flag to decide how to render the command's own confirmation output. The flag must be
// named something else (here "default-output") so -o/--output stay the root's alone.
func TestUpdateCommandDoesNotShadowRootOutputFlag(t *testing.T) {
	if updateCmd.Flags().Lookup("output") != nil {
		t.Error(`updateCmd has a local "output" flag, which would shadow the root persistent -o/--output flag`)
	}
	if updateCmd.Flags().ShorthandLookup("o") != nil {
		t.Error(`updateCmd has a local "-o" shorthand flag, which would conflict with the root persistent -o/--output flag`)
	}
	if updateCmd.Flags().Lookup("default-output") == nil {
		t.Error(`updateCmd is missing the "default-output" flag that replaces the shadowing "output" one`)
	}

	if createCmd.Flags().Lookup("output") != nil {
		t.Error(`createCmd has a local "output" flag, which would shadow the root persistent -o/--output flag`)
	}
	if createCmd.Flags().ShorthandLookup("o") != nil {
		t.Error(`createCmd has a local "-o" shorthand flag, which would conflict with the root persistent -o/--output flag`)
	}
	if createCmd.Flags().Lookup("default-output") == nil {
		t.Error(`createCmd is missing the "default-output" flag that replaces the shadowing "output" one`)
	}
}

// TestUpdateProcessNeverEchoesVaultLoadedAccessToken reproduces critical finding #3:
// `profile update` printed the live, in-memory Profile after saving, which can hold an access
// token loaded from the vault at runtime (e.g. loadAccessToken authorizing a dynamic
// --default-workspace/--default-project lookup during flag parsing), so the secret was echoed to
// stdout even though it was never written to the config file. updateProcess must print
// profile.forSave() instead, which blanks any vault-loaded secret before rendering.
func TestUpdateProcessNeverEchoesVaultLoadedAccessToken(t *testing.T) {
	withUpdateOptions(t)
	withIsolatedProfilesConfig(t)

	const (
		profileName = "echo-test"
		vaultToken  = "s3cr3t-token-must-not-be-echoed"
	)

	target := &Profile{Name: profileName, VaultKey: "bitbucket-cli"}
	target.AccessToken = vaultToken
	target.vault.accessToken = true // simulates loadAccessToken's vault fallback populating it at runtime
	Profiles = append(Profiles, target)

	cmd := newIsolatedUpdateCmd()
	cmd.SetArgs([]string{profileName, "--description", "updated description"})
	cmd.SetContext(context.Background())

	stdout := captureStdout(t, func() {
		if err := cmd.Execute(); err != nil {
			t.Fatalf("profile update error = %v", err)
		}
	})

	if strings.Contains(stdout, vaultToken) {
		t.Fatalf("stdout = %q, must NOT contain the vault-loaded access token", stdout)
	}
}

// TestUpdateProcessMigratesAccessTokenVaultEntryOnRename reproduces critical finding #5: an access
// token is keyed in the vault by the profile's name, so `profile update <name> --name <new>`
// without also setting a new --access-token used to leave the vault entry stranded under the old
// name -- unreachable (loadAccessToken looks it up under the new name) and unreclaimable (`profile
// delete` only purges the vault entry under the profile's current name).
func TestUpdateProcessMigratesAccessTokenVaultEntryOnRename(t *testing.T) {
	keyring.MockInit()
	withUpdateOptions(t)
	withIsolatedProfilesConfig(t)

	const (
		oldName    = "old-name"
		newName    = "new-name"
		vaultKey   = "bitbucket-cli-test-rename"
		vaultToken = "s3cr3t-token-must-follow-the-rename"
	)

	if err := (Profile{}).SetCredentialInVault(vaultKey, oldName, vaultToken); err != nil {
		t.Fatalf("cannot seed the fake vault: %v", err)
	}

	target := &Profile{Name: oldName, VaultKey: vaultKey}
	Profiles = append(Profiles, target)

	cmd := newIsolatedUpdateCmd()
	cmd.SetArgs([]string{oldName, "--name", newName})
	cmd.SetContext(context.Background())

	if err := cmd.Execute(); err != nil {
		t.Fatalf("profile update error = %v", err)
	}

	if _, err := (Profile{}).GetCredentialFromVault(vaultKey, oldName); err == nil {
		t.Error("vault entry still reachable under the old name after rename, want it migrated away")
	}
	credential, err := (Profile{}).GetCredentialFromVault(vaultKey, newName)
	if err != nil {
		t.Fatalf("cannot get credential under the new name after rename: %v", err)
	}
	if credential.Password != vaultToken {
		t.Errorf("migrated token = %q, want %q", credential.Password, vaultToken)
	}
}

// TestMoveCredentialsToVaultMovesEveryPlainTextSecretIndependently reproduces major finding #11:
// moveCredentialsToVault used a switch (mutually exclusive cases), so a profile holding more than
// one plain-text secret at once (only reachable via a hand-edited config file, since the
// create/update flags that set them are mutually exclusive on the command line) had only the
// first match moved to the vault by `--to-vault`, silently leaving the rest in plain text despite
// reporting overall success.
func TestMoveCredentialsToVaultMovesEveryPlainTextSecretIndependently(t *testing.T) {
	keyring.MockInit()

	target := &Profile{
		Name:         "multi-secret",
		ClientID:     "client-id",
		ClientSecret: "client-secret-value",
		User:         "user",
		Password:     "password-value",
	}

	if err := moveCredentialsToVault(target, "bitbucket-cli-test-move-all"); err != nil {
		t.Fatalf("moveCredentialsToVault() error = %v", err)
	}

	if target.ClientSecret != "" {
		t.Errorf("ClientSecret = %q, want blanked after moving to the vault", target.ClientSecret)
	}
	if target.Password != "" {
		t.Errorf("Password = %q, want blanked after moving to the vault", target.Password)
	}

	if _, err := target.GetCredentialFromVault("bitbucket-cli-test-move-all", target.ClientID); err != nil {
		t.Errorf("client secret was not stored in the vault: %v", err)
	}
	if _, err := target.GetCredentialFromVault("bitbucket-cli-test-move-all", target.User); err != nil {
		t.Errorf("user password was not stored in the vault: %v", err)
	}
}
