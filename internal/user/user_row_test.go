package user

import (
	"testing"
	"time"

	"github.com/avitsrimer/bitbucket-cli/internal/common"
)

// TestUserGetRowAllDocumentedColumns proves every documented column produces its value regardless
// of which spelling (underscore, space, or hyphen; any case) is used, since GetRow matches headers
// through common.NormalizeColumnKey.
func TestUserGetRowAllDocumentedColumns(t *testing.T) {
	id, err := common.ParseUUID("{11111111-1111-1111-1111-111111111111}")
	if err != nil {
		t.Fatalf("cannot parse fixture uuid: %v", err)
	}
	createdOn := time.Date(2024, 1, 2, 3, 4, 5, 0, time.UTC)

	target := User{
		ID:            id,
		Username:      "jdoe",
		Name:          "Jane Doe",
		Nickname:      "jane",
		AccountID:     "account-123",
		CreatedOn:     createdOn,
		AccountStatus: "active",
	}

	tests := []struct {
		header string
		want   string
	}{
		{"id", id.String()},
		{"ID", id.String()},
		{"username", "jdoe"},
		{"USERNAME", "jdoe"},
		{"name", "Jane Doe"},
		{"Name", "Jane Doe"},
		{"nickname", "jane"},
		{"NICKNAME", "jane"},
		{"account", "account-123"},
		{"ACCOUNT", "account-123"},
		{"created_on", createdOn.Format(common.TableTimeFormat)},
		{"created on", createdOn.Format(common.TableTimeFormat)},
		{"created-on", createdOn.Format(common.TableTimeFormat)},
		{"CREATED_ON", createdOn.Format(common.TableTimeFormat)},
		{"account_status", "active"},
		{"account status", "active"},
		{"account-status", "active"},
		{"ACCOUNT_STATUS", "active"},
	}

	for _, test := range tests {
		t.Run(test.header, func(t *testing.T) {
			row := target.GetRow([]string{test.header})
			if len(row) != 1 || row[0] != test.want {
				t.Errorf("GetRow([%q]) = %v, want [%q]", test.header, row, test.want)
			}
		})
	}
}

// TestUserGetRowUsernameFallsBackToNickname proves the username column falls back to Nickname
// when Username is empty.
func TestUserGetRowUsernameFallsBackToNickname(t *testing.T) {
	target := User{Nickname: "jane"}
	row := target.GetRow([]string{"username"})
	if len(row) != 1 || row[0] != "jane" {
		t.Errorf("GetRow() = %v, want [%q]", row, "jane")
	}
}

// TestUserGetRowBlanksZeroCreatedOn proves created_on renders as a blank cell rather than a
// zero-value timestamp when CreatedOn has never been set.
func TestUserGetRowBlanksZeroCreatedOn(t *testing.T) {
	target := User{}
	row := target.GetRow([]string{"created_on"})
	if len(row) != 1 || row[0] != " " {
		t.Errorf("GetRow() = %v, want a single blank cell", row)
	}
}

// TestUserGetRowBlanksEmptyAccountStatus proves account_status renders as a blank cell rather
// than an empty string when AccountStatus has never been set.
func TestUserGetRowBlanksEmptyAccountStatus(t *testing.T) {
	target := User{}
	row := target.GetRow([]string{"account_status"})
	if len(row) != 1 || row[0] != " " {
		t.Errorf("GetRow() = %v, want a single blank cell", row)
	}
}

// TestUserGetRowUnknownColumnIsBlank proves an unrecognized column produces a blank cell instead
// of panicking or being silently dropped from the row.
func TestUserGetRowUnknownColumnIsBlank(t *testing.T) {
	target := User{}
	row := target.GetRow([]string{"not_a_real_column"})
	if len(row) != 1 || row[0] != " " {
		t.Errorf("GetRow() = %v, want a single blank cell", row)
	}
}

// TestUserGetHeadersDefaultColumnsAllProduceValues proves every column in GetHeaders' own
// default list is recognized by GetRow (i.e. the two are actually kept in sync).
func TestUserGetHeadersDefaultColumnsAllProduceValues(t *testing.T) {
	id, err := common.ParseUUID("{11111111-1111-1111-1111-111111111111}")
	if err != nil {
		t.Fatalf("cannot parse fixture uuid: %v", err)
	}
	target := User{ID: id, Username: "jdoe", Name: "Jane Doe"}
	headers := target.GetHeaders(nil)
	row := target.GetRow(headers)
	if len(row) != len(headers) {
		t.Fatalf("GetRow() returned %d cells for %d headers", len(row), len(headers))
	}
	for i, cell := range row {
		if cell == "" || cell == " " {
			t.Errorf("GetRow() cell %d (header %q) = %q, want a non-blank value", i, headers[i], cell)
		}
	}
}
