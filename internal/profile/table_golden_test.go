package profile

import (
	"context"
	"os"
	"strings"
	"testing"

	"github.com/avitsrimer/bitbucket-cli/internal/common"
	"github.com/mattn/go-runewidth"
	"github.com/spf13/cobra"
)

// The golden constants below were captured verbatim from printTable running against
// kataras/tablewriter (commit 5cbbf79's implementation) using a throwaway capture harness (never
// committed) driving the exact same fixtures defined in the test table below. Any byte
// difference from these constants is a behavior regression in writeTable, not an acceptable
// rendering variation.
const (
	goldenNumericOnlyColumn = `+------+
|  ID  |
+------+
|    1 |
|    2 |
|   10 |
|   -5 |
| 3.14 |
| -0.5 |
+------+
`
	goldenMixedNumericTextColumn = `+-------------+
|    VALUE    |
+-------------+
|          42 |
| abc         |
|        3.14 |
| hello world |
+-------------+
`
	goldenEmptyCell = `+----+------+
| ID | NAME |
+----+------+
|  1 |      |
|  2 | bob  |
+----+------+
`
	goldenCjkEmojiCell = `+-----------+
|   NAME    |
+-----------+
| 宽宽宽    |
| 🎉party🎉 |
| plain     |
+-----------+
`
	goldenCellWithNewline = `+----+--------+
| ID | NOTES  |
+----+--------+
|  1 | line1  |
|    | line2  |
|    | line3  |
|  2 | single |
+----+--------+
`
	goldenRaggedFewerCells = `+---+---+---+
| A | B | C |
+---+---+---+
| x | y |
| 1 | 2 | 3 |
+---+---+---+
`
	goldenRaggedMoreCells = `+---+---+---+-------+
| A | B | C |       |
+---+---+---+-------+
| x | y | z | extra |
| 1 | 2 | 3 |
+---+---+---+-------+
`
	goldenHeadersWithUnderscoreAndDot = `+-----------------+------------+-----------+-----------+
| PULL REQUEST ID | CREATED ON | REPO NAME | V1.2 BETA |
+-----------------+------------+-----------+-----------+
|               1 | 2020-01-01 | myrepo    | ok        |
+-----------------+------------+-----------+-----------+
`
	goldenTruncatedFreeTextCell = `+----+----------------------------------------------------------------------------------+
| ID |                                   DESCRIPTION                                    |
+----+----------------------------------------------------------------------------------+
|  1 | aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa… |
+----+----------------------------------------------------------------------------------+
`
	goldenZeroRowsWithHeaders = `+----+------+
| ID | NAME |
+----+------+
+----+------+
`
	goldenNilHeaders                = "+\n+\n"
	goldenMultilineRowFillerLiteral = "+----+---+\n| ID | X |\n+----+---+\n|  1 |   |\n|  2 |    |\n+----+---+\n"
)

// goldenFixture is a minimal common.Tableables the test fully controls: headers and rows are
// supplied verbatim, including ragged rows no real domain type would ever produce, so every
// pinned tablewriter default behavior (numeric right-align, odd-gap-right header centering,
// Title() header rewriting, multi-line cells, ragged rows, runewidth-based widths) can be
// exercised directly through printTable end to end.
type goldenFixture struct {
	headers []string
	rows    [][]string
}

func (f goldenFixture) GetHeaders(*cobra.Command) []string { return f.headers }
func (f goldenFixture) Size() int                          { return len(f.rows) }
func (f goldenFixture) GetRowAt(index int, _ []string) []string {
	return f.rows[index]
}

// TestPrintTableGoldenMatrix drives printTable end to end (through tableableRows and
// truncateTableRow, exactly as the real `bb ... -o table` path does) against a fixed matrix
// covering every pinned tablewriter default behavior, and asserts byte-for-byte equality with
// the golden constants captured from kataras/tablewriter above.
func TestPrintTableGoldenMatrix(t *testing.T) {
	cmd := &cobra.Command{Use: "test"}
	cmd.SetContext(context.Background())

	tests := []struct {
		name    string
		headers []string
		rows    [][]string
		want    string
	}{
		{
			name:    "numeric-only column right-aligns every cell",
			headers: []string{"id"},
			rows:    [][]string{{"1"}, {"2"}, {"10"}, {"-5"}, {"3.14"}, {"-0.5"}},
			want:    goldenNumericOnlyColumn,
		},
		{
			name:    "mixed numeric/text column aligns each cell independently",
			headers: []string{"value"},
			rows:    [][]string{{"42"}, {"abc"}, {"3.14"}, {"hello world"}},
			want:    goldenMixedNumericTextColumn,
		},
		{
			name:    "empty cell (common.EmptyCell) renders blank",
			headers: []string{"id", "name"},
			rows:    [][]string{{"1", common.EmptyCell}, {"2", "bob"}},
			want:    goldenEmptyCell,
		},
		{
			name:    "CJK and emoji cells size columns by display width, not rune count",
			headers: []string{"name"},
			rows:    [][]string{{"宽宽宽"}, {"🎉party🎉"}, {"plain"}},
			want:    goldenCjkEmojiCell,
		},
		{
			name:    "a cell containing \\n expands its row to multiple lines",
			headers: []string{"id", "notes"},
			rows:    [][]string{{"1", "line1\nline2\nline3"}, {"2", "single"}},
			want:    goldenCellWithNewline,
		},
		{
			name:    "a ragged row with fewer cells than headers renders narrower than the borders",
			headers: []string{"a", "b", "c"},
			rows:    [][]string{{"x", "y"}, {"1", "2", "3"}},
			want:    goldenRaggedFewerCells,
		},
		{
			name:    "a ragged row with more cells than headers widens every border",
			headers: []string{"a", "b", "c"},
			rows:    [][]string{{"x", "y", "z", "extra"}, {"1", "2", "3"}},
			want:    goldenRaggedMoreCells,
		},
		{
			name:    "headers with _ and . are rewritten by Title() -- .2 stays attached to its digits",
			headers: []string{"pull_request.id", "created_on", "repo.name", "v1.2_beta"},
			rows:    [][]string{{"1", "2020-01-01", "myrepo", "ok"}},
			want:    goldenHeadersWithUnderscoreAndDot,
		},
		{
			name:    "an 80-column-truncated free-text cell keeps its ellipsis",
			headers: []string{"id", "description"},
			rows:    [][]string{{"1", strings.Repeat("a", 200)}},
			want:    goldenTruncatedFreeTextCell,
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			fixture := goldenFixture{headers: test.headers, rows: test.rows}
			got := captureStdout(t, func() {
				if err := (Profile{}).printTable(cmd, fixture); err != nil {
					t.Fatalf("printTable() error = %v", err)
				}
			})
			if got != test.want {
				t.Errorf("printTable() output =\n%s\nwant:\n%s", got, test.want)
			}
		})
	}
}

// TestPrintTableGoldenZeroRowsWithHeaders covers the one matrix case printTable's own
// tableableRows() never reaches through the real Tableables path (see tableableRows' doc
// comment: a genuinely empty Tableables never calls GetHeaders, so headers stays nil): headers
// set, but not a single row ever appended. It drives writeTable directly with the same headers
// and zero rows -- the shape printTable would produce were headers non-nil but rows empty, and
// the shape writeTable itself must handle correctly regardless of how printTable's own
// headers-nil guard (see TestPrintTableGoldenNilHeadersIgnoresRows) is reached.
func TestPrintTableGoldenZeroRowsWithHeaders(t *testing.T) {
	got := captureStdout(t, func() {
		writeTable(os.Stdout, []string{"id", "name"}, nil)
	})
	if got != goldenZeroRowsWithHeaders {
		t.Errorf("writeTable() output =\n%s\nwant:\n%s", got, goldenZeroRowsWithHeaders)
	}
}

// TestPrintTableGoldenNilHeadersIgnoresRows pins printTable's headers-nil guard: when headers
// comes back nil, printTable renders only the bare top/bottom rule lines and discards rows
// entirely, matching upstream tablewriter's NewWriter().Render() with no SetHeader/Append call
// ever made. GetHeaders returning nil while Size() > 0 cannot happen through any --columns flag
// today (EnumSliceFlag.Set rejects an empty value), so this drives it directly through a fixture
// whose GetHeaders always returns nil despite non-empty rows, proving those rows are discarded
// rather than rendered without a header.
func TestPrintTableGoldenNilHeadersIgnoresRows(t *testing.T) {
	cmd := &cobra.Command{Use: "test"}
	cmd.SetContext(context.Background())

	fixture := goldenFixture{headers: nil, rows: [][]string{{"1", "should never render"}}}
	got := captureStdout(t, func() {
		if err := (Profile{}).printTable(cmd, fixture); err != nil {
			t.Fatalf("printTable() error = %v", err)
		}
	})
	if got != goldenNilHeaders {
		t.Errorf("printTable() output =\n%s\nwant:\n%s", got, goldenNilHeaders)
	}
}

// TestWriteTableMultilineRowFillerUsesLiteralTwoSpaces pins upstream tablewriter's own printRow
// padding behavior: a row cell shorter than its row's tallest cell is padded out with the two
// literal space characters "  ", not an empty string, before that filler line is aligned to the
// column's width. For a column narrower than two display columns (unreachable with any header
// this codebase ships -- see TestNoHeaderExceedsWrapWidth -- but well within writeTable's own
// contract), the literal's own two-column width wins over the narrower target width, producing a
// filler line one column wider than every other line in that column. writeTable reproduces this
// byte-for-byte per this cycle's compatibility contract even though it never surfaces in
// production output.
func TestWriteTableMultilineRowFillerUsesLiteralTwoSpaces(t *testing.T) {
	got := captureStdout(t, func() {
		writeTable(os.Stdout, []string{"id", "x"}, [][]string{{"1\n2", ""}})
	})
	if got != goldenMultilineRowFillerLiteral {
		t.Errorf("writeTable() output =\n%s\nwant:\n%s", got, goldenMultilineRowFillerLiteral)
	}
}

// TestNoHeaderExceedsWrapWidth pins the claim table.go's doc comments rely on: every default
// header this codebase ships (every Column[T].Name literal across internal/*/*.go and
// internal/*/*/*.go, collected by grep -- internal/profile cannot import the packages that
// define these Columns[T] tables directly, since every one of them already imports
// internal/profile, which would be an import cycle) stays well under maxHeaderWidth, so upstream
// tablewriter's word-wrap-at-30 (active only while SetHeader parses headers, before printTable's
// SetAutoWrapText(false) takes effect) never actually triggers for any header this renderer must
// reproduce, and writeTable's simpler "split on \n only" is safe to use in its place.
//
// This list is a snapshot: a --columns flag override supplies arbitrary user-typed text with no
// length bound this test -- or any static test -- could enforce, and a future header literal
// added to a Columns[T] table without updating this list would not be caught here. Both are
// accepted, deliberate limits of a guard test that can only check what is knowable at compile
// time from a package that cannot import its own callers.
func TestNoHeaderExceedsWrapWidth(t *testing.T) {
	headers := []string{
		"accesstoken", "account", "account_status", "apiroot", "approved", "author", "branch",
		"branching_model", "build_number", "callbackport", "clientid", "cloneprotocol",
		"cloneuser", "closed_by", "comments", "commit", "completed_on", "content", "created_on",
		"creator", "date", "default", "default_merge_strategy", "defaultpagelength",
		"defaultproject", "defaultrepository", "defaultworkspace", "deleted", "description",
		"destination", "downloads", "duration", "errorprocessing", "file", "fork_policy",
		"full_name", "has_issues", "has_wiki", "hash", "id", "image", "is_private", "language",
		"longhash", "main_branch", "max_time", "merge_strategies", "message", "name", "nickname",
		"outputformat", "owner", "parent", "participants", "pending", "progress", "project",
		"pull_request", "pullrequest", "reason", "repository", "resolution", "resolved_by",
		"resolved_on", "run_number", "size", "slug", "source", "sshkeyfilename", "started_on",
		"state", "target", "tasks", "title", "updated_on", "user", "username", "uuid",
		"vaultkey", "workspace",
	}

	for _, header := range headers {
		if width := runewidth.StringWidth(header); width >= maxHeaderWidth {
			t.Errorf("header %q has display width %d, want < %d (upstream tablewriter would word-wrap it)", header, width, maxHeaderWidth)
		}
	}
}
