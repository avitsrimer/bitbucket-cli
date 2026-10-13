package remote_test

import (
	"context"
	"strings"
	"testing"

	"github.com/avitsrimer/bitbucket-cli/internal/remote"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestCanGetRepositoryNameWithGitAt(t *testing.T) {
	payload := `
[core]
	repositoryformatversion = 0
	filemode = true
	bare = false
	logallrefupdates = true
[remote "origin"]
	url = git@bitbucket.org:myworkspace/bitbucket-cli.git
	fetch = +refs/heads/*:refs/remotes/origin/*
[remote "alternate"]
	url = git@bitbucket.org:myworkspace/bitbucket-cli
	fetch = +refs/heads/*:refs/remotes/origin/*
[branch "master"]
	remote = origin
	merge = refs/heads/master
[branch "dev"]
	remote = origin
	merge = refs/heads/dev
	`
	r, err := remote.GetRemoteFromReader(context.Background(), strings.NewReader(payload), "origin")
	require.NoError(t, err)
	assert.NotNil(t, r)
	assert.Equal(t, "myworkspace/bitbucket-cli", r.RepositoryName())

	r, err = remote.GetRemoteFromReader(context.Background(), strings.NewReader(payload), "alternate")
	require.NoError(t, err)
	assert.NotNil(t, r)
	assert.Equal(t, "myworkspace/bitbucket-cli", r.RepositoryName())
}

func TestCanGetRepositoryNameWithHTTPS(t *testing.T) {
	payload := `
[core]
	repositoryformatversion = 0
	filemode = true
	bare = false
	logallrefupdates = true
[remote "origin"]
	url = https://bitbucket.org/myworkspace/bitbucket-cli.git
	fetch = +refs/heads/*:refs/remotes/origin/*
[remote "alternate"]
	url = https://bitbucket.org/myworkspace/bitbucket-cli
	fetch = +refs/heads/*:refs/remotes/origin/*
[branch "master"]
	remote = origin
	merge = refs/heads/master
[branch "dev"]
	remote = origin
	merge = refs/heads/dev
	`
	r, err := remote.GetRemoteFromReader(context.Background(), strings.NewReader(payload), "origin")
	require.NoError(t, err)
	assert.NotNil(t, r)
	assert.Equal(t, "myworkspace/bitbucket-cli", r.RepositoryName())

	r, err = remote.GetRemoteFromReader(context.Background(), strings.NewReader(payload), "alternate")
	require.NoError(t, err)
	assert.NotNil(t, r)
	assert.Equal(t, "myworkspace/bitbucket-cli", r.RepositoryName())
}

func TestCanGetWorkspaceNameWithGitAt(t *testing.T) {
	payload := `
[core]
	repositoryformatversion = 0
	filemode = true
	bare = false
	logallrefupdates = true
[remote "origin"]
	url = git@bitbucket.org:myworkspace/bitbucket-cli.git
	fetch = +refs/heads/*:refs/remotes/origin/*
[remote "alternate"]
	url = git@bitbucket.org:myworkspace/bitbucket-cli
	fetch = +refs/heads/*:refs/remotes/origin/*
[branch "master"]
	remote = origin
	merge = refs/heads/master
[branch "dev"]
	remote = origin
	merge = refs/heads/dev
	`
	r, err := remote.GetRemoteFromReader(context.Background(), strings.NewReader(payload), "origin")
	require.NoError(t, err)
	assert.NotNil(t, r)
	assert.Equal(t, "myworkspace", r.WorkspaceName())

	r, err = remote.GetRemoteFromReader(context.Background(), strings.NewReader(payload), "alternate")
	require.NoError(t, err)
	assert.NotNil(t, r)
	assert.Equal(t, "myworkspace", r.WorkspaceName())
}

func TestCanGetWorkspaceNameWithHTTPS(t *testing.T) {
	payload := `
[core]
	repositoryformatversion = 0
	filemode = true
	bare = false
	logallrefupdates = true
[remote "origin"]
	url = https://bitbucket.org/myworkspace/bitbucket-cli.git
	fetch = +refs/heads/*:refs/remotes/origin/*
[remote "alternate"]
	url = https://bitbucket.org/myworkspace/bitbucket-cli
	fetch = +refs/heads/*:refs/remotes/origin/*
[branch "master"]
	remote = origin
	merge = refs/heads/master
[branch "dev"]
	remote = origin
	merge = refs/heads/dev
	`
	r, err := remote.GetRemoteFromReader(context.Background(), strings.NewReader(payload), "origin")
	require.NoError(t, err)
	assert.NotNil(t, r)
	assert.Equal(t, "myworkspace", r.WorkspaceName())

	r, err = remote.GetRemoteFromReader(context.Background(), strings.NewReader(payload), "alternate")
	require.NoError(t, err)
	assert.NotNil(t, r)
	assert.Equal(t, "myworkspace", r.WorkspaceName())
}

func TestCanGetWorkspaceNameWithoutSlash(t *testing.T) {
	r := remote.Remote{URL: "https://bitbucket.org"}
	assert.Empty(t, r.WorkspaceName())
}

// TestGetRemoteFromReaderReportsMissingRemoteAsNotFound reproduces the regression where
// GetGitSection used ini.File.Section (which fabricates and returns an empty section instead of
// reporting one is missing) instead of GetSection: a genuinely absent --git-remote name produced
// an empty, url-less section that read as "not a bitbucket remote" rather than "does not exist".
func TestGetRemoteFromReaderReportsMissingRemoteAsNotFound(t *testing.T) {
	payload := `
[remote "origin"]
	url = git@bitbucket.org:myworkspace/bitbucket-cli.git
	fetch = +refs/heads/*:refs/remotes/origin/*
	`
	_, err := remote.GetRemoteFromReader(context.Background(), strings.NewReader(payload), "does-not-exist")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "not found")
}

// TestRemoteRedactedURLMasksEmbeddedPassword proves an https remote URL carrying a userinfo
// password (e.g. an app password) never appears in RedactedURL's output -- the string GetRemote
// callers must log instead of Remote.URL.
func TestRemoteRedactedURLMasksEmbeddedPassword(t *testing.T) {
	target := &remote.Remote{URL: "https://myuser:super-secret-app-password@bitbucket.org/acme/widgets.git"}

	got := target.RedactedURL()
	if strings.Contains(got, "super-secret-app-password") {
		t.Errorf("RedactedURL() = %q, leaked the embedded password", got)
	}
	if !strings.Contains(got, "myuser") {
		t.Errorf("RedactedURL() = %q, want it to still name the user", got)
	}
	if !strings.Contains(got, "bitbucket.org/acme/widgets.git") {
		t.Errorf("RedactedURL() = %q, want the rest of the URL intact", got)
	}
}

// TestRemoteRedactedURLNoCredentials proves a remote URL carrying no userinfo (the common case)
// is returned unchanged.
func TestRemoteRedactedURLNoCredentials(t *testing.T) {
	target := &remote.Remote{URL: "https://bitbucket.org/acme/widgets.git"}

	got := target.RedactedURL()
	if got != target.URL {
		t.Errorf("RedactedURL() = %q, want it unchanged as %q", got, target.URL)
	}
}

// TestRemoteRedactedURLSSHShortFormUnchanged proves the SSH short form (which carries no
// password, so there is nothing to redact) is returned unchanged rather than mangled by url.Parse.
func TestRemoteRedactedURLSSHShortFormUnchanged(t *testing.T) {
	target := &remote.Remote{URL: "git@bitbucket.org:acme/widgets.git"}

	got := target.RedactedURL()
	if got != target.URL {
		t.Errorf("RedactedURL() = %q, want it unchanged as %q", got, target.URL)
	}
}

// remoteConfig renders a git config payload carrying a single remote with the given name and url.
func remoteConfig(name, url string) string {
	return "[remote \"" + name + "\"]\n\turl = " + url + "\n\tfetch = +refs/heads/*:refs/remotes/" + name + "/*\n"
}

func TestGetRemoteFromReaderAcceptsBitbucketHosts(t *testing.T) {
	tests := []string{
		"git@bitbucket.org:ws/repo.git",
		"git@bitbucket.org:ws/repo",
		"bitbucket.org:ws/repo.git",
		"https://bitbucket.org/ws/repo.git",
		"https://user@bitbucket.org/ws/repo.git",
		"https://user:secret@bitbucket.org/ws/repo.git",
		"ssh://git@bitbucket.org/ws/repo.git",
		"ssh://git@bitbucket.org:22/ws/repo.git",
		"git@bitbucket-work:ws/repo.git",
		"bitbucket-work:ws/repo.git",
		"ssh://git@bitbucket-work/ws/repo.git",
		"git@BitBucket-Work:ws/repo.git",
		"git@altssh.bitbucket.org:ws/repo.git",
		"ssh://git@bitbucket-work:22/ws/repo.git",
		"http://bitbucket.org/ws/repo.git",
		"git+ssh://git@bitbucket-work/ws/repo.git",
		"git@bitbucket-work:ws/repo.git/",
	}
	for _, url := range tests {
		t.Run(url, func(t *testing.T) {
			t.Run("named", func(t *testing.T) {
				r, err := remote.GetRemoteFromReader(context.Background(), strings.NewReader(remoteConfig("origin", url)), "origin")
				require.NoError(t, err)
				assert.Equal(t, url, r.URL)
			})
			t.Run("unnamed", func(t *testing.T) {
				r, err := remote.GetRemoteFromReader(context.Background(), strings.NewReader(remoteConfig("origin", url)), "")
				require.NoError(t, err)
				assert.Equal(t, url, r.URL)
			})
		})
	}
}

func TestGetRemoteFromReaderRejectsNonBitbucketHosts(t *testing.T) {
	tests := []string{
		"git@github.com:me/bitbucket-cli.git",
		"https://github.com/bitbucket/repo.git",
		"https://github.com/me/bitbucket.org.git",
		"git@gitlab.com:bitbucket/x.git",
		"github.com:bitbucket/x",
		"https://evil.com/@bitbucket/x.git",
		"../bitbucket-x/repo",
		"/srv/bitbucket:repo",
		"bitbucket-mirror/x:y",
		"",
	}
	for _, url := range tests {
		t.Run(url, func(t *testing.T) {
			t.Run("named", func(t *testing.T) {
				_, err := remote.GetRemoteFromReader(context.Background(), strings.NewReader(remoteConfig("origin", url)), "origin")
				require.Error(t, err)
				assert.Contains(t, err.Error(), "argument remote is invalid")
			})
			t.Run("unnamed", func(t *testing.T) {
				_, err := remote.GetRemoteFromReader(context.Background(), strings.NewReader(remoteConfig("origin", url)), "")
				require.Error(t, err)
				assert.Contains(t, err.Error(), "no remote found")
			})
		})
	}
}

func TestGetRemoteFromReaderUnnamedSkipsNonBitbucketRemote(t *testing.T) {
	payload := remoteConfig("github", "git@github.com:me/bitbucket-cli.git") + remoteConfig("work", "git@bitbucket-work:ws/repo.git")
	r, err := remote.GetRemoteFromReader(context.Background(), strings.NewReader(payload), "")
	require.NoError(t, err)
	assert.Equal(t, "git@bitbucket-work:ws/repo.git", r.URL)
}
