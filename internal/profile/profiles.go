package profile

import (
	"context"
	"errors"
	"fmt"
	"os"

	"github.com/avitsrimer/bitbucket-cli/internal/common"
	"github.com/go-pkgz/lgr"
	"github.com/spf13/cobra"
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
	defer func() { _ = gitConfig.Close() }()
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
	configPath := ""
	if config := common.CurrentConfig(); config != nil {
		configPath = config.Path
	}
	lgr.Printf("[WARN] profile %s not found in %s", profileName, configPath)
	fmt.Fprintf(os.Stderr, "Profile %s from your git config was not found in %s, ignored.\n", profileName, configPath)
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

// Load loads the profiles from the configuration file
func (profiles *profiles) Load(_ context.Context, cmd *cobra.Command) error {
	if len(*profiles) > 0 {
		return nil
	}

	config := common.CurrentConfig()
	if config == nil {
		if err := common.Initialize(cmd); err != nil {
			return fmt.Errorf("cannot initialize: %w", err)
		}
		config = common.CurrentConfig()
	}

	lgr.Printf("[DEBUG] loading profiles from %s", config.Path)
	if err := config.GetSection("profiles", profiles); err != nil {
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

// saveProfilesConfig persists the in-memory Profiles collection to the active config file.
//
// It converts each profile to its forSave view before handing them to Config.SetSection, so any
// of the three secrets (AccessToken, ClientSecret, Password) populated at runtime from the vault
// is blanked out for this encoding without touching the in-memory Profiles the rest of the process
// keeps using: a secret fetched from the vault to authorize one command can never be written back
// to the config file in plain text.
func saveProfilesConfig() error {
	config := common.CurrentConfig()
	if config == nil {
		return errors.New("configuration not loaded")
	}
	lgr.Printf("[DEBUG] writing configuration to %s", config.Path)
	toSave := make([]profileForSave, len(Profiles))
	for i, target := range Profiles {
		toSave[i] = target.forSave()
	}
	if err := config.SetSection("profiles", toSave); err != nil {
		return fmt.Errorf("cannot write config file: %w", err)
	}
	return nil
}
