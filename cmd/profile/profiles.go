package profile

import (
	"context"
	"fmt"
	"os"
	"path/filepath"

	"github.com/gildas/bitbucket-cli/cmd/common"
	"github.com/go-pkgz/lgr"
	"github.com/spf13/cobra"
	"github.com/spf13/viper"
)

// Profiles is a collection of Profile
type profiles []*Profile

// Profiles is the collection of profiles
var Profiles profiles

// Current gets the current profile
func (profiles profiles) Current(context context.Context) *Profile {
	if profile := profiles.profileFromGitConfig(context); profile != nil {
		return profile
	}

	lgr.Printf("[DEBUG] no profile found in git config, looking for default profile in %d profiles", len(profiles))
	for _, profile := range profiles {
		if profile.Default {
			lgr.Printf("[DEBUG] using default profile %s", profile.Name)
			return profile
		}
	}
	if len(profiles) > 0 {
		lgr.Printf("[DEBUG] using first profile %s", profiles[0].Name)
		return profiles[0]
	}
	lgr.Printf("[WARN] no profile found")
	return nil
}

// profileFromGitConfig looks up the profile named in the git config's bitbucket "cli" section,
// returning nil when no git config, no such section, no profile name, or no matching profile is found
func (profiles profiles) profileFromGitConfig(context context.Context) *Profile {
	gitConfig, err := common.OpenGitConfig(context)
	if err != nil {
		return nil
	}
	lgr.Printf("[DEBUG] found a git config file")

	section, err := common.GetGitSection(context, gitConfig, `bitbucket "cli"`)
	if err != nil {
		return nil
	}
	lgr.Printf("[DEBUG] found a bitbucket \"cli\" section in git config: name=%s", section.Name())

	profileName := section.Key("profile").String()
	if profileName == "" {
		return nil
	}
	lgr.Printf("[DEBUG] found a profile in git config: %s", profileName)

	profile, found := profiles.Find(profileName)
	if found {
		lgr.Printf("[DEBUG] using profile %s from git config", profileName)
		return profile
	}
	lgr.Printf("[WARN] profile %s not found in %s", profileName, viper.ConfigFileUsed())
	fmt.Fprintf(os.Stderr, "Profile %s from your git config was not found in %s, ignored.\n", profileName, viper.ConfigFileUsed())
	return nil
}

// Names gets the names of the profiles
func (profiles profiles) Names() []string {
	names := make([]string, 0, len(profiles))
	for _, profile := range profiles {
		names = append(names, profile.Name)
	}
	return names
}

// GetHeaders gets the header for a table
//
// implements common.Tableables
func (profiles profiles) GetHeaders(cmd *cobra.Command) []string {
	return Profile{}.GetHeaders(cmd)
}

// GetRowAt gets the row for a table
//
// implements common.Tableables
func (profiles profiles) GetRowAt(index int, headers []string) []string {
	if index < 0 || index >= len(profiles) {
		return []string{}
	}
	return profiles[index].GetRow(headers)
}

// Size gets the number of elements
//
// implements common.Tableables
func (profiles profiles) Size() int {
	return len(profiles)
}

// Find finds a profile by name
func (profiles profiles) Find(name string) (profile *Profile, found bool) {
	for _, profile = range profiles {
		if profile.Name == name {
			return profile, true
		}
	}
	return nil, false
}

// Add adds a profile to the collection
func (profiles *profiles) Add(profile *Profile) {
	*profiles = append(*profiles, profile)
	if profile.Default {
		profiles.SetCurrent(profile.Name)
	}
}

// Delete deletes one or more profiles by their names
func (profiles *profiles) Delete(names ...string) (deleted int) {
	for _, name := range names {
		for i, profile := range *profiles {
			if profile.Name == name {
				deleted++
				*profiles = append((*profiles)[:i], (*profiles)[i+1:]...)
				break
			}
		}
	}
	return deleted
}

// SetCurrent sets the current profile
func (profiles profiles) SetCurrent(name string) {
	if name == "" {
		return
	}
	if _, found := profiles.Find(name); !found {
		return
	}
	for _, profile := range profiles {
		if profile.Name == name {
			profile.Default = true
		} else {
			profile.Default = false
		}
	}
}

// Load loads the profiles from a viper key
func (profiles *profiles) Load(_ context.Context, cmd *cobra.Command) error {
	if len(*profiles) > 0 {
		return nil
	}

	if len(viper.AllKeys()) == 0 {
		if err := common.Initialize(cmd); err != nil {
			return fmt.Errorf("cannot initialize: %w", err)
		}
	}

	lgr.Printf("[DEBUG] loading profiles from %s", viper.ConfigFileUsed())
	if err := viper.UnmarshalKey("profiles", &profiles); err != nil {
		return fmt.Errorf("cannot read config file: %w", err)
	}
	lgr.Printf("[DEBUG] loaded %d profiles", len(*profiles))
	return nil
}

// ValidProfileNames gets the valid profile names
func ValidProfileNames(cmd *cobra.Command, args []string, toComplete string) ([]string, cobra.ShellCompDirective) {
	if len(args) != 0 {
		return nil, cobra.ShellCompDirectiveNoFileComp
	}

	if _, err := GetProfileFromCommand(cmd.Context(), cmd); err != nil {
		return []string{}, cobra.ShellCompDirectiveNoFileComp
	}

	names := Profiles.Names()
	return common.FilterValidArgs(names, args, toComplete), cobra.ShellCompDirectiveNoFileComp
}

// saveProfilesConfig persists the in-memory Profiles collection to the active config file
func saveProfilesConfig() error {
	viper.Set("profiles", Profiles)
	if viper.ConfigFileUsed() != "" {
		lgr.Printf("[DEBUG] writing configuration to %s", viper.ConfigFileUsed())
		if err := viper.WriteConfig(); err != nil {
			return fmt.Errorf("cannot write config file: %w", err)
		}
		return nil
	}
	if configDir, _ := os.UserConfigDir(); configDir != "" {
		return writeProfilesConfigToDir(configDir)
	}

	homeDir, err := os.UserHomeDir()
	if err != nil {
		return fmt.Errorf("cannot determine home directory: %w", err)
	}
	if err := viper.WriteConfigAs(filepath.Join(homeDir, ".bitbucket-cli")); err != nil {
		return fmt.Errorf("cannot write config file: %w", err)
	}
	return nil
}

// writeProfilesConfigToDir writes the profiles config file into configDir/bitbucket/config-cli.yml,
// creating the directory as needed and restricting the file to 0600 once written
func writeProfilesConfigToDir(configDir string) error {
	configPath := filepath.Join(configDir, "bitbucket")
	if err := os.MkdirAll(configPath, 0o750); err != nil {
		return fmt.Errorf("cannot create config directory: %w", err)
	}
	configFile := filepath.Join(configPath, "config-cli.yml")
	if err := viper.WriteConfigAs(configFile); err != nil {
		return fmt.Errorf("cannot write config file: %w", err)
	}
	if info, err := os.Stat(configFile); err == nil && info.Mode() != 0o600 {
		if err := os.Chmod(configFile, 0o600); err != nil {
			return fmt.Errorf("cannot set config file permissions: %w", err)
		}
	}
	return nil
}
