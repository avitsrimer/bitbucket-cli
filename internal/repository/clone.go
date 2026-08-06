package repository

import (
	"fmt"
	"net/url"
	"os"
	"os/exec"
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

var cloneOptions struct {
	Protocol       *common.EnumFlag
	SSHKeyFilename string
}

func init() {
	Command.AddCommand(cloneCmd)

	cloneOptions.Protocol = common.NewEnumFlag("git", "https", "ssh")
	cloneCmd.Flags().Var(cloneOptions.Protocol, "protocol", "Protocol to use for cloning (git, https, or ssh). Default is the profile's clone-protocol, or git")
	cloneCmd.Flags().StringVar(&cloneOptions.SSHKeyFilename, "ssh-key-file", "", "Path to the SSH private key file to use when cloning. Default is the profile's ssh-key-file, if any")
	_ = cloneCmd.RegisterFlagCompletionFunc(cloneOptions.Protocol.CompletionFunc("protocol"))
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

	workspaceSlug, err := repositoryWorkspaceSlug(target)
	if err != nil {
		return err
	}

	destination := target.Slug
	if len(args) > 1 {
		destination = args[1]
	}

	protocol := resolveProtocol(cloneOptions.Protocol.Value, currentProfile.CloneProtocol)

	lgr.Printf("[DEBUG] cloning %s/%s (protocol=%s) to %s", workspaceSlug, target.Slug, protocol, destination)
	if !common.WhatIf(cmd, "cloning %s/%s to %s", workspaceSlug, target.Slug, destination) {
		return nil
	}

	gitURL := buildCloneURL(protocol, workspaceSlug, target.Slug, currentProfile.CloneUser)

	gitCmd := exec.CommandContext(cmd.Context(), "git", "clone", gitURL, destination) //nolint:gosec // explicit argv, no shell: gitURL/destination are never shell-interpreted, so there is no injection vector here
	gitCmd.Stdin = os.Stdin
	gitCmd.Stdout = os.Stdout
	gitCmd.Stderr = os.Stderr

	if sshKeyFilename := resolveSSHKeyFilename(cloneOptions.SSHKeyFilename, currentProfile.SshKeyFilename); sshKeyFilename != "" {
		lgr.Printf("[DEBUG] using ssh key file %s", sshKeyFilename)
		gitCmd.Env = append(os.Environ(), "GIT_SSH_COMMAND=ssh -i "+sshKeyFilename)
	}

	if err := gitCmd.Run(); err != nil {
		return fmt.Errorf("git clone failed: %w", err)
	}
	return nil
}

// repositoryWorkspaceSlug resolves the workspace slug that owns repository, preferring its
// embedded Workspace object and falling back to the first component of FullName
// ("workspace-slug/repository-slug") when the API response omitted it.
func repositoryWorkspaceSlug(repository *Repository) (string, error) {
	if repository.Workspace != nil && repository.Workspace.Slug != "" {
		return repository.Workspace.Slug, nil
	}
	if parts := strings.SplitN(repository.FullName, "/", 2); len(parts) == 2 && parts[0] != "" {
		return parts[0], nil
	}
	return "", fmt.Errorf("cannot determine workspace for repository %s", repository.Slug)
}

// resolveProtocol resolves the clone protocol using --protocol, then profile.CloneProtocol, then
// "git" as the final default.
func resolveProtocol(flagValue, profileProtocol string) string {
	if flagValue != "" {
		return flagValue
	}
	if profileProtocol != "" {
		return profileProtocol
	}
	return "git"
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
	case "ssh":
		return fmt.Sprintf("ssh://git@bitbucket.org/%s/%s.git", workspaceSlug, repositorySlug)
	case "https":
		target := url.URL{Scheme: "https", Host: "bitbucket.org", Path: fmt.Sprintf("/%s/%s.git", workspaceSlug, repositorySlug)}
		if cloneUser != "" {
			target.User = url.User(cloneUser)
		}
		return target.String()
	default: // "git"
		return fmt.Sprintf("git@bitbucket.org:%s/%s.git", workspaceSlug, repositorySlug)
	}
}
