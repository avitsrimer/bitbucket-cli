package profile

import (
	"errors"
	"fmt"
	"os"
	"runtime"

	"github.com/avitsrimer/bitbucket-cli/internal/common"
	"github.com/go-pkgz/lgr"
	"github.com/spf13/cobra"
)

var updateCmd = &cobra.Command{
	Use:               "update [flags] <profile-name>",
	Aliases:           []string{"edit"},
	Short:             "update a profile by its <profile-name>.",
	Args:              cobra.ExactArgs(1),
	ValidArgsFunction: ValidProfileNames,
	PreRunE:           disableUnsupportedFlags,
	RunE:              updateProcess,
}

var updateOptions struct {
	Profile
	DefaultWorkspace *common.EnumFlag
	DefaultProject   *common.EnumFlag
	OutputFormat     *common.EnumFlag
	CloneProtocol    *common.EnumFlag
	ToVault          bool
	NoVault          bool
}

func init() {
	Command.AddCommand(updateCmd)

	updateOptions.DefaultWorkspace = common.NewEnumFlagWithFunc(updateCmd, "", getWorkspaceSlugs)
	updateOptions.DefaultProject = common.NewEnumFlagWithFunc(updateCmd, "", getProjectKeys)
	updateOptions.OutputFormat = common.NewEnumFlag("json", "yaml", "table", "csv", "tsv")
	updateOptions.CloneProtocol = common.NewEnumFlag("+git", "https", "ssh")
	updateCmd.Flags().StringVarP(&updateOptions.Name, "name", "n", "", "Name of the profile")
	updateCmd.Flags().StringVar(&updateOptions.Description, "description", "", "Description of the profile")
	updateCmd.Flags().BoolVar(&updateOptions.Default, "default", false, "True if this is the default profile")
	if runtime.GOOS != "windows" {
		updateCmd.Flags().StringVar(&updateOptions.VaultKey, "vault-key", "bitbucket-cli", "Vault key to use for storing credentials. Default is bitbucket-cli. On Windows, the Windows Credential Manager will be used, On Linux and macOS, the system keychain will be used.")
	}
	updateCmd.Flags().StringVarP(&updateOptions.User, "user", "u", "", "User's name of the profile")
	updateCmd.Flags().StringVar(&updateOptions.Password, "password", "", "Password of the profile")
	updateCmd.Flags().Bool("password-stdin", false, "Read the password from stdin instead of --password, so it never appears in shell history.")
	updateCmd.Flags().StringVar(&updateOptions.ClientID, "client-id", "", "Client ID of the profile")
	updateCmd.Flags().StringVar(&updateOptions.ClientSecret, "client-secret", "", "Client Secret of the profile")
	updateCmd.Flags().Uint16Var(&updateOptions.CallbackPort, "callback-port", 0, "Callback port to use for OAuth2 authentication. If not set, a random port will be used.")
	updateCmd.Flags().StringVar(&updateOptions.AccessToken, "access-token", "", "Access Token of the profile")
	updateCmd.Flags().Bool("access-token-stdin", false, "Read the access token from stdin instead of --access-token, so it never appears in shell history.")
	updateCmd.Flags().BoolVar(&updateOptions.ToVault, "to-vault", false, "Store credentials in the vault. This will remove any credentials from the profile and store them in the vault. If the vault key is not provided, it will use the existing vault key of the profile or the default vault key if not set.")
	updateCmd.Flags().BoolVar(&updateOptions.NoVault, "no-vault", false, "Do not use a vault for storing credentials")
	updateCmd.Flags().Var(updateOptions.DefaultWorkspace, "default-workspace", "Default workspace of the profile")
	updateCmd.Flags().Var(updateOptions.DefaultProject, "default-project", "Default project of the profile")
	updateCmd.Flags().Var(updateOptions.CloneProtocol, "clone-protocol", "Default protocol to use for cloning repositories. Default is git, can be https, git, or ssh")
	updateCmd.Flags().StringVar(&updateOptions.CloneUser, "clone-user", "", "Username to use when cloning repositories. Default is the username of the profile.")
	updateCmd.Flags().StringVar(&updateOptions.SshKeyFilename, "default-ssh-key-file", "", "Path to the SSH private key file to use when cloning repositories with the ssh protocol.")
	// Named "default-output", not "output": a local "output" flag would shadow the root
	// persistent -o/--output flag (local flags win over inherited ones of the same name),
	// breaking -o on this command and making Profile.Print read this flag's value instead of the
	// root one to decide how to render *this command's own* confirmation output.
	updateCmd.Flags().Var(updateOptions.OutputFormat, "default-output", "Default output format of the profile (json, yaml, table, csv, tsv).")
	updateCmd.Flags().IntVar(&updateOptions.DefaultPageLength, "default-page-length", 0, "Default number of items per page to retrieve from Bitbucket (Default: 50).")
	updateCmd.Flags().Var(&updateOptions.ErrorProcessing, "error-processing", "Error processing (StopOnError, WarnOnError, IgnoreErrors).")
	updateCmd.Flags().BoolVar(&updateOptions.Progress, "progress", false, "Show progress during upload/download operations.")
	updateCmd.MarkFlagsRequiredTogether("client-id", "client-secret")
	// "user" is deliberately not required-together with "password"/"password-stdin" here: --user
	// given alone is valid and triggers an interactive, no-echo password prompt instead (see
	// resolveUpdateSecretInput). requireUserForPasswordSource enforces the other direction: a
	// password source given without --user is still rejected.
	updateCmd.MarkFlagsMutuallyExclusive("user", "client-id", "access-token", "access-token-stdin")
	updateCmd.MarkFlagsMutuallyExclusive("password", "password-stdin")
	updateCmd.MarkFlagsMutuallyExclusive("access-token", "access-token-stdin")
	updateCmd.MarkFlagsMutuallyExclusive("password-stdin", "access-token-stdin")
	updateCmd.MarkFlagsMutuallyExclusive("to-vault", "no-vault")
	updateCmd.MarkFlagsMutuallyExclusive("to-vault", "access-token")
	updateCmd.MarkFlagsMutuallyExclusive("to-vault", "access-token-stdin")
	updateCmd.MarkFlagsMutuallyExclusive("to-vault", "client-id")
	updateCmd.MarkFlagsMutuallyExclusive("to-vault", "client-secret")
	updateCmd.MarkFlagsMutuallyExclusive("to-vault", "user")
	updateCmd.MarkFlagsMutuallyExclusive("to-vault", "password")
	updateCmd.MarkFlagsMutuallyExclusive("to-vault", "password-stdin")
	_ = updateCmd.MarkFlagFilename("default-ssh-key-file")
	_ = updateCmd.RegisterFlagCompletionFunc(updateOptions.DefaultWorkspace.CompletionFunc("default-workspace"))
	_ = updateCmd.RegisterFlagCompletionFunc(updateOptions.DefaultProject.CompletionFunc("default-project"))
	_ = updateCmd.RegisterFlagCompletionFunc(updateOptions.CloneProtocol.CompletionFunc("clone-protocol"))
	_ = updateCmd.RegisterFlagCompletionFunc(updateOptions.OutputFormat.CompletionFunc("default-output"))
	_ = updateCmd.RegisterFlagCompletionFunc("error-processing", updateOptions.ErrorProcessing.CompletionFunc())
	updateCmd.SetHelpFunc(hideUnsupportedFlags)
}

func updateProcess(cmd *cobra.Command, args []string) (err error) {
	ctx := cmd.Context()

	if len(args) == 0 {
		return errors.New("argument profile is missing")
	}
	_, err = GetProfileFromCommand(ctx, cmd)
	if errors.Is(err, ErrNoProfiles) || len(Profiles) == 0 {
		return errors.New("no profiles found")
	}
	if err != nil {
		return err
	}

	applyUpdateOverrides()

	// Resolved before Validate/WhatIf so --dry-run still prompts/reads stdin for the secret: a dry
	// run needs the value structurally to validate the command line, it only skips the vault
	// write and the actual profile update below.
	if secretErr := resolveUpdateSecretInput(cmd); secretErr != nil {
		return secretErr
	}

	lgr.Printf("[DEBUG] loading profile %s (valid names: %v)", args[0], Profiles.Names())
	profile, found := Profiles.Find(args[0])
	if !found {
		return fmt.Errorf("profile %s not found", args[0])
	}

	// profile.Redact() masks secrets before they hit the debug log
	lgr.Printf("[DEBUG] updating profile %s: %+v", profile.Name, profile.Redact())
	if !common.WhatIf(cmd, "Updating profile %s", profile.Name) {
		return nil
	}

	err = resolveProfileCredentials(cmd, profile)
	if err != nil {
		return err
	}

	err = profile.Update(updateOptions.Profile)
	if err != nil {
		return err
	}
	if cmd.Flags().Changed("progress") {
		profile.Progress = updateOptions.Progress
	}
	// updateSimpleFields cannot tell an explicit --error-processing StopOnError apart from the
	// flag being absent (both are ErrorProcessing's zero value): handle that one case here, the
	// same way --progress is handled above.
	if cmd.Flags().Changed("error-processing") && updateOptions.ErrorProcessing == common.StopOnError {
		profile.ErrorProcessing = common.StopOnError
	}
	if updateOptions.Default {
		Profiles.SetCurrent(profile.Name)
	}
	// profile.Redact() masks secrets before they hit the debug log
	lgr.Printf("[DEBUG] updated profile %s: %+v", profile.Name, profile.Redact())

	if err := saveProfilesConfig(); err != nil {
		return err
	}
	// profile.forSave() blanks out any vault-loaded secret (e.g. an access token loaded from the
	// vault while resolving --default-workspace/--default-project's dynamic allowed values during
	// flag parsing) so it is never echoed to the console in plain text.
	return profile.Print(ctx, cmd, profile.forSave())
}

// resolveCredentialOwner returns updated when flagName changed on the command line and updated is
// non-empty, otherwise current: the username/clientID/profile name a credential should be looked
// up or stored under in the vault.
func resolveCredentialOwner(cmd *cobra.Command, flagName, current, updated string) string {
	if cmd.Flag(flagName).Changed && updated != "" {
		return updated
	}
	return current
}

// resolveProfileCredentials moves existing plain-text credentials into the vault when requested,
// then stores any credential values changed on the command line into the vault when in use
func resolveProfileCredentials(cmd *cobra.Command, profile *Profile) error {
	state := readSecretFlagState(cmd)

	// 1. move any credentials still stored in plain text into the vault, when requested
	if updateOptions.ToVault {
		updateOptions.NoVault = false
		vaultKey := profile.VaultKey
		if runtime.GOOS != "windows" && cmd.Flag("vault-key").Changed && updateOptions.VaultKey != "" {
			vaultKey = updateOptions.VaultKey
		}
		if err := moveCredentialsToVault(profile, vaultKey); err != nil {
			return err
		}
	}

	// 2. keep a profile that already stores its credentials in plain text that way
	if profile.hasPlainTextSecret() {
		lgr.Printf("[DEBUG] profile %s stored its credentials in plain text, we should keep it that way", profile.Name)
		updateOptions.NoVault = true
	}

	// 3. resolve the vault key early, so it's available below when storing secrets
	if runtime.GOOS != "windows" && !cmd.Flag("vault-key").Changed {
		if profile.VaultKey == "" {
			profile.VaultKey = "bitbucket-cli"
		}
		updateOptions.VaultKey = profile.VaultKey
	}

	// 4. an access token is keyed in the vault by the profile's name: a rename that does not also
	// set a new --access-token/--access-token-stdin would otherwise leave its vault entry stranded
	// under the old name, unreachable (loadAccessToken looks it up under the new name) and
	// unreclaimable (profile delete only purges the vault entry under the profile's current name).
	// Migrate it across the rename before anything below saves the profile under its new name.
	if !updateOptions.NoVault && cmd.Flag("name").Changed && updateOptions.Name != "" && updateOptions.Name != profile.Name && !state.accessTokenSourceGiven() {
		migrateAccessTokenOnRename(profile, updateOptions.VaultKey, profile.Name, updateOptions.Name)
	}

	// 5. resolve and store each of the three secrets: client secret, password, access token.
	// Password/access token additionally count as "given" when they came from --password-stdin/
	// --access-token-stdin, or (password only) an interactive prompt resolveUpdateSecretInput
	// already ran before this -- both leave the flag itself un-Changed, but updateOptions.Password/
	// AccessToken holds a freshly provided value all the same.
	clientID := resolveCredentialOwner(cmd, "client-id", profile.ClientID, updateOptions.ClientID)
	updateOptions.ClientSecret = storeCredentialIfChanged(state.clientSecret, "client secret", updateOptions.VaultKey, clientID, updateOptions.ClientSecret)

	user := resolveCredentialOwner(cmd, "user", profile.User, updateOptions.User)
	updateOptions.Password = storeCredentialIfChanged(state.user, "password", updateOptions.VaultKey, user, updateOptions.Password)

	name := resolveCredentialOwner(cmd, "name", profile.Name, updateOptions.Name)
	updateOptions.AccessToken = storeCredentialIfChanged(state.accessTokenSourceGiven(), "access token", updateOptions.VaultKey, name, updateOptions.AccessToken)

	return nil
}

// resolveUpdateSecretInput fills in updateOptions.Password from whichever secret source the
// command line asked for: --password-stdin, or (when --user was given with no password source)
// an interactive, no-echo terminal prompt. --access-token-stdin needs no such resolution here --
// it names the source directly and unambiguously, so applyStdinSecrets alone is enough. Unlike
// `profile create`, an update with no credential flags at all (e.g. `profile update foo
// --description ...`) never prompts: it is a legitimate, common update to fields that have nothing
// to do with credentials.
//
// It runs before Validate/WhatIf so the prompt/stdin read always happens, even under --dry-run;
// resolveProfileCredentials (the vault write) stays gated behind WhatIf as before.
func resolveUpdateSecretInput(cmd *cobra.Command) error {
	state := readSecretFlagState(cmd)
	if err := requireUserForPasswordSource(state); err != nil {
		return err
	}
	if err := applyStdinSecrets(cmd, state, &updateOptions.Password, &updateOptions.AccessToken); err != nil {
		return err
	}

	if state.user && !state.passwordSourceGiven() {
		secret, err := promptForSecret(cmd, updateOptions.User)
		if err != nil {
			return err
		}
		updateOptions.Password = secret
	}
	return nil
}

// applyUpdateOverrides copies the enum-flag-backed options onto the profile being updated
func applyUpdateOverrides() {
	if updateOptions.DefaultWorkspace.String() != "" {
		updateOptions.Profile.DefaultWorkspace = updateOptions.DefaultWorkspace.String()
	}
	if updateOptions.DefaultProject.String() != "" {
		updateOptions.Profile.DefaultProject = updateOptions.DefaultProject.String()
	}
	if updateOptions.OutputFormat.String() != "" {
		updateOptions.Profile.OutputFormat = updateOptions.OutputFormat.String()
	}
	if updateOptions.CloneProtocol.String() != "" {
		updateOptions.Profile.CloneProtocol = updateOptions.CloneProtocol.String()
	}
}

// moveCredentialsToVault moves every credential still stored in plain text on profile into the
// vault. The three secrets are independent -- a hand-edited config file can plausibly hold more
// than one of them at once, even though the create/update flags that set them are mutually
// exclusive -- so each is checked and moved on its own instead of stopping at the first match.
func moveCredentialsToVault(profile *Profile, vaultKey string) error {
	if profile.ClientSecret != "" {
		if err := profile.SetCredentialInVault(vaultKey, profile.ClientID, profile.ClientSecret); err != nil {
			return fmt.Errorf("failed to store client secret in the vault: %w", err)
		}
		lgr.Printf("[DEBUG] stored client secret in the vault for %s", profile.ClientID)
		profile.ClientSecret = ""
		profile.vault.clientSecret = false
		updateOptions.ClientSecret = ""
	}
	if profile.Password != "" {
		if err := profile.SetCredentialInVault(vaultKey, profile.User, profile.Password); err != nil {
			return fmt.Errorf("failed to store user password in the vault: %w", err)
		}
		lgr.Printf("[DEBUG] stored user password in the vault for %s", profile.User)
		profile.Password = ""
		profile.vault.password = false
		updateOptions.Password = ""
	}
	if profile.AccessToken != "" {
		if err := profile.SetCredentialInVault(vaultKey, profile.Name, profile.AccessToken); err != nil {
			return fmt.Errorf("failed to store access token in the vault: %w", err)
		}
		lgr.Printf("[DEBUG] stored access token in the vault for %s", profile.Name)
		profile.AccessToken = ""
		profile.vault.accessToken = false
		updateOptions.AccessToken = ""
	}
	return nil
}

// migrateAccessTokenOnRename moves the vault entry for a profile's access token (keyed by the
// profile's name) from oldName to newName, so a plain `profile update <name> --name <new>` does
// not strand it under a name the profile no longer has. A missing entry under oldName (no access
// token was ever stored in the vault for this profile) is not an error, just nothing to migrate.
func migrateAccessTokenOnRename(profile *Profile, vaultKey, oldName, newName string) {
	credential, err := profile.GetCredentialFromVault(vaultKey, oldName)
	if err != nil {
		return
	}
	if err := profile.SetCredentialInVault(vaultKey, newName, credential.Password); err != nil {
		lgr.Printf("[ERROR] failed to migrate access token in the vault from %s to %s: %v", oldName, newName, err)
		fmt.Fprintf(os.Stderr, "Failed to migrate the access token in the vault from %s to %s: %s\n", oldName, newName, err)
		return
	}
	if err := profile.DeleteCredentialFromVault(vaultKey, oldName); err != nil {
		lgr.Printf("[WARN] failed to delete the old access token in the vault for %s: %v", oldName, err)
	}
	lgr.Printf("[DEBUG] migrated access token in the vault from %s to %s", oldName, newName)
}

// storeCredentialIfChanged stores secret in the vault when changed is true (the caller's signal
// that a fresh value for this secret was given, by whichever of its flags/prompt applies) and the
// vault is in use; it returns the secret to keep in memory (cleared once stored successfully)
func storeCredentialIfChanged(changed bool, kind, vaultKey, username, secret string) string {
	if !changed || secret == "" || updateOptions.NoVault {
		return secret
	}
	if err := updateOptions.SetCredentialInVault(vaultKey, username, secret); err != nil {
		lgr.Printf("[ERROR] failed to store %s in the vault, it will be stored in plain text in the configuration file: %v", kind, err)
		fmt.Fprintf(os.Stderr, "Failed to store %s in the vault, it will be stored in plain text in the configuration file: %s\n", kind, err)
		return secret
	}
	lgr.Printf("[DEBUG] stored %s in the vault for %s", kind, username)
	return ""
}
