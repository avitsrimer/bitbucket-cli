package common

import (
	"context"
	"fmt"
	"slices"
	"strings"

	"github.com/spf13/cobra"
)

// AllowedFunc resolves the allowed values for an EnumFlag/EnumSliceFlag at parse or
// shell-completion time.
//
// See https://pkg.go.dev/github.com/spf13/cobra#Command.RegisterFlagCompletionFunc
type AllowedFunc func(ctx context.Context, cmd *cobra.Command, args []string, toComplete string) ([]string, error)

// EnumFlag is a flag that only accepts one value out of a fixed or dynamically resolved list.
//
// implements pflag.Value
type EnumFlag struct {
	Allowed     []string
	AllowedFunc AllowedFunc
	Value       string
	cmd         *cobra.Command
}

// NewEnumFlag creates an EnumFlag from a fixed list of allowed values.
//
// Prefixing a value with "+" marks it as the default; the prefix is stripped from the
// allowed list itself. If more than one value is prefixed, the first one is used as the
// default and the "+" is still stripped from the others.
//
// Example:
//
//	flag := common.NewEnumFlag("one", "+two", "three")
func NewEnumFlag(allowed ...string) *EnumFlag {
	flag := &EnumFlag{}
	for _, value := range allowed {
		if trimmed, isDefault := strings.CutPrefix(value, "+"); isDefault {
			if flag.Value == "" {
				flag.Value = trimmed
			}
			flag.Allowed = append(flag.Allowed, trimmed)
		} else {
			flag.Allowed = append(flag.Allowed, value)
		}
	}
	return flag
}

// NewEnumFlagWithFunc creates an EnumFlag whose allowed values are resolved by allowedFunc
// the first time the flag is set or completed.
func NewEnumFlagWithFunc(cmd *cobra.Command, defaultValue string, allowedFunc AllowedFunc) *EnumFlag {
	if cmd == nil {
		panic("cobra.Command cmd cannot be nil")
	}
	return &EnumFlag{AllowedFunc: allowedFunc, Value: defaultValue, cmd: cmd}
}

// Type returns the type of the flag.
//
// implements pflag.Value
func (flag EnumFlag) Type() string {
	return "string"
}

// String returns the string representation of the flag.
//
// implements fmt.Stringer and pflag.Value
func (flag EnumFlag) String() string {
	return flag.Value
}

// Set sets the flag value.
//
// If AllowedFunc is set and the allowed values have not been resolved yet, it is called
// first to populate them.
//
// implements pflag.Value
func (flag *EnumFlag) Set(value string) error {
	if flag.AllowedFunc != nil && len(flag.Allowed) == 0 {
		allowed, err := flag.AllowedFunc(flag.cmd.Context(), flag.cmd, nil, "")
		if err != nil {
			return fmt.Errorf("cannot resolve allowed values: %w", err)
		}
		flag.Allowed = allowed
	}
	if !slices.Contains(flag.Allowed, value) {
		return fmt.Errorf("flag value %q is invalid, expected one of: %s", value, strings.Join(flag.Allowed, ", "))
	}
	flag.Value = value
	return nil
}

// CompletionFunc returns a cobra shell-completion function for this flag, to be passed to
// cobra.Command.RegisterFlagCompletionFunc.
func (flag *EnumFlag) CompletionFunc(flagName string) (string, func(*cobra.Command, []string, string) ([]string, cobra.ShellCompDirective)) {
	return flagName, func(cmd *cobra.Command, args []string, toComplete string) ([]string, cobra.ShellCompDirective) {
		if flag.AllowedFunc != nil {
			allowed, err := flag.AllowedFunc(cmd.Context(), cmd, args, toComplete)
			if err != nil {
				return []string{}, cobra.ShellCompDirectiveError
			}
			flag.Allowed = allowed
		}
		return flag.Allowed, cobra.ShellCompDirectiveDefault
	}
}

// EnumSliceFlag is a repeatable flag that only accepts values out of a fixed or dynamically
// resolved list. It can be set multiple times, or with a single comma-separated value.
//
// implements pflag.Value
type EnumSliceFlag struct {
	Allowed     []string
	Values      []string
	AllowedFunc AllowedFunc
	AllAllowed  bool
	cmd         *cobra.Command
}

// NewEnumSliceFlag creates an EnumSliceFlag from a fixed list of allowed values.
//
// Example:
//
//	flag := common.NewEnumSliceFlag("one", "two", "three")
func NewEnumSliceFlag(allowed ...string) *EnumSliceFlag {
	return &EnumSliceFlag{Allowed: allowed}
}

// NewEnumSliceFlagWithAllAllowed creates an EnumSliceFlag like NewEnumSliceFlag, additionally
// accepting the literal value "all" to select every allowed value at once.
func NewEnumSliceFlagWithAllAllowed(allowed ...string) *EnumSliceFlag {
	flag := NewEnumSliceFlag(allowed...)
	flag.AllAllowed = true
	return flag
}

// NewEnumSliceFlagWithAllAllowedAndFunc creates an EnumSliceFlag whose allowed values are
// resolved by allowedFunc, additionally accepting the literal value "all".
func NewEnumSliceFlagWithAllAllowedAndFunc(cmd *cobra.Command, allowedFunc AllowedFunc) *EnumSliceFlag {
	if cmd == nil {
		panic("cobra.Command cmd cannot be nil")
	}
	return &EnumSliceFlag{AllowedFunc: allowedFunc, AllAllowed: true, cmd: cmd}
}

// Type returns the type of the flag.
//
// implements pflag.Value
func (flag EnumSliceFlag) Type() string {
	return "stringSlice"
}

// String returns the string representation of the flag.
//
// implements fmt.Stringer and pflag.Value
func (flag EnumSliceFlag) String() string {
	return "[" + strings.Join(flag.Values, ",") + "]"
}

// Set sets the flag value. It accepts a single value or a comma-separated list of values; every
// value in the list must be allowed, or the whole call fails naming the first offending one (no
// values are appended on a partial match).
//
// If AllowedFunc is set and the allowed values have not been resolved yet, it is called
// first to populate them.
//
// implements pflag.Value
func (flag *EnumSliceFlag) Set(value string) error {
	if flag.AllowedFunc != nil && len(flag.Allowed) == 0 {
		allowed, err := flag.AllowedFunc(flag.cmd.Context(), flag.cmd, nil, "")
		if err != nil {
			return fmt.Errorf("cannot resolve allowed values: %w", err)
		}
		flag.Allowed = allowed
	}
	if value == "all" && flag.AllAllowed {
		flag.Values = slices.Clone(flag.Allowed)
		return nil
	}
	values := strings.Split(value, ",")
	for _, v := range values {
		if !slices.Contains(flag.Allowed, v) {
			return fmt.Errorf("flag value %q is invalid, expected one of: %s", v, strings.Join(flag.Allowed, ", "))
		}
	}
	for _, v := range values {
		if !slices.Contains(flag.Values, v) {
			flag.Values = append(flag.Values, v)
		}
	}
	return nil
}

// GetSlice returns the flag value list as a slice of strings.
//
// implements cobra.SliceValue, so shell completion recognizes this flag can be specified
// multiple times; production code reads the resolved values through cmd.Flags().GetStringSlice,
// which goes through Type/String instead.
func (flag EnumSliceFlag) GetSlice() []string {
	return flag.Values
}

// CompletionFunc returns a cobra shell-completion function for this flag, to be passed to
// cobra.Command.RegisterFlagCompletionFunc.
func (flag *EnumSliceFlag) CompletionFunc(flagName string) (string, func(*cobra.Command, []string, string) ([]string, cobra.ShellCompDirective)) {
	return flagName, func(cmd *cobra.Command, args []string, toComplete string) ([]string, cobra.ShellCompDirective) {
		var allowed []string

		if flag.AllowedFunc != nil {
			var err error

			allowed, err = flag.AllowedFunc(cmd.Context(), cmd, args, toComplete)
			if err != nil {
				return []string{}, cobra.ShellCompDirectiveError
			}
		} else {
			current, _ := cmd.Flags().GetStringSlice(flagName)
			for _, value := range flag.Allowed {
				if !slices.Contains(current, value) {
					allowed = append(allowed, value)
				}
			}
		}
		if flag.AllAllowed && len(allowed) > 0 {
			allowed = append(allowed, "all")
		}
		return allowed, cobra.ShellCompDirectiveDefault
	}
}
