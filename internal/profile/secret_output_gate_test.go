package profile_test

import (
	"io"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"

	"github.com/avitsrimer/bitbucket-cli/internal/common"
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

// clearCachedFlags resets each named flag back to its zero value on every profile subcommand
// that resolves it, so a test relying on a flag being unset really sees it unset. cobra caches a
// subcommand's merged flag set (and its parents' persistent flags) on the package-level command
// singleton the first time flag parsing runs, and never invalidates it for a later run built
// around a different root command -- so both the value and the Changed bit an earlier explicit
// `--output json` or `--dry-run` set survive into every subsequent run in the same test binary.
// Calling InheritedFlags() first is what forces cobra to build that merged set, since Flags()
// alone does not.
func clearCachedFlags(t *testing.T, names ...string) {
	t.Helper()
	for _, sub := range profile.Command.Commands() {
		sub.InheritedFlags()
		for _, name := range names {
			flag := sub.Flags().Lookup(name)
			if flag == nil {
				continue
			}
			if err := flag.Value.Set(flag.DefValue); err != nil {
				t.Fatalf("cannot reset the %s flag of %s: %v", name, sub.Name(), err)
			}
			flag.Changed = false
		}
	}
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

			output := runProfileCommand(t, configPath, test.args...)

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

// TestProfileDisplayCommandsShowSecretsWithExplicitOutputFlag pins the other side of the gate: an
// explicit -o json|yaml on the command line is the sanctioned scripting path for retrieving a
// stored secret, so each of the three display paths must print the real values for it.
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

				output := runProfileCommand(t, configPath, args...)

				for _, secret := range gatedSecrets {
					if !strings.Contains(output, secret) {
						t.Errorf("%v must still print the cleartext secret %s:\n%s", args, secret, output)
					}
				}
			})
		}
	}
}

// TestProfileGetSingleProfileConfigReportsDefaultTrue pins that `profile get <name>` marks the
// only profile of a single-profile config as the default one (get.go's len(Profiles) == 1
// adjustment), whichever way the json format was chosen -- the printed payload is built after
// that adjustment, so masking never renders it invisible.
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

			output := runProfileCommand(t, configPath, test.args...)

			if !strings.Contains(output, `"default": true`) {
				t.Errorf("%v must report the only profile of a single-profile config as the default one:\n%s", test.args, output)
			}
		})
	}
}

// gatedUpdatedDescription is the only field the persistence guard below changes, so the config
// file a save writes after a gate-closed display can be compared against a control save that ran
// no display command at all.
const gatedUpdatedDescription = "secret output gate updated description"

// runSecretGateCommandKeepingState drives the real profile command tree against configPath
// exactly like runProfileCommand, except that it leaves the package-level
// profile.Profiles/profile.Current collection alone instead of clearing it first, so an earlier
// invocation's in-memory state is what this one operates on.
func runSecretGateCommandKeepingState(t *testing.T, configPath string, args ...string) string {
	t.Helper()
	clearCachedFlags(t, "output")

	root := newTestRootCommand()
	root.AddCommand(profile.Command)
	if err := root.PersistentFlags().Set("config", configPath); err != nil {
		t.Fatalf("cannot set config flag: %v", err)
	}
	if err := common.Initialize(root); err != nil {
		t.Fatalf("cannot initialize config: %v", err)
	}
	root.SetArgs(args)

	return captureStdout(t, func() {
		if err := root.Execute(); err != nil {
			t.Fatalf("profile command %v failed: %v", args, err)
		}
	})
}

// runSecretGateSequence runs each invocation in order against configPath and returns their
// stdout. Only the first starts from a cleared Profiles/Current collection; every later one
// reuses the in-memory state its predecessor left behind, since profiles.Load is a no-op while
// the collection is populated and the config path is unchanged. That sharing is the whole point:
// reloading from disk between a display command and a save would reload the plaintext secrets
// too, hiding a secretMask the display path had left reachable from Profiles -- the exact
// corruption Profile.MarshalYAML being shared with profileForSave makes possible.
func runSecretGateSequence(t *testing.T, configPath string, invocations ...[]string) []string {
	t.Helper()
	profile.Profiles = nil
	profile.Current = nil

	outputs := make([]string, 0, len(invocations))
	for _, args := range invocations {
		outputs = append(outputs, runSecretGateCommandKeepingState(t, configPath, args...))
	}
	return outputs
}

// readSecretGateConfig returns the raw bytes of the config file at path as a string.
func readSecretGateConfig(t *testing.T, path string) string {
	t.Helper()
	content, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("cannot read the saved config: %v", err)
	}
	return string(content)
}

// TestGateClosedDisplayThenSaveKeepsPlaintextSecretsInConfigFile is the persistence guard: a
// gate-closed display command must not leave a masked profile reachable from the package-level
// Profiles collection, because Profile.MarshalYAML is shared with the persistence path
// (profileForSave embeds Profile to inherit it, and saveProfilesConfig marshals through it), so a
// secretMask reachable from Profiles would overwrite a real credential the very next time any
// command saves.
//
// The save is driven through the production path -- `profile update <name> --description ...`,
// which calls saveProfilesConfig -- because saveProfilesConfig is unexported and this test
// package is external. Each case's config is additionally compared byte-for-byte against a
// control save that ran no display command at all, so a secret merely blanked (rather than
// masked) is caught too.
func TestGateClosedDisplayThenSaveKeepsPlaintextSecretsInConfigFile(t *testing.T) {
	updateArgs := []string{"profile", "update", gatedProfileName, "--description", gatedUpdatedDescription}

	controlPath := writeSecretGateConfig(t, "json", true)
	func() {
		defer resetProfilesState()()
		runSecretGateSequence(t, controlPath, updateArgs)
	}()
	control := readSecretGateConfig(t, controlPath)

	tests := []struct {
		name        string
		displayArgs []string
	}{
		{
			name:        "list then save",
			displayArgs: []string{"profile", "list"},
		},
		{
			name:        "get then save",
			displayArgs: []string{"profile", "get", gatedProfileName},
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			defer resetProfilesState()()
			configPath := writeSecretGateConfig(t, "json", true)

			outputs := runSecretGateSequence(t, configPath, test.displayArgs, updateArgs)

			// the display command must really have been gate-closed, otherwise the rest of this
			// case would prove nothing about masking reaching the config file
			if !strings.Contains(outputs[0], displaySecretMask) {
				t.Fatalf("%v was expected to print masked secrets, so the save below is guarded against a real mask:\n%s", test.displayArgs, outputs[0])
			}

			saved := readSecretGateConfig(t, configPath)
			for _, secret := range gatedSecrets {
				if !strings.Contains(saved, secret) {
					t.Errorf("the config saved after %v no longer holds the plaintext secret %s:\n%s", test.displayArgs, secret, saved)
				}
			}
			if strings.Contains(saved, displaySecretMask) {
				t.Errorf("the config saved after %v holds %s, overwriting a real credential:\n%s", test.displayArgs, displaySecretMask, saved)
			}
			if !strings.Contains(saved, gatedUpdatedDescription) {
				t.Errorf("the config saved after %v did not record the updated description:\n%s", test.displayArgs, saved)
			}
			if saved != control {
				t.Errorf("the config saved after %v differs from a save with no display command before it:\ngot:\n%s\nwant:\n%s", test.displayArgs, saved, control)
			}
		})
	}
}

// the two profiles writeTwoTokenGateConfig writes, carrying DIFFERENT plaintext access tokens so
// the row-based output formats can be checked for per-value redaction.
const (
	firstTokenProfileName  = "secret-gate-token-profile-a"
	secondTokenProfileName = "secret-gate-token-profile-b"
	firstGatedAccessToken  = "PLAINTEXT-ACCESS-TOKEN-a1b2c3"
	secondGatedAccessToken = "PLAINTEXT-ACCESS-TOKEN-d4e5f6"
)

// redactedHashPattern matches one redactWithHash result, "REDACTED-" followed by the first ten
// hex digits of the value's sha256.
var redactedHashPattern = regexp.MustCompile(`REDACTED-[0-9a-f]{10}`)

// writeTwoTokenGateConfig writes a plain-YAML config file holding two profiles whose only
// difference that matters here is their plaintext access token, returning the file's path.
func writeTwoTokenGateConfig(t *testing.T) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), "config-cli.yml")
	content := "profiles:\n" +
		"    - name: " + firstTokenProfileName + "\n      default: true\n      accesstoken: " + firstGatedAccessToken + "\n" +
		"    - name: " + secondTokenProfileName + "\n      accesstoken: " + secondGatedAccessToken + "\n"
	if err := os.WriteFile(path, []byte(content), 0o600); err != nil {
		t.Fatalf("cannot write test config: %v", err)
	}
	return path
}

// TestRowFormatsRedactAccessTokenPerValue guards the row-based output formats (table, csv and
// tsv, which all share GetRow) against being handed a masked profile: GetRow renders the
// accesstoken cell as redactWithHash(profile.AccessToken), so feeding it secretMask would
// collapse the column to one identical REDACTED-<hash> for every profile, destroying the very
// property redactWithHash exists for -- repeated values stay distinguishable, distinct ones stay
// distinct.
//
// display_mask_test.go's assertions cannot catch that: they only check that a REDACTED- prefix is
// present, which one constant hash shared by every profile would satisfy. This test asserts
// DISTINCTNESS across two profiles carrying different tokens instead.
func TestRowFormatsRedactAccessTokenPerValue(t *testing.T) {
	for _, format := range []string{"table", "csv", "tsv"} {
		t.Run(format, func(t *testing.T) {
			defer resetProfilesState()()
			t.Setenv("BB_OUTPUT_FORMAT", "")
			configPath := writeTwoTokenGateConfig(t)

			output := runProfileCommand(t, configPath, "profile", "list", "--columns", "name,accesstoken", "--output", format)

			for _, token := range []string{firstGatedAccessToken, secondGatedAccessToken} {
				if strings.Contains(output, token) {
					t.Errorf("%s output rendered the raw access token %s:\n%s", format, token, output)
				}
			}
			if strings.Contains(output, displaySecretMask) {
				t.Errorf("%s output masked the accesstoken column with %s instead of redacting each value on its own:\n%s", format, displaySecretMask, output)
			}

			hashes := redactedHashPattern.FindAllString(output, -1)
			if len(hashes) != 2 {
				t.Fatalf("%s output must carry one REDACTED-<hash> accesstoken cell per profile, got %d:\n%s", format, len(hashes), output)
			}
			if hashes[0] == hashes[1] {
				t.Errorf("%s output redacted two different access tokens to the same %s, so the accesstoken column no longer distinguishes values:\n%s", format, hashes[0], output)
			}
		})
	}
}

// captureStderr redirects os.Stderr for the duration of fn and returns what was written. It is
// the stderr counterpart of display_mask_test.go's captureStdout, needed because common.WhatIf
// writes its "Dry run: ..." line straight to os.Stderr rather than to cmd.ErrOrStderr(), so
// neither a stdout capture nor cmd.SetErr can observe it.
func captureStderr(t *testing.T, fn func()) string {
	t.Helper()
	r, w, err := os.Pipe()
	if err != nil {
		t.Fatalf("cannot create pipe: %v", err)
	}
	original := os.Stderr
	os.Stderr = w
	defer func() { os.Stderr = original }()

	captured := make(chan string, 1)
	go func() {
		data, _ := io.ReadAll(r)
		captured <- string(data)
	}()

	fn()

	_ = w.Close()
	return <-captured
}

// dryRunCachedFlags names the flags a --dry-run case sets on the profile command singletons and
// which therefore have to be reset once the case is over, since cobra keeps the value visible to
// every later run in the same test binary (see clearCachedFlags).
var dryRunCachedFlags = []string{"dry-run", "current"}

// runDryRunCommand drives the real profile command tree against configPath like
// runProfileCommand does, returning its stdout and its stderr separately so the dry-run line can
// be asserted apart from whatever the command printed as output.
func runDryRunCommand(t *testing.T, configPath string, args ...string) (stdout, stderr string) {
	t.Helper()
	stderr = captureStderr(t, func() {
		stdout = runProfileCommand(t, configPath, args...)
	})
	return stdout, stderr
}

// TestProfileDisplayCommandsHonorDryRun pins that all four profile display paths stop at
// common.WhatIf's gate: --dry-run must report what would be shown on stderr and print no profile
// at all.
func TestProfileDisplayCommandsHonorDryRun(t *testing.T) {
	tests := []struct {
		name     string
		args     []string
		wantLine string
	}{
		{
			name:     "list",
			args:     []string{"profile", "list", "--dry-run"},
			wantLine: "Dry run: Showing profiles",
		},
		{
			name:     "get by name",
			args:     []string{"profile", "get", gatedProfileName, "--dry-run"},
			wantLine: "Dry run: Showing profile " + gatedProfileName,
		},
		{
			name:     "get current",
			args:     []string{"profile", "get", "--current", "--dry-run"},
			wantLine: "Dry run: Showing current profile",
		},
		{
			name:     "which",
			args:     []string{"profile", "which", "--dry-run"},
			wantLine: "Dry run: Showing current profile name",
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			defer resetProfilesState()()
			defer clearCachedFlags(t, dryRunCachedFlags...)
			t.Setenv("BB_OUTPUT_FORMAT", "")
			configPath := writeSecretGateConfig(t, "table", true)

			stdout, stderr := runDryRunCommand(t, configPath, test.args...)

			// the trailing newline makes the match exact, so "Showing current profile" cannot
			// pass a case expecting "Showing current profile name"
			if !strings.Contains(stderr, test.wantLine+"\n") {
				t.Errorf("%v did not report %q on stderr:\n%s", test.args, test.wantLine, stderr)
			}
			for _, printed := range []string{gatedProfileName, gatedUser, "|"} {
				if strings.Contains(stdout, printed) {
					t.Errorf("%v printed %q on stdout instead of stopping at the dry-run gate:\n%s", test.args, printed, stdout)
				}
			}
		})
	}
}
