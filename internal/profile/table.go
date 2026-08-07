package profile

import (
	"io"
	"regexp"
	"strings"

	"github.com/mattn/go-runewidth"
)

// ansiEscape matches the ANSI escape sequences upstream tablewriter stripped before measuring a
// cell's display width.
var ansiEscape = regexp.MustCompile("\033\\[(?:\\d{1,3}(?:;\\d{1,3})*)?[m|K]")

// decimalCell matches, after TrimSpace, a cell upstream tablewriter's default column alignment
// treats as numeric and right-aligns: an optional leading run of "-", digits, an optional ".",
// more digits -- every part optional, so an empty string matches too (a blank filler line inside
// a multi-line cell renders identically either way it is aligned). Upstream also consulted a
// second "percent" pattern (`^-*\d*\.?\d*$%$`), but that pattern can never match any string --
// the `$` anchors sandwich a literal `%`, which is unsatisfiable -- so it contributes nothing and
// has no counterpart here.
var decimalCell = regexp.MustCompile(`^-*\d*\.?\d*$`)

// maxHeaderWidth is the width past which upstream tablewriter's SetHeader -- parsed while
// autoWrap was still on, since it runs before printTable's SetAutoWrapText(false) -- would start
// word-wrapping a header into multiple lines at its 30-column default. Every GetHeaders in this
// codebase returns headers well under that width (see TestNoHeaderExceedsWrapWidth), so this
// renderer treats a header exactly like a row cell -- split on "\n" only, never word-wrapped --
// without reproducing that wrapping algorithm.
const maxHeaderWidth = 30

// displayWidth is runewidth.StringWidth after stripping ANSI escape sequences, the same measure
// upstream tablewriter used to size columns.
func displayWidth(s string) int {
	return runewidth.StringWidth(ansiEscape.ReplaceAllLiteralString(s, ""))
}

// maxLineWidth returns the widest display width among lines, or 0 for an empty slice.
func maxLineWidth(lines []string) int {
	width := 0
	for _, line := range lines {
		if w := displayWidth(line); w > width {
			width = w
		}
	}
	return width
}

// titleHeader reproduces upstream tablewriter's Title(): "_" always becomes a space; "." becomes
// a space unless both its neighbors (where present) are a digit or a space -- the guard that
// keeps a floating-point-looking header like "v1.2" intact while still splitting "repo.name";
// TrimSpace then runs, and an all-whitespace non-empty input collapses to a single space rather
// than an empty string (preserving a blank line's presence in a multi-line header); the result is
// upper-cased.
func titleHeader(name string) string {
	runes := []rune(name)
	isNumOrSpace := func(r rune) bool { return ('0' <= r && r <= '9') || r == ' ' }
	for i, r := range runes {
		switch r {
		case '_':
			runes[i] = ' '
		case '.':
			if (i != 0 && !isNumOrSpace(runes[i-1])) || (i != len(runes)-1 && !isNumOrSpace(runes[i+1])) {
				runes[i] = ' '
			}
		}
	}
	title := strings.TrimSpace(string(runes))
	if title == "" && len(runes) > 0 {
		title = " "
	}
	return strings.ToUpper(title)
}

// pad centers s within width, upstream tablewriter's default header alignment: a positive gap
// splits with the smaller half on the left -- upstream computes the left half as
// math.Ceil(float64(gap/2)), which is a no-op because gap/2 already truncated via integer
// division before the float conversion, so an odd gap's extra column lands on the right, not the
// left.
func pad(s string, width int) string {
	gap := width - displayWidth(s)
	if gap <= 0 {
		return s
	}
	left := gap / 2
	right := gap - left
	return strings.Repeat(" ", left) + s + strings.Repeat(" ", right)
}

// padLeft right-aligns s within width by padding on the left, upstream tablewriter's alignment
// for a cell matching decimalCell.
func padLeft(s string, width int) string {
	gap := width - displayWidth(s)
	if gap <= 0 {
		return s
	}
	return strings.Repeat(" ", gap) + s
}

// padRight left-aligns s within width by padding on the right, upstream tablewriter's alignment
// for a cell not matching decimalCell.
func padRight(s string, width int) string {
	gap := width - displayWidth(s)
	if gap <= 0 {
		return s
	}
	return s + strings.Repeat(" ", gap)
}

// cellLines splits cell on "\n", upstream tablewriter's getLines. autoWrap is off for every row
// (printTable calls SetAutoWrapText(false) right after SetHeader), and -- per maxHeaderWidth --
// never actually word-wraps any header in this codebase either, so this single split covers both.
func cellLines(cell string) []string {
	return strings.Split(cell, "\n")
}

// writeRuleLine writes one horizontal border/separator line: "+" then, for every column, a run of
// "-" two longer than the column's width (one padding column on each side) followed by "+".
func writeRuleLine(w io.Writer, colWidths []int) {
	var b strings.Builder
	b.WriteByte('+')
	for _, width := range colWidths {
		b.WriteString(strings.Repeat("-", width+2))
		b.WriteByte('+')
	}
	b.WriteByte('\n')
	_, _ = io.WriteString(w, b.String())
}

// writeCellLine writes one row of a table line: "|" then, for every column present in cells (a
// ragged row may hold fewer columns than the table's overall width -- printed exactly as narrow as
// its own cell count, not padded out to the full column count -- or, for the widest row seen,
// exactly the full count), " "+aligned+" " and a closing "|". align chooses, per cell, padLeft for
// a cell matching decimalCell after TrimSpace or padRight otherwise (headers always use the
// centering pad instead, via writeHeaderLine).
func writeCellLine(w io.Writer, cells []string, colWidths []int) {
	var b strings.Builder
	b.WriteByte('|')
	for i, cell := range cells {
		b.WriteByte(' ')
		if decimalCell.MatchString(strings.TrimSpace(cell)) {
			b.WriteString(padLeft(cell, colWidths[i]))
		} else {
			b.WriteString(padRight(cell, colWidths[i]))
		}
		b.WriteString(" |")
	}
	b.WriteByte('\n')
	_, _ = io.WriteString(w, b.String())
}

// writeHeaderLine writes one row of the header: "|" then, for every column in colWidths (a header
// shorter than the widest row is padded out with blank cells so the header always spans the full
// column count, unlike a data row), " "+centered+" " and a closing "|". cells holds this
// particular header line's text per column, already Title-cased; a column past len(cells) (a
// ragged row wider than the header) renders blank.
func writeHeaderLine(w io.Writer, cells []string, colWidths []int) {
	var b strings.Builder
	b.WriteByte('|')
	for i, width := range colWidths {
		cell := ""
		if i < len(cells) {
			cell = cells[i]
		}
		b.WriteByte(' ')
		b.WriteString(pad(cell, width))
		b.WriteString(" |")
	}
	b.WriteByte('\n')
	_, _ = io.WriteString(w, b.String())
}

// writeTable renders headers and rows as a bordered ASCII table to w, reproducing
// kataras/tablewriter's NewWriter defaults byte-for-byte as printTable called them: rows use
// ALIGN_DEFAULT (a decimalCell match right-aligned, everything else left-aligned); headers are
// always centered and rendered through titleHeader; top, bottom, and header-separator rule lines
// are drawn, row-separator lines are not (upstream's SetRowLine defaults to off and printTable
// never turns it on); a row longer than every header widens every rule line -- header included --
// past the header's own column count, while a row shorter than the table's overall width renders
// only as many columns as it actually has, narrower than the borders above and below it -- both
// are upstream's own behavior, reproduced rather than "fixed" here. Column widths are measured
// with displayWidth (ANSI-stripped runewidth), the same measure upstream used.
//
// A header row is only drawn -- and only then does the header-separator rule line follow -- when
// headers is non-empty, matching upstream's printHeading() returning immediately for zero
// headers; rows still render regardless of whether headers is empty. "just the top and bottom
// rule lines" (no header block AND no rows) is not a shape writeTable produces on its own from a
// non-empty rows argument -- printTable achieves it by calling writeTable(w, nil, nil), passing
// nil for both.
func writeTable(w io.Writer, headers []string, rows [][]string) {
	numCols := len(headers)
	for _, row := range rows {
		if len(row) > numCols {
			numCols = len(row)
		}
	}

	colWidths := make([]int, numCols)
	headerLines := make([][]string, len(headers))
	headerHeight := 0
	for i, header := range headers {
		lines := cellLines(header)
		headerLines[i] = lines
		colWidths[i] = maxLineWidth(lines)
		if len(lines) > headerHeight {
			headerHeight = len(lines)
		}
	}

	rowLines := make([][][]string, len(rows))
	rowHeights := make([]int, len(rows))
	for r, row := range rows {
		lines := make([][]string, len(row))
		height := 0
		for c, cell := range row {
			cellSplit := cellLines(cell)
			lines[c] = cellSplit
			if width := maxLineWidth(cellSplit); width > colWidths[c] {
				colWidths[c] = width
			}
			if len(cellSplit) > height {
				height = len(cellSplit)
			}
		}
		rowLines[r] = lines
		rowHeights[r] = height
	}

	writeRuleLine(w, colWidths)
	if len(headers) > 0 {
		for x := range headerHeight {
			line := make([]string, len(headers))
			for i, lines := range headerLines {
				text := ""
				if x < len(lines) {
					text = lines[x]
				}
				line[i] = titleHeader(text)
			}
			writeHeaderLine(w, line, colWidths)
		}
		writeRuleLine(w, colWidths)
	}
	for r, row := range rows {
		height := rowHeights[r]
		for x := range height {
			line := make([]string, len(row))
			for c, lines := range rowLines[r] {
				if x < len(lines) {
					line[c] = lines[x]
				} else {
					line[c] = "  " // upstream's literal two-space filler for a row cell shorter than the tallest cell in its row
				}
			}
			writeCellLine(w, line, colWidths[:len(row)])
		}
	}
	writeRuleLine(w, colWidths)
}
