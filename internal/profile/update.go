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
	updateOptions.OutputFormat = common.NewEnumFlag("json", "yaml", "table")
	updateOptions.CloneProtocol = common.NewEnumFlag("+git", "https", "ssh")
	updateCmd.Flags().StringVarP(&updateOptions.Name, "name", "n", "", "Name of the profile")
	updateCmd.Flags().StringVar(&updateOptions.Description, "description", "", "Description of the profile")
	updateCmd.Flags().BoolVar(&updateOptions.Default, "default", false, "True if this is the default profile")
	if runtime.GOOS != "windows" {
		updateCmd.Flags().StringVar(&updateOptions.VaultKey, "vault-key", "bitbucket-cli", "Vault key to use for storing credentials. Default is bitbucket-cli. On Windows, the Windows Credential Manager will be used, On Linux and macOS, the system keychain will be used.")
	}
	updateCmd.Flags().StringVarP(&updateOptions.User, "user", "u", "", "User's name of the profile")
	updateCmd.Flags().StringVar(&updateOptions.Password, "password", "", "Password of the profile")
	updateCmd.Flags().StringVar(&updateOptions.ClientID, "client-id", "", "Client ID of the profile")
	updateCmd.Flags().StringVar(&updateOptions.ClientSecret, "client-secret", "", "Client Secret of the profile")
	updateCmd.Flags().Uint16Var(&updateOptions.CallbackPort, "callback-port", 0, "Callback port to use for OAuth2 authentication. If not set, a random port will be used.")
	updateCmd.Flags().StringVar(&updateOptions.AccessToken, "access-token", "", "Access Token of the profile")
	updateCmd.Flags().BoolVar(&updateOptions.ToVault, "to-vault", false, "Store credentials in the vault. This will remove any credentials from the profile and store them in the vault. If the vault key is not provided, it will use the existing vault key of the profile or the default vault key if not set.")
	updateCmd.Flags().BoolVar(&updateOptions.NoVault, "no-vault", false, "Do not use a vault for storing credentials")
	updateCmd.Flags().Var(updateOptions.DefaultWorkspace, "default-workspace", "Default workspace of the profile")
	updateCmd.Flags().Var(updateOptions.DefaultProject, "default-project", "Default project of the profile")
	updateCmd.Flags().Var(updateOptions.CloneProtocol, "clone-protocol", "Default protocol to use for cloning repositories. Default is git, can be https, git, or ssh")
	updateCmd.Flags().StringVar(&updateOptions.CloneUser, "clone-user", "", "Username to use when cloning repositories. Default is the username of the profile.")
	updateCmd.Flags().StringVar(&updateOptions.SshKeyFilename, "default-ssh-key-file", "", "Path to the SSH private key file to use when cloning repositories with the ssh protocol.")
	updateCmd.Flags().Var(updateOptions.OutputFormat, "output", "Output format (json, yaml, table).")
	updateCmd.Flags().IntVar(&updateOptions.DefaultPageLength, "default-page-length", 0, "Default number of items per page to retrieve from Bitbucket (Default: 50).")
	updateCmd.Flags().Var(&updateOptions.ErrorProcessing, "error-processing", "Error processing (StopOnError, WanOnError, IgnoreErrors).")
	updateCmd.Flags().BoolVar(&updateOptions.Progress, "progress", false, "Show progress during upload/download operations.")
	updateCmd.MarkFlagsRequiredTogether("user", "password")
	updateCmd.MarkFlagsRequiredTogether("client-id", "client-secret")
	updateCmd.MarkFlagsMutuallyExclusive("user", "client-id", "access-token")
	updateCmd.MarkFlagsMutuallyExclusive("to-vault", "no-vault")
	updateCmd.MarkFlagsMutuallyExclusive("to-vault", "access-token")
	updateCmd.MarkFlagsMutuallyExclusive("to-vault", "client-id")
	updateCmd.MarkFlagsMutuallyExclusive("to-vault", "client-secret")
	updateCmd.MarkFlagsMutuallyExclusive("to-vault", "user")
	updateCmd.MarkFlagsMutuallyExclusive("to-vault", "password")
	_ = updateCmd.MarkFlagFilename("default-ssh-key-file")
	_ = updateCmd.RegisterFlagCompletionFunc(updateOptions.DefaultWorkspace.CompletionFunc("default-workspace"))
	_ = updateCmd.RegisterFlagCompletionFunc(updateOptions.DefaultProject.CompletionFunc("default-project"))
	_ = updateCmd.RegisterFlagCompletionFunc(updateOptions.CloneProtocol.CompletionFunc("clone-protocol"))
	_ = updateCmd.RegisterFlagCompletionFunc(updateOptions.OutputFormat.CompletionFunc("output"))
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
	if updateOptions.Default {
		Profiles.SetCurrent(profile.Name)
	}
	// profile.Redact() masks secrets before they hit the debug log
	lgr.Printf("[DEBUG] updated profile %s: %+v", profile.Name, profile.Redact())

	if err := saveProfilesConfig(); err != nil {
		return err
	}
	return profile.Print(ctx, cmd, profile)
}

// resolveProfileCredentials moves existing plain-text credentials into the vault when requested,
// then stores any credential values changed on the command line into the vault when in use
func resolveProfileCredentials(cmd *cobra.Command, profile *Profile) error {
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

	if profile.AccessToken != "" || profile.ClientSecret != "" || profile.Password != "" {
		lgr.Printf("[DEBUG] profile %s stored its credentials in plain text, we should keep it that way", profile.Name)
		updateOptions.NoVault = true
	}

	// We need to check updates to the vault key early, so we can store the client secret and password in the vault if provided
	if runtime.GOOS != "windows" && !cmd.Flag("vault-key").Changed {
		if profile.VaultKey == "" {
			profile.VaultKey = "bitbucket-cli"
		}
		updateOptions.VaultKey = profile.VaultKey
	}

	clientID := profile.ClientID
	if cmd.Flag("client-id").Changed && updateOptions.ClientID != "" {
		clientID = updateOptions.ClientID
	}
	updateOptions.ClientSecret = storeCredentialIfChanged(cmd, "client-secret", "client secret", updateOptions.VaultKey, clientID, updateOptions.ClientSecret)

	user := profile.User
	if cmd.Flag("user").Changed && updateOptions.User != "" {
		user = updateOptions.User
	}
	updateOptions.Password = storeCredentialIfChanged(cmd, "password", "user password", updateOptions.VaultKey, user, updateOptions.Password)

	name := profile.Name
	if cmd.Flag("name").Changed && updateOptions.Name != "" {
		name = updateOptions.Name
	}
	updateOptions.AccessToken = storeCredentialIfChanged(cmd, "access-token", "access token", updateOptions.VaultKey, name, updateOptions.AccessToken)

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

// moveCredentialsToVault moves any credential still stored in plain text on profile into the vault
func moveCredentialsToVault(profile *Profile, vaultKey string) error {
	switch {
	case profile.ClientSecret != "":
		if err := profile.SetCredentialInVault(vaultKey, profile.ClientID, profile.ClientSecret); err != nil {
			return fmt.Errorf("failed to store client secret in the vault: %w", err)
		}
		lgr.Printf("[DEBUG] stored client secret in the vault for %s", profile.ClientID)
		profile.ClientSecret = ""
		updateOptions.ClientSecret = ""
	case profile.Password != "":
		if err := profile.SetCredentialInVault(vaultKey, profile.User, profile.Password); err != nil {
			return fmt.Errorf("failed to store user password in the vault: %w", err)
		}
		lgr.Printf("[DEBUG] stored user password in the vault for %s", profile.User)
		profile.Password = ""
		updateOptions.Password = ""
	case profile.AccessToken != "":
		if err := profile.SetCredentialInVault(vaultKey, profile.Name, profile.AccessToken); err != nil {
			return fmt.Errorf("failed to store access token in the vault: %w", err)
		}
		lgr.Printf("[DEBUG] stored access token in the vault for %s", profile.Name)
		profile.AccessToken = ""
		updateOptions.AccessToken = ""
	}
	return nil
}

// storeCredentialIfChanged stores secret in the vault when flagName changed on the command line and
// the vault is in use; it returns the secret to keep in memory (cleared once stored successfully)
func storeCredentialIfChanged(cmd *cobra.Command, flagName, kind, vaultKey, username, secret string) string {
	if !cmd.Flag(flagName).Changed || secret == "" || updateOptions.NoVault {
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
