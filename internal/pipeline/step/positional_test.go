package step

import (
	"net/http"
	"strings"
	"testing"
)

// failIfCalled returns a handler that fails the test if it is ever invoked, for asserting a
// rejected argument short-circuits before any HTTP request is issued.
func failIfCalled(t *testing.T) http.HandlerFunc {
	t.Helper()
	return func(http.ResponseWriter, *http.Request) {
		t.Error("unexpected HTTP request for an invalid argument")
	}
}

func describeArg(value string) string {
	if value == "" {
		return "empty"
	}
	return value
}

// TestSubcommandsRejectInvalidPipelineID proves every step subcommand validates its (now
// required, positional) pipeline argument via common.ValidatePathIdentifier before it ever
// reaches a GetPath call, for each of the three values path.Join silently mishandles.
func TestSubcommandsRejectInvalidPipelineID(t *testing.T) {
	cases := []struct {
		name string
		run  func(t *testing.T, pipelineID string) error
	}{
		{"get", func(t *testing.T, id string) error {
			cmd := setupTest(t, failIfCalled(t), false)
			return getProcess(cmd, []string{id, "{cec5beef-dead-deed-bead-5ae1bedd9ada}"})
		}},
		{"list", func(t *testing.T, id string) error {
			cmd := setupTest(t, failIfCalled(t), false)
			return listProcess(cmd, []string{id})
		}},
		{"logs", func(t *testing.T, id string) error {
			cmd := setupTest(t, failIfCalled(t), false)
			return logsProcess(cmd, []string{id, "{cec5beef-dead-deed-bead-5ae1bedd9ada}"})
		}},
		{"report", func(t *testing.T, id string) error {
			cmd := setupTest(t, failIfCalled(t), false)
			return reportProcess(cmd, []string{id, "{cec5beef-dead-deed-bead-5ae1bedd9ada}"})
		}},
		{"cases", func(t *testing.T, id string) error {
			cmd := setupTest(t, failIfCalled(t), false)
			return casesProcess(cmd, []string{id, "{cec5beef-dead-deed-bead-5ae1bedd9ada}"})
		}},
	}

	for _, tc := range cases {
		for _, invalid := range []string{"", ".", ".."} {
			t.Run(tc.name+"/"+describeArg(invalid), func(t *testing.T) {
				err := tc.run(t, invalid)
				if err == nil {
					t.Fatalf("%s(%q) expected an error, got nil", tc.name, invalid)
				}
				if !strings.Contains(err.Error(), "pipeline") {
					t.Errorf("%s(%q) error = %q, want it to name pipeline", tc.name, invalid, err.Error())
				}
			})
		}
	}
}

// TestSubcommandsRejectInvalidStepArg proves get/logs/report/cases validate their (now required,
// positional) step argument via common.ValidatePathIdentifier before it ever reaches a GetPath
// call, for each of the three values path.Join silently mishandles.
func TestSubcommandsRejectInvalidStepArg(t *testing.T) {
	cases := []struct {
		name string
		run  func(t *testing.T, stepArg string) error
	}{
		{"get", func(t *testing.T, id string) error {
			cmd := setupTest(t, failIfCalled(t), false)
			return getProcess(cmd, []string{"42", id})
		}},
		{"logs", func(t *testing.T, id string) error {
			cmd := setupTest(t, failIfCalled(t), false)
			return logsProcess(cmd, []string{"42", id})
		}},
		{"report", func(t *testing.T, id string) error {
			cmd := setupTest(t, failIfCalled(t), false)
			return reportProcess(cmd, []string{"42", id})
		}},
		{"cases", func(t *testing.T, id string) error {
			cmd := setupTest(t, failIfCalled(t), false)
			return casesProcess(cmd, []string{"42", id})
		}},
	}

	for _, tc := range cases {
		for _, invalid := range []string{"", ".", ".."} {
			t.Run(tc.name+"/"+describeArg(invalid), func(t *testing.T) {
				err := tc.run(t, invalid)
				if err == nil {
					t.Fatalf("%s(%q) expected an error, got nil", tc.name, invalid)
				}
				if !strings.Contains(err.Error(), "pipeline-step-uuid-or-name") {
					t.Errorf("%s(%q) error = %q, want it to name pipeline-step-uuid-or-name", tc.name, invalid, err.Error())
				}
			})
		}
	}
}

// TestArgsValidation proves each step subcommand's positional-arg count is enforced by cobra: too
// few and too many args each error.
func TestArgsValidation(t *testing.T) {
	cases := []struct {
		name         string
		validate     func(args []string) error
		wantArgCount int
	}{
		{"get", func(args []string) error { return getCmd.Args(getCmd, args) }, 2},
		{"list", func(args []string) error { return listCmd.Args(listCmd, args) }, 1},
		{"logs", func(args []string) error { return logsCmd.Args(logsCmd, args) }, 2},
		{"report", func(args []string) error { return reportCmd.Args(reportCmd, args) }, 2},
		{"cases", func(args []string) error { return casesCmd.Args(casesCmd, args) }, 2},
	}

	for _, tc := range cases {
		t.Run(tc.name+"/missing-arg", func(t *testing.T) {
			args := make([]string, tc.wantArgCount-1)
			for i := range args {
				args[i] = "1"
			}
			if err := tc.validate(args); err == nil {
				t.Errorf("%s: expected a missing-arg error with %d args, got nil", tc.name, len(args))
			}
		})
		t.Run(tc.name+"/too-many-args", func(t *testing.T) {
			args := make([]string, tc.wantArgCount+1)
			for i := range args {
				args[i] = "1"
			}
			if err := tc.validate(args); err == nil {
				t.Errorf("%s: expected a too-many-args error with %d args, got nil", tc.name, len(args))
			}
		})
	}
}
