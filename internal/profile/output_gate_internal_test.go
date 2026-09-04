package profile

import (
	"testing"

	"github.com/avitsrimer/bitbucket-cli/internal/common"
	"github.com/spf13/cobra"
)

// newOutputFormatCmd builds a throwaway command carrying its own --output flag, shaped like the
// root command's: a common.EnumFlag over the same allowed formats, whose Value doubles as the
// place BB_OUTPUT_FORMAT lands when it is already exported at flag-registration time. explicit,
// when non-empty, is set through the flag set so the flag's Changed bit is raised exactly as an
// -o/--output on the command line raises it.
//
// Every subtest builds its own command: cobra assembles a command's flag set once and never
// invalidates it, so reusing one across subtests would carry an earlier explicit value, and its
// Changed bit, into the next.
func newOutputFormatCmd(t *testing.T, flagDefault, explicit string) *cobra.Command {
	t.Helper()
	cmd := &cobra.Command{Use: "print"}
	format := &common.EnumFlag{Allowed: []string{"csv", "json", "yaml", "table", "tsv"}, Value: flagDefault}
	cmd.Flags().VarP(format, "output", "o", "output format")
	if explicit != "" {
		if err := cmd.Flags().Set("output", explicit); err != nil {
			t.Fatalf("cannot set the output flag to %s: %v", explicit, err)
		}
	}
	return cmd
}

// TestResolvedOutputFormat pins the full precedence chain Print renders through: cmd's own
// --output value (whether given explicitly or carried as the flag's BB_OUTPUT_FORMAT-derived
// default) wins over a BB_OUTPUT_FORMAT only visible to a lazy os.Getenv, which wins over the
// profile's own configured OutputFormat, with an empty result left for Print's table default.
func TestResolvedOutputFormat(t *testing.T) {
	tests := []struct {
		name          string
		profileFormat string
		flagDefault   string
		envFormat     string
		explicit      string
		want          string
	}{
		{name: "explicit csv wins over every other source", profileFormat: "table", flagDefault: "yaml", envFormat: "json", explicit: "csv", want: "csv"},
		{name: "explicit json wins over every other source", profileFormat: "table", flagDefault: "yaml", envFormat: "csv", explicit: "json", want: "json"},
		{name: "explicit yaml wins over every other source", profileFormat: "table", flagDefault: "json", envFormat: "csv", explicit: "yaml", want: "yaml"},
		{name: "explicit table wins over every other source", profileFormat: "json", flagDefault: "yaml", envFormat: "csv", explicit: "table", want: "table"},
		{name: "explicit tsv wins over every other source", profileFormat: "json", flagDefault: "yaml", envFormat: "csv", explicit: "tsv", want: "tsv"},
		{name: "flag default from the environment beats the profile field", profileFormat: "table", flagDefault: "json", want: "json"},
		{name: "flag default yaml beats the profile field", profileFormat: "csv", flagDefault: "yaml", want: "yaml"},
		{name: "flag default beats a lazily read environment variable", profileFormat: "table", flagDefault: "csv", envFormat: "json", want: "csv"},
		{name: "lazily read environment json beats the profile field", profileFormat: "table", envFormat: "json", want: "json"},
		{name: "lazily read environment yaml beats the profile field", profileFormat: "table", envFormat: "yaml", want: "yaml"},
		{name: "lazily read environment tsv beats the profile field", profileFormat: "json", envFormat: "tsv", want: "tsv"},
		{name: "profile json used when nothing else is set", profileFormat: "json", want: "json"},
		{name: "profile yaml used when nothing else is set", profileFormat: "yaml", want: "yaml"},
		{name: "profile csv used when nothing else is set", profileFormat: "csv", want: "csv"},
		{name: "profile table used when nothing else is set", profileFormat: "table", want: "table"},
		{name: "profile tsv used when nothing else is set", profileFormat: "tsv", want: "tsv"},
		{name: "empty when no source sets a format", want: ""},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			t.Setenv("BB_OUTPUT_FORMAT", test.envFormat)
			cmd := newOutputFormatCmd(t, test.flagDefault, test.explicit)
			profile := Profile{Name: "test", OutputFormat: test.profileFormat}
			if got := profile.resolvedOutputFormat(cmd); got != test.want {
				t.Errorf("resolvedOutputFormat() = %q, want %q", got, test.want)
			}
		})
	}
}

// TestMaskSecrets pins the gate the display commands mask their payload through: only a resolved
// json or yaml format renders a secret verbatim, and only an explicit -o/--output json|yaml opts
// out of masking it.
func TestMaskSecrets(t *testing.T) {
	tests := []struct {
		name          string
		profileFormat string
		flagDefault   string
		envFormat     string
		explicit      string
		want          bool
	}{
		{name: "profile configured json masks", profileFormat: "json", want: true},
		{name: "profile configured yaml masks", profileFormat: "yaml", want: true},
		{name: "flag default json from the environment masks", profileFormat: "table", flagDefault: "json", want: true},
		{name: "flag default yaml from the environment masks", profileFormat: "table", flagDefault: "yaml", want: true},
		{name: "lazily read environment json masks", profileFormat: "table", envFormat: "json", want: true},
		{name: "lazily read environment yaml masks", profileFormat: "table", envFormat: "yaml", want: true},
		{name: "explicit json does not mask", profileFormat: "table", explicit: "json", want: false},
		{name: "explicit yaml does not mask", profileFormat: "table", explicit: "yaml", want: false},
		{name: "explicit json over a configured yaml does not mask", profileFormat: "yaml", explicit: "json", want: false},
		{name: "explicit table over a configured json does not mask", profileFormat: "json", explicit: "table", want: false},
		{name: "explicit csv over an environment json does not mask", profileFormat: "json", envFormat: "json", explicit: "csv", want: false},
		{name: "explicit tsv does not mask", profileFormat: "json", explicit: "tsv", want: false},
		{name: "profile configured table does not mask", profileFormat: "table", want: false},
		{name: "profile configured csv does not mask", profileFormat: "csv", want: false},
		{name: "profile configured tsv does not mask", profileFormat: "tsv", want: false},
		{name: "flag default csv does not mask", profileFormat: "json", flagDefault: "csv", want: false},
		{name: "lazily read environment tsv does not mask", profileFormat: "json", envFormat: "tsv", want: false},
		{name: "lazily read environment table does not mask", profileFormat: "json", envFormat: "table", want: false},
		{name: "no format at all does not mask", want: false},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			t.Setenv("BB_OUTPUT_FORMAT", test.envFormat)
			cmd := newOutputFormatCmd(t, test.flagDefault, test.explicit)
			profile := Profile{Name: "test", OutputFormat: test.profileFormat}
			if got := profile.maskSecrets(cmd); got != test.want {
				t.Errorf("maskSecrets() = %t, want %t", got, test.want)
			}
		})
	}
}
