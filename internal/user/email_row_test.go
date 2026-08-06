package user

import (
	"strconv"
	"testing"
)

// TestEmailGetRowAllDocumentedColumns proves every documented column produces its value
// regardless of case or underscore/space spelling.
func TestEmailGetRowAllDocumentedColumns(t *testing.T) {
	target := Email{Email: "jane@example.com", IsPrimary: true, IsConfirmed: false}

	tests := []struct {
		header string
		want   string
	}{
		{"Email", "jane@example.com"},
		{"email", "jane@example.com"},
		{"EMAIL", "jane@example.com"},
		{"Is Primary", strconv.FormatBool(true)},
		{"is primary", strconv.FormatBool(true)},
		{"is_primary", strconv.FormatBool(true)},
		{"IS_PRIMARY", strconv.FormatBool(true)},
		{"Is Confirmed", strconv.FormatBool(false)},
		{"is confirmed", strconv.FormatBool(false)},
		{"is_confirmed", strconv.FormatBool(false)},
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

// TestEmailGetRowUnknownColumnIsBlank proves an unrecognized column produces a blank cell instead
// of panicking or being silently dropped from the row.
func TestEmailGetRowUnknownColumnIsBlank(t *testing.T) {
	target := Email{Email: "jane@example.com"}
	row := target.GetRow([]string{"not_a_real_column"})
	if len(row) != 1 || row[0] != " " {
		t.Errorf("GetRow() = %v, want a single blank cell", row)
	}
}

// TestEmailGetHeadersDefaultColumnsAllProduceValues proves every column in GetHeaders' own
// default list is recognized by GetRow (i.e. the two are kept in sync).
func TestEmailGetHeadersDefaultColumnsAllProduceValues(t *testing.T) {
	target := Email{Email: "jane@example.com", IsPrimary: true, IsConfirmed: true}
	headers := target.GetHeaders(nil)
	row := target.GetRow(headers)
	if len(row) != len(headers) {
		t.Fatalf("GetRow() returned %d cells for %d headers", len(row), len(headers))
	}
	for i, cell := range row {
		if cell == "" {
			t.Errorf("GetRow() cell %d (header %q) = \"\", want a non-blank value", i, headers[i])
		}
	}
}
