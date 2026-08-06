package profile

import (
	"errors"
	"fmt"

	"github.com/avitsrimer/bitbucket-cli/internal/common"
	"github.com/spf13/cobra"
)

// secretFlagState summarizes which secret-related flags were explicitly given on the command
// line, factoring out the repeated cmd.Flags().Changed(...) checks that both `profile create` and
// `profile update` need in order to decide whether to read a secret from stdin, prompt for it
// interactively, or reject the command line outright.
type secretFlagState struct {
	user             bool
	password         bool
	passwordStdin    bool
	clientID         bool
	clientSecret     bool
	accessToken      bool
	accessTokenStdin bool
}

// readSecretFlagState reads secretFlagState off cmd's own flags.
func readSecretFlagState(cmd *cobra.Command) secretFlagState {
	changed := cmd.Flags().Changed
	return secretFlagState{
		user:             changed("user"),
		password:         changed("password"),
		passwordStdin:    changed("password-stdin"),
		clientID:         changed("client-id"),
		clientSecret:     changed("client-secret"),
		accessToken:      changed("access-token"),
		accessTokenStdin: changed("access-token-stdin"),
	}
}

// passwordSourceGiven reports whether a password was given either directly or via stdin.
func (state secretFlagState) passwordSourceGiven() bool {
	return state.password || state.passwordStdin
}

// accessTokenSourceGiven reports whether an access token was given either directly or via stdin.
func (state secretFlagState) accessTokenSourceGiven() bool {
	return state.accessToken || state.accessTokenStdin
}

// anyCredentialGiven reports whether any credential-related flag -- of any of the three
// authentication styles this package supports -- was given on the command line.
func (state secretFlagState) anyCredentialGiven() bool {
	return state.user || state.clientID || state.clientSecret ||
		state.passwordSourceGiven() || state.accessTokenSourceGiven()
}

// requireUserForPasswordSource rejects --password/--password-stdin given without --user: a
// password with no user to attach it to is meaningless. --user given without a password source is
// deliberately allowed through here -- the caller prompts for it interactively instead.
func requireUserForPasswordSource(state secretFlagState) error {
	if state.passwordSourceGiven() && !state.user {
		return errors.New("argument user is missing: --password/--password-stdin requires --user")
	}
	return nil
}

// applyStdinSecrets reads whichever of --password-stdin/--access-token-stdin was given into
// *password/*accessToken, leaving them untouched otherwise. Cobra's MarkFlagsMutuallyExclusive
// (registered by createCmd/updateCmd's init) already guarantees at most one of {password,
// password-stdin} and at most one of {access-token, access-token-stdin} was given, so at most one
// read happens per secret.
func applyStdinSecrets(cmd *cobra.Command, state secretFlagState, password, accessToken *string) error {
	if state.passwordStdin {
		secret, err := common.ReadSecretFromStdin(cmd)
		if err != nil {
			return fmt.Errorf("cannot read password from stdin: %w", err)
		}
		*password = secret
	}
	if state.accessTokenStdin {
		secret, err := common.ReadSecretFromStdin(cmd)
		if err != nil {
			return fmt.Errorf("cannot read access token from stdin: %w", err)
		}
		*accessToken = secret
	}
	return nil
}

// promptForSecret interactively prompts (echo disabled) for a password or API token belonging to
// who. It refuses outright, instead of hanging, when cmd's input is not a real, interactive
// terminal -- matching common.Confirm's own non-interactive handling -- naming
// --password-stdin/--access-token-stdin as the non-interactive alternative.
func promptForSecret(cmd *cobra.Command, who string) (string, error) {
	if !common.StdinIsInteractive(cmd) {
		return "", fmt.Errorf("no password or access token given for %s: pass --password/--password-stdin (or --access-token/--access-token-stdin), or run this command interactively to be prompted for one", who)
	}
	secret, err := common.ReadSecret(fmt.Sprintf("Password or API token for %s:", who))
	if err != nil {
		return "", fmt.Errorf("cannot read secret interactively: %w", err)
	}
	return secret, nil
}
