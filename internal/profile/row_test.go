package profile_test

import (
	"net/url"
	"strings"
	"testing"

	"github.com/avitsrimer/bitbucket-cli/internal/profile"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestProfileGetRow proves GetRow renders every plain column verbatim, but masks AccessToken (see
// GetHeaders' doc comment: it goes through redactWithHash rather than cleartext) -- this is the
// mechanism table/csv/tsv output (both share GetRow) relies on to never leak a live token.
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

	require.Len(t, row, len(headers))
	assert.Equal(t, []string{"myprofile", "my description", "true", "https://api.bitbucket.org"}, row[:4])
	assert.True(t, strings.HasPrefix(row[4], "REDACTED-"), "accesstoken column must be masked via redactWithHash, got %q", row[4])
	assert.NotContains(t, row[4], "mytoken", "accesstoken column must never render the raw token")
	assert.Equal(t, []string{"25", "acme/myrepo", " "}, row[5:])
}

func TestProfileGetRowBlanksEmptyAPIRootAndAccessToken(t *testing.T) {
	target := profile.Profile{Name: "myprofile"}

	row := target.GetRow([]string{"apiroot", "accesstoken"})

	assert.Equal(t, []string{" ", " "}, row)
}
