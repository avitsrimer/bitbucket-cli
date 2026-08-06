package profile_test

import (
	"net/url"
	"testing"

	"github.com/avitsrimer/bitbucket-cli/internal/profile"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestProfileGetRow(t *testing.T) {
	apiRoot, err := url.Parse("https://api.bitbucket.org")
	require.NoError(t, err)

	target := profile.Profile{
		Name:              "myprofile",
		Description:       "my description",
		Default:           true,
		APIRoot:           apiRoot,
		User:              "myuser",
		ClientID:          "myclientid",
		AccessToken:       "mytoken",
		DefaultPageLength: 25,
		DefaultRepository: "acme/myrepo",
	}

	headers := []string{"name", "description", "default", "apiroot", "accesstoken", "defaultpagelength", "defaultrepository", "unknownheader"}
	row := target.GetRow(headers)

	assert.Equal(t, []string{"myprofile", "my description", "true", "https://api.bitbucket.org", "mytoken", "25", "acme/myrepo", " "}, row)
}

func TestProfileGetRowBlanksEmptyAPIRootAndAccessToken(t *testing.T) {
	target := profile.Profile{Name: "myprofile"}

	row := target.GetRow([]string{"apiroot", "accesstoken"})

	assert.Equal(t, []string{" ", " "}, row)
}
