package profile

import (
	"context"
	"fmt"
	"os"
	"path/filepath"

	"github.com/gildas/bitbucket-cli/cmd/common"
	"github.com/gildas/go-logger"
	"github.com/spf13/cobra"
	"github.com/spf13/viper"
)

// Profiles is a collection of Profile
type profiles []*Profile

// Profiles is the collection of profiles
var Profiles profiles

// Current gets the current profile
func (profiles profiles) Current(context context.Context) *Profile {
	log := logger.Must(logger.FromContext(context)).Child("profile", "current")

	if profile := profiles.profileFromGitConfig(context, log); profile != nil {
		return profile
	}

	log.Debugf("No profile found in git config, looking for default profile in %d profiles", len(profiles))
	for _, profile := range profiles {
		if profile.Default {
			log.Infof("Using default profile %s", profile.Name)
			return profile
		}
	}
	if len(profiles) > 0 {
		log.Infof("Using first profile %s", profiles[0].Name)
		return profiles[0]
	}
	log.Warnf("No profile found")
	return nil
}

// profileFromGitConfig looks up the profile named in the git config's bitbucket "cli" section,
// returning nil when no git config, no such section, no profile name, or no matching profile is found
func (profiles profiles) profileFromGitConfig(context context.Context, log *logger.Logger) *Profile {
	gitConfig, err := common.OpenGitConfig(context)
	if err != nil {
		return nil
	}
	log.Debugf("Found a git config file")

	section, err := common.GetGitSection(context, gitConfig, `bitbucket "cli"`)
	if err != nil {
		return nil
	}
	log.Debugf("Found a bitbucket \"cli\" section in git config: name=%s", section.Name())

	profileName := section.Key("profile").String()
	if profileName == "" {
		return nil
	}
	log.Debugf("Found a profile in git config: %s", profileName)

	profile, found := profiles.Find(profileName)
	if found {
		log.Infof("Using profile %s from git config", profileName)
		return profile
	}
	log.Warnf("Profile %s not found in %s", profileName, viper.ConfigFileUsed())
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
func (profiles *profiles) Load(ctx context.Context, cmd *cobra.Command) error {
	log := logger.Must(logger.FromContext(ctx)).Child("profiles", "load")

	if len(*profiles) > 0 {
		return nil
	}

	if len(viper.AllKeys()) == 0 {
		if err := common.Initialize(ctx, cmd); err != nil {
			return err
		}
	}

	log.Infof("Loading profiles from %s", viper.ConfigFileUsed())
	if err := viper.UnmarshalKey("profiles", &profiles); err != nil {
		return err
	}
	log.Debugf("Loaded %d profiles", len(*profiles))
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
func saveProfilesConfig(log *logger.Logger) error {
	viper.Set("profiles", Profiles)
	if viper.ConfigFileUsed() != "" {
		log.Infof("Writing configuration to %s", viper.ConfigFileUsed())
		return viper.WriteConfig()
	}
	if configDir, _ := os.UserConfigDir(); configDir != "" {
		configPath := filepath.Join(configDir, "bitbucket")
		if err := os.MkdirAll(configPath, 0o750); err != nil {
			return err
		}
		configFile := filepath.Join(configPath, "config-cli.yml")
		if err := viper.WriteConfigAs(configFile); err != nil {
			return err
		}
		if info, err := os.Stat(configFile); err == nil && info.Mode() != 0o600 {
			return os.Chmod(configFile, 0o600)
		}
	}
	homeDir, err := os.UserHomeDir()
	if err != nil {
		return err
	}
	return viper.WriteConfigAs(filepath.Join(homeDir, ".bitbucket-cli"))
}
