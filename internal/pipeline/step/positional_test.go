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

// emptyStepsListHandler answers any request with an empty steps page, for asserting the "no step
// named ... found" branch of resolveStepID rather than an actual step fetch.
func emptyStepsListHandler(t *testing.T) http.HandlerFunc {
	t.Helper()
	return func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"values": []}`))
	}
}

// TestSubcommandsRejectInvalidStepArg proves get/logs/report/cases reject "." and ".." step
// arguments -- neither is ever a valid step name or UUID, so resolveStepID's own zero-match
// branch reports them, rather than ValidatePathIdentifier short-circuiting before any request: a
// step name is free to contain any character (including "/"), so ValidatePathIdentifier only ever
// guards the resolved UUID that reaches GetPath, never the user-typed step argument itself (see
// TestGetProcessSuccessNameContainingSlash / TestLogsProcessSuccessNameContainingSlash). "" is
// covered separately by TestSubcommandsRejectEmptyStepArg: resolveStepID rejects it before ever
// listing steps, so it never reaches this zero-match branch at all.
func TestSubcommandsRejectInvalidStepArg(t *testing.T) {
	cases := []struct {
		name string
		run  func(t *testing.T, stepArg string) error
	}{
		{"get", func(t *testing.T, id string) error {
			cmd := setupTest(t, emptyStepsListHandler(t), false)
			return getProcess(cmd, []string{"42", id})
		}},
		{"logs", func(t *testing.T, id string) error {
			cmd := setupTest(t, emptyStepsListHandler(t), false)
			return logsProcess(cmd, []string{"42", id})
		}},
		{"report", func(t *testing.T, id string) error {
			cmd := setupTest(t, emptyStepsListHandler(t), false)
			return reportProcess(cmd, []string{"42", id})
		}},
		{"cases", func(t *testing.T, id string) error {
			cmd := setupTest(t, emptyStepsListHandler(t), false)
			return casesProcess(cmd, []string{"42", id})
		}},
	}

	for _, tc := range cases {
		for _, invalid := range []string{".", ".."} {
			t.Run(tc.name+"/"+describeArg(invalid), func(t *testing.T) {
				err := tc.run(t, invalid)
				if err == nil {
					t.Fatalf("%s(%q) expected an error, got nil", tc.name, invalid)
				}
				if !strings.Contains(err.Error(), "no step named") {
					t.Errorf("%s(%q) error = %q, want it to report no matching step", tc.name, invalid, err.Error())
				}
			})
		}
	}
}

// unnamedStepListHandler answers any request with a single pipeline step fixture that has no
// Name at all, for TestSubcommandsRejectEmptyStepArg: this is the shape that let an empty stepArg
// silently resolve to an unnamed step before resolveStepID gained its up-front empty-arg guard,
// and it also proves the guard fires without ever issuing this request.
func unnamedStepListHandler(t *testing.T, requests *int) http.HandlerFunc {
	t.Helper()
	return func(w http.ResponseWriter, _ *http.Request) {
		*requests++
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"values":[{"type":"pipeline_step","uuid":"{11111111-1111-1111-1111-111111111111}"}]}`))
	}
}

// TestSubcommandsRejectEmptyStepArg proves get/logs/report/cases reject an empty or all-whitespace
// step argument up front, before any step-listing request, against a pipeline that has an unnamed
// step -- the exact shape that previously let "" silently resolve to that step (resolveStepID
// matched strings.TrimSpace("") against strings.TrimSpace(step.Name) == "" and returned its UUID
// instead of erroring). Regression coverage for that resolution bug.
func TestSubcommandsRejectEmptyStepArg(t *testing.T) {
	cases := []struct {
		name string
		run  func(t *testing.T, requests *int, stepArg string) error
	}{
		{"get", func(t *testing.T, requests *int, id string) error {
			cmd := setupTest(t, unnamedStepListHandler(t, requests), false)
			return getProcess(cmd, []string{"42", id})
		}},
		{"logs", func(t *testing.T, requests *int, id string) error {
			cmd := setupTest(t, unnamedStepListHandler(t, requests), false)
			return logsProcess(cmd, []string{"42", id})
		}},
		{"report", func(t *testing.T, requests *int, id string) error {
			cmd := setupTest(t, unnamedStepListHandler(t, requests), false)
			return reportProcess(cmd, []string{"42", id})
		}},
		{"cases", func(t *testing.T, requests *int, id string) error {
			cmd := setupTest(t, unnamedStepListHandler(t, requests), false)
			return casesProcess(cmd, []string{"42", id})
		}},
	}

	for _, tc := range cases {
		for _, invalid := range []string{"", "   "} {
			t.Run(tc.name+"/"+describeArg(invalid), func(t *testing.T) {
				var requests int
				err := tc.run(t, &requests, invalid)
				if err == nil {
					t.Fatalf("%s(%q) expected an error, got nil", tc.name, invalid)
				}
				if !strings.Contains(err.Error(), "missing") {
					t.Errorf("%s(%q) error = %q, want it to say the argument is missing", tc.name, invalid, err.Error())
				}
				if requests != 0 {
					t.Errorf("%s(%q) issued %d step-list requests, want 0 (guard must fire before listing steps)", tc.name, invalid, requests)
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
