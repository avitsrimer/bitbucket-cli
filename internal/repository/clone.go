package repository

import (
	"fmt"
	"net/url"
	"os"
	"os/exec"
	"slices"
	"strings"

	"github.com/avitsrimer/bitbucket-cli/internal/common"
	"github.com/avitsrimer/bitbucket-cli/internal/profile"
	"github.com/go-pkgz/lgr"
	"github.com/spf13/cobra"
)

var cloneCmd = &cobra.Command{
	Use:               "clone [flags] <repository-slug-or-uuid> [destination]",
	Short:             "clone a repository by its <repository-slug-or-uuid>",
	Args:              cobra.RangeArgs(1, 2),
	ValidArgsFunction: cloneValidArgs,
	PreRunE:           common.DisableUnsupportedFlags("repository", "repository"),
	RunE:              cloneProcess,
}

// cloneProtocols are the transports buildCloneURL knows how to build a clone URL for -- the only
// values --protocol and a profile's clone-protocol field ever validate against.
const (
	cloneProtocolGit   = "git"
	cloneProtocolHTTPS = "https"
	cloneProtocolSSH   = "ssh"
)

var cloneProtocols = []string{cloneProtocolGit, cloneProtocolHTTPS, cloneProtocolSSH}

func init() {
	Command.AddCommand(cloneCmd)

	protocolFlag := common.NewEnumFlag(cloneProtocols...)
	cloneCmd.Flags().Var(protocolFlag, "protocol", "Protocol to use for cloning (git, https, or ssh). Default is the profile's clone-protocol, or git")
	cloneCmd.Flags().String("ssh-key-file", "", "Path to the SSH private key file to use when cloning. Default is the profile's ssh-key-file, if any")
	_ = cloneCmd.RegisterFlagCompletionFunc(protocolFlag.CompletionFunc("protocol"))
	_ = cloneCmd.MarkFlagFilename("ssh-key-file")
	cloneCmd.SetHelpFunc(common.HideUnsupportedFlags("repository"))
}

func cloneValidArgs(cmd *cobra.Command, args []string, toComplete string) ([]string, cobra.ShellCompDirective) {
	if len(args) != 0 {
		// the second, optional argument is a destination path: let the shell complete it normally
		return nil, cobra.ShellCompDirectiveDefault
	}

	slugs, err := GetRepositorySlugs(cmd.Context(), cmd)
	if err != nil {
		cobra.CompErrorln(err.Error())
		return []string{}, cobra.ShellCompDirectiveError
	}
	return common.FilterValidArgs(slugs, args, toComplete), cobra.ShellCompDirectiveNoFileComp
}

func cloneProcess(cmd *cobra.Command, args []string) error {
	currentProfile, err := profile.GetProfileFromCommand(cmd.Context(), cmd)
	if err != nil {
		return fmt.Errorf("cannot get profile: %w", err)
	}

	target, err := GetRepositoryBySlugOrID(cmd.Context(), cmd, args[0])
	if err != nil {
		return fmt.Errorf("cannot get repository %s: %w", args[0], err)
	}

	workspaceSlug, err := target.GetWorkspaceSlug(cmd.Context(), cmd)
	if err != nil {
		return err
	}

	destination := strings.TrimSuffix(target.Slug, ".git")
	if len(args) > 1 {
		destination = args[1]
	}

	protocolFlagValue, err := cmd.Flags().GetString("protocol")
	if err != nil {
		return fmt.Errorf("cannot read protocol flag: %w", err)
	}
	protocol, err := resolveProtocol(protocolFlagValue, currentProfile.CloneProtocol)
	if err != nil {
		return err
	}

	lgr.Printf("[DEBUG] cloning %s/%s (protocol=%s) to %s", workspaceSlug, target.Slug, protocol, destination)
	if !common.WhatIf(cmd, "cloning %s/%s to %s", workspaceSlug, target.Slug, destination) {
		return nil
	}

	gitURL := buildCloneURL(protocol, workspaceSlug, target.Slug, currentProfile.CloneUser)

	gitCmd := exec.CommandContext(cmd.Context(), "git", "clone", gitURL, destination) //nolint:gosec // explicit argv, no shell: gitURL/destination are never shell-interpreted, so there is no injection vector here
	gitCmd.Stdin = os.Stdin
	gitCmd.Stdout = os.Stdout
	gitCmd.Stderr = os.Stderr

	sshKeyFileFlagValue, err := cmd.Flags().GetString("ssh-key-file")
	if err != nil {
		return fmt.Errorf("cannot read ssh-key-file flag: %w", err)
	}

	// GIT_SSH_COMMAND is only meaningful for the git/ssh protocols: git never shells out over ssh
	// for an https remote, and setting it unconditionally would be misleading (and, per the
	// README, is documented as applying to git/ssh only).
	if sshKeyFilename := resolveSSHKeyFilename(sshKeyFileFlagValue, currentProfile.SshKeyFilename); sshKeyFilename != "" && protocol != cloneProtocolHTTPS {
		lgr.Printf("[DEBUG] using ssh key file %s", sshKeyFilename)
		// git passes GIT_SSH_COMMAND to /bin/sh, not an argv vector, so the key path must be
		// single-quoted and escaped here even though gitCmd's own argv above needs no quoting.
		gitCmd.Env = append(os.Environ(), "GIT_SSH_COMMAND=ssh -i "+shellQuoteSingle(sshKeyFilename))
	}

	if err := gitCmd.Run(); err != nil {
		return fmt.Errorf("git clone failed: %w", err)
	}
	return nil
}

// resolveProtocol resolves the clone protocol using --protocol, then profile.CloneProtocol, then
// cloneProtocolGit as the final default. profileProtocol is validated against the same allowed
// values as --protocol (cloneProtocols): an unrecognized value (e.g. a typo, or "http") is
// rejected rather than silently falling through buildCloneURL's default arm.
func resolveProtocol(flagValue, profileProtocol string) (string, error) {
	if flagValue != "" {
		return flagValue, nil
	}
	if profileProtocol != "" {
		if !slices.Contains(cloneProtocols, profileProtocol) {
			return "", fmt.Errorf("invalid clone-protocol %q in profile, expected one of: %s", profileProtocol, strings.Join(cloneProtocols, ", "))
		}
		return profileProtocol, nil
	}
	return cloneProtocolGit, nil
}

// shellQuoteSingle single-quotes s for safe inclusion in a string passed to /bin/sh (as
// GIT_SSH_COMMAND is), escaping any embedded single quotes.
func shellQuoteSingle(s string) string {
	return "'" + strings.ReplaceAll(s, "'", `'\''`) + "'"
}

// resolveSSHKeyFilename resolves the SSH private key file using --ssh-key-file, then
// profile.SshKeyFilename. An empty result means the system's own default key resolution (e.g.
// ssh-agent, ~/.ssh/config) applies, and GIT_SSH_COMMAND is left unset.
func resolveSSHKeyFilename(flagValue, profileSSHKeyFilename string) string {
	if flagValue != "" {
		return flagValue
	}
	return profileSSHKeyFilename
}

// buildCloneURL builds the repository clone URL for protocol ("git", "https", or "ssh").
//
// The returned URL may carry userinfo (the https form, when cloneUser is set) — callers must
// never log it.
func buildCloneURL(protocol, workspaceSlug, repositorySlug, cloneUser string) string {
	switch protocol {
	case cloneProtocolSSH:
		return fmt.Sprintf("ssh://git@bitbucket.org/%s/%s.git", workspaceSlug, repositorySlug)
	case cloneProtocolHTTPS:
		target := url.URL{Scheme: "https", Host: "bitbucket.org", Path: fmt.Sprintf("/%s/%s.git", workspaceSlug, repositorySlug)}
		if cloneUser != "" {
			target.User = url.User(cloneUser)
		}
		return target.String()
	default: // cloneProtocolGit
		return fmt.Sprintf("git@bitbucket.org:%s/%s.git", workspaceSlug, repositorySlug)
	}
}
