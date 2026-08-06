package common

import (
	"fmt"
	"strconv"
	"strings"

	"github.com/spf13/cobra"
	"gopkg.in/yaml.v3"
)

type ErrorProcessing int

const (
	StopOnError ErrorProcessing = iota
	WarnOnError
	IgnoreErrors
)

// Type returns the type of the ErrorProcessing
//
// implements the pflag.Value interface
func (ep ErrorProcessing) Type() string {
	return "string"
}

// Values returns the allowed values of the ErrorProcessing
func (ep ErrorProcessing) Values() []string {
	return []string{StopOnError.String(), WarnOnError.String(), IgnoreErrors.String()}
}

// Set sets the ErrorProcessing value
//
// implements the pflag.Value interface
func (ep *ErrorProcessing) Set(value string) error {
	switch value {
	case "StopOnError":
		*ep = StopOnError
	case "WarnOnError":
		*ep = WarnOnError
	case "IgnoreErrors":
		*ep = IgnoreErrors
	default:
		return fmt.Errorf("argument value is invalid (value: %s, expected one of: %s)", value, strings.Join(ep.Values(), ", "))
	}
	return nil
}

// CompletionFunc returns the completion function of the ErrorProcessing
//
// implements the pflag.Value interface
func (ep ErrorProcessing) CompletionFunc() func(*cobra.Command, []string, string) ([]string, cobra.ShellCompDirective) {
	return func(*cobra.Command, []string, string) ([]string, cobra.ShellCompDirective) {
		return ep.Values(), cobra.ShellCompDirectiveNoFileComp
	}
}

// String returns the string representation of the ErrorProcessing
//
// implements the fmt.Stringer interface
func (ep ErrorProcessing) String() string {
	return [...]string{"StopOnError", "WarnOnError", "IgnoreErrors"}[ep]
}

// UnmarshalYAML implements yaml.Unmarshaler, decoding a config file's errorProcessing either as
// the same string spelling Set/CompletionFunc accept and MarshalJSON emits ("StopOnError",
// "WarnOnError", "IgnoreErrors" - what a user hand-editing the file would reasonably write), or as
// the plain integer ErrorProcessing already round-trips as by default (yaml.v3's decoding of an
// untagged int-based type).
func (ep *ErrorProcessing) UnmarshalYAML(node *yaml.Node) error {
	var value string
	if err := node.Decode(&value); err != nil {
		return fmt.Errorf("cannot decode errorProcessing: %w", err)
	}
	if err := ep.Set(value); err == nil {
		return nil
	}
	n, err := strconv.Atoi(value)
	if err != nil || n < int(StopOnError) || n > int(IgnoreErrors) {
		return fmt.Errorf("argument errorProcessing is invalid (value: %s, expected one of: %s)", value, strings.Join(ep.Values(), ", "))
	}
	*ep = ErrorProcessing(n)
	return nil
}
