package profile_test

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/avitsrimer/bitbucket-cli/internal/profile"
)

// the fixture profile written by writeSecretGateConfig: one profile carrying all three secrets
// (Password, ClientSecret, AccessToken) directly in the config file, the shape a `--no-vault`
// profile has on disk. Every value is distinctive enough that an accidental match against any
// other fixture in this package would be obvious, and long enough that no output format's own
// boilerplate could ever contain it by chance.
const (
	gatedProfileName  = "secret-output-gate-profile"
	gatedUser         = "secret-output-gate-user"
	gatedClientID     = "secret-output-gate-client-id"
	gatedPassword     = "PLAINTEXT-PASSWORD-3f9c2e"
	gatedClientSecret = "PLAINTEXT-CLIENT-SECRET-3f9c2e"
	gatedAccessToken  = "PLAINTEXT-ACCESS-TOKEN-3f9c2e"
)

// displaySecretMask mirrors the unexported profile.secretMask, the value forDisplay substitutes
// for a secret; this external test package cannot reference the constant itself.
const displaySecretMask = "********"

// gatedSecrets lists every secret value writeSecretGateConfig puts in the config file, so each
// assertion below covers all three rather than just the access token.
var gatedSecrets = []string{gatedPassword, gatedClientSecret, gatedAccessToken}

// writeSecretGateConfig writes a plain-YAML config file holding a single profile with all three
// secrets in cleartext and outputformat set to outputFormat (omitted when empty), returning the
// file's path. markDefault controls whether the profile carries `default: true` on disk: the
// tests that need profile.Current resolved deterministically set it, while the Default-ordering
// guard leaves it off so only get.go's own single-profile adjustment can produce it.
func writeSecretGateConfig(t *testing.T, outputFormat string, markDefault bool) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), "config-cli.yml")
	var content strings.Builder
	content.WriteString("profiles:\n    - name: " + gatedProfileName + "\n")
	content.WriteString("      description: secret output gate fixture\n")
	if markDefault {
		content.WriteString("      default: true\n")
	}
	content.WriteString("      user: " + gatedUser + "\n")
	content.WriteString("      password: " + gatedPassword + "\n")
	content.WriteString("      clientid: " + gatedClientID + "\n")
	content.WriteString("      clientsecret: " + gatedClientSecret + "\n")
	content.WriteString("      accesstoken: " + gatedAccessToken + "\n")
	if outputFormat != "" {
		content.WriteString("      outputformat: " + outputFormat + "\n")
	}
	if err := os.WriteFile(path, []byte(content.String()), 0o600); err != nil {
		t.Fatalf("cannot write test config: %v", err)
	}
	return path
}

// clearCachedOutputFlag resets the --output flag every profile subcommand resolves, so a test
// relying on the flag being unset really sees it unset. cobra caches a subcommand's merged
// flag set (and its parents' persistent flags) on the package-level command singleton the first
// time flag parsing runs, and never invalidates it for a later run built around a different root
// command -- so both the value and the Changed bit an earlier explicit `--output json` set
// survive into every subsequent run in the same test binary. Calling InheritedFlags() first is
// what forces cobra to build that merged set, since Flags() alone does not.
func clearCachedOutputFlag(t *testing.T) {
	t.Helper()
	for _, sub := range profile.Command.Commands() {
		sub.InheritedFlags()
		flag := sub.Flags().Lookup("output")
		if flag == nil {
			continue
		}
		if err := flag.Value.Set(""); err != nil {
			t.Fatalf("cannot reset the output flag of %s: %v", sub.Name(), err)
		}
		flag.Changed = false
	}
}

// runSecretGateCommand drives the real profile command tree against configPath and returns its
// stdout, clearing the cached --output flag first so args alone decide whether the format was
// requested explicitly.
func runSecretGateCommand(t *testing.T, configPath string, args ...string) string {
	t.Helper()
	clearCachedOutputFlag(t)
	return runProfileCommand(t, configPath, args...)
}

// TestProfileDisplayCommandsMaskSecretsWhenFormatNotExplicit pins that none of the four profile
// display paths may render a stored secret in cleartext when the json/yaml output format was
// chosen by the profile's own outputFormat or by BB_OUTPUT_FORMAT rather than by an explicit
// -o/--output on the command line: the secrets sit in the config file already, so gating the
// vault fetch changes nothing about what gets printed. Each case must print displaySecretMask
// in place of every secret.
func TestProfileDisplayCommandsMaskSecretsWhenFormatNotExplicit(t *testing.T) {
	tests := []struct {
		name                string
		args                []string
		profileOutputFormat string
		envOutputFormat     string
	}{
		{
			name:                "list with profile outputFormat json",
			args:                []string{"profile", "list"},
			profileOutputFormat: "json",
		},
		{
			name:                "list with BB_OUTPUT_FORMAT yaml",
			args:                []string{"profile", "list"},
			profileOutputFormat: "table",
			envOutputFormat:     "yaml",
		},
		{
			name:                "get with profile outputFormat json",
			args:                []string{"profile", "get", gatedProfileName},
			profileOutputFormat: "json",
		},
		{
			name:                "get with BB_OUTPUT_FORMAT yaml",
			args:                []string{"profile", "get", gatedProfileName},
			profileOutputFormat: "table",
			envOutputFormat:     "yaml",
		},
		{
			name:                "which with profile outputFormat json",
			args:                []string{"profile", "which"},
			profileOutputFormat: "json",
		},
		{
			name:                "which with BB_OUTPUT_FORMAT yaml",
			args:                []string{"profile", "which"},
			profileOutputFormat: "table",
			envOutputFormat:     "yaml",
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			defer resetProfilesState()()
			t.Setenv("BB_OUTPUT_FORMAT", test.envOutputFormat)
			configPath := writeSecretGateConfig(t, test.profileOutputFormat, true)

			output := runSecretGateCommand(t, configPath, test.args...)

			for _, secret := range gatedSecrets {
				if strings.Contains(output, secret) {
					t.Errorf("%v printed the cleartext secret %s without an explicit -o json|yaml:\n%s", test.args, secret, output)
				}
			}
			if !strings.Contains(output, displaySecretMask) {
				t.Errorf("%v did not mask its secrets with %s:\n%s", test.args, displaySecretMask, output)
			}
		})
	}
}

// TestProfileDisplayCommandsShowSecretsWithExplicitOutputFlag is the preserved-behavior
// counterpart: an explicit -o json|yaml on the command line is the sanctioned scripting path for
// retrieving a stored secret and must keep printing the real values.
func TestProfileDisplayCommandsShowSecretsWithExplicitOutputFlag(t *testing.T) {
	commands := map[string][]string{
		"list":  {"profile", "list"},
		"get":   {"profile", "get", gatedProfileName},
		"which": {"profile", "which"},
	}

	for _, command := range []string{"list", "get", "which"} {
		for _, format := range []string{"json", "yaml"} {
			t.Run(command+" with explicit -o "+format, func(t *testing.T) {
				defer resetProfilesState()()
				configPath := writeSecretGateConfig(t, "table", true)
				args := append(append([]string{}, commands[command]...), "--output", format)

				output := runSecretGateCommand(t, configPath, args...)

				for _, secret := range gatedSecrets {
					if !strings.Contains(output, secret) {
						t.Errorf("%v must still print the cleartext secret %s:\n%s", args, secret, output)
					}
				}
			})
		}
	}
}

// TestProfileGetSingleProfileConfigReportsDefaultTrue is the second preserved-behavior guard:
// `profile get <name>` marks the only profile of a single-profile config as the default one
// (get.go's len(Profiles) == 1 adjustment), whichever way the json format was chosen. It guards
// the ordering trap of masking before that adjustment runs.
func TestProfileGetSingleProfileConfigReportsDefaultTrue(t *testing.T) {
	tests := []struct {
		name                string
		profileOutputFormat string
		args                []string
	}{
		{
			name:                "explicit -o json",
			profileOutputFormat: "table",
			args:                []string{"profile", "get", gatedProfileName, "--output", "json"},
		},
		{
			name:                "profile outputFormat json",
			profileOutputFormat: "json",
			args:                []string{"profile", "get", gatedProfileName},
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			defer resetProfilesState()()
			configPath := writeSecretGateConfig(t, test.profileOutputFormat, false)

			output := runSecretGateCommand(t, configPath, test.args...)

			if !strings.Contains(output, `"default": true`) {
				t.Errorf("%v must report the only profile of a single-profile config as the default one:\n%s", test.args, output)
			}
		})
	}
}
