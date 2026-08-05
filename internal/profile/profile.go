package profile

import (
	"context"
	"crypto/sha256"
	"encoding/csv"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"net/url"
	"os"
	"runtime"
	"strconv"
	"strings"

	"github.com/avitsrimer/bitbucket-cli/internal/common"
	"github.com/gildas/go-core"
	"github.com/go-pkgz/lgr"
	"github.com/kataras/tablewriter"
	"github.com/spf13/cobra"
	"gopkg.in/yaml.v3"
)

// ErrNoProfiles is returned by GetProfileFromCommand when no profile is configured yet.
var ErrNoProfiles = errors.New("no profiles configured")

// redactWithHash redacts a secret value, keeping a short hash so repeated values remain
// distinguishable in logs without exposing the value itself
func redactWithHash(value string) string {
	if value == "" {
		return ""
	}
	sum := sha256.Sum256([]byte(value))
	return "REDACTED-" + hex.EncodeToString(sum[:])[:10]
}

// Profile describes the configuration needed to connect to BitBucket
type Profile struct {
	Name              string                 `json:"name"                        mapstructure:"name"`
	Description       string                 `json:"description,omitempty"       mapstructure:"description,omitempty"       yaml:",omitempty"`
	Default           bool                   `json:"default"                     mapstructure:"default"                     yaml:",omitempty"`
	APIRoot           *url.URL               `json:"apiRoot,omitempty"           mapstructure:"apiRoot,omitempty"           yaml:",omitempty"`
	DefaultWorkspace  string                 `json:"defaultWorkspace,omitempty"  mapstructure:"defaultWorkspace,omitempty"  yaml:",omitempty"`
	DefaultProject    string                 `json:"defaultProject,omitempty"    mapstructure:"defaultProject,omitempty"    yaml:",omitempty"`
	ErrorProcessing   common.ErrorProcessing `json:"errorProcessing,omitempty"   mapstructure:"errorProcessing,omitempty"   yaml:",omitempty"`
	DefaultPageLength int                    `json:"defaultPageLength,omitempty" mapstructure:"defaultPageLength,omitempty" yaml:",omitempty"`
	OutputFormat      string                 `json:"outputFormat,omitempty"      mapstructure:"outputFormat,omitempty"      yaml:",omitempty"`
	Progress          bool                   `json:"progress,omitempty"          mapstructure:"progress,omitempty"          yaml:",omitempty"`
	CloneProtocol     string                 `json:"cloneProtocol,omitempty"     mapstructure:"cloneProtocol,omitempty"     yaml:",omitempty"`
	CloneUser         string                 `json:"cloneUser,omitempty"         mapstructure:"cloneUser,omitempty"         yaml:",omitempty"`
	SshKeyFilename    string                 `json:"sshKeyFilename,omitempty"    mapstructure:"sshKeyFilename,omitempty"    yaml:",omitempty"`
	VaultKey          string                 `json:"vaultKey,omitempty"          mapstructure:"vaultKey,omitempty"          yaml:",omitempty"`
	User              string                 `json:"user,omitempty"              mapstructure:"user"                        yaml:",omitempty"`
	Password          string                 `json:"password,omitempty"          mapstructure:"password"                    yaml:",omitempty"`
	ClientID          string                 `json:"clientID,omitempty"          mapstructure:"clientID"                    yaml:",omitempty"`
	ClientSecret      string                 `json:"clientSecret,omitempty"      mapstructure:"clientSecret"                yaml:",omitempty"`
	CallbackPort      uint16                 `json:"callbackPort,omitempty"      mapstructure:"callbackPort"                yaml:",omitempty"`
	AccessToken       string                 `json:"accessToken,omitempty"       mapstructure:"accessToken,omitempty"       yaml:",omitempty"`
	token             *Token                 `json:"-"                           mapstructure:"-"                           yaml:"-"`
}

// Current is the current profile
var Current *Profile

const (
	DefaultPageLength = 50 // DefaultPageLength is the default number of items per page to retrieve from Bitbucket
)

// Command represents this folder's command
var Command = &cobra.Command{
	Use:   "profile",
	Short: "Manage profiles",
	Run: func(cmd *cobra.Command, args []string) {
		fmt.Println("Profile requires a subcommand:")
		for _, command := range cmd.Commands() {
			fmt.Println(command.Name())
		}
	},
}

var columns = common.Columns[*Profile]{
	{Name: "name", DefaultSorter: true, Compare: func(a, b *Profile) bool {
		return strings.ToLower(a.Name) < strings.ToLower(b.Name)
	}},
	{Name: "description", DefaultSorter: false, Compare: func(a, b *Profile) bool {
		return strings.ToLower(a.Description) < strings.ToLower(b.Description)
	}},
	{Name: "default", DefaultSorter: false, Compare: func(a, b *Profile) bool {
		return a.Default == b.Default
	}},
	{Name: "user", DefaultSorter: false, Compare: func(a, b *Profile) bool {
		return strings.ToLower(a.User) < strings.ToLower(b.User)
	}},
	{Name: "clientid", DefaultSorter: false, Compare: func(a, b *Profile) bool {
		return strings.ToLower(a.ClientID) < strings.ToLower(b.ClientID)
	}},
	{Name: "accesstoken", DefaultSorter: false, Compare: func(a, b *Profile) bool {
		return strings.ToLower(a.AccessToken) < strings.ToLower(b.AccessToken)
	}},
	{Name: "apiRoot", DefaultSorter: false, Compare: func(a, b *Profile) bool {
		return a.APIRoot != nil && b.APIRoot != nil && strings.ToLower(a.APIRoot.String()) < strings.ToLower(b.APIRoot.String())
	}},
	{Name: "defaultworkspace", DefaultSorter: false, Compare: func(a, b *Profile) bool {
		return strings.ToLower(a.DefaultWorkspace) < strings.ToLower(b.DefaultWorkspace)
	}},
	{Name: "defaultproject", DefaultSorter: false, Compare: func(a, b *Profile) bool {
		return strings.ToLower(a.DefaultProject) < strings.ToLower(b.DefaultProject)
	}},
	{Name: "callbackport", DefaultSorter: false, Compare: func(a, b *Profile) bool {
		return a.CallbackPort < b.CallbackPort
	}},
	{Name: "outputformat", DefaultSorter: false, Compare: func(a, b *Profile) bool {
		return strings.ToLower(a.OutputFormat) < strings.ToLower(b.OutputFormat)
	}},
	{Name: "defaultpagelength", DefaultSorter: false, Compare: func(a, b *Profile) bool {
		return a.DefaultPageLength < b.DefaultPageLength
	}},
	{Name: "cloneprotocol", DefaultSorter: false, Compare: func(a, b *Profile) bool {
		return strings.ToLower(a.CloneProtocol) < strings.ToLower(b.CloneProtocol)
	}},
	{Name: "cloneuser", DefaultSorter: false, Compare: func(a, b *Profile) bool {
		return strings.ToLower(a.CloneUser) < strings.ToLower(b.CloneUser)
	}},
	{Name: "sshkeyfilename", DefaultSorter: false, Compare: func(a, b *Profile) bool {
		return strings.ToLower(a.SshKeyFilename) < strings.ToLower(b.SshKeyFilename)
	}},
	{Name: "vaultkey", DefaultSorter: false, Compare: func(a, b *Profile) bool {
		return strings.ToLower(a.VaultKey) < strings.ToLower(b.VaultKey)
	}},
	{Name: "errorprocessing", DefaultSorter: false, Compare: func(a, b *Profile) bool {
		return strings.ToLower(a.ErrorProcessing.String()) < strings.ToLower(b.ErrorProcessing.String())
	}},
	{Name: "progress", DefaultSorter: false, Compare: func(a, b *Profile) bool {
		return a.Progress == b.Progress
	}},
}

// GetProfileFromCommand gets the profile from the command line
//
// If the profile is not given, it will use the current profile
func GetProfileFromCommand(context context.Context, cmd *cobra.Command) (profile *Profile, err error) {
	if err := Profiles.Load(context, cmd); err != nil {
		return nil, err
	}

	switch {
	case cmd.Flag("profile").Changed:
		var found bool
		lgr.Printf("[DEBUG] command line has profile flag set to %s", cmd.Flag("profile").Value.String())
		if profile, found = Profiles.Find(cmd.Flag("profile").Value.String()); !found {
			return nil, fmt.Errorf("argument profile is invalid (value: %s)", cmd.Flag("profile").Value.String())
		}
	case Current == nil:
		if len(Profiles) == 0 {
			return nil, ErrNoProfiles
		}
		Current = Profiles.Current(context)
		if Current == nil {
			return nil, errors.New("argument profile is missing")
		}
		profile = Current
	default:
		profile = Current
	}
	return
}

// GetHeaders gets the header for a table
//
// implements common.Tableable
func (profile Profile) GetHeaders(cmd *cobra.Command) []string {
	if cmd != nil && cmd.Flag("columns") != nil && cmd.Flag("columns").Changed {
		if columns, err := cmd.Flags().GetStringSlice("columns"); err == nil {
			return core.Map(columns, func(column string) string { return strings.ReplaceAll(column, "_", " ") })
		}
	}
	return []string{"Name", "Description", "Default", "User", "ClientID", "AccessToken"}
}

// GetRow gets the row for a table
//
// implements common.Tableable
func (profile Profile) GetRow(headers []string) []string {
	simple := map[string]string{
		"name":              profile.Name,
		"description":       profile.Description,
		"default":           strconv.FormatBool(profile.Default),
		"defaultworkspace":  profile.DefaultWorkspace,
		"defaultproject":    profile.DefaultProject,
		"callbackport":      strconv.FormatUint(uint64(profile.CallbackPort), 10),
		"user":              profile.User,
		"clientid":          profile.ClientID,
		"outputformat":      profile.OutputFormat,
		"defaultpagelength": strconv.Itoa(profile.DefaultPageLength),
		"cloneprotocol":     profile.CloneProtocol,
		"cloneuser":         profile.CloneUser,
		"sshkeyfilename":    profile.SshKeyFilename,
		"vaultkey":          profile.VaultKey,
		"errorprocessing":   profile.ErrorProcessing.String(),
		"progress":          strconv.FormatBool(profile.Progress),
	}

	row := make([]string, 0, len(headers))
	for _, header := range headers {
		switch key := strings.ToLower(header); key {
		case "apiroot":
			if profile.APIRoot != nil {
				row = append(row, profile.APIRoot.String())
			} else {
				row = append(row, " ")
			}
		case "accesstoken":
			if profile.AccessToken != "" {
				row = append(row, profile.AccessToken)
			} else {
				row = append(row, " ")
			}
		default:
			if value, found := simple[key]; found {
				row = append(row, value)
			} else {
				row = append(row, " ")
			}
		}
	}
	return row
}

// redactedProfile is a redacted view of Profile for logging.
//
// It is a distinct type (not Profile itself) so that it does NOT inherit Profile's
// fmt.Stringer implementation (Profile.String returns just the profile name): fmt prefers
// Stringer over struct field formatting for %v/%+v, so logging a Profile value directly would
// silently print only the name and discard every redacted field, making Redact's work pointless.
type redactedProfile Profile

// Redact redacts sensitive information from the profile, for logging purposes
func (profile Profile) Redact() any {
	redacted := profile
	if redacted.ClientID != "" {
		redacted.ClientID = redactWithHash(redacted.ClientID)
	}
	if redacted.ClientSecret != "" {
		redacted.ClientSecret = redactWithHash(redacted.ClientSecret)
	}
	if redacted.User != "" {
		redacted.User = redactWithHash(redacted.User)
	}
	if redacted.Password != "" {
		redacted.Password = redactWithHash(redacted.Password)
	}
	if redacted.AccessToken != "" {
		redacted.AccessToken = redactWithHash(redacted.AccessToken)
	}
	if redacted.CloneUser != "" {
		redacted.CloneUser = redactWithHash(redacted.CloneUser)
	}
	return redactedProfile(redacted)
}

// GetClientSecret gets the client secret from the profile, either from the vault or from the profile
func (profile *Profile) GetClientSecret(ctx context.Context) (string, error) {
	return profile.getSecretOrFromVault(ctx, "client secret", profile.ClientSecret, profile.ClientID)
}

// GetPassword gets the password from the profile, either from the vault or from the profile
func (profile *Profile) GetPassword(ctx context.Context) (string, error) {
	return profile.getSecretOrFromVault(ctx, "password", profile.Password, profile.User)
}

// getSecretOrFromVault returns secret when already set, otherwise loads it from the vault for username.
func (profile *Profile) getSecretOrFromVault(_ context.Context, kind, secret, username string) (string, error) {
	if secret != "" {
		lgr.Printf("[DEBUG] the %s for profile %s is set in the profile", kind, profile.Name)
		return secret, nil
	}
	credential, err := profile.GetCredentialFromVault(profile.VaultKey, username)
	if err == nil {
		lgr.Printf("[DEBUG] loaded %s for %s from the vault", kind, username)
		return credential.Password, nil
	}
	return "", fmt.Errorf("profile %s does not have a %s: %w", profile.Name, kind, err)
}

// LoadSecrets fills the profile with its secret from the Vault as needed
func (profile *Profile) LoadSecrets(ctx context.Context) (err error) {
	if profile.ClientID != "" {
		profile.ClientSecret, err = profile.GetClientSecret(ctx)
		return err
	}
	if profile.User != "" {
		profile.Password, err = profile.GetPassword(ctx)
		return err
	}
	return profile.loadAccessToken(ctx)
}

// Update updates this profile with the given one
func (profile *Profile) Update(other Profile) error {
	profile.updateSimpleFields(other)
	profile.updateCredentials(other)
	return profile.Validate()
}

// updateSimpleFields updates the fields that have no side effect other than being overwritten
func (profile *Profile) updateSimpleFields(other Profile) {
	if other.Name != "" {
		profile.Name = other.Name
	}
	if other.Description != "" {
		profile.Description = other.Description
	}
	if other.Default {
		profile.Default = other.Default
	}
	if other.OutputFormat != "" {
		profile.OutputFormat = other.OutputFormat
	}
	if other.CallbackPort > 0 {
		profile.CallbackPort = other.CallbackPort
	}
	if other.DefaultWorkspace != "" {
		profile.DefaultWorkspace = other.DefaultWorkspace
	}
	if other.DefaultProject != "" {
		profile.DefaultProject = other.DefaultProject
	}
	if other.CloneProtocol != "" {
		profile.CloneProtocol = other.CloneProtocol
	}
	if other.CloneUser != "" {
		profile.CloneUser = other.CloneUser
	}
	if other.SshKeyFilename != "" {
		profile.SshKeyFilename = other.SshKeyFilename
	}
}

// updateCredentials updates the profile's credentials, clearing the cached token when the
// client ID or client secret change
func (profile *Profile) updateCredentials(other Profile) {
	if other.AccessToken != "" && other.AccessToken != profile.AccessToken {
		profile.AccessToken = other.AccessToken
	}
	if other.User != "" && other.User != profile.User {
		profile.User = other.User
	}
	if other.Password != "" && other.Password != profile.Password {
		profile.Password = other.Password
	}
	if other.ClientID != "" && other.ClientID != profile.ClientID {
		profile.ClientID = other.ClientID
		profile.token = nil
	}
	if other.ClientSecret != "" && other.ClientSecret != profile.ClientSecret {
		profile.ClientSecret = other.ClientSecret
		profile.token = nil
	}
}

// ShouldStopOnError tells if the command should stop on error
func (profile Profile) ShouldStopOnError(cmd *cobra.Command) bool {
	if cmd.Flag("stop-on-error").Changed {
		return cmd.Flag("stop-on-error").Value.String() == "true"
	}
	return profile.ErrorProcessing == common.StopOnError
}

// ShouldWarnOnError tells if the command should warn on error
func (profile Profile) ShouldWarnOnError(cmd *cobra.Command) bool {
	if cmd.Flag("warn-on-error").Changed {
		return cmd.Flag("warn-on-error").Value.String() == "true"
	}
	return profile.ErrorProcessing == common.WarnOnError
}

// ShouldIgnoreErrors tells if the command should ignore errors
func (profile Profile) ShouldIgnoreErrors(cmd *cobra.Command) bool {
	if cmd.Flag("ignore-errors").Changed {
		return cmd.Flag("ignore-errors").Value.String() == "true"
	}
	return profile.ErrorProcessing == common.IgnoreErrors
}

// String gets a string representation of this profile
//
// implements fmt.Stringer
func (profile Profile) String() string {
	return profile.Name
}

// Print prints the given payload to the console
func (profile Profile) Print(context context.Context, cmd *cobra.Command, payload any) error {
	outputFormat := profile.OutputFormat

	// cmd.Flag("output").Value carries the --output flag's value, which also holds the
	// BB_OUTPUT_FORMAT environment variable as its default; checking Changed alone would miss
	// the env-only case, since setting a flag's default never marks it as Changed.
	if commandFormat := cmd.Flag("output").Value.String(); commandFormat != "" {
		outputFormat = commandFormat
		lgr.Printf("[DEBUG] command output format: %s (was: %s)", outputFormat, profile.OutputFormat)
	}
	switch outputFormat {
	case "json":
		return profile.printJSON(payload)
	case "yaml":
		return profile.printYAML(payload)
	case "csv":
		return profile.printDelimited(cmd, payload, ',')
	case "tsv":
		return profile.printDelimited(cmd, payload, '\t')
	default:
		return profile.printTable(cmd, payload)
	}
}

// printJSON prints the given payload to the console as JSON
func (profile Profile) printJSON(payload any) error {
	lgr.Printf("[DEBUG] printing payload as JSON")
	data, err := json.MarshalIndent(payload, "", "  ")
	if err != nil {
		return fmt.Errorf("cannot marshal payload to json: %w", err)
	}
	fmt.Println(string(data))
	return nil
}

// printYAML prints the given payload to the console as YAML
func (profile Profile) printYAML(payload any) error {
	lgr.Printf("[DEBUG] printing payload as YAML")
	data, err := yaml.Marshal(payload)
	if err != nil {
		return fmt.Errorf("cannot marshal payload to yaml: %w", err)
	}
	fmt.Println(string(data))
	return nil
}

// printDelimited prints the given payload to the console as delimiter-separated values
func (profile Profile) printDelimited(cmd *cobra.Command, payload any, comma rune) error {
	lgr.Printf("[DEBUG] printing payload as delimited text (comma=%q)", comma)
	writer := csv.NewWriter(os.Stdout)
	writer.Comma = comma
	defer writer.Flush()

	switch actual := payload.(type) {
	case common.Tableable:
		headers := actual.GetHeaders(cmd)
		_ = writer.Write(headers)
		_ = writer.Write(actual.GetRow(headers))
	case common.Tableables:
		lgr.Printf("[DEBUG] payload is a slice of %d elements", actual.Size())
		if actual.Size() > 0 {
			headers := actual.GetHeaders(cmd)
			_ = writer.Write(headers)
			for i := range actual.Size() {
				_ = writer.Write(actual.GetRowAt(i, headers))
			}
		}
	default:
		return errors.New("argument payload is invalid: not a tableable")
	}
	return nil
}

// printTable prints the given payload to the console as a table
func (profile Profile) printTable(cmd *cobra.Command, payload any) error {
	lgr.Printf("[DEBUG] printing payload as table")
	table := tablewriter.NewWriter(os.Stdout)

	switch actual := payload.(type) {
	case common.Tableable:
		headers := actual.GetHeaders(cmd)
		table.SetHeader(headers)
		table.SetAutoWrapText(false)
		table.Append(actual.GetRow(headers))
	case common.Tableables:
		lgr.Printf("[DEBUG] payload is a slice of %d elements", actual.Size())
		if actual.Size() > 0 {
			headers := actual.GetHeaders(cmd)
			table.SetHeader(headers)
			table.SetAutoWrapText(false)
			for i := range actual.Size() {
				table.Append(actual.GetRowAt(i, headers))
			}
		}
	default:
		return errors.New("argument payload is invalid: not a tableable")
	}
	table.Render()
	return nil
}

// Validate validates a Profile
func (profile *Profile) Validate() error {
	var errs []error

	if profile.Name == "" {
		errs = append(errs, errors.New("argument name is missing"))
	}

	if profile.VaultKey == "" && runtime.GOOS != "windows" {
		profile.VaultKey = "bitbucket-cli"
	}

	if profile.CloneProtocol == "" {
		profile.CloneProtocol = "git"
	}
	if profile.CloneProtocol != "git" && profile.CloneProtocol != "https" && profile.CloneProtocol != "ssh" {
		errs = append(errs, fmt.Errorf("argument cloneProtocol is invalid (value: %s)", profile.CloneProtocol))
	}
	if profile.OutputFormat == "" {
		profile.OutputFormat = "table"
	}
	if profile.DefaultPageLength == 0 {
		profile.DefaultPageLength = DefaultPageLength
	} else if profile.DefaultPageLength < 0 || profile.DefaultPageLength > 100 {
		errs = append(errs, fmt.Errorf("default page length must be between 0 and 100 (value: %d)", profile.DefaultPageLength))
	}
	return errors.Join(errs...)
}

// MarshalJSON marshals this profile to JSON
//
// implements json.Marshaler
func (profile Profile) MarshalJSON() ([]byte, error) {
	type surrogate Profile

	if profile.OutputFormat == "table" {
		profile.OutputFormat = ""
	}
	if profile.DefaultPageLength == DefaultPageLength {
		profile.DefaultPageLength = 0
	}
	errorProcessing := profile.ErrorProcessing.String()
	if errorProcessing == common.StopOnError.String() {
		errorProcessing = ""
	}
	data, err := json.Marshal(struct {
		surrogate
		APIRoot         *core.URL `json:"apiRoot,omitempty"`
		ErrorProcessing string    `json:"errorProcessing,omitempty"`
	}{
		surrogate:       surrogate(profile),
		APIRoot:         (*core.URL)(profile.APIRoot),
		ErrorProcessing: errorProcessing,
	})
	if err != nil {
		return nil, fmt.Errorf("cannot marshal profile to json: %w", err)
	}
	return data, nil
}

// UnmarshalJSON unmarshals this profile from JSON
//
// implements json.Unmarshaler
func (profile *Profile) UnmarshalJSON(data []byte) error {
	type surrogate Profile
	var inner struct {
		surrogate
		APIRoot *core.URL `json:"apiRoot,omitempty"`
	}
	if err := json.Unmarshal(data, &inner); err != nil {
		return fmt.Errorf("cannot unmarshal profile: %w", err)
	}
	*profile = Profile(inner.surrogate)
	profile.APIRoot = (*url.URL)(inner.APIRoot)
	if err := profile.Validate(); err != nil {
		return fmt.Errorf("cannot unmarshal profile: %w", err)
	}
	return nil
}

// getWorkspaceSlugs gets the slugs of all workspaces
func getWorkspaceSlugs(context context.Context, cmd *cobra.Command, args []string, toComplete string) (slugs []string, err error) {
	// We have to repeat the code here because of the circular dependency with the workspace package
	type Workspace struct {
		Workspace struct {
			Slug string `json:"slug"`
		} `json:"workspace"`
	}

	lgr.Printf("[DEBUG] getting all workspaces")
	workspaces, err := GetAll[Workspace](context, cmd, "/user/workspaces")
	if err != nil {
		lgr.Printf("[ERROR] failed to get workspaces: %v", err)
		return []string{}, err
	}
	lgr.Printf("[DEBUG] found %d workspaces", len(workspaces))
	slugs = core.Map(workspaces, func(workspace Workspace) string { return workspace.Workspace.Slug })
	core.Sort(slugs, func(a, b string) bool { return strings.ToLower(a) < strings.ToLower(b) })
	return slugs, nil
}

// getProjectKeys gets the keys of all projects
func getProjectKeys(context context.Context, cmd *cobra.Command, args []string, toComplete string) (keys []string, err error) {
	type Project struct {
		Key string `json:"key"`
	}

	workspace := cmd.Flag("default-workspace").Value.String()
	if workspace == "" {
		lgr.Printf("[WARN] no workspace given")
		return
	}

	lgr.Printf("[DEBUG] getting all projects in workspace %s", workspace)
	projects, err := GetAll[Project](context, cmd, fmt.Sprintf("/workspaces/%s/projects", workspace))
	if err != nil {
		lgr.Printf("[ERROR] failed to get projects: %v", err)
		return
	}
	keys = core.Map(projects, func(project Project) string { return project.Key })
	core.Sort(keys, func(a, b string) bool { return strings.ToLower(a) < strings.ToLower(b) })
	return keys, nil
}

// disableUnsupportedFlags disables the flags that are not supported by the profile command
func disableUnsupportedFlags(cmd *cobra.Command, args []string) error {
	if cmd.Flags().Changed("repository") {
		return errors.New("the --repository flag is not supported by the profile command")
	}
	if cmd.Flags().Changed("workspace") {
		return errors.New("the --workspace flag is not supported by the profile command")
	}
	return nil
}

// hideUnsupportedFlags hides the flags that are not supported by the profile command
func hideUnsupportedFlags(cmd *cobra.Command, args []string) {
	_ = cmd.Flags().MarkHidden("repository")
	_ = cmd.Flags().MarkHidden("workspace")
	cmd.Parent().HelpFunc()(cmd, args)
}
