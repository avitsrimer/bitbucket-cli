package common

import (
	"strconv"
	"strings"
)

type FileAnchor struct {
	From uint64 `json:"from,omitempty"`
	To   uint64 `json:"to,omitempty"`
	Path string `json:"path"`
}

// String gets a string representation of this FileAnchor
//
// the new (to) side wins when both sides are set, since that is what "line N" means
// everywhere else in the CLI; an old-side-only anchor is marked with a space-free
// "(old)" suffix so the "path:line" token stays copy-pasteable
//
// implements fmt.Stringer
func (anchor FileAnchor) String() string {
	var value strings.Builder

	value.WriteString(anchor.Path)
	switch {
	case anchor.To > 0:
		value.WriteString(":")
		value.WriteString(strconv.FormatUint(anchor.To, 10))
	case anchor.From > 0:
		value.WriteString(":")
		value.WriteString(strconv.FormatUint(anchor.From, 10))
		value.WriteString("(old)")
	}
	return value.String()
}
