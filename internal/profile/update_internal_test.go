package profile

import (
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
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
	cmd.Flags().Bool("password-stdin", false, "")
	cmd.Flags().StringVar(&updateOptions.ClientID, "client-id", "", "")
	cmd.Flags().StringVar(&updateOptions.ClientSecret, "client-secret", "", "")
	cmd.Flags().Bool("client-secret-stdin", false, "")
	cmd.Flags().StringVar(&updateOptions.AccessToken, "access-token", "", "")
	cmd.Flags().Bool("access-token-stdin", false, "")
	cmd.Flags().BoolVar(&updateOptions.ToVault, "to-vault", false, "")
	cmd.Flags().BoolVar(&updateOptions.NoVault, "no-vault", false, "")
	cmd.Flags().Var(updateOptions.DefaultWorkspace, "default-workspace", "")
	cmd.Flags().Var(updateOptions.DefaultProject, "default-project", "")
	cmd.Flags().IntVar(&updateOptions.DefaultPageLength, "default-page-length", 0, "")
	cmd.Flags().Var(&updateOptions.ErrorProcessing, "error-processing", "")
	cmd.Flags().BoolVar(&updateOptions.Progress, "progress", false, "")
	cmd.Flags().String("profile", "", "")
	cmd.Flags().String("config", "", "")
	cmd.Flags().String("output", "", "")
	cmd.Flags().Bool("dry-run", false, "")
	cmd.MarkFlagsMutuallyExclusive("user", "client-id", "access-token", "access-token-stdin")
	cmd.MarkFlagsMutuallyExclusive("password", "password-stdin")
	cmd.MarkFlagsMutuallyExclusive("access-token", "access-token-stdin")
	cmd.MarkFlagsMutuallyExclusive("client-secret", "client-secret-stdin")
	cmd.MarkFlagsMutuallyExclusive("password-stdin", "access-token-stdin")
	cmd.MarkFlagsMutuallyExclusive("password-stdin", "client-secret-stdin")
	cmd.MarkFlagsMutuallyExclusive("access-token-stdin", "client-secret-stdin")
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

// TestUpdateProcessNeverEchoesFreshlyPipedSecretOnPlainTextProfile reproduces the FINAL CRITICAL
// GATE's priority-2 finding: forSave() only blanks vault-provenance secrets, so on a profile that
// already stores its credentials in plain text (NoVault forced true by hasPlainTextSecret), a
// freshly piped --access-token-stdin value landed unchanged in profile.AccessToken and was echoed
// straight to stdout by the old forSave()-based confirmation print -- defeating the entire point
// of piping the secret in instead of typing it on the command line. updateProcess must print
// profile.forDisplay() instead, which masks every secret unconditionally, regardless of
// provenance.
func TestUpdateProcessNeverEchoesFreshlyPipedSecretOnPlainTextProfile(t *testing.T) {
	withUpdateOptions(t)
	withIsolatedProfilesConfig(t)

	const (
		profileName = "plaintext-echo-test"
		oldToken    = "old-plaintext-token"
		newToken    = "s3cr3t-piped-token-must-not-be-echoed"
	)

	// A plain-text profile: AccessToken is already set and vault.accessToken is (the zero value)
	// false, so hasPlainTextSecret() is true and resolveProfileCredentials forces NoVault, the
	// exact precondition that made forSave() a no-op for this secret.
	target := &Profile{Name: profileName, AccessToken: oldToken}
	Profiles = append(Profiles, target)

	cmd := newIsolatedUpdateCmd()
	// -o json (rather than the default table, which no longer even lists AccessToken among its
	// default columns -- see GetHeaders) forces the confirmation output through MarshalJSON,
	// proving forDisplay masks the secret unconditionally rather than merely being absent from the
	// default column set.
	cmd.SetArgs([]string{profileName, "--access-token-stdin", "--output", "json"})
	cmd.SetContext(context.Background())
	cmd.SetIn(strings.NewReader(newToken + "\n"))

	stdout := captureStdout(t, func() {
		if err := cmd.Execute(); err != nil {
			t.Fatalf("profile update error = %v", err)
		}
	})

	if strings.Contains(stdout, newToken) {
		t.Fatalf("stdout = %q, must NOT contain the freshly piped access token", stdout)
	}
	if !strings.Contains(stdout, secretMask) {
		t.Errorf("stdout = %q, want it to contain the masked secret marker %q", stdout, secretMask)
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

// TestUpdateProcessSwitchingShapeClearsTheOldOne reproduces the FINAL CRITICAL GATE's priority-3
// finding: switching a profile's credential shape via `profile update` used to be a silent no-op
// for the shape being replaced -- updateCredentials only ever set non-empty fields, never cleared
// the others -- so `profile update work --access-token-stdin` on a user/password profile stored
// the token but left User/Password intact, and resolveAuthorization prefers Basic auth whenever
// User != "", meaning the profile kept authenticating with the old password until it was revoked,
// while the user believed they had moved to a token. After the fix, switching to a new shape must
// clear the other two: User/Password gone, AccessToken set.
func TestUpdateProcessSwitchingShapeClearsTheOldOne(t *testing.T) {
	keyring.MockInit()
	withUpdateOptions(t)
	withIsolatedProfilesConfig(t)

	const (
		profileName = "shape-switch-test"
		vaultKey    = "bitbucket-cli-test-shape-switch"
		oldUser     = "alice"
		oldPassword = "old-plaintext-password"
		newToken    = "new-access-token-must-win"
	)

	// A plain-text user/password profile (vault.password stays false -- the zero value).
	target := &Profile{Name: profileName, VaultKey: vaultKey, User: oldUser, Password: oldPassword}
	Profiles = append(Profiles, target)

	cmd := newIsolatedUpdateCmd()
	cmd.SetArgs([]string{profileName, "--access-token", newToken})
	cmd.SetContext(context.Background())

	if err := cmd.Execute(); err != nil {
		t.Fatalf("profile update error = %v", err)
	}

	updated, found := Profiles.Find(profileName)
	if !found {
		t.Fatal("profile not found after update")
	}
	if updated.User != "" {
		t.Errorf("User = %q, want it cleared after switching to the access-token shape", updated.User)
	}
	if updated.Password != "" {
		t.Errorf("Password = %q, want it cleared after switching to the access-token shape", updated.Password)
	}
	// The new access token is stored in the vault (the default, since no plain-text secret is
	// left on the profile once the old shape is cleared), leaving AccessToken itself blank in
	// memory/on disk -- exactly like any other freshly given --access-token on a vault-backed
	// profile; see storeCredentialIfChanged.
	credential, err := (Profile{}).GetCredentialFromVault(vaultKey, profileName)
	if err != nil {
		t.Fatalf("cannot get the new access token from the vault: %v", err)
	}
	if credential.Password != newToken {
		t.Errorf("vault access token = %q, want %q", credential.Password, newToken)
	}
}

// TestUpdateProcessSwitchingShapeDeletesTheOldVaultEntry is
// TestUpdateProcessSwitchingShapeClearsTheOldOne's vault-provenance sibling: when the shape being
// replaced held a secret in the vault (not just in-memory plain text), switching shapes must
// delete that stranded vault entry too, not just clear the in-memory field.
func TestUpdateProcessSwitchingShapeDeletesTheOldVaultEntry(t *testing.T) {
	keyring.MockInit()
	withUpdateOptions(t)
	withIsolatedProfilesConfig(t)

	const (
		profileName = "shape-switch-vault-test"
		vaultKey    = "bitbucket-cli-test-shape-switch-vault"
		oldUser     = "bob"
		oldPassword = "old-vaulted-password"
		newToken    = "new-access-token-must-win"
	)

	if err := (Profile{}).SetCredentialInVault(vaultKey, oldUser, oldPassword); err != nil {
		t.Fatalf("cannot seed the fake vault: %v", err)
	}

	target := &Profile{Name: profileName, VaultKey: vaultKey, User: oldUser}
	target.vault.password = true // simulates the password having been loaded from the vault at runtime
	Profiles = append(Profiles, target)

	cmd := newIsolatedUpdateCmd()
	cmd.SetArgs([]string{profileName, "--access-token", newToken})
	cmd.SetContext(context.Background())

	if err := cmd.Execute(); err != nil {
		t.Fatalf("profile update error = %v", err)
	}

	if _, err := (Profile{}).GetCredentialFromVault(vaultKey, oldUser); err == nil {
		t.Error("old password is still reachable in the vault after switching to the access-token shape, want it deleted")
	}
}

// TestUpdateProcessRotatesPasswordWithoutRetypingUser reproduces the FINAL CRITICAL GATE's
// priority-3 finding: rotating a secret required re-typing --user even when the profile already
// has one on record, because requireUserForPasswordSource/storeCredentialIfChanged were keyed on
// whether --user itself changed on this command line, not on whether the profile already has a
// user. `op read ... | bb profile update work --password-stdin` (no --user) must succeed and
// store the new password under the profile's existing user.
func TestUpdateProcessRotatesPasswordWithoutRetypingUser(t *testing.T) {
	keyring.MockInit()
	withUpdateOptions(t)
	withIsolatedProfilesConfig(t)

	const (
		profileName = "rotate-password-test"
		vaultKey    = "bitbucket-cli-test-rotate-password"
		user        = "carol"
		newPassword = "s3cr3t-rotated-password"
	)

	target := &Profile{Name: profileName, VaultKey: vaultKey, User: user}
	Profiles = append(Profiles, target)

	cmd := newIsolatedUpdateCmd()
	cmd.SetIn(strings.NewReader(newPassword + "\n"))
	cmd.SetArgs([]string{profileName, "--password-stdin"})
	cmd.SetContext(context.Background())

	if err := cmd.Execute(); err != nil {
		t.Fatalf("profile update error = %v, want --password-stdin to be accepted without --user when the profile already has one", err)
	}

	credential, err := (Profile{}).GetCredentialFromVault(vaultKey, user)
	if err != nil {
		t.Fatalf("cannot get credential from vault: %v", err)
	}
	if credential.Password != newPassword {
		t.Errorf("vault password = %q, want %q", credential.Password, newPassword)
	}
}

// TestUpdateClientSecretStdinReadsAndTrims proves `bb profile update foo --client-id abc
// --client-secret-stdin` reads the OAuth2 client secret piped on stdin instead of requiring
// --client-secret on the command line (which would land in shell history), and stores it exactly
// like --client-secret would.
func TestUpdateClientSecretStdinReadsAndTrims(t *testing.T) {
	keyring.MockInit()
	withUpdateOptions(t)
	withIsolatedProfilesConfig(t)

	const (
		profileName = "client-secret-stdin-test"
		vaultKey    = "bitbucket-cli-test-client-secret-stdin"
		clientID    = "client-id-stdin-test"
		piped       = "s3cr3t-piped-client-secret\n"
	)

	target := &Profile{Name: profileName, VaultKey: vaultKey}
	Profiles = append(Profiles, target)

	cmd := newIsolatedUpdateCmd()
	cmd.SetIn(strings.NewReader(piped))
	cmd.SetArgs([]string{profileName, "--client-id", clientID, "--client-secret-stdin"})
	cmd.SetContext(context.Background())

	if err := cmd.Execute(); err != nil {
		t.Fatalf("profile update error = %v", err)
	}

	credential, err := (Profile{}).GetCredentialFromVault(vaultKey, clientID)
	if err != nil {
		t.Fatalf("cannot get credential from vault: %v", err)
	}
	if credential.Password != "s3cr3t-piped-client-secret" {
		t.Errorf("vault client secret = %q, want the piped secret trimmed of its trailing newline", credential.Password)
	}
}

// TestUpdateProcessValidatesDefaultWorkspaceAgainstLiveWorkspaces reproduces the FINAL CRITICAL
// GATE's priority-5 finding: --default-workspace's EnumFlag is dynamic (AllowedFunc-backed), so
// EnumFlag.Set deliberately never validates it at parse time (see common/flags.go) -- correct for
// most flags sharing that mechanism, but wrong here specifically, since the workspace *is* the
// thing `profile update --default-workspace` sets, not an incidental value for some other
// operation. A typo must be rejected before it is ever persisted to the config file, instead of
// resurfacing later as a 404 from an unrelated command.
func TestUpdateProcessValidatesDefaultWorkspaceAgainstLiveWorkspaces(t *testing.T) {
	withUpdateOptions(t)
	withIsolatedProfilesConfig(t)

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"values":[{"workspace":{"slug":"acme"}}]}`))
	}))
	t.Cleanup(server.Close)
	apiRoot, err := url.Parse(server.URL)
	if err != nil {
		t.Fatalf("cannot parse test server URL: %v", err)
	}

	const profileName = "default-workspace-validation-test"
	target := &Profile{Name: profileName, APIRoot: apiRoot, AccessToken: "dummy-token"}
	Profiles = append(Profiles, target)

	t.Run("a typo is rejected and never persisted", func(t *testing.T) {
		cmd := newIsolatedUpdateCmd()
		cmd.SetArgs([]string{profileName, "--default-workspace", "acmee-typo"})
		cmd.SetContext(context.Background())

		if err := cmd.Execute(); err == nil {
			t.Fatal("cmd.Execute() error = nil, want an error for an unknown workspace slug")
		}

		updated, found := Profiles.Find(profileName)
		if !found {
			t.Fatal("profile disappeared after the failed update")
		}
		if updated.DefaultWorkspace != "" {
			t.Errorf("DefaultWorkspace = %q, want it left unset after the rejected update", updated.DefaultWorkspace)
		}
	})

	t.Run("a real workspace slug is accepted", func(t *testing.T) {
		cmd := newIsolatedUpdateCmd()
		cmd.SetArgs([]string{profileName, "--default-workspace", "acme"})
		cmd.SetContext(context.Background())

		if err := cmd.Execute(); err != nil {
			t.Fatalf("cmd.Execute() error = %v, want the real workspace slug to be accepted", err)
		}

		updated, found := Profiles.Find(profileName)
		if !found {
			t.Fatal("profile not found after update")
		}
		if updated.DefaultWorkspace != "acme" {
			t.Errorf("DefaultWorkspace = %q, want %q", updated.DefaultWorkspace, "acme")
		}
	})
}
