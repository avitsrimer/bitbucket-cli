package common_test

import (
	"strings"
	"testing"

	"github.com/avitsrimer/bitbucket-cli/internal/common"
	"gopkg.in/yaml.v3"
)

// TestErrorProcessingUnmarshalYAMLAcceptsStringForm proves a config file's errorProcessing written
// as its documented string name (the spelling Set/CompletionFunc accept and MarshalJSON emits)
// decodes successfully via ErrorProcessing's yaml.Unmarshaler implementation.
func TestErrorProcessingUnmarshalYAMLAcceptsStringForm(t *testing.T) {
	tests := []struct {
		value string
		want  common.ErrorProcessing
	}{
		{"StopOnError", common.StopOnError},
		{"WarnOnError", common.WarnOnError},
		{"IgnoreErrors", common.IgnoreErrors},
	}
	for _, test := range tests {
		t.Run(test.value, func(t *testing.T) {
			var got common.ErrorProcessing
			if err := yaml.Unmarshal([]byte(test.value+"\n"), &got); err != nil {
				t.Fatalf("yaml.Unmarshal() error = %v", err)
			}
			if got != test.want {
				t.Errorf("got = %v, want %v", got, test.want)
			}
		})
	}
}

// TestErrorProcessingUnmarshalYAMLAcceptsIntegerForm proves the plain integer form (what
// config.Save already writes, since ErrorProcessing has no MarshalYAML of its own, and what
// profiles saved before string support was added already have on disk) still round-trips.
func TestErrorProcessingUnmarshalYAMLAcceptsIntegerForm(t *testing.T) {
	tests := []struct {
		value string
		want  common.ErrorProcessing
	}{
		{"0", common.StopOnError},
		{"1", common.WarnOnError},
		{"2", common.IgnoreErrors},
	}
	for _, test := range tests {
		t.Run(test.value, func(t *testing.T) {
			var got common.ErrorProcessing
			if err := yaml.Unmarshal([]byte(test.value+"\n"), &got); err != nil {
				t.Fatalf("yaml.Unmarshal() error = %v", err)
			}
			if got != test.want {
				t.Errorf("got = %v, want %v", got, test.want)
			}
		})
	}
}

// TestErrorProcessingUnmarshalYAMLRejectsInvalidValue proves an unrecognized value is a clear
// decode error rather than silently defaulting to StopOnError.
func TestErrorProcessingUnmarshalYAMLRejectsInvalidValue(t *testing.T) {
	var got common.ErrorProcessing
	err := yaml.Unmarshal([]byte("NotAValidValue\n"), &got)
	if err == nil {
		t.Fatal("yaml.Unmarshal() expected an error for an invalid value, got nil")
	}
	if !strings.Contains(err.Error(), "errorProcessing") {
		t.Errorf("error = %q, want it to mention errorProcessing", err.Error())
	}
}
