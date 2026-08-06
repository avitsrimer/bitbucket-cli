package profile

import (
	"context"
	"path/filepath"
	"strings"
	"testing"

	"github.com/avitsrimer/bitbucket-cli/internal/common"
	"github.com/zalando/go-keyring"
)

// withCreateOptions saves/restores the package-level createOptions, matching withUpdateOptions's
// rationale in update_internal_test.go.
func withCreateOptions(t *testing.T) {
	t.Helper()
	old := createOptions
	t.Cleanup(func() { createOptions = old })
}

// TestCreatePasswordStdinReadsAndTrims proves `bb profile create -n foo -u alice
// --password-stdin` reads the password piped on stdin (trimming its trailing newline) instead of
// requiring --password on the command line, and stores it exactly like --password would.
func TestCreatePasswordStdinReadsAndTrims(t *testing.T) {
	keyring.MockInit()
	withCreateOptions(t)
	oldProfiles, oldCurrent := Profiles, Current
	oldConfig := common.CurrentConfig()
	t.Cleanup(func() {
		Profiles = oldProfiles
		Current = oldCurrent
		common.SetCurrentConfig(oldConfig)
	})
	Profiles = nil
	Current = nil
	common.SetCurrentConfig(&common.Config{Path: filepath.Join(t.TempDir(), "config-cli.yml"), Data: map[string]any{}})

	const (
		profileName = "password-stdin-test"
		user        = "alice"
		vaultKey    = "bitbucket-cli-test-password-stdin"
		piped       = "s3cr3t-piped-password\n"
	)

	cmd := newIsolatedCreateCmd()
	cmd.SetIn(strings.NewReader(piped))
	cmd.SetArgs([]string{
		"--name", profileName,
		"--user", user,
		"--password-stdin",
		"--vault-key", vaultKey,
	})
	cmd.SetContext(context.Background())

	if err := cmd.Execute(); err != nil {
		t.Fatalf("profile create error = %v", err)
	}

	credential, err := (Profile{}).GetCredentialFromVault(vaultKey, user)
	if err != nil {
		t.Fatalf("cannot get credential from vault: %v", err)
	}
	if credential.Password != "s3cr3t-piped-password" {
		t.Errorf("vault password = %q, want the piped password trimmed of its trailing newline", credential.Password)
	}
}

// TestUpdateAccessTokenStdinReadsAndTrims proves `bb profile update foo --access-token-stdin`
// reads the access token piped on stdin (trimming its trailing newline) and stores it in the
// vault exactly like --access-token would.
func TestUpdateAccessTokenStdinReadsAndTrims(t *testing.T) {
	keyring.MockInit()
	withUpdateOptions(t)
	withIsolatedProfilesConfig(t)

	const (
		profileName = "access-token-stdin-test"
		vaultKey    = "bitbucket-cli-test-access-token-stdin"
		piped       = "s3cr3t-piped-token\r\n"
	)

	target := &Profile{Name: profileName, VaultKey: vaultKey}
	Profiles = append(Profiles, target)

	cmd := newIsolatedUpdateCmd()
	cmd.SetIn(strings.NewReader(piped))
	cmd.SetArgs([]string{profileName, "--access-token-stdin"})
	cmd.SetContext(context.Background())

	if err := cmd.Execute(); err != nil {
		t.Fatalf("profile update error = %v", err)
	}

	credential, err := (Profile{}).GetCredentialFromVault(vaultKey, profileName)
	if err != nil {
		t.Fatalf("cannot get credential from vault: %v", err)
	}
	if credential.Password != "s3cr3t-piped-token" {
		t.Errorf("vault access token = %q, want the piped token trimmed of its trailing CRLF", credential.Password)
	}
}

// TestCreateSecretFlagsMutualExclusion proves the new stdin-secret flags reject the same
// nonsensical combinations gh/docker-style flags reject: a secret cannot be given both directly
// and via stdin, and at most one secret can come from stdin at a time.
func TestCreateSecretFlagsMutualExclusion(t *testing.T) {
	tests := []struct {
		name string
		args []string
	}{
		{"password and password-stdin", []string{"--name", "x", "--user", "u", "--password", "p", "--password-stdin"}},
		{"access-token and access-token-stdin", []string{"--name", "x", "--access-token", "t", "--access-token-stdin"}},
		{"password-stdin and access-token-stdin", []string{"--name", "x", "--user", "u", "--password-stdin", "--access-token-stdin"}},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			withCreateOptions(t)
			cmd := newIsolatedCreateCmd()
			cmd.SetIn(strings.NewReader("irrelevant\n"))
			cmd.SetArgs(tc.args)
			cmd.SetContext(context.Background())

			if err := cmd.Execute(); err == nil {
				t.Fatal("cmd.Execute() error = nil, want a mutual exclusion error")
			}
		})
	}
}

// TestUpdateSecretFlagsMutualExclusion mirrors TestCreateSecretFlagsMutualExclusion for `profile
// update`.
func TestUpdateSecretFlagsMutualExclusion(t *testing.T) {
	tests := []struct {
		name string
		args []string
	}{
		{"password and password-stdin", []string{"x", "--user", "u", "--password", "p", "--password-stdin"}},
		{"access-token and access-token-stdin", []string{"x", "--access-token", "t", "--access-token-stdin"}},
		{"password-stdin and access-token-stdin", []string{"x", "--user", "u", "--password-stdin", "--access-token-stdin"}},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			withUpdateOptions(t)
			withIsolatedProfilesConfig(t)
			Profiles = append(Profiles, &Profile{Name: "x"})

			cmd := newIsolatedUpdateCmd()
			cmd.SetIn(strings.NewReader("irrelevant\n"))
			cmd.SetArgs(tc.args)
			cmd.SetContext(context.Background())

			if err := cmd.Execute(); err == nil {
				t.Fatal("cmd.Execute() error = nil, want a mutual exclusion error")
			}
		})
	}
}

// TestRequireUserForPasswordSourceRejectsPasswordWithoutUser proves --password/--password-stdin
// given without --user is rejected outright with a message naming --user, rather than silently
// doing nothing or panicking on a username-less vault lookup.
func TestRequireUserForPasswordSourceRejectsPasswordWithoutUser(t *testing.T) {
	tests := []struct {
		name  string
		state secretFlagState
	}{
		{"password without user", secretFlagState{password: true}},
		{"password-stdin without user", secretFlagState{passwordStdin: true}},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			err := requireUserForPasswordSource(tc.state)
			if err == nil {
				t.Fatal("requireUserForPasswordSource() error = nil, want an error")
			}
			if !strings.Contains(err.Error(), "user") {
				t.Errorf("requireUserForPasswordSource() error = %q, want it to mention the missing --user", err)
			}
		})
	}
	if err := requireUserForPasswordSource(secretFlagState{user: true, password: true}); err != nil {
		t.Errorf("requireUserForPasswordSource() error = %v, want nil when --user is given alongside --password", err)
	}
	if err := requireUserForPasswordSource(secretFlagState{}); err != nil {
		t.Errorf("requireUserForPasswordSource() error = %v, want nil when neither --password nor --user is given", err)
	}
}

// TestPromptForSecretNonInteractiveErrorsNamingStdinFlags proves that when a password/access token
// is needed but nothing was given and the command's input is not a real, interactive terminal
// (cmd.SetIn's replaced reader, exactly like a CI run or a piped invocation), promptForSecret fails
// fast with a clear error naming --password-stdin/--access-token-stdin instead of hanging or
// silently reading whatever was piped in for something else.
func TestPromptForSecretNonInteractiveErrorsNamingStdinFlags(t *testing.T) {
	cmd := newIsolatedCreateCmd()
	cmd.SetIn(strings.NewReader("this is not a secret prompt answer\n"))

	_, err := promptForSecret(cmd, "alice")
	if err == nil {
		t.Fatal("promptForSecret() error = nil, want an error for non-interactive input")
	}
	if !strings.Contains(err.Error(), "--password-stdin") || !strings.Contains(err.Error(), "--access-token-stdin") {
		t.Errorf("promptForSecret() error = %q, want it to name --password-stdin and --access-token-stdin", err)
	}
	if !strings.Contains(err.Error(), "alice") {
		t.Errorf("promptForSecret() error = %q, want it to name the user it was prompting for", err)
	}
}

// TestCreateProcessNonInteractiveWithNoCredentialErrorsNamingPasswordStdin proves `bb profile
// create -n foo` run non-interactively (no --user/--client-id/--access-token and no secret piped
// in) fails fast with a clear error naming --password-stdin, rather than silently creating a
// credential-less profile or hanging waiting for a terminal that will never answer.
func TestCreateProcessNonInteractiveWithNoCredentialErrorsNamingPasswordStdin(t *testing.T) {
	withCreateOptions(t)
	oldProfiles, oldCurrent := Profiles, Current
	oldConfig := common.CurrentConfig()
	t.Cleanup(func() {
		Profiles = oldProfiles
		Current = oldCurrent
		common.SetCurrentConfig(oldConfig)
	})
	Profiles = nil
	Current = nil
	common.SetCurrentConfig(&common.Config{Path: filepath.Join(t.TempDir(), "config-cli.yml"), Data: map[string]any{}})

	cmd := newIsolatedCreateCmd()
	cmd.SetIn(strings.NewReader("")) // non-interactive: a replaced reader, not the real terminal
	cmd.SetArgs([]string{"--name", "no-credential-test"})
	cmd.SetContext(context.Background())

	err := cmd.Execute()
	if err == nil {
		t.Fatal("cmd.Execute() error = nil, want an error when no credential is given non-interactively")
	}
	if !strings.Contains(err.Error(), "--password-stdin") {
		t.Errorf("cmd.Execute() error = %q, want it to name --password-stdin", err)
	}
	if _, found := Profiles.Find("no-credential-test"); found {
		t.Error("profile was created despite the missing-credential error")
	}
}

// TestUpdateProcessDoesNotPromptWithoutCredentialFlags proves a `profile update foo
// --description ...` with no credential-related flag at all never prompts for a secret: unlike
// `profile create`, updating fields unrelated to credentials is a legitimate, common operation and
// must not be blocked on stdin/a terminal that was never meant to answer a secret prompt.
func TestUpdateProcessDoesNotPromptWithoutCredentialFlags(t *testing.T) {
	withUpdateOptions(t)
	withIsolatedProfilesConfig(t)

	target := &Profile{Name: "no-prompt-test"}
	Profiles = append(Profiles, target)

	cmd := newIsolatedUpdateCmd()
	cmd.SetIn(strings.NewReader("")) // would fail promptForSecret's interactive check if ever reached
	cmd.SetArgs([]string{"no-prompt-test", "--description", "just a description update"})
	cmd.SetContext(context.Background())

	if err := cmd.Execute(); err != nil {
		t.Fatalf("profile update error = %v, want nil: a metadata-only update must not require a credential", err)
	}
}
