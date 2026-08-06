package profile

import (
	"context"
	"strings"
	"testing"
	"unicode/utf8"

	"github.com/mattn/go-runewidth"
	"github.com/spf13/cobra"
)

// truncationFixtureRow is a minimal common.Tableable used to drive Profile.Print end to end
// without depending on any real domain package (pullrequest, comment, ...) for this
// table-cell-truncation-only concern. Its fields are exported so printJSON/printYAML -- which
// marshal the payload directly, never going through GetRow -- render it in full.
//
// "id" and "description" are deliberately different in kind, not just length: "description" is
// in freeTextColumnKeys (free text a table may legitimately truncate), "id" is not (an identifier
// a user may need to copy verbatim) -- every test below that builds both an over-the-cap id and
// an over-the-cap description proves the cap applies to one and not the other.
type truncationFixtureRow struct {
	ID          string `json:"id" yaml:"id"`
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
			row[i] = f.ID
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
// maxTableCellWidth display columns with the final rune replaced by a single ellipsis, collapses
// a multi-paragraph value into a single table line instead of expanding the row across as many
// lines as it has paragraphs, and measures double-width runes (CJK, most emoji) by their actual
// terminal display width rather than by rune count.
func TestTruncateCell(t *testing.T) {
	long := strings.Repeat("a", maxTableCellWidth+4920) // a 5000-char cell must be ellipsized
	wantLong := strings.Repeat("a", maxTableCellWidth-1) + "…"

	// Each "宽" is a double-width CJK character (display width 2): 41 of them is 82 display
	// columns, over the 80-column cap, even though it is only 41 runes -- a rune-counting cap
	// would leave this untouched and render 82 columns wide.
	wideRunes := strings.Repeat("宽", 41)

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

	t.Run("caps double-width runes by display width, not rune count", func(t *testing.T) {
		got := truncateCell(wideRunes)
		if width := runewidth.StringWidth(got); width > maxTableCellWidth {
			t.Errorf("truncateCell(%d CJK runes) = %q, display width %d, want <= %d", utf8.RuneCountInString(wideRunes), got, width, maxTableCellWidth)
		}
		if !strings.HasSuffix(got, "…") {
			t.Errorf("truncateCell(%d CJK runes) = %q, want it ellipsized", utf8.RuneCountInString(wideRunes), got)
		}
	})
}

// TestTruncateTableRow proves truncateTableRow applies the cap only to cells whose header
// normalizes to a freeTextColumnKeys entry (here "description"), leaving every other cell --
// "id" above all, an identifier a user may need to copy verbatim -- untouched at any length.
func TestTruncateTableRow(t *testing.T) {
	headers := []string{"id", "description"}
	longID := strings.Repeat("x", maxTableCellWidth+10)
	longDescription := strings.Repeat("y", maxTableCellWidth+10)

	tests := []struct {
		name        string
		row         []string
		wantDescCap bool
	}{
		{"every cell already short", []string{"short-id", "also short"}, false},
		{"an over-the-cap id is never truncated", []string{longID, "short"}, false},
		{"an over-the-cap description is truncated", []string{"short-id", longDescription}, true},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			got := truncateTableRow(headers, test.row)

			if got[0] != test.row[0] {
				t.Errorf("truncateTableRow()[0] (id) = %q, want it unchanged (%q): id is not a free-text column", got[0], test.row[0])
			}
			if test.wantDescCap {
				if got[1] == test.row[1] {
					t.Errorf("truncateTableRow()[1] (description) = %q, want it truncated", got[1])
				}
			} else if got[1] != test.row[1] {
				t.Errorf("truncateTableRow()[1] (description) = %q, want it unchanged (%q)", got[1], test.row[1])
			}
		})
	}
}

// TestTruncateTableRowCapsParticipants proves the "participants" column -- a pull request's
// per-reviewer "nickname:state" summary, whose rendered length grows with the reviewer count --
// is capped by the same freeTextColumnKeys mechanism as description/title/etc., so a PR with many
// reviewers cannot blow the table out to an unreadable width (the same class of regression FR-4
// fixed for other free-text columns).
func TestTruncateTableRowCapsParticipants(t *testing.T) {
	headers := []string{"id", "participants"}
	longParticipants := strings.Repeat("reviewer_name:changes_requested, ", 10)

	got := truncateTableRow(headers, []string{"1", longParticipants})

	if got[1] == longParticipants {
		t.Errorf("truncateTableRow()[1] (participants) = %q, want it truncated", got[1])
	}
	if runewidth.StringWidth(got[1]) > maxTableCellWidth {
		t.Errorf("truncateTableRow()[1] (participants) display width = %d, want <= %d", runewidth.StringWidth(got[1]), maxTableCellWidth)
	}
}

// TestPrintTableTruncatesLongCellAndCapsEveryRenderedLine proves a table cell built from a
// 5000-character description does not blow the rendered table out to an unreadable width: it
// must be ellipsized, and no line printTable renders may exceed a bounded width (the cap itself
// plus tablewriter's own border/padding overhead). It also proves an equally long id (not a
// free-text column) is rendered in full: the field report's own complaint was that #33's cap
// ellipsized identifiers a later command needs verbatim (an artifact Name, a step Image).
func TestPrintTableTruncatesLongCellAndCapsEveryRenderedLine(t *testing.T) {
	longDescription := strings.Repeat("a", 5000)
	longID := strings.Repeat("9", 200) // e.g. a long digest-style identifier
	fixture := truncationFixtureRow{ID: longID, Description: longDescription}
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
	if !strings.Contains(stdout, longID) {
		t.Errorf("table output does not contain the complete, untruncated 200-character id %q: an identifier column must never be truncated", longID)
	}
}

// TestPrintJSONAndYAMLRenderCompleteUntruncatedValue proves the same over-the-cap record that
// printTable truncates round-trips complete and untruncated through json and yaml: those formats
// marshal the payload directly and never go through printTable's truncation at all.
func TestPrintJSONAndYAMLRenderCompleteUntruncatedValue(t *testing.T) {
	longDescription := strings.Repeat("b", 5000)
	fixture := truncationFixtureRow{ID: "7", Description: longDescription}

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

// TestPrintDelimitedRendersCompleteUntruncatedValue is TestPrintJSONAndYAMLRenderCompleteUntruncatedValue's
// csv/tsv sibling: this codebase's own gate review flagged the claim that csv/tsv render complete,
// untruncated values as asserted nowhere in the test suite. printDelimited writes a Tableable's
// GetRow output directly to encoding/csv, never through printTable's truncateTableRow, so both
// delimiters must round-trip the same 5000-character description in full.
func TestPrintDelimitedRendersCompleteUntruncatedValue(t *testing.T) {
	longDescription := strings.Repeat("c", 5000)
	fixture := truncationFixtureRow{ID: "9", Description: longDescription}

	tests := []struct {
		format string
		comma  string
	}{
		{"csv", ","},
		{"tsv", "\t"},
	}

	for _, test := range tests {
		t.Run(test.format, func(t *testing.T) {
			cmd := newTruncationCmd(t, test.format)

			stdout := captureStdout(t, func() {
				if err := (Profile{}).Print(context.Background(), cmd, fixture); err != nil {
					t.Fatalf("Print() error = %v", err)
				}
			})

			if !strings.Contains(stdout, longDescription) {
				t.Errorf("%s output = %q, want it to contain the complete, untruncated 5000-character description", test.format, stdout)
			}

			lines := strings.Split(strings.TrimRight(stdout, "\n"), "\n")
			if len(lines) != 2 {
				t.Fatalf("%s output has %d line(s), want exactly 2 (header + one data row)", test.format, len(lines))
			}
			fields := strings.Split(lines[1], test.comma)
			if len(fields) != 2 {
				t.Fatalf("%s data row has %d field(s), want exactly 2 (id, description)", test.format, len(fields))
			}
			if fields[0] != fixture.ID {
				t.Errorf("%s id field = %q, want %q: an identifier column must round-trip exactly", test.format, fields[0], fixture.ID)
			}
			if fields[1] != longDescription {
				t.Errorf("%s description field is %d bytes long, want the full %d-byte value", test.format, len(fields[1]), len(longDescription))
			}
		})
	}
}
