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

// TestTruncateCellLeavesShortValueUnchanged proves the cap never touches a value already under
// it.
func TestTruncateCellLeavesShortValueUnchanged(t *testing.T) {
	const short = "short value"
	if got := truncateCell(short); got != short {
		t.Errorf("truncateCell(%q) = %q, want it unchanged", short, got)
	}
}

// TestTruncateCellEllipsizesLongValue proves a value over the cap is cut to exactly
// maxTableCellWidth runes, with the final rune replaced by a single ellipsis.
func TestTruncateCellEllipsizesLongValue(t *testing.T) {
	long := strings.Repeat("a", maxTableCellWidth+4920) // matches the 5000-char field report repro
	got := truncateCell(long)

	runes := []rune(got)
	if len(runes) != maxTableCellWidth {
		t.Fatalf("truncateCell() result is %d runes long, want exactly %d", len(runes), maxTableCellWidth)
	}
	want := strings.Repeat("a", maxTableCellWidth-1) + "…"
	if got != want {
		t.Errorf("truncateCell() = %q, want %q", got, want)
	}
}

// TestTruncateCellCollapsesEmbeddedNewlines proves a multi-paragraph value renders as a single
// table line instead of expanding the row across as many lines as it has paragraphs.
func TestTruncateCellCollapsesEmbeddedNewlines(t *testing.T) {
	got := truncateCell("first paragraph.\n\nsecond paragraph.")
	want := "first paragraph. second paragraph."
	if got != want {
		t.Errorf("truncateCell() = %q, want %q", got, want)
	}
}

// TestTruncateTableRowTruncatesEveryCellIndependently proves truncateTableRow applies the cap
// cell-by-cell, leaving cells under the cap untouched.
func TestTruncateTableRowTruncatesEveryCellIndependently(t *testing.T) {
	row := []string{"short", strings.Repeat("x", maxTableCellWidth+10)}
	got := truncateTableRow(row)

	if got[0] != "short" {
		t.Errorf("truncateTableRow()[0] = %q, want it unchanged", got[0])
	}
	if n := utf8.RuneCountInString(got[1]); n != maxTableCellWidth {
		t.Errorf("truncateTableRow()[1] is %d runes long, want exactly %d", n, maxTableCellWidth)
	}
}

// TestPrintTableTruncatesLongCellAndCapsEveryRenderedLine reproduces field report FR-4: a table
// cell built from a 5000-character description must not blow the rendered table out to an
// unreadable width. It must be ellipsized, and no line printTable renders may exceed a bounded
// width (the cap itself plus tablewriter's own border/padding overhead).
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
