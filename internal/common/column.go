package common

import (
	"strings"
	"time"

	"github.com/gildas/go-core"
	"github.com/spf13/cobra"
)

// TableTimeFormat is the time layout used to render timestamps in table/csv/tsv output.
const TableTimeFormat = "2006-01-02 15:04:05"

// JSONTimeFormat is the time layout used to render timestamps in json/yaml output.
const JSONTimeFormat = "2006-01-02T15:04:05.999999999-07:00"

// EmptyCell is the filler a GetRow implementation renders for a column with no value (a zero
// timestamp, a nil pointer field, an unrecognized column key).
const EmptyCell = " "

// TimeCell renders when as common.TableTimeFormat, or EmptyCell when when is the zero time.
func TimeCell(when time.Time) string {
	if when.IsZero() {
		return EmptyCell
	}
	return when.Format(TableTimeFormat)
}

// FormatOptionalTime returns a pointer to when formatted as common.JSONTimeFormat, or nil when
// when is the zero time. Assign the result to a `json:",omitempty"`-tagged *string field in a
// MarshalJSON implementation so a zero time.Time (e.g. a still-open PullRequest/Comment/Commit/
// ActivityUpdate that was never updated) omits the key entirely, instead of emitting time.Time's
// own zero-value marshaling, "0001-01-01T00:00:00Z", into machine-readable JSON/YAML output --
// a year-1 timestamp with no meaning to a caller scripting against it.
func FormatOptionalTime(when time.Time) *string {
	if when.IsZero() {
		return nil
	}
	formatted := when.Format(JSONTimeFormat)
	return &formatted
}

// HeadersFromFlag returns the value of cmd's --columns flag, with underscores substituted for
// spaces, when it was explicitly set, or defaults otherwise. It is the common GetHeaders body
// shared by every Tableable that supports --columns.
func HeadersFromFlag(cmd *cobra.Command, defaults ...string) []string {
	if cmd != nil && cmd.Flag("columns") != nil && cmd.Flag("columns").Changed {
		if values, err := cmd.Flags().GetStringSlice("columns"); err == nil {
			return core.Map(values, func(value string) string { return strings.ReplaceAll(value, "_", " ") })
		}
	}
	return defaults
}

// NormalizeColumnKey lowercases header and replaces spaces and hyphens with underscores, so a
// --columns value (or a GetHeaders default label) can be matched against a single canonical,
// underscore-separated case in a GetRow implementation regardless of which of those three forms
// the caller used (e.g. "created on", "created-on", and "created_on" all normalize to
// "created_on").
func NormalizeColumnKey(header string) string {
	key := strings.ToLower(header)
	key = strings.ReplaceAll(key, " ", "_")
	key = strings.ReplaceAll(key, "-", "_")
	return key
}

type Column[T any] struct {
	Name          string
	DefaultSorter bool
	Compare       func(a, b T) bool
}

type Columns[T any] []Column[T]

func (columns Columns[T]) Columns() []string {
	return core.Map(columns, func(column Column[T]) string { return column.Name })
}

func (columns Columns[T]) Sorters() []string {
	return core.Map(columns, func(column Column[T]) string {
		if column.DefaultSorter {
			return "+" + column.Name
		}
		return column.Name
	})
}

// SortBy returns the Compare function for the named sorter. Every Columns table defines Compare
// for each of its columns and EnumFlag only ever accepts a sorter name already in that table, so
// the never-equal fallback below is unreachable in practice; it exists only to keep this method
// total (never panics on an unrecognized name) rather than to express real sort behavior.
func (columns Columns[T]) SortBy(sorter string) func(a, b T) bool {
	for _, column := range columns {
		if column.Name == sorter {
			return column.Compare
		}
	}
	return func(a, b T) bool { return false }
}
