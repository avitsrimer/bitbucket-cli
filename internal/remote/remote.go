package remote

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net/url"
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

// RedactedURL returns URL with any embedded userinfo password masked, safe to write to a log
// line. A git remote occasionally embeds a credential directly (e.g. an https remote of the form
// "https://user:app-password@bitbucket.org/workspace/repo.git"); url.Parse recognizes that form
// and url.URL.Redacted replaces the password with "xxxxx". A remote URL url.Parse cannot make
// sense of (e.g. the SSH short form "git@bitbucket.org:workspace/repo.git", which carries no
// password) is returned unchanged.
func (remote *Remote) RedactedURL() string {
	parsed, err := url.Parse(remote.URL)
	if err != nil || parsed.User == nil {
		return remote.URL
	}
	return parsed.Redacted()
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
// - If the remote URL's host is not a Bitbucket host (see isBitbucketHost), it returns an error
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
			if isBitbucketHost(remoteHost(url)) {
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
	if !isBitbucketHost(remoteHost(url)) {
		return nil, fmt.Errorf("argument remote is invalid (value: %s)", name)
	}
	return &Remote{
		URL:   url,
		Fetch: section.Key("fetch").String(),
	}, nil
}

// remoteHost returns the lower-cased host of a git remote URL, or "" when there is none.
//
// It understands the URL form scheme://[user[:password]@]host[:port]/path and the scp-like form
// [user@]host:path. Like git itself, an scp-like candidate whose host part contains a "/" is a
// local path, which has no host.
func remoteHost(rawURL string) string {
	if strings.Contains(rawURL, "://") {
		parsed, err := url.Parse(rawURL)
		if err != nil {
			return ""
		}
		return strings.ToLower(parsed.Hostname())
	}
	host, _, found := strings.Cut(rawURL, ":")
	if !found || strings.Contains(host, "/") {
		return ""
	}
	if at := strings.LastIndex(host, "@"); at >= 0 {
		host = host[at+1:]
	}
	return strings.ToLower(host)
}

// isBitbucketHost tells if a lower-cased remote host designates Bitbucket: bitbucket.org itself,
// one of its subdomains (e.g. altssh.bitbucket.org), or an ssh Host alias starting with
// "bitbucket" (e.g. bitbucket-work).
func isBitbucketHost(host string) bool {
	return host == "bitbucket.org" || strings.HasSuffix(host, ".bitbucket.org") || strings.HasPrefix(host, "bitbucket")
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
