package profile

import (
	"fmt"
	"os"
	"runtime"

	"github.com/gildas/bitbucket-cli/cmd/common"
	"github.com/gildas/go-errors"
	"github.com/gildas/go-flags"
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
	OutputFormat     *flags.EnumFlag
	CloneProtocol    *flags.EnumFlag
	NoVault          bool
}

func init() {
	Command.AddCommand(createCmd)

	createOptions.OutputFormat = flags.NewEnumFlag("json", "yaml", "table")
	createOptions.CloneProtocol = flags.NewEnumFlag("+git", "https", "ssh")
	createCmd.Flags().StringVarP(&createOptions.Name, "name", "n", "", "Name of the profile")
	createCmd.Flags().StringVar(&createOptions.Description, "description", "", "Description of the profile")
	createCmd.Flags().BoolVar(&createOptions.Default, "default", false, "True if this is the default profile")
	if runtime.GOOS != "windows" {
		createCmd.Flags().StringVar(&createOptions.VaultKey, "vault-key", "bitbucket-cli", "Vault key to use for storing credentials. Default is bitbucket-cli. On Windows, the Windows Credential Manager will be used, On Linux and macOS, the system keychain will be used.")
	}
	createCmd.Flags().BoolVar(&createOptions.NoVault, "no-vault", false, "Do not store credentials in the vault. This will store them in plain text in the configuration file.")
	createCmd.Flags().StringVarP(&createOptions.User, "user", "u", "", "User's name of the profile")
	createCmd.Flags().StringVar(&createOptions.Password, "password", "", "Password of the profile")
	createCmd.Flags().StringVar(&createOptions.ClientID, "client-id", "", "Client ID of the profile")
	createCmd.Flags().StringVar(&createOptions.ClientSecret, "client-secret", "", "Client Secret of the profile")
	createCmd.Flags().Uint16Var(&createOptions.CallbackPort, "callback-port", 0, "Port to listen to for the Authorization Code Grant")
	createCmd.Flags().StringVar(&createOptions.AccessToken, "access-token", "", "Access Token of the profile")
	createCmd.Flags().StringVar(&createOptions.DefaultWorkspace, "default-workspace", "", "Default workspace of the profile")
	createCmd.Flags().StringVar(&createOptions.DefaultProject, "default-project", "", "Default project of the profile")
	createCmd.Flags().Var(createOptions.CloneProtocol, "clone-protocol", "Default protocol to use for cloning repositories. Default is git, can be https, git, or ssh")
	createCmd.Flags().StringVar(&createOptions.CloneUser, "clone-user", "", "Username to use when cloning repositories. Default is the username of the profile.")
	createCmd.Flags().StringVar(&createOptions.SshKeyFilename, "default-ssh-key-file", "", "Path to the SSH private key file to use when cloning repositories with the ssh protocol.")
	createCmd.Flags().Var(createOptions.OutputFormat, "output", "Output format (json, yaml, table).")
	createCmd.Flags().IntVar(&createOptions.DefaultPageLength, "default-page-length", 0, "Default number of items per page to retrieve from Bitbucket (Default: 50).")
	createCmd.Flags().Var(&createOptions.ErrorProcessing, "error-processing", "Error processing (StopOnError, WanOnError, IgnoreErrors).")
	createCmd.Flags().BoolVar(&createOptions.Progress, "progress", false, "Show progress during upload/download operations.")
	_ = createCmd.MarkFlagRequired("name")
	_ = createCmd.MarkFlagFilename("default-ssh-key-file")
	createCmd.MarkFlagsRequiredTogether("user", "password")
	createCmd.MarkFlagsRequiredTogether("client-id", "client-secret")
	createCmd.MarkFlagsMutuallyExclusive("user", "client-id", "access-token")
	if runtime.GOOS != "windows" {
		createCmd.MarkFlagsMutuallyExclusive("vault-key", "no-vault")
	}
	_ = createCmd.RegisterFlagCompletionFunc(createOptions.CloneProtocol.CompletionFunc("clone-protocol"))
	_ = createCmd.RegisterFlagCompletionFunc(createOptions.OutputFormat.CompletionFunc("output"))
	_ = createCmd.RegisterFlagCompletionFunc("error-processing", createOptions.ErrorProcessing.CompletionFunc())
	createCmd.SetHelpFunc(hideUnsupportedFlags)
}

func createProcess(cmd *cobra.Command, args []string) (err error) {
	ctx := cmd.Context()

	_, err = GetProfileFromCommand(ctx, cmd)
	if err != nil && !errors.Is(err, errors.Empty) {
		return err
	}

	applyCreateOverrides()

	lgr.Printf("[DEBUG] creating profile %s", createOptions.Name)
	err = createOptions.Validate()
	if err != nil {
		return err
	}
	if _, found := Profiles.Find(createOptions.Name); found {
		return errors.DuplicateFound.With("name", createOptions.Name)
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

// resolveCreateSecrets stores the client secret/password/access token in the vault if provided,
// or validates that they are set in the profile when the vault is not used
func resolveCreateSecrets() error {
	if createOptions.NoVault {
		switch {
		case createOptions.ClientID != "" && createOptions.ClientSecret == "":
			return errors.ArgumentMissing.With("clientSecret", "A client secret is required when using a client ID since it is not stored in the vault.")
		case createOptions.User != "" && createOptions.Password == "":
			return errors.ArgumentMissing.With("password", "A password is required when using a user since it is not stored in the vault.")
		case createOptions.ClientID == "" && createOptions.User == "" && createOptions.AccessToken == "":
			return errors.ArgumentMissing.With("accessToken", "An access token is required when using a user since it is not stored in the vault")
		}
		return nil
	}

	switch {
	case createOptions.ClientID != "":
		secret, err := resolveVaultSecret("client secret", createOptions.VaultKey, createOptions.ClientID, createOptions.ClientSecret, createOptions.SetCredentialInVault, createOptions.GetCredentialFromVault)
		if err != nil {
			return errors.New("A client secret is required when using a client ID since it is not stored in the vault. Please provide it with --client-secret or store it in the vault with the command")
		}
		createOptions.ClientSecret = secret
	case createOptions.User != "":
		secret, err := resolveVaultSecret("user password", createOptions.VaultKey, createOptions.User, createOptions.Password, createOptions.SetCredentialInVault, createOptions.GetCredentialFromVault)
		if err != nil {
			return errors.New("A password is required when using a user since it is not stored in the vault. Please provide it with --password or store it in the vault with the command")
		}
		createOptions.Password = secret
	case createOptions.AccessToken != "":
		secret, err := resolveVaultSecret("access token", createOptions.VaultKey, createOptions.Name, createOptions.AccessToken, createOptions.SetCredentialInVault, createOptions.GetCredentialFromVault)
		if err != nil {
			return err
		}
		createOptions.AccessToken = secret
	}
	return nil
}

// resolveVaultSecret stores secret in the vault when provided, or loads it from the vault when not.
//
// It returns the secret to keep in the profile in memory: cleared when successfully stored in the
// vault, unchanged when the store failed (so it falls back to being saved in plain text), or the
// value loaded from the vault.
func resolveVaultSecret(kind, vaultKey, username, secret string, set func(vaultKey, username, secret string) error, get func(vaultKey, username string) (*Credential, error)) (string, error) {
	if secret != "" {
		if err := set(vaultKey, username, secret); err != nil {
			lgr.Printf("[ERROR] failed to store %s in the %s vault, the secret will be stored in plain text in the configuration file: %v", kind, vaultKey, err)
			fmt.Fprintf(os.Stderr, "Failed to store %s in the %s vault, the secret will be stored in plain text in the configuration file: %s\n", kind, vaultKey, err)
			return secret, nil
		}
		lgr.Printf("[DEBUG] stored %s in the %s vault for %s", kind, vaultKey, username)
		return "", nil
	}
	credential, err := get(vaultKey, username)
	if err != nil {
		return "", err
	}
	return credential.Password, nil
}
