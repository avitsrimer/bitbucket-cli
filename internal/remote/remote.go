package remote

import (
	"context"
	"errors"
	"fmt"
	"io"
	"regexp"
	"strings"

	"github.com/avitsrimer/bitbucket-cli/internal/common"
	"github.com/spf13/cobra"
)

// Remote represents a remote repository in a git configuration
type Remote struct {
	URL   string
	Fetch string
}

// GetRemoteFromGitConfig gets a remote from the git configuration
func GetRemoteFromGitConfig(context context.Context, name string) (remote *Remote, err error) {
	file, err := common.OpenGitConfig(context)
	if err != nil {
		return nil, fmt.Errorf("cannot open git config: %w", err)
	}
	defer file.Close()
	return GetRemoteFromReader(context, file, name)
}

// GetRemoteFromReader gets a remote from a reader
//
// - If the name is empty, it gets the first bitbucket remote in the reader
//
// - If the remote URL does not contain "bitbucket.org", it returns an error
func GetRemoteFromReader(context context.Context, reader io.Reader, name string) (remote *Remote, err error) {
	if name == "" {
		sections, sectionsErr := common.GetGitSectionsMatching(context, reader, regexp.MustCompile("remote \".*\""))
		if sectionsErr != nil {
			return nil, fmt.Errorf("cannot read git config: %w", sectionsErr)
		}
		if len(sections) == 0 {
			return nil, errors.New("no remote found")
		}
		for _, section := range sections {
			url := section.Key("url").String()
			if strings.Contains(url, "bitbucket.org") {
				return &Remote{
					URL:   url,
					Fetch: section.Key("fetch").String(),
				}, nil
			}
		}
		return nil, errors.New("no remote found")
	}
	section, err := common.GetGitSection(context, reader, "remote \""+name+"\"")
	if err != nil {
		return nil, fmt.Errorf("cannot read git config: %w", err)
	}
	url := section.Key("url").String()
	if !strings.Contains(url, "bitbucket.org") {
		return nil, fmt.Errorf("argument remote is invalid (value: %s)", name)
	}
	return &Remote{
		URL:   url,
		Fetch: section.Key("fetch").String(),
	}, nil
}

// GetRemote gets the remote from the command flags or the git configuration
//
// Checks the --git-remote flag first,
//
// Then checks the "origin" remote in the git configuration,
//
// Falls back to the first remote in the git configuration
func GetRemote(context context.Context, cmd *cobra.Command) (remote *Remote, err error) {
	if cmd.Flag("git-remote") != nil {
		remoteName := cmd.Flag("git-remote").Value.String()
		if remoteName != "" {
			return GetRemoteFromGitConfig(context, remoteName)
		}
	}
	if remote, err = GetRemoteFromGitConfig(context, "origin"); err == nil {
		return remote, nil
	}
	return GetRemoteFromGitConfig(context, "")
}

// RepositoryName gets the full repository name from the remote URL (without the .git extension)
func (remote Remote) RepositoryName() string {
	switch {
	case strings.HasPrefix(remote.URL, "bitbucket.org:"):
		if strings.HasSuffix(remote.URL, ".git") {
			return remote.URL[strings.Index(remote.URL, ":")+1 : len(remote.URL)-4]
		}
		return remote.URL[strings.Index(remote.URL, ":")+1:]
	case strings.HasPrefix(remote.URL, "git@"):
		if strings.HasSuffix(remote.URL, ".git") {
			return remote.URL[strings.LastIndex(remote.URL, ":")+1 : len(remote.URL)-4]
		}
		return remote.URL[strings.LastIndex(remote.URL, ":")+1:]
	case strings.HasPrefix(remote.URL, "https://"), strings.HasPrefix(remote.URL, "ssh://"):
		previousToLastSlash := strings.LastIndex(remote.URL[:strings.LastIndex(remote.URL, "/")], "/")
		if strings.HasSuffix(remote.URL, ".git") {
			return remote.URL[previousToLastSlash+1 : len(remote.URL)-4]
		}
		return remote.URL[previousToLastSlash+1:]
	}
	return remote.URL
}

// WorkspaceName gets the workspace name from the remote URL
func (remote Remote) WorkspaceName() string {
	repositoryName := remote.RepositoryName()
	workspace, _, found := strings.Cut(repositoryName, "/")
	if !found {
		return ""
	}
	return workspace
}
