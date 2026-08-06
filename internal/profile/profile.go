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
	"github.com/mattn/go-runewidth"
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
	Name              string                 `json:"name"`
	Description       string                 `json:"description,omitempty"       yaml:",omitempty"`
	Default           bool                   `json:"default"                     yaml:",omitempty"`
	APIRoot           *url.URL               `json:"apiRoot,omitempty"           yaml:",omitempty"`
	DefaultWorkspace  string                 `json:"defaultWorkspace,omitempty"  yaml:",omitempty"`
	DefaultRepository string                 `json:"defaultRepository,omitempty" yaml:",omitempty"`
	DefaultProject    string                 `json:"defaultProject,omitempty"    yaml:",omitempty"`
	ErrorProcessing   common.ErrorProcessing `json:"errorProcessing,omitempty"   yaml:",omitempty"`
	DefaultPageLength int                    `json:"defaultPageLength,omitempty" yaml:",omitempty"`
	OutputFormat      string                 `json:"outputFormat,omitempty"      yaml:",omitempty"`
	Progress          bool                   `json:"progress,omitempty"          yaml:",omitempty"`
	CloneProtocol     string                 `json:"cloneProtocol,omitempty"     yaml:",omitempty"`
	CloneUser         string                 `json:"cloneUser,omitempty"         yaml:",omitempty"`
	SshKeyFilename    string                 `json:"sshKeyFilename,omitempty"    yaml:",omitempty"`
	VaultKey          string                 `json:"vaultKey,omitempty"          yaml:",omitempty"`
	User              string                 `json:"user,omitempty"                        yaml:",omitempty"`
	Password          string                 `json:"password,omitempty"                    yaml:",omitempty"`
	ClientID          string                 `json:"clientID,omitempty"                    yaml:",omitempty"`
	ClientSecret      string                 `json:"clientSecret,omitempty"                yaml:",omitempty"`
	CallbackPort      uint16                 `json:"callbackPort,omitempty"                yaml:",omitempty"`
	AccessToken       string                 `json:"accessToken,omitempty"       yaml:",omitempty"`
	token             *Token                 `json:"-"                           yaml:"-"`
	// vault tracks, per secret field, whether that value was populated at runtime by
	// LoadSecrets/loadAccessToken's vault fallback rather than configured by the user (explicitly
	// on the command line or already present in the config file). Profile.forSave blanks out
	// exactly the fields this marks before a Profile is persisted, so a secret fetched from the
	// vault to authorize one command is never written back to the config file in plain text --
	// for any of the three secrets, on every path that saves a Profile. Display paths (profile
	// get/list -o yaml/json) marshal the Profile directly and show the secret, since the user
	// explicitly asked LoadSecrets to populate it for display.
	vault vaultProvenance `json:"-" yaml:"-"`
}

// vaultProvenance records, for each of Profile's three secret fields, whether its current value
// was loaded from the vault at runtime rather than configured directly.
type vaultProvenance struct {
	accessToken  bool
	clientSecret bool
	password     bool
}

// hasPlainTextSecret tells whether profile currently holds any of its three secrets in plain
// text, as opposed to having it only because it was loaded from the vault at runtime (e.g. by
// loadAccessToken authorizing the workspace/project lookups --default-workspace/--default-project
// trigger during flag parsing): a vault-loaded value must never be mistaken for proof the profile
// stores its credentials in plain text, or a profile that deliberately keeps them in the vault
// would be forced back out of it by the mere act of using them.
func (profile *Profile) hasPlainTextSecret() bool {
	return (profile.AccessToken != "" && !profile.vault.accessToken) ||
		(profile.ClientSecret != "" && !profile.vault.clientSecret) ||
		(profile.Password != "" && !profile.vault.password)
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
	Run:   common.SubcommandRequired("Profile"),
}

var columns = common.Columns[*Profile]{
	{Name: "name", DefaultSorter: true, Compare: func(a, b *Profile) bool {
		return strings.ToLower(a.Name) < strings.ToLower(b.Name)
	}},
	{Name: "description", DefaultSorter: false, Compare: func(a, b *Profile) bool {
		return strings.ToLower(a.Description) < strings.ToLower(b.Description)
	}},
	{Name: "default", DefaultSorter: false, Compare: func(a, b *Profile) bool {
		return !a.Default && b.Default
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
	{Name: "apiroot", DefaultSorter: false, Compare: func(a, b *Profile) bool {
		return a.APIRoot != nil && b.APIRoot != nil && strings.ToLower(a.APIRoot.String()) < strings.ToLower(b.APIRoot.String())
	}},
	{Name: "defaultworkspace", DefaultSorter: false, Compare: func(a, b *Profile) bool {
		return strings.ToLower(a.DefaultWorkspace) < strings.ToLower(b.DefaultWorkspace)
	}},
	{Name: "defaultrepository", DefaultSorter: false, Compare: func(a, b *Profile) bool {
		return strings.ToLower(a.DefaultRepository) < strings.ToLower(b.DefaultRepository)
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
		return !a.Progress && b.Progress
	}},
}

// GetProfileFromCommand gets the profile from the command line
//
// If the profile is not given, it will use the current profile
func GetProfileFromCommand(context context.Context, cmd *cobra.Command) (profile *Profile, err error) {
	if err := Profiles.Load(context, cmd); err != nil {
		return nil, err
	}

	// The --profile flag's own value covers both an explicit flag and the BB_PROFILE environment
	// variable already present when the root command's flags were registered; os.Getenv is
	// checked directly too, covering BB_PROFILE having only been set by a .env file loaded (in
	// main()) after that flag registration ran at package-init time.
	profileName := cmd.Flag("profile").Value.String()
	if profileName == "" {
		profileName = os.Getenv("BB_PROFILE")
	}

	switch {
	case profileName != "":
		var found bool
		lgr.Printf("[DEBUG] command line has profile flag set to %s", profileName)
		if profile, found = Profiles.Find(profileName); !found {
			return nil, fmt.Errorf("argument profile is invalid (value: %s)", profileName)
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
	return profile, nil
}

// GetHeaders gets the header for a table
//
// implements common.Tableable
//
// AccessToken is deliberately not part of the default column set: unlike User/ClientID (which
// name who a profile authenticates as, not a secret), a profile's access token is one of its
// three secret fields, and printing it in a plain `bb profile list`/`bb profile get` table -- with
// no --dry-run, no piping, no obvious reason to expect a secret onscreen -- is exactly the kind of
// leak forDisplay's masking exists to prevent. It remains available on demand via --columns
// accesstoken, still masked (see GetRow's "accesstoken" case).
func (profile Profile) GetHeaders(cmd *cobra.Command) []string {
	return common.HeadersFromFlag(cmd, "Name", "Description", "Default", "User", "ClientID")
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
		"defaultrepository": profile.DefaultRepository,
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
		switch key := common.NormalizeColumnKey(header); key {
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
	if redacted.APIRoot != nil {
		// Clone before mutating: redacted.APIRoot still points at the same *url.URL as the
		// original profile's (Profile is copied shallowly by value above), so writing through it
		// directly would corrupt the live profile's APIRoot for whoever logs it.
		cloned := *redacted.APIRoot
		if _, hasPassword := cloned.User.Password(); hasPassword {
			cloned.User = url.UserPassword(cloned.User.Username(), "xxxxx")
			redacted.APIRoot = &cloned
		}
	}
	return redactedProfile(redacted)
}

// GetClientSecret gets the client secret from the profile, either from the vault or from the profile
func (profile *Profile) GetClientSecret(_ context.Context) (string, error) {
	secret, fromVault, err := profile.getSecretOrFromVault("client secret", profile.ClientSecret, profile.ClientID)
	if fromVault {
		profile.vault.clientSecret = true // must never be written back to the config file in plain text
	}
	return secret, err
}

// GetPassword gets the password from the profile, either from the vault or from the profile
func (profile *Profile) GetPassword(_ context.Context) (string, error) {
	secret, fromVault, err := profile.getSecretOrFromVault("password", profile.Password, profile.User)
	if fromVault {
		profile.vault.password = true // must never be written back to the config file in plain text
	}
	return secret, err
}

// getSecretOrFromVault returns secret when already set, otherwise loads it from the vault for
// username, reporting whether the returned value came from the vault. It takes no context: the
// vault client (zalando/go-keyring) is not context-aware.
func (profile *Profile) getSecretOrFromVault(kind, secret, username string) (value string, fromVault bool, err error) {
	if secret != "" {
		lgr.Printf("[DEBUG] the %s for profile %s is set in the profile", kind, profile.Name)
		return secret, false, nil
	}
	credential, err := profile.GetCredentialFromVault(profile.VaultKey, username)
	if err == nil {
		lgr.Printf("[DEBUG] loaded %s for %s from the vault", kind, username)
		return credential.Password, true, nil
	}
	return "", false, fmt.Errorf("profile %s does not have a %s: %w", profile.Name, kind, err)
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
	if other.DefaultRepository != "" {
		profile.DefaultRepository = other.DefaultRepository
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
	if other.VaultKey != "" {
		profile.VaultKey = other.VaultKey
	}
	if other.DefaultPageLength != 0 {
		profile.DefaultPageLength = other.DefaultPageLength
	}
	// ErrorProcessing's zero value (StopOnError) is indistinguishable from "not set on other", so
	// an explicit --error-processing StopOnError cannot be told apart from the flag being absent
	// here; WarnOnError/IgnoreErrors are unambiguous and always copied. updateProcess additionally
	// copies StopOnError itself when the --error-processing flag was explicitly given, the same way
	// it already special-cases --progress.
	if other.ErrorProcessing != common.StopOnError {
		profile.ErrorProcessing = other.ErrorProcessing
	}
}

// updateCredentials updates the profile's credentials, clearing the cached token when the
// client ID or client secret change
func (profile *Profile) updateCredentials(other Profile) {
	if other.AccessToken != "" && other.AccessToken != profile.AccessToken {
		profile.AccessToken = other.AccessToken
		profile.vault.accessToken = false // explicitly user-provided, not a runtime vault copy
	}
	if other.User != "" && other.User != profile.User {
		profile.User = other.User
	}
	if other.Password != "" && other.Password != profile.Password {
		profile.Password = other.Password
		profile.vault.password = false // explicitly user-provided, not a runtime vault copy
	}
	if other.ClientID != "" && other.ClientID != profile.ClientID {
		profile.ClientID = other.ClientID
		profile.token = nil
	}
	if other.ClientSecret != "" && other.ClientSecret != profile.ClientSecret {
		profile.ClientSecret = other.ClientSecret
		profile.vault.clientSecret = false // explicitly user-provided, not a runtime vault copy
		profile.token = nil
	}
}

// ShouldStopOnError tells if the command should stop on error
//
// Precedence: an explicit --stop-on-error always wins; otherwise an explicit --warn-on-error or
// --ignore-errors wins over the profile's configured ErrorProcessing (so those flags can override
// a profile that would otherwise stop, or a profile with no ErrorProcessing set at all, which
// defaults to StopOnError); with none of the three flags given, the profile's ErrorProcessing
// decides, and with neither a flag nor a configured ErrorProcessing, the default is to stop.
func (profile Profile) ShouldStopOnError(cmd *cobra.Command) bool {
	if cmd.Flag("stop-on-error").Changed {
		return cmd.Flag("stop-on-error").Value.String() == "true"
	}
	if profile.ShouldWarnOnError(cmd) || profile.ShouldIgnoreErrors(cmd) {
		return false
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
	// the env-only case, since setting a flag's default never marks it as Changed. That default
	// was baked in at the root command's package-init time though, before main() has had a chance
	// to load a .env file, so BB_OUTPUT_FORMAT set only via .env still needs a direct, lazy
	// os.Getenv fallback here.
	commandFormat := cmd.Flag("output").Value.String()
	if commandFormat == "" {
		commandFormat = os.Getenv("BB_OUTPUT_FORMAT")
	}
	if commandFormat != "" {
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

// tableableRows extracts headers and rows out of payload, which must implement common.Tableable
// (exactly one row) or common.Tableables (zero or more rows) -- the row-extraction logic
// printTable and printDelimited both need before rendering it in their own format. headers is
// nil when payload is an empty Tableables, the signal both callers use to skip writing headers
// for a genuinely empty list.
func tableableRows(cmd *cobra.Command, payload any) (headers []string, rows [][]string, err error) {
	switch actual := payload.(type) {
	case common.Tableable:
		headers = actual.GetHeaders(cmd)
		rows = [][]string{actual.GetRow(headers)}
	case common.Tableables:
		lgr.Printf("[DEBUG] payload is a slice of %d elements", actual.Size())
		if actual.Size() > 0 {
			headers = actual.GetHeaders(cmd)
			rows = make([][]string, actual.Size())
			for i := range actual.Size() {
				rows[i] = actual.GetRowAt(i, headers)
			}
		}
	default:
		return nil, nil, errors.New("argument payload is invalid: not a tableable")
	}
	return headers, rows, nil
}

// printDelimited prints the given payload to the console as delimiter-separated values
func (profile Profile) printDelimited(cmd *cobra.Command, payload any, comma rune) error {
	lgr.Printf("[DEBUG] printing payload as delimited text (comma=%q)", comma)
	headers, rows, err := tableableRows(cmd, payload)
	if err != nil {
		return err
	}

	writer := csv.NewWriter(os.Stdout)
	writer.Comma = comma
	defer writer.Flush()

	if headers == nil {
		return nil
	}
	_ = writer.Write(headers)
	for _, row := range rows {
		_ = writer.Write(row)
	}
	return nil
}

// printTable prints the given payload to the console as a table
func (profile Profile) printTable(cmd *cobra.Command, payload any) error {
	lgr.Printf("[DEBUG] printing payload as table")
	headers, rows, err := tableableRows(cmd, payload)
	if err != nil {
		return err
	}

	table := tablewriter.NewWriter(os.Stdout)
	if headers != nil {
		table.SetHeader(headers)
		table.SetAutoWrapText(false)
		for _, row := range rows {
			table.Append(truncateTableRow(headers, row))
		}
	}
	table.Render()
	return nil
}

// maxTableCellWidth caps how many display columns of a single free-text cell's content (see
// freeTextColumnKeys) the human-facing table renderer will show before cutting it off with an
// ellipsis. A Tableable's GetRow/GetRowAt is shared with printDelimited (csv/tsv) and read
// directly by json/yaml via the untouched payload, so this cap is applied here, once, on the
// already-rendered row slice, rather than inside any GetRow/GetRowAt implementation: printTable
// is the only caller of truncateTableRow, which keeps every other output format -- csv, tsv,
// json, yaml -- exactly as complete as the underlying data, for scripting.
//
// A single fixed cap (deliberately not derived from the terminal's actual width via $COLUMNS or
// an stty call, both of which would add real complexity for a cosmetic table-only concern) is the
// smallest change that keeps the default table readable regardless of terminal size or whether
// stdout is a TTY.
const maxTableCellWidth = 80

// freeTextColumnKeys is the fixed set of normalized column keys (see common.NormalizeColumnKey)
// whose values are unbounded free text -- a pull request/commit/comment title, description,
// message, or content, a decline/close reason, or a pull request's per-participant
// "nickname:state" summary -- as opposed to an identifier a user must copy verbatim (a UUID, an
// artifact name, a container image reference) or a short, bounded value that already reads fine
// at any length. Only a cell under one of these keys is truncated for the table renderer; every
// other column, including every identifier, is left exactly as returned by the API, at any
// length. participants is included because its rendered value grows with the reviewer count, the
// same "one long cell blows out the table" shape the other keys here exist to cap.
var freeTextColumnKeys = map[string]bool{
	"title":        true,
	"description":  true,
	"message":      true,
	"content":      true,
	"reason":       true,
	"participants": true,
}

// truncateTableRow returns a copy of row with every cell under a freeTextColumnKeys header (per
// headers, positionally matched to row) cut down to maxTableCellWidth display columns (see
// truncateCell), so one long free-text value -- a multi-paragraph pull request description, a
// comment body, a commit message -- can no longer blow a table's column, and so every other
// column, out to an unreadable width. Every other cell -- identifiers most of all, e.g. an
// artifact Name a later `artifact download` needs verbatim, or a step Image reference -- is left
// untouched.
func truncateTableRow(headers, row []string) []string {
	truncated := make([]string, len(row))
	for i, cell := range row {
		if i < len(headers) && freeTextColumnKeys[common.NormalizeColumnKey(headers[i])] {
			truncated[i] = truncateCell(cell)
		} else {
			truncated[i] = cell
		}
	}
	return truncated
}

// truncateCell collapses cell's internal whitespace (including embedded newlines, so a
// multi-paragraph value renders as one table line instead of expanding the row across as many
// lines as it has paragraphs) down to single spaces, then cuts the result to maxTableCellWidth
// display columns, replacing the final rune with an ellipsis when a cut was needed. Display width
// (github.com/mattn/go-runewidth, the same measure tablewriter itself uses to size columns), not
// rune count, is what is compared against the cap: a rune count would still let maxTableCellWidth
// double-width runes (CJK, most emoji) render at up to twice the cap's actual terminal columns,
// defeating the "readable regardless of terminal size" goal for exactly the values most likely to
// need it.
func truncateCell(cell string) string {
	flattened := strings.Join(strings.Fields(cell), " ")
	if runewidth.StringWidth(flattened) <= maxTableCellWidth {
		return flattened
	}
	runes := []rune(flattened)
	width := 0
	cut := len(runes)
	for i, r := range runes {
		w := runewidth.RuneWidth(r)
		if width+w > maxTableCellWidth-1 { // reserve 1 display column for the ellipsis
			cut = i
			break
		}
		width += w
	}
	return string(runes[:cut]) + "…"
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
// This is a display-only encoding, reached only from `profile get`/`profile list -o json` (see
// Profile.printJSON): unlike MarshalYAML it has no persistence counterpart to worry about, since
// nothing in this codebase persists a Profile as JSON -- config files are YAML (Config.Save), and
// the only other json.Marshal call in this package serializes a Token, not a Profile. It always
// shows every field, including any secret LoadSecrets populated from the vault for display.
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

// MarshalYAML implements yaml.Marshaler.
//
// This is the *display* encoding, reached whenever a Profile (or a slice of them) is marshaled
// directly: `profile get`/`profile list -o yaml`, and any other code that calls yaml.Marshal on a
// Profile it already holds. It shows exactly what the caller has in memory, including any secret
// LoadSecrets populated from the vault for display -- callers that instead need the *persistence*
// encoding (never writing a vault-loaded secret back to the config file) must marshal
// Profile.forSave()'s result instead; see saveProfilesConfig.
//
// It rewrites one field on top of the default struct encoding, working directly on the resulting
// mapping node: yaml.v3 has no field-shadowing equivalent to encoding/json's anonymous struct
// override (see MarshalJSON above), so declaring a sibling field with the same key as one inside
// an embedded, ,inline surrogate would collide and error at encode time instead of overriding it.
//
// apiRoot is rewritten to its plain string form, matching the form UnmarshalYAML accepts back and
// preserving userinfo credentials, which url.URL's default field-by-field mapping form cannot
// represent (its User field's members are all unexported).
func (profile Profile) MarshalYAML() (any, error) {
	type surrogate Profile
	var node yaml.Node
	if err := node.Encode(surrogate(profile)); err != nil {
		return nil, fmt.Errorf("cannot encode profile: %w", err)
	}
	for i := 0; i+1 < len(node.Content); i += 2 {
		if node.Content[i].Value == "apiroot" && profile.APIRoot != nil {
			if err := node.Content[i+1].Encode(profile.APIRoot.String()); err != nil {
				return nil, fmt.Errorf("cannot encode apiRoot: %w", err)
			}
		}
	}
	return &node, nil
}

// profileForSave is the *persistence* view of a Profile: encoding it (via yaml.Marshal, and so
// via Config.SetSection/Config.Save) blanks out any of the three secret fields that were loaded
// from the vault at runtime rather than configured by the user, so a secret fetched from the
// vault to authorize one command is never written back to the config file in plain text. It
// embeds Profile so it inherits MarshalYAML's apiRoot rewrite unchanged; only the blanked fields
// differ from a direct encoding of the Profile it was built from.
type profileForSave struct {
	Profile
}

// forSave returns the persistence view of profile: a copy with every vault-loaded secret
// (AccessToken, ClientSecret, Password) blanked out, leaving profile itself untouched. Pass the
// result to yaml.Marshal (directly, or via Config.SetSection) whenever a Profile is being
// persisted, e.g. saveProfilesConfig.
func (profile Profile) forSave() profileForSave {
	saved := profile
	if saved.vault.accessToken {
		saved.AccessToken = ""
	}
	if saved.vault.clientSecret {
		saved.ClientSecret = ""
	}
	if saved.vault.password {
		saved.Password = ""
	}
	return profileForSave{Profile: saved}
}

// secretMask replaces a secret value forDisplay masks, in any output format.
const secretMask = "********"

// forDisplay returns a copy of profile with all three secret fields (Password, ClientSecret,
// AccessToken) unconditionally replaced by secretMask when set, regardless of their provenance
// (freshly given on the command line/stdin/prompt, or loaded from the vault). Unlike forSave,
// which only blanks vault-provenance secrets for persistence, this masks every secret every time:
// it is for confirmation output after a mutation (`profile update`), where the point is confirming
// the mutation happened, not echoing a secret the caller went out of their way to keep out of
// shell history/scrollback via --password-stdin/--access-token-stdin. `profile get`/`profile list
// -o json/yaml` intentionally still show the real secret on request (see MarshalJSON's doc
// comment); forDisplay is deliberately not used there.
func (profile Profile) forDisplay() Profile {
	displayed := profile
	if displayed.Password != "" {
		displayed.Password = secretMask
	}
	if displayed.ClientSecret != "" {
		displayed.ClientSecret = secretMask
	}
	if displayed.AccessToken != "" {
		displayed.AccessToken = secretMask
	}
	return displayed
}

// UnmarshalYAML implements yaml.Unmarshaler.
//
// url.URL has no UnmarshalYAML of its own, so yaml.v3 only ever decodes APIRoot from the nested
// mapping form its own exported fields produce (scheme/host/path/...) -- and even that form is
// lossy, since url.URL.User is a *url.Userinfo whose fields are all unexported and so never
// round-trip through reflection-based decoding. A config file's apiRoot is just as likely to be
// the plain string form every other apiRoot-shaped value in this codebase accepts (a URL literal,
// e.g. "apiRoot: https://user:pw@api.bitbucket.org"), which would otherwise fail to decode
// ("cannot unmarshal !!str into url.URL") and abort loading every profile.
//
// This extracts a scalar apiroot key before the surrogate decode ever sees it and parses it
// directly with url.Parse (preserving userinfo losslessly, unlike the mapping form). The surrogate
// decode -- which only understands the mapping form -- then runs against a shallow copy of node
// with that one key's value replaced by a null scalar, so it skips over it cleanly instead of
// erroring on a bare string (or, for "", an empty one), without mutating node itself or any node
// reachable from it: decoding the same node twice (e.g. a config layer that retains a parsed
// document tree) must yield the same profile both times. The parsed URL is assigned back onto
// profile.APIRoot afterward. An already-nested mapping (or a missing key) is left for the
// surrogate decode to handle as before.
func (profile *Profile) UnmarshalYAML(node *yaml.Node) error {
	decodeNode := node
	var apiRoot *url.URL
	if node.Kind == yaml.MappingNode {
		parsed, isScalar, err := extractAPIRootValue(node)
		if err != nil {
			return err
		}
		if isScalar {
			// Even an empty string (parsed == nil) must still be nulled in the copy handed to the
			// surrogate decode below: it is a plain string scalar just like a populated one, and
			// url.URL's mapping-form decode rejects any string just as it would a populated one.
			apiRoot = parsed
			decodeNode = nodeWithNulledKey(node, "apiroot")
		}
	}
	type surrogate Profile
	var inner surrogate
	if err := decodeNode.Decode(&inner); err != nil {
		return fmt.Errorf("cannot decode profile: %w", err)
	}
	*profile = Profile(inner)
	if apiRoot != nil {
		profile.APIRoot = apiRoot
	}
	return nil
}

// extractAPIRootValue locates a mapping node's "apiroot" key and, if it is currently a plain
// string scalar, parses and returns it, reading the node without modifying it. isScalar is true
// whenever the key holds a plain string scalar, empty or not -- both shapes must still be nulled
// out of the copy handed to the surrogate decode, since url.URL's mapping-form decode rejects any
// bare string -- and false when the key was absent, already null, or already a nested mapping,
// which the caller must leave for the surrogate decode to handle as before. apiRoot itself is
// non-nil only for a non-empty string.
func extractAPIRootValue(node *yaml.Node) (apiRoot *url.URL, isScalar bool, err error) {
	for i := 0; i+1 < len(node.Content); i += 2 {
		if node.Content[i].Value != "apiroot" {
			continue
		}
		value := node.Content[i+1]
		if value.Kind != yaml.ScalarNode || value.Tag == "!!null" {
			return nil, false, nil
		}
		var raw string
		if decodeErr := value.Decode(&raw); decodeErr != nil {
			return nil, false, fmt.Errorf("cannot decode apiRoot: %w", decodeErr)
		}
		if raw == "" {
			return nil, true, nil
		}
		parsed, parseErr := url.Parse(raw)
		if parseErr != nil {
			return nil, false, fmt.Errorf("cannot parse apiRoot %q: %w", raw, parseErr)
		}
		return parsed, true, nil
	}
	return nil, false, nil
}

// nodeWithNulledKey returns a shallow copy of node whose mapping value for key is replaced with a
// fresh null scalar node, leaving node's own Content slice and every node reachable from it
// completely untouched -- unlike mutating the value node in place, this lets the same node be
// decoded again later with identical results.
func nodeWithNulledKey(node *yaml.Node, key string) *yaml.Node {
	copied := *node
	copied.Content = append([]*yaml.Node(nil), node.Content...)
	for i := 0; i+1 < len(copied.Content); i += 2 {
		if copied.Content[i].Value == key {
			copied.Content[i+1] = &yaml.Node{Kind: yaml.ScalarNode, Tag: "!!null", Value: "null"}
			break
		}
	}
	return &copied
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

// disableUnsupportedFlags rejects the --repository and --workspace root flags for profile
// subcommands, which operate on the profile store itself rather than any repository or workspace.
var disableUnsupportedFlags = common.DisableUnsupportedFlags("profile", "repository", "workspace")

// hideUnsupportedFlags hides the --repository and --workspace root flags from a profile
// subcommand's help output.
var hideUnsupportedFlags = common.HideUnsupportedFlags("repository", "workspace")
