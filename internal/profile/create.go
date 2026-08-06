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

var createCmd = &cobra.Command{
	Use:     "create",
	Aliases: []string{"add", "new"},
	Short:   "create a profile",
	Args:    cobra.NoArgs,
	PreRunE: disableUnsupportedFlags,
	RunE:    createProcess,
}

var createOptions struct {
	Profile
	DefaultWorkspace string
	DefaultProject   string
	OutputFormat     *common.EnumFlag
	CloneProtocol    *common.EnumFlag
	NoVault          bool
}

func init() {
	Command.AddCommand(createCmd)

	createOptions.OutputFormat = common.NewEnumFlag("json", "yaml", "table", "csv", "tsv")
	createOptions.CloneProtocol = common.NewEnumFlag("+git", "https", "ssh")
	createCmd.Flags().StringVarP(&createOptions.Name, "name", "n", "", "Name of the profile")
	createCmd.Flags().StringVar(&createOptions.Description, "description", "", "Description of the profile")
	createCmd.Flags().BoolVar(&createOptions.Default, "default", false, "True if this is the default profile")
	if runtime.GOOS != "windows" {
		createCmd.Flags().StringVar(&createOptions.VaultKey, "vault-key", "bitbucket-cli", "Vault key to use for storing credentials. Default is bitbucket-cli. On Windows, the Windows Credential Manager will be used, On Linux and macOS, the system keychain will be used.")
	}
	createCmd.Flags().BoolVar(&createOptions.NoVault, "no-vault", false, "Do not store credentials in the vault. This will store them in plain text in the configuration file.")
	createCmd.Flags().StringVarP(&createOptions.User, "user", "u", "", "User's name of the profile")
	createCmd.Flags().StringVar(&createOptions.Password, "password", "", "Password of the profile")
	createCmd.Flags().Bool("password-stdin", false, "Read the password from stdin instead of --password, so it never appears in shell history.")
	createCmd.Flags().StringVar(&createOptions.ClientID, "client-id", "", "Client ID of the profile")
	createCmd.Flags().StringVar(&createOptions.ClientSecret, "client-secret", "", "Client Secret of the profile")
	createCmd.Flags().Bool("client-secret-stdin", false, "Read the OAuth2 client secret from stdin instead of --client-secret, so it never appears in shell history.")
	createCmd.Flags().Uint16Var(&createOptions.CallbackPort, "callback-port", 0, "Port to listen to for the Authorization Code Grant")
	createCmd.Flags().StringVar(&createOptions.AccessToken, "access-token", "", "Access Token of the profile")
	createCmd.Flags().Bool("access-token-stdin", false, "Read the access token from stdin instead of --access-token, so it never appears in shell history.")
	createCmd.Flags().StringVar(&createOptions.DefaultWorkspace, "default-workspace", "", "Default workspace of the profile")
	createCmd.Flags().StringVar(&createOptions.DefaultProject, "default-project", "", "Default project of the profile")
	createCmd.Flags().Var(createOptions.CloneProtocol, "clone-protocol", "Default protocol to use for cloning repositories. Default is git, can be https, git, or ssh")
	createCmd.Flags().StringVar(&createOptions.CloneUser, "clone-user", "", "Username to use when cloning repositories. Default is the username of the profile.")
	createCmd.Flags().StringVar(&createOptions.SshKeyFilename, "default-ssh-key-file", "", "Path to the SSH private key file to use when cloning repositories with the ssh protocol.")
	// Named "default-output", not "output": a local "output" flag would shadow the root
	// persistent -o/--output flag (local flags win over inherited ones of the same name),
	// breaking -o on this command and making Profile.Print read this flag's value instead of the
	// root one to decide how to render *this command's own* confirmation output.
	createCmd.Flags().Var(createOptions.OutputFormat, "default-output", "Default output format of the profile (json, yaml, table, csv, tsv).")
	createCmd.Flags().IntVar(&createOptions.DefaultPageLength, "default-page-length", 0, "Default number of items per page to retrieve from Bitbucket (Default: 50).")
	createCmd.Flags().Var(&createOptions.ErrorProcessing, "error-processing", "Error processing (StopOnError, WarnOnError, IgnoreErrors).")
	createCmd.Flags().BoolVar(&createOptions.Progress, "progress", false, "Show progress during upload/download operations.")
	_ = createCmd.MarkFlagRequired("name")
	_ = createCmd.MarkFlagFilename("default-ssh-key-file")
	// "client-id" is deliberately not required-together with "client-secret" here: cobra's
	// MarkFlagsRequiredTogether only recognizes one fixed pair, which would force the client
	// secret onto the command line even though --client-secret-stdin exists precisely so it need
	// not be. requireClientIDForSecretSource enforces the same pairing, accepting either secret
	// source.
	//
	// "user" is deliberately not required-together with "password"/"password-stdin" here: --user
	// given alone is valid and triggers an interactive, no-echo password prompt instead (see
	// resolveCreateSecretInput). requireUserForPasswordSource enforces the other direction: a
	// password source given without --user is still rejected.
	createCmd.MarkFlagsMutuallyExclusive("user", "client-id", "access-token", "access-token-stdin")
	createCmd.MarkFlagsMutuallyExclusive("password", "password-stdin")
	createCmd.MarkFlagsMutuallyExclusive("access-token", "access-token-stdin")
	createCmd.MarkFlagsMutuallyExclusive("client-secret", "client-secret-stdin")
	createCmd.MarkFlagsMutuallyExclusive("password-stdin", "access-token-stdin")
	createCmd.MarkFlagsMutuallyExclusive("password-stdin", "client-secret-stdin")
	createCmd.MarkFlagsMutuallyExclusive("access-token-stdin", "client-secret-stdin")
	if runtime.GOOS != "windows" {
		createCmd.MarkFlagsMutuallyExclusive("vault-key", "no-vault")
	}
	_ = createCmd.RegisterFlagCompletionFunc(createOptions.CloneProtocol.CompletionFunc("clone-protocol"))
	_ = createCmd.RegisterFlagCompletionFunc(createOptions.OutputFormat.CompletionFunc("default-output"))
	_ = createCmd.RegisterFlagCompletionFunc("error-processing", createOptions.ErrorProcessing.CompletionFunc())
	createCmd.SetHelpFunc(hideUnsupportedFlags)
}

func createProcess(cmd *cobra.Command, args []string) (err error) {
	ctx := cmd.Context()

	_, err = GetProfileFromCommand(ctx, cmd)
	if err != nil && !errors.Is(err, ErrNoProfiles) {
		return err
	}

	applyCreateOverrides()

	lgr.Printf("[DEBUG] creating profile %s", createOptions.Name)
	err = createOptions.Validate()
	if err != nil {
		return err
	}
	if _, found := Profiles.Find(createOptions.Name); found {
		return fmt.Errorf("profile %s already exists", createOptions.Name)
	}

	// Resolved after the duplicate-name/Validate checks above (so a doomed `profile create`
	// never prompts/reads stdin for a secret only to then fail on an unrelated problem) but still
	// before WhatIf, so --dry-run still prompts/reads stdin for the secret: a dry run needs the
	// value structurally to validate the command line, it only skips the vault write and the
	// actual profile creation below.
	if secretErr := resolveCreateSecretInput(cmd); secretErr != nil {
		return secretErr
	}

	if !common.WhatIf(cmd, "Creating profile %s", createOptions.Name) {
		return nil
	}

	if common.IsWSL() {
		// For now, we do not support vaults in WSL.
		lgr.Printf("[WARN] vaults are not supported in WSL, the credentials will be stored in plain text in the configuration file")
		createOptions.NoVault = true
	}

	err = resolveCreateSecrets()
	if err != nil {
		return err
	}

	Profiles.Add(&createOptions.Profile)
	return saveProfilesConfig()
}

// applyCreateOverrides copies the enum-flag-backed options onto the profile being created
func applyCreateOverrides() {
	if createOptions.DefaultWorkspace != "" {
		createOptions.Profile.DefaultWorkspace = createOptions.DefaultWorkspace
	}
	if createOptions.DefaultProject != "" {
		createOptions.Profile.DefaultProject = createOptions.DefaultProject
	}
	if createOptions.OutputFormat.String() != "" {
		createOptions.Profile.OutputFormat = createOptions.OutputFormat.String()
	}
	if createOptions.CloneProtocol.String() != "" {
		createOptions.Profile.CloneProtocol = createOptions.CloneProtocol.String()
	}
}

// resolveCreateSecretInput fills in createOptions.Password/AccessToken/ClientSecret from
// whichever secret source the command line asked for: --password-stdin/--access-token-stdin/
// --client-secret-stdin, or (when --user was given with no password source, and the vault does
// not already hold one for that user) an interactive, no-echo terminal prompt. It runs after the
// duplicate-name/Validate checks but still before WhatIf, so the prompt/stdin read always happens
// (even under --dry-run) but never for a command line already known to fail; resolveCreateSecrets
// (the vault write) stays gated behind WhatIf as before.
//
// Deliberately does NOT prompt when no credential flag of any kind was given at all: a profile
// created with zero credentials is a legitimate, supported flow (e.g. vault-preseeded/CI
// provisioning, where the secret is stored in the vault by another tool and resolved on demand
// the first time the profile is actually used, or a later `profile update` fills it in) --
// resolveCreateSecrets below already validates that a --no-vault profile cannot be saved with no
// secret, and LoadSecrets reports a clean error at first use otherwise.
func resolveCreateSecretInput(cmd *cobra.Command) error {
	state := readSecretFlagState(cmd)
	if err := requireUserForPasswordSource(state, false); err != nil {
		return err
	}
	if err := requireClientIDForSecretSource(state); err != nil {
		return err
	}
	if err := applyStdinSecrets(cmd, state, &createOptions.Password, &createOptions.AccessToken, &createOptions.ClientSecret); err != nil {
		return err
	}

	if state.user && !state.passwordSourceGiven() && !createOptions.NoVault && hasVaultCredential(createOptions.VaultKey, createOptions.User) {
		// The vault already holds a password for this user (e.g. reused from an earlier
		// profile): resolveCreateSecrets' own vault branch loads it below, so prompting here
		// would only overwrite that entry with a freshly typed value nobody asked to change.
		lgr.Printf("[DEBUG] password for %s already in the vault, skipping the interactive prompt", createOptions.User)
		return nil
	}
	if state.user && !state.passwordSourceGiven() {
		secret, err := promptForSecret(cmd, createOptions.User)
		if err != nil {
			return err
		}
		createOptions.Password = secret
	}
	return nil
}

// hasVaultCredential reports whether the vault already holds a credential for username under
// vaultKey, without ever creating or modifying an entry.
func hasVaultCredential(vaultKey, username string) bool {
	if username == "" {
		return false
	}
	_, err := createOptions.GetCredentialFromVault(vaultKey, username)
	return err == nil
}

// resolveCreateSecrets stores the client secret/password/access token in the vault if provided,
// or validates that they are set in the profile when the vault is not used
func resolveCreateSecrets() error {
	if createOptions.NoVault {
		switch {
		case createOptions.ClientID != "" && createOptions.ClientSecret == "":
			return errors.New("argument clientSecret is missing: a client secret is required when using a client ID since it is not stored in the vault")
		case createOptions.User != "" && createOptions.Password == "":
			return errors.New("argument password is missing: a password is required when using a user since it is not stored in the vault")
		case createOptions.ClientID == "" && createOptions.User == "" && createOptions.AccessToken == "":
			return errors.New("argument accessToken is missing: an access token is required when no client ID or user is given since it is not stored in the vault")
		}
		return nil
	}

	switch {
	case createOptions.ClientID != "":
		secret, fromVault, err := resolveVaultSecret("client secret", createOptions.VaultKey, createOptions.ClientID, createOptions.ClientSecret)
		if err != nil {
			return errors.New("a client secret is required when using a client ID since it is not stored in the vault. Please provide it with --client-secret or store it in the vault with the command")
		}
		createOptions.ClientSecret = secret
		if fromVault {
			createOptions.vault.clientSecret = true // must never be written back to the config file in plain text
		}
	case createOptions.User != "":
		secret, fromVault, err := resolveVaultSecret("password", createOptions.VaultKey, createOptions.User, createOptions.Password)
		if err != nil {
			return errors.New("a password is required when using a user since it is not stored in the vault. Please provide it with --password or store it in the vault with the command")
		}
		createOptions.Password = secret
		if fromVault {
			createOptions.vault.password = true // must never be written back to the config file in plain text
		}
	case createOptions.AccessToken != "":
		secret, fromVault, err := resolveVaultSecret("access token", createOptions.VaultKey, createOptions.Name, createOptions.AccessToken)
		if err != nil {
			return err
		}
		createOptions.AccessToken = secret
		if fromVault {
			createOptions.vault.accessToken = true // must never be written back to the config file in plain text
		}
	}
	return nil
}

// resolveVaultSecret stores secret in the vault when provided, or loads it from the vault when not.
//
// It returns the secret to keep in the profile in memory: cleared when successfully stored in the
// vault, unchanged when the store failed (so it falls back to being saved in plain text), or the
// value loaded from the vault -- along with fromVault, reporting whether the returned value came
// from the vault (mirroring getSecretOrFromVault). The caller must set the matching
// createOptions.vault.* bit when fromVault is true, or Profile.forSave has no way to tell the
// loaded secret apart from one the user typed in plain text, and will persist it verbatim.
func resolveVaultSecret(kind, vaultKey, username, secret string) (value string, fromVault bool, err error) {
	if secret != "" {
		if setErr := createOptions.SetCredentialInVault(vaultKey, username, secret); setErr != nil {
			lgr.Printf("[ERROR] failed to store %s in the %s vault, the secret will be stored in plain text in the configuration file: %v", kind, vaultKey, setErr)
			fmt.Fprintf(os.Stderr, "Failed to store %s in the %s vault, the secret will be stored in plain text in the configuration file: %s\n", kind, vaultKey, setErr)
			return secret, false, nil
		}
		lgr.Printf("[DEBUG] stored %s in the %s vault for %s", kind, vaultKey, username)
		return "", false, nil
	}
	credential, err := createOptions.GetCredentialFromVault(vaultKey, username)
	if err != nil {
		return "", false, err
	}
	return credential.Password, true, nil
}
