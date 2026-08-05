package profile_test

import (
	"net/url"
	"testing"

	"github.com/gildas/bitbucket-cli/cmd/profile"
	"github.com/stretchr/testify/assert"
)

func TestProfileGetRow(t *testing.T) {
	apiRoot, err := url.Parse("https://api.bitbucket.org")
	assert.NoError(t, err)

	target := profile.Profile{
		Name:              "myprofile",
		Description:       "my description",
		Default:           true,
		APIRoot:           apiRoot,
		User:              "myuser",
		ClientID:          "myclientid",
		AccessToken:       "mytoken",
		DefaultPageLength: 25,
	}

	headers := []string{"name", "description", "default", "apiroot", "accesstoken", "defaultpagelength", "unknownheader"}
	row := target.GetRow(headers)

	assert.Equal(t, []string{"myprofile", "my description", "true", "https://api.bitbucket.org", "mytoken", "25", " "}, row)
}

func TestProfileGetRowBlanksEmptyAPIRootAndAccessToken(t *testing.T) {
	target := profile.Profile{Name: "myprofile"}

	row := target.GetRow([]string{"apiroot", "accesstoken"})

	assert.Equal(t, []string{" ", " "}, row)
}
