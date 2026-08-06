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
