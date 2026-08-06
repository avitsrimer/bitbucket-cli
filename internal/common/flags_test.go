package common

import (
	"context"
	"errors"
	"testing"

	"github.com/spf13/cobra"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestNewEnumFlag(t *testing.T) {
	t.Run("has no default value when none is prefixed with +", func(t *testing.T) {
		flag := NewEnumFlag("one", "two", "three")

		assert.Equal(t, []string{"one", "two", "three"}, flag.Allowed)
		assert.Empty(t, flag.Value)
	})

	t.Run("uses the +prefixed value as the default and strips the prefix", func(t *testing.T) {
		flag := NewEnumFlag("one", "+two", "three")

		assert.Equal(t, []string{"one", "two", "three"}, flag.Allowed)
		assert.Equal(t, "two", flag.Value)
	})

	t.Run("uses the first +prefixed value when more than one is given", func(t *testing.T) {
		flag := NewEnumFlag("+one", "+two", "three")

		assert.Equal(t, []string{"one", "two", "three"}, flag.Allowed)
		assert.Equal(t, "one", flag.Value)
	})
}

func TestEnumFlagTypeAndString(t *testing.T) {
	flag := NewEnumFlag("+one", "two")

	assert.Equal(t, "string", flag.Type())
	assert.Equal(t, "one", flag.String())
}

func TestEnumFlagSet(t *testing.T) {
	t.Run("accepts an allowed value", func(t *testing.T) {
		flag := NewEnumFlag("one", "two", "three")

		err := flag.Set("two")

		require.NoError(t, err)
		assert.Equal(t, "two", flag.Value)
	})

	t.Run("rejects a value outside the allowed list and lists the allowed values", func(t *testing.T) {
		flag := NewEnumFlag("one", "two", "three")

		err := flag.Set("four")

		require.Error(t, err)
		assert.Contains(t, err.Error(), "four")
		assert.Contains(t, err.Error(), "one, two, three")
		assert.Empty(t, flag.Value)
	})

	t.Run("accepts an explicit value without ever calling AllowedFunc", func(t *testing.T) {
		// A dynamic flag's AllowedFunc can require a broader privilege scope than the operation
		// itself needs (e.g. --workspace's AllowedFunc calls an endpoint needing read:workspace,
		// even for a command that only needs read:repository against an explicitly named
		// repository). Set must never invoke it: an explicitly supplied value is accepted as-is
		// and left for the server to reject if wrong.
		var calls int
		flag := NewEnumFlagWithFunc("", func(context.Context, *cobra.Command, []string, string) ([]string, error) {
			calls++
			return []string{"alpha", "beta"}, nil
		})

		err := flag.Set("some-value-not-in-any-list")

		require.NoError(t, err)
		assert.Equal(t, "some-value-not-in-any-list", flag.Value)
		assert.Equal(t, 0, calls, "AllowedFunc must not be called by Set on a dynamic flag")
	})

	t.Run("still accepts an explicit value even when AllowedFunc would fail", func(t *testing.T) {
		var calls int
		flag := NewEnumFlagWithFunc("", func(context.Context, *cobra.Command, []string, string) ([]string, error) {
			calls++
			return nil, errors.New("workspace lookup failed: insufficient scope")
		})

		err := flag.Set("beta")

		require.NoError(t, err)
		assert.Equal(t, "beta", flag.Value)
		assert.Equal(t, 0, calls, "AllowedFunc must not be called by Set on a dynamic flag")
	})
}

func TestEnumFlagCompletionFunc(t *testing.T) {
	t.Run("returns the fixed allowed values", func(t *testing.T) {
		flag := NewEnumFlag("one", "two")
		cmd := &cobra.Command{Use: "test"}

		name, completeFunc := flag.CompletionFunc("output")
		values, directive := completeFunc(cmd, nil, "")

		assert.Equal(t, "output", name)
		assert.Equal(t, []string{"one", "two"}, values)
		assert.Equal(t, cobra.ShellCompDirectiveDefault, directive)
	})

	t.Run("resolves values dynamically via AllowedFunc", func(t *testing.T) {
		cmd := &cobra.Command{Use: "test"}
		flag := NewEnumFlagWithFunc("", func(context.Context, *cobra.Command, []string, string) ([]string, error) {
			return []string{"dynamic-one", "dynamic-two"}, nil
		})

		_, completeFunc := flag.CompletionFunc("workspace")
		values, directive := completeFunc(cmd, nil, "")

		assert.Equal(t, []string{"dynamic-one", "dynamic-two"}, values)
		assert.Equal(t, cobra.ShellCompDirectiveDefault, directive)
	})

	t.Run("returns an error directive when AllowedFunc fails", func(t *testing.T) {
		cmd := &cobra.Command{Use: "test"}
		flag := NewEnumFlagWithFunc("", func(context.Context, *cobra.Command, []string, string) ([]string, error) {
			return nil, errors.New("boom")
		})

		_, completeFunc := flag.CompletionFunc("workspace")
		values, directive := completeFunc(cmd, nil, "")

		assert.Empty(t, values)
		assert.Equal(t, cobra.ShellCompDirectiveError, directive)
	})
}

func TestNewEnumSliceFlag(t *testing.T) {
	flag := NewEnumSliceFlag("one", "two")

	assert.Equal(t, []string{"one", "two"}, flag.Allowed)
}

func TestEnumSliceFlagTypeAndString(t *testing.T) {
	flag := NewEnumSliceFlag("one", "two")
	require.NoError(t, flag.Set("one"))

	assert.Equal(t, "stringSlice", flag.Type())
	assert.Equal(t, "[one]", flag.String())
}

func TestEnumSliceFlagSet(t *testing.T) {
	t.Run("accepts a single allowed value", func(t *testing.T) {
		flag := NewEnumSliceFlag("one", "two", "three")

		err := flag.Set("two")

		require.NoError(t, err)
		assert.Equal(t, []string{"two"}, flag.Values)
	})

	t.Run("accepts a comma-separated list of allowed values", func(t *testing.T) {
		flag := NewEnumSliceFlag("one", "two", "three")

		err := flag.Set("one,three")

		require.NoError(t, err)
		assert.Equal(t, []string{"one", "three"}, flag.Values)
	})

	t.Run("deduplicates repeated values", func(t *testing.T) {
		flag := NewEnumSliceFlag("one", "two")

		require.NoError(t, flag.Set("one"))
		require.NoError(t, flag.Set("one"))

		assert.Equal(t, []string{"one"}, flag.Values)
	})

	t.Run("rejects a value outside the allowed list", func(t *testing.T) {
		flag := NewEnumSliceFlag("one", "two")

		err := flag.Set("four")

		require.Error(t, err)
		assert.Contains(t, err.Error(), "four")
		assert.Contains(t, err.Error(), "one, two")
	})

	t.Run("rejects a comma-separated list naming the first invalid element instead of silently dropping it", func(t *testing.T) {
		flag := NewEnumSliceFlag("id", "title")

		err := flag.Set("id,titel")

		require.Error(t, err)
		assert.Contains(t, err.Error(), "titel")
		assert.Empty(t, flag.Values, "no value should be appended when any element of the list is invalid")
	})

	t.Run("selects every allowed value on \"all\" when AllAllowed is set", func(t *testing.T) {
		flag := NewEnumSliceFlagWithAllAllowed("one", "two", "three")

		err := flag.Set("all")

		require.NoError(t, err)
		assert.Equal(t, []string{"one", "two", "three"}, flag.Values)
	})

	t.Run("\"all\" does not alias the Allowed backing array", func(t *testing.T) {
		flag := NewEnumSliceFlagWithAllAllowed("one", "two")

		require.NoError(t, flag.Set("all"))
		flag.Values = append(flag.Values, "mutated")

		assert.Equal(t, []string{"one", "two"}, flag.Allowed, "appending to Values must not mutate Allowed")
	})

	t.Run("rejects \"all\" when AllAllowed is not set", func(t *testing.T) {
		flag := NewEnumSliceFlag("one", "two")

		err := flag.Set("all")

		require.Error(t, err)
	})

	t.Run("accepts an explicit value without ever calling AllowedFunc", func(t *testing.T) {
		cmd := &cobra.Command{Use: "test"}
		var calls int
		flag := NewEnumSliceFlagWithAllAllowedAndFunc(cmd, func(context.Context, *cobra.Command, []string, string) ([]string, error) {
			calls++
			return nil, errors.New("workspace lookup failed: insufficient scope")
		})

		err := flag.Set("one,two")

		require.NoError(t, err)
		assert.Equal(t, []string{"one", "two"}, flag.Values)
		assert.Equal(t, 0, calls, "AllowedFunc must not be called by Set for an explicit (non-\"all\") value")
	})

	t.Run("\"all\" still propagates the AllowedFunc error, since it has no fixed list to fall back to", func(t *testing.T) {
		cmd := &cobra.Command{Use: "test"}
		flag := NewEnumSliceFlagWithAllAllowedAndFunc(cmd, func(context.Context, *cobra.Command, []string, string) ([]string, error) {
			return nil, errors.New("workspace lookup failed")
		})

		err := flag.Set("all")

		require.Error(t, err)
		assert.Contains(t, err.Error(), "workspace lookup failed")
	})

	t.Run("\"all\" resolves the allowed values from AllowedFunc", func(t *testing.T) {
		cmd := &cobra.Command{Use: "test"}
		flag := NewEnumSliceFlagWithAllAllowedAndFunc(cmd, func(context.Context, *cobra.Command, []string, string) ([]string, error) {
			return []string{"alpha", "beta"}, nil
		})

		err := flag.Set("all")

		require.NoError(t, err)
		assert.Equal(t, []string{"alpha", "beta"}, flag.Values)
	})
}

func TestNewEnumSliceFlagWithAllAllowedAndFuncPanicsOnNilCommand(t *testing.T) {
	assert.Panics(t, func() {
		NewEnumSliceFlagWithAllAllowedAndFunc(nil, nil)
	})
}

func TestEnumSliceFlagGetSlice(t *testing.T) {
	t.Run("returns nil when nothing was set", func(t *testing.T) {
		flag := NewEnumSliceFlag("one", "two")

		assert.Empty(t, flag.GetSlice())
	})

	t.Run("returns the set values when some were set", func(t *testing.T) {
		flag := NewEnumSliceFlag("one", "two")
		require.NoError(t, flag.Set("two"))

		assert.Equal(t, []string{"two"}, flag.GetSlice())
	})
}

func TestEnumSliceFlagCompletionFunc(t *testing.T) {
	t.Run("excludes already-selected values and appends \"all\" when AllAllowed is set", func(t *testing.T) {
		flag := NewEnumSliceFlagWithAllAllowed("one", "two", "three")
		cmd := &cobra.Command{Use: "test"}
		cmd.Flags().Var(flag, "columns", "columns to display")
		require.NoError(t, cmd.Flags().Set("columns", "one"))

		_, completeFunc := flag.CompletionFunc("columns")
		values, directive := completeFunc(cmd, nil, "")

		assert.ElementsMatch(t, []string{"two", "three", "all"}, values)
		assert.Equal(t, cobra.ShellCompDirectiveDefault, directive)
	})

	t.Run("resolves values dynamically via AllowedFunc", func(t *testing.T) {
		cmd := &cobra.Command{Use: "test"}
		flag := NewEnumSliceFlagWithAllAllowedAndFunc(cmd, func(context.Context, *cobra.Command, []string, string) ([]string, error) {
			return []string{"dyn-one", "dyn-two"}, nil
		})

		_, completeFunc := flag.CompletionFunc("reviewer")
		values, directive := completeFunc(cmd, nil, "")

		assert.ElementsMatch(t, []string{"dyn-one", "dyn-two", "all"}, values)
		assert.Equal(t, cobra.ShellCompDirectiveDefault, directive)
	})

	t.Run("returns an error directive when AllowedFunc fails", func(t *testing.T) {
		cmd := &cobra.Command{Use: "test"}
		flag := NewEnumSliceFlagWithAllAllowedAndFunc(cmd, func(context.Context, *cobra.Command, []string, string) ([]string, error) {
			return nil, errors.New("boom")
		})

		_, completeFunc := flag.CompletionFunc("reviewer")
		values, directive := completeFunc(cmd, nil, "")

		assert.Empty(t, values)
		assert.Equal(t, cobra.ShellCompDirectiveError, directive)
	})
}
