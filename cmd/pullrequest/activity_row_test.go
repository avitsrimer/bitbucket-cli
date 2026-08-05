package pullrequest_test

import (
	"testing"
	"time"

	"github.com/gildas/bitbucket-cli/cmd/pullrequest"
	"github.com/gildas/bitbucket-cli/cmd/user"
	"github.com/stretchr/testify/assert"
)

func TestActivityGetRowForApproval(t *testing.T) {
	when := time.Date(2024, 1, 2, 3, 4, 5, 0, time.UTC)
	target := pullrequest.Activity{
		Approval: &pullrequest.ActivityApproval{
			Date: when,
			User: user.User{Name: "Jane Doe"},
		},
	}

	row := target.GetRow([]string{"date", "approved", "state", "user", "description"})

	assert.Equal(t, []string{"2024-01-02 03:04:05", "true", "N/A", "Jane Doe", " "}, row)
}

func TestActivityGetRowForUpdate(t *testing.T) {
	when := time.Date(2024, 1, 2, 3, 4, 5, 0, time.UTC)
	target := pullrequest.Activity{
		Update: &pullrequest.ActivityUpdate{
			Date:        when,
			State:       "OPEN",
			Author:      user.User{Name: "John Doe"},
			Description: "some description",
		},
	}

	row := target.GetRow([]string{"date", "approved", "state", "author", "description", "destination"})

	assert.Equal(t, []string{"2024-01-02 03:04:05", "false", "OPEN", "John Doe", "some description", " "}, row)
}
