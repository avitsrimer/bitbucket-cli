package common

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
)

func TestGetEnvAsString(t *testing.T) {
	t.Run("returns the environment value when set", func(t *testing.T) {
		t.Setenv("BB_TEST_ENV_STRING", "actual")

		assert.Equal(t, "actual", GetEnvAsString("BB_TEST_ENV_STRING", "fallback"))
	})

	t.Run("returns fallback when unset", func(t *testing.T) {
		assert.Equal(t, "fallback", GetEnvAsString("BB_TEST_ENV_STRING_UNSET", "fallback"))
	})

	t.Run("returns fallback when set to an empty string", func(t *testing.T) {
		t.Setenv("BB_TEST_ENV_STRING_EMPTY", "")

		assert.Equal(t, "fallback", GetEnvAsString("BB_TEST_ENV_STRING_EMPTY", "fallback"))
	})
}

func TestGetEnvAsDuration(t *testing.T) {
	t.Run("returns the parsed duration when set to a valid Go duration string", func(t *testing.T) {
		t.Setenv("BB_TEST_ENV_DURATION", "30s")

		assert.Equal(t, 30*time.Second, GetEnvAsDuration("BB_TEST_ENV_DURATION", time.Minute))
	})

	t.Run("returns fallback when unset", func(t *testing.T) {
		assert.Equal(t, time.Minute, GetEnvAsDuration("BB_TEST_ENV_DURATION_UNSET", time.Minute))
	})

	t.Run("returns fallback when set to an empty string", func(t *testing.T) {
		t.Setenv("BB_TEST_ENV_DURATION_EMPTY", "")

		assert.Equal(t, time.Minute, GetEnvAsDuration("BB_TEST_ENV_DURATION_EMPTY", time.Minute))
	})

	t.Run("returns fallback when set to an invalid duration string", func(t *testing.T) {
		t.Setenv("BB_TEST_ENV_DURATION_INVALID", "PT30S")

		assert.Equal(t, time.Minute, GetEnvAsDuration("BB_TEST_ENV_DURATION_INVALID", time.Minute))
	})
}
