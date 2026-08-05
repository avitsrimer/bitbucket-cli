package task

import (
	"strconv"
	"testing"
	"time"

	"github.com/avitsrimer/bitbucket-cli/internal/common"
	"github.com/avitsrimer/bitbucket-cli/internal/user"
)

// TestTaskGetRowAllDocumentedColumns is a regression test: GetRow used to switch on the raw
// underscore spelling ("created_on", "resolved_by", ...) while GetHeaders maps a --columns value
// through strings.ReplaceAll(column, "_", " "), turning "resolved_by" into "resolved by" before it
// ever reaches GetRow. Every documented column must produce its value regardless of which spelling
// (underscore or space, any case) is used.
func TestTaskGetRowAllDocumentedColumns(t *testing.T) {
	createdOn := time.Date(2024, 1, 2, 3, 4, 5, 0, time.UTC)
	updatedOn := time.Date(2024, 1, 3, 3, 4, 5, 0, time.UTC)
	resolvedOn := time.Date(2024, 1, 4, 3, 4, 5, 0, time.UTC)
	resolvedBy := user.User{Name: "Jane Doe"}

	target := Task{
		ID:         42,
		Content:    common.RenderedText{Raw: "do the thing"},
		Creator:    user.User{Name: "John Doe"},
		State:      "RESOLVED",
		IsPending:  false,
		ResolvedBy: &resolvedBy,
		CreatedOn:  createdOn,
		UpdatedOn:  updatedOn,
		ResolvedOn: &resolvedOn,
	}

	tests := []struct {
		header string
		want   string
	}{
		{"id", "42"},
		{"content", "do the thing"},
		{"creator", "John Doe"},
		{"created_on", createdOn.Format(time.RFC3339)},
		{"created on", createdOn.Format(time.RFC3339)},
		{"CREATED_ON", createdOn.Format(time.RFC3339)},
		{"updated_on", updatedOn.Format(time.RFC3339)},
		{"updated on", updatedOn.Format(time.RFC3339)},
		{"resolved_on", resolvedOn.Format(time.RFC3339)},
		{"resolved on", resolvedOn.Format(time.RFC3339)},
		{"state", "RESOLVED"},
		{"resolved_by", "Jane Doe"},
		{"resolved by", "Jane Doe"},
		{"RESOLVED_BY", "Jane Doe"},
		{"pending", strconv.FormatBool(false)},
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

// TestTaskGetRowBlanksNilResolvedFields proves resolved_on/resolved_by render blank rather than
// panicking when the task has not been resolved yet.
func TestTaskGetRowBlanksNilResolvedFields(t *testing.T) {
	target := Task{ID: 1}
	row := target.GetRow([]string{"resolved on", "resolved by"})
	if len(row) != 2 || row[0] != "" || row[1] != "" {
		t.Errorf("GetRow() = %v, want two blank cells", row)
	}
}

// TestTaskGetRowUnknownColumnIsBlank proves an unrecognized column produces a blank cell instead
// of panicking or being silently dropped from the row.
func TestTaskGetRowUnknownColumnIsBlank(t *testing.T) {
	target := Task{ID: 1}
	row := target.GetRow([]string{"not_a_real_column"})
	if len(row) != 1 || row[0] != "" {
		t.Errorf("GetRow() = %v, want a single blank cell", row)
	}
}

// TestTaskGetHeadersDefaultColumnsAllProduceValues proves every column in GetHeaders' own default
// list is recognized by GetRow (i.e. the two are actually kept in sync).
func TestTaskGetHeadersDefaultColumnsAllProduceValues(t *testing.T) {
	target := Task{ID: 7, State: "OPEN"}
	headers := target.GetHeaders(nil)
	row := target.GetRow(headers)
	if len(row) != len(headers) {
		t.Fatalf("GetRow() returned %d cells for %d headers", len(row), len(headers))
	}
}
