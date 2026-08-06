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
	user              bool
	password          bool
	passwordStdin     bool
	clientID          bool
	clientSecret      bool
	clientSecretStdin bool
	accessToken       bool
	accessTokenStdin  bool
}

// readSecretFlagState reads secretFlagState off cmd's own flags.
func readSecretFlagState(cmd *cobra.Command) secretFlagState {
	changed := cmd.Flags().Changed
	return secretFlagState{
		user:              changed("user"),
		password:          changed("password"),
		passwordStdin:     changed("password-stdin"),
		clientID:          changed("client-id"),
		clientSecret:      changed("client-secret"),
		clientSecretStdin: changed("client-secret-stdin"),
		accessToken:       changed("access-token"),
		accessTokenStdin:  changed("access-token-stdin"),
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

// clientSecretSourceGiven reports whether a client secret was given either directly or via stdin.
func (state secretFlagState) clientSecretSourceGiven() bool {
	return state.clientSecret || state.clientSecretStdin
}

// requireUserForPasswordSource rejects --password/--password-stdin given without --user, unless
// hasExistingUser is true: a password with no user to attach it to is meaningless, but `profile
// update` already knows the profile's current user when one is not being changed in the same
// call, so rotating a password there (e.g. `op read ... | bb profile update work
// --password-stdin`) must not force the caller to retype --user just to satisfy this check.
// `profile create` has no existing profile to fall back to, so it always passes hasExistingUser
// as false. --user given without a password source is deliberately allowed through here in
// either case -- the caller prompts for it interactively instead.
func requireUserForPasswordSource(state secretFlagState, hasExistingUser bool) error {
	if state.passwordSourceGiven() && !state.user && !hasExistingUser {
		return errors.New("argument user is missing: --password/--password-stdin requires --user")
	}
	return nil
}

// requireClientIDForSecretSource rejects --client-secret/--client-secret-stdin given without
// --client-id, and --client-id given without any client-secret source, since a client ID needs
// its secret from exactly one place. This replaces a plain cobra.MarkFlagsRequiredTogether("client-
// id", "client-secret"): that helper only ever recognizes one fixed pair of flags as "given", so it
// cannot express "--client-id needs --client-secret *or* --client-secret-stdin" -- pairing it with
// --client-secret alone would force the OAuth2 client secret onto the command line even though
// --client-secret-stdin exists specifically so it need not be.
func requireClientIDForSecretSource(state secretFlagState) error {
	switch {
	case state.clientSecretSourceGiven() && !state.clientID:
		return errors.New("argument clientId is missing: --client-secret/--client-secret-stdin requires --client-id")
	case state.clientID && !state.clientSecretSourceGiven():
		return errors.New("argument clientSecret is missing: --client-id requires --client-secret or --client-secret-stdin")
	}
	return nil
}

// applyStdinSecrets reads whichever of --password-stdin/--access-token-stdin/--client-secret-stdin
// was given into *password/*accessToken/*clientSecret, leaving them untouched otherwise. Cobra's
// MarkFlagsMutuallyExclusive (registered by createCmd/updateCmd's init) already guarantees at most
// one secret source flag was given, so at most one read happens.
func applyStdinSecrets(cmd *cobra.Command, state secretFlagState, password, accessToken, clientSecret *string) error {
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
	if state.clientSecretStdin {
		secret, err := common.ReadSecretFromStdin(cmd)
		if err != nil {
			return fmt.Errorf("cannot read client secret from stdin: %w", err)
		}
		*clientSecret = secret
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
	secret, err := common.ReadSecret(fmt.Sprintf("Password or API token for %s:", who), "use --password-stdin or --access-token-stdin instead")
	if err != nil {
		return "", fmt.Errorf("cannot read secret interactively: %w", err)
	}
	return secret, nil
}
