package pullrequest_test

import (
	"testing"
	"time"

	"github.com/avitsrimer/bitbucket-cli/internal/pullrequest"
	"github.com/avitsrimer/bitbucket-cli/internal/user"
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

// TestActivityGetRowPullRequestColumn reproduces critical finding #2: GetRow had no case at all
// for the "pull_request" column, even though activityColumns declares it (and as the
// DefaultSorter) and the --columns flag admits it, so requesting it produced a 0-cell row for a
// 1-header table instead of the pull request's ID.
func TestActivityGetRowPullRequestColumn(t *testing.T) {
	target := pullrequest.Activity{
		PullRequest: pullrequest.PullRequestReference{ID: 42},
	}

	row := target.GetRow([]string{"pull_request"})

	assert.Equal(t, []string{"42"}, row)
}

// TestActivityGetRowUnknownColumnFillsPlaceholderInsteadOfShorteningTheRow reproduces the second
// half of critical finding #2: without a default case, a header GetRow does not recognize was
// simply skipped, producing a row with fewer cells than the header count it will be rendered
// against -- e.g. requesting all 13 headers via --columns all only ever produced up to 12 cells,
// shifting every subsequent value under the wrong header in table/csv/tsv output.
func TestActivityGetRowUnknownColumnFillsPlaceholderInsteadOfShorteningTheRow(t *testing.T) {
	target := pullrequest.Activity{
		PullRequest: pullrequest.PullRequestReference{ID: 42},
	}

	row := target.GetRow([]string{"pull_request", "not-a-real-column", "approved"})

	assert.Len(t, row, 3, "GetRow must return exactly one cell per header, even for an unrecognized one")
	assert.Equal(t, []string{"42", " ", "false"}, row)
}

// TestActivityGetRowAcceptsHyphenAndSpaceColumnSpellings proves GetRow now normalizes headers
// through common.NormalizeColumnKey the same way every other Tableable.GetRow does, so
// "closed by", "closed-by", and "closed_by" (the three forms --columns/GetHeaders can produce)
// are all accepted, not just the single literal spelling the old strings.ToLower(header) switch
// happened to match.
func TestActivityGetRowAcceptsHyphenAndSpaceColumnSpellings(t *testing.T) {
	target := pullrequest.Activity{
		Update: &pullrequest.ActivityUpdate{ClosedBy: user.User{Name: "Jane Doe"}},
	}

	for _, spelling := range []string{"closed by", "closed-by", "closed_by"} {
		row := target.GetRow([]string{spelling})
		assert.Equal(t, []string{"Jane Doe"}, row, "spelling %q", spelling)
	}
}
