package profile

import (
	"context"
	"strconv"
	"strings"
	"testing"
	"unicode/utf8"

	"github.com/spf13/cobra"
)

// truncationFixtureRow is a minimal common.Tableable used to drive Profile.Print end to end
// without depending on any real domain package (pullrequest, comment, ...) for this
// table-cell-truncation-only concern. Its fields are exported so printJSON/printYAML -- which
// marshal the payload directly, never going through GetRow -- render it in full.
type truncationFixtureRow struct {
	ID          int    `json:"id" yaml:"id"`
	Description string `json:"description" yaml:"description"`
}

// GetHeaders implements common.Tableable.
func (f truncationFixtureRow) GetHeaders(*cobra.Command) []string {
	return []string{"id", "description"}
}

// GetRow implements common.Tableable.
func (f truncationFixtureRow) GetRow(headers []string) []string {
	row := make([]string, len(headers))
	for i, header := range headers {
		switch header {
		case "id":
			row[i] = strconv.Itoa(f.ID)
		case "description":
			row[i] = f.Description
		}
	}
	return row
}

// newTruncationCmd builds a throwaway *cobra.Command carrying only the "output" flag Profile.Print
// reads, defaulted to output so the test never needs cmd.Flags().Set/Changed.
func newTruncationCmd(t *testing.T, output string) *cobra.Command {
	t.Helper()
	cmd := &cobra.Command{Use: "test"}
	cmd.SetContext(context.Background())
	cmd.Flags().String("output", output, "")
	return cmd
}

// TestTruncateCell proves the cap leaves a short value unchanged, cuts a long value to exactly
// maxTableCellWidth runes with the final rune replaced by a single ellipsis, and collapses a
// multi-paragraph value into a single table line instead of expanding the row across as many
// lines as it has paragraphs.
func TestTruncateCell(t *testing.T) {
	long := strings.Repeat("a", maxTableCellWidth+4920) // a 5000-char cell must be ellipsized
	wantLong := strings.Repeat("a", maxTableCellWidth-1) + "…"

	tests := []struct {
		name string
		in   string
		want string
	}{
		{"leaves a short value unchanged", "short value", "short value"},
		{"ellipsizes a long value to exactly maxTableCellWidth runes", long, wantLong},
		{"collapses embedded newlines into a single line", "first paragraph.\n\nsecond paragraph.", "first paragraph. second paragraph."},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			if got := truncateCell(test.in); got != test.want {
				t.Errorf("truncateCell(%q) = %q, want %q", test.in, got, test.want)
			}
		})
	}
}

// TestTruncateTableRow proves truncateTableRow applies the cap cell-by-cell, leaving cells under
// the cap untouched and cutting cells over it to exactly maxTableCellWidth runes.
func TestTruncateTableRow(t *testing.T) {
	tests := []struct {
		name      string
		row       []string
		wantWidth []int // -1 means "unchanged", any other value is the exact wanted rune count
	}{
		{"every cell already short", []string{"short", "also short"}, []int{-1, -1}},
		{"a mix of short and over-the-cap cells", []string{"short", strings.Repeat("x", maxTableCellWidth+10)}, []int{-1, maxTableCellWidth}},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			got := truncateTableRow(test.row)
			for i, wantWidth := range test.wantWidth {
				if wantWidth == -1 {
					if got[i] != test.row[i] {
						t.Errorf("truncateTableRow()[%d] = %q, want it unchanged (%q)", i, got[i], test.row[i])
					}
					continue
				}
				if n := utf8.RuneCountInString(got[i]); n != wantWidth {
					t.Errorf("truncateTableRow()[%d] is %d runes long, want exactly %d", i, n, wantWidth)
				}
			}
		})
	}
}

// TestPrintTableTruncatesLongCellAndCapsEveryRenderedLine proves a table cell built from a
// 5000-character description does not blow the rendered table out to an unreadable width: it
// must be ellipsized, and no line printTable renders may exceed a bounded width (the cap itself
// plus tablewriter's own border/padding overhead).
func TestPrintTableTruncatesLongCellAndCapsEveryRenderedLine(t *testing.T) {
	longDescription := strings.Repeat("a", 5000)
	fixture := truncationFixtureRow{ID: 1, Description: longDescription}
	cmd := newTruncationCmd(t, "table")

	stdout := captureStdout(t, func() {
		if err := (Profile{}).Print(context.Background(), cmd, fixture); err != nil {
			t.Fatalf("Print() error = %v", err)
		}
	})

	if strings.Contains(stdout, longDescription) {
		t.Error("table output contains the full 5000-character description, want it truncated")
	}
	wantCell := strings.Repeat("a", maxTableCellWidth-1) + "…"
	if !strings.Contains(stdout, wantCell) {
		t.Errorf("table output = %q, want it to contain the ellipsized cell %q", stdout, wantCell)
	}

	const maxLineWidth = maxTableCellWidth + 40 // generous allowance for tablewriter's own borders/padding
	for line := range strings.SplitSeq(stdout, "\n") {
		if n := utf8.RuneCountInString(line); n > maxLineWidth {
			t.Errorf("rendered line %q is %d runes wide, want <= %d", line, n, maxLineWidth)
		}
	}
}

// TestPrintJSONAndYAMLRenderCompleteUntruncatedValue proves the same over-the-cap record that
// printTable truncates round-trips complete and untruncated through json and yaml: those formats
// marshal the payload directly and never go through printTable's truncation at all.
func TestPrintJSONAndYAMLRenderCompleteUntruncatedValue(t *testing.T) {
	longDescription := strings.Repeat("b", 5000)
	fixture := truncationFixtureRow{ID: 7, Description: longDescription}

	for _, format := range []string{"json", "yaml"} {
		t.Run(format, func(t *testing.T) {
			cmd := newTruncationCmd(t, format)

			stdout := captureStdout(t, func() {
				if err := (Profile{}).Print(context.Background(), cmd, fixture); err != nil {
					t.Fatalf("Print() error = %v", err)
				}
			})

			if !strings.Contains(stdout, longDescription) {
				t.Errorf("%s output does not contain the complete, untruncated 5000-character description", format)
			}
		})
	}
}
