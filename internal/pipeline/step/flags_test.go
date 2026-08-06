package step

import (
	"net/http"
	"strings"
	"testing"

	"github.com/avitsrimer/bitbucket-cli/internal/testutil"
	"github.com/spf13/cobra"
)

// TestPipelineValidArgsArg0OffersPipelineIDs proves arg 0 completion goes through
// plcommon.GetPipelineIDs.
func TestPipelineValidArgsArg0OffersPipelineIDs(t *testing.T) {
	cmd := setupTest(t, func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"values":[{"build_number":42},{"build_number":7}]}`))
	}, false)

	ids, directive := pipelineValidArgs(cmd, nil, "")

	if len(ids) != 2 {
		t.Errorf("pipelineValidArgs() = %v, want 2 suggestions", ids)
	}
	if directive != cobra.ShellCompDirectiveNoFileComp {
		t.Errorf("directive = %v, want ShellCompDirectiveNoFileComp", directive)
	}
}

// TestPipelineValidArgsNoSuggestionsPastArg0 proves completion offers nothing once the pipeline
// positional is already filled.
func TestPipelineValidArgsNoSuggestionsPastArg0(t *testing.T) {
	cmd := setupTest(t, func(http.ResponseWriter, *http.Request) {
		t.Error("unexpected HTTP request past arg 0")
	}, false)

	ids, directive := pipelineValidArgs(cmd, []string{"42"}, "")

	if len(ids) != 0 {
		t.Errorf("pipelineValidArgs() = %v, want no suggestions past arg 0", ids)
	}
	if directive != cobra.ShellCompDirectiveNoFileComp {
		t.Errorf("directive = %v, want ShellCompDirectiveNoFileComp", directive)
	}
}

// TestPipelineAndStepValidArgsArg0DelegatesToPipelineValidArgs proves arg 0 of the shared
// get/logs/report/cases completion function offers pipeline ids.
func TestPipelineAndStepValidArgsArg0DelegatesToPipelineValidArgs(t *testing.T) {
	cmd := setupTest(t, func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"values":[{"build_number":42}]}`))
	}, false)

	ids, directive := pipelineAndStepValidArgs(cmd, nil, "")

	if len(ids) != 1 || ids[0] != "42" {
		t.Errorf("pipelineAndStepValidArgs() = %v, want [42]", ids)
	}
	if directive != cobra.ShellCompDirectiveNoFileComp {
		t.Errorf("directive = %v, want ShellCompDirectiveNoFileComp", directive)
	}
}

// TestPipelineAndStepValidArgsArg1OffersNamesThenUUIDs proves arg 1 completion reads the pipeline
// from args[0] and offers step names before their UUIDs.
func TestPipelineAndStepValidArgsArg1OffersNamesThenUUIDs(t *testing.T) {
	var requests []*http.Request
	cmd := setupTest(t, func(w http.ResponseWriter, r *http.Request) {
		requests = append(requests, r)
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"values":[{"type":"pipeline_step","uuid":"{11111111-1111-1111-1111-111111111111}","name":"Build"}]}`))
	}, false)

	names, directive := pipelineAndStepValidArgs(cmd, []string{"42"}, "")

	if len(requests) != 1 {
		t.Fatalf("expected exactly 1 request, got %d", len(requests))
	}
	wantPath := "/2.0/repositories/" + testutil.FixtureRepositoryFlag + "/pipelines/42/steps"
	if requests[0].URL.Path != wantPath {
		t.Errorf("path = %s, want %s", requests[0].URL.Path, wantPath)
	}
	if len(names) != 2 || names[0] != "Build" || names[1] != "{11111111-1111-1111-1111-111111111111}" {
		t.Errorf("pipelineAndStepValidArgs() = %v, want [Build {11111111-1111-1111-1111-111111111111}] (name before uuid)", names)
	}
	if directive != cobra.ShellCompDirectiveNoFileComp {
		t.Errorf("directive = %v, want ShellCompDirectiveNoFileComp", directive)
	}
}

// TestPipelineAndStepValidArgsNoSuggestionsPastArg1 proves completion offers nothing once both
// positionals are already filled.
func TestPipelineAndStepValidArgsNoSuggestionsPastArg1(t *testing.T) {
	cmd := setupTest(t, func(http.ResponseWriter, *http.Request) {
		t.Error("unexpected HTTP request past arg 1")
	}, false)

	ids, directive := pipelineAndStepValidArgs(cmd, []string{"42", "{11111111-1111-1111-1111-111111111111}"}, "")

	if len(ids) != 0 {
		t.Errorf("pipelineAndStepValidArgs() = %v, want no suggestions past arg 1", ids)
	}
	if directive != cobra.ShellCompDirectiveNoFileComp {
		t.Errorf("directive = %v, want ShellCompDirectiveNoFileComp", directive)
	}
}

// TestGetStepsIgnoresLimitFlag proves a completion/resolution getter uses GetAllUnbounded, so a
// --limit flag registered on cmd (belonging to a different, unrelated output query) never
// truncates the enumeration.
func TestGetStepsIgnoresLimitFlag(t *testing.T) {
	cmd := setupTest(t, func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"values":[` +
			`{"type":"pipeline_step","uuid":"{11111111-1111-1111-1111-111111111111}"},` +
			`{"type":"pipeline_step","uuid":"{22222222-2222-2222-2222-222222222222}"}` +
			`]}`))
	}, false)
	if err := cmd.Flags().Set("limit", "1"); err != nil {
		t.Fatalf("cannot set limit flag: %v", err)
	}

	steps, err := getSteps(cmd.Context(), cmd, "42")
	if err != nil {
		t.Fatalf("getSteps() error = %v", err)
	}
	if len(steps) != 2 {
		t.Errorf("steps = %v, want 2 steps despite --limit=1 on cmd", steps)
	}
}

// TestResolveStepIDUUIDPassthrough proves a value that parses as a UUID resolves without issuing
// any list request.
func TestResolveStepIDUUIDPassthrough(t *testing.T) {
	var requests int
	cmd := setupTest(t, func(http.ResponseWriter, *http.Request) { requests++ }, false)

	got, err := resolveStepID(cmd.Context(), cmd, "42", "{11111111-1111-1111-1111-111111111111}")
	if err != nil {
		t.Fatalf("resolveStepID() error = %v", err)
	}
	if got != "{11111111-1111-1111-1111-111111111111}" {
		t.Errorf("resolveStepID() = %q, want the uuid passed through unchanged", got)
	}
	if requests != 0 {
		t.Errorf("expected no HTTP request for a uuid passthrough, got %d", requests)
	}
}

// TestResolveStepIDNameMatchCaseInsensitiveTrimmed proves a single case-insensitive, trimmed name
// match resolves to that step's UUID.
func TestResolveStepIDNameMatchCaseInsensitiveTrimmed(t *testing.T) {
	cmd := setupTest(t, func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"values":[{"type":"pipeline_step","uuid":"{11111111-1111-1111-1111-111111111111}","name":"Build and Test"}]}`))
	}, false)

	got, err := resolveStepID(cmd.Context(), cmd, "42", "  BUILD AND TEST  ")
	if err != nil {
		t.Fatalf("resolveStepID() error = %v", err)
	}
	if got != "{11111111-1111-1111-1111-111111111111}" {
		t.Errorf("resolveStepID() = %q, want {11111111-1111-1111-1111-111111111111}", got)
	}
}

// TestResolveStepIDUnknownNameListsAvailable proves a zero-match name error names the value and
// lists the available step names.
func TestResolveStepIDUnknownNameListsAvailable(t *testing.T) {
	cmd := setupTest(t, func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"values":[` +
			`{"type":"pipeline_step","uuid":"{11111111-1111-1111-1111-111111111111}","name":"Build"},` +
			`{"type":"pipeline_step","uuid":"{22222222-2222-2222-2222-222222222222}","name":"Test"}` +
			`]}`))
	}, false)

	_, err := resolveStepID(cmd.Context(), cmd, "42", "Deploy")
	if err == nil {
		t.Fatal("resolveStepID() expected an error, got nil")
	}
	if !strings.Contains(err.Error(), `"Deploy"`) {
		t.Errorf("error = %q, want it to name the unresolved value", err.Error())
	}
	if !strings.Contains(err.Error(), "Build") || !strings.Contains(err.Error(), "Test") {
		t.Errorf("error = %q, want it to list the available step names", err.Error())
	}
}

// TestResolveStepIDAmbiguousNameListsCandidates proves a duplicate step name (BitBucket allows
// it) errors listing every ambiguous candidate with its UUID and tells the caller to pass a UUID.
func TestResolveStepIDAmbiguousNameListsCandidates(t *testing.T) {
	cmd := setupTest(t, func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"values":[` +
			`{"type":"pipeline_step","uuid":"{11111111-1111-1111-1111-111111111111}","name":"Build"},` +
			`{"type":"pipeline_step","uuid":"{22222222-2222-2222-2222-222222222222}","name":"Build"}` +
			`]}`))
	}, false)

	_, err := resolveStepID(cmd.Context(), cmd, "42", "build")
	if err == nil {
		t.Fatal("resolveStepID() expected an error, got nil")
	}
	if !strings.Contains(err.Error(), "{11111111-1111-1111-1111-111111111111}") ||
		!strings.Contains(err.Error(), "{22222222-2222-2222-2222-222222222222}") {
		t.Errorf("error = %q, want it to list both ambiguous candidates' uuids", err.Error())
	}
	if !strings.Contains(err.Error(), "UUID") {
		t.Errorf("error = %q, want it to tell the caller to pass a uuid", err.Error())
	}
}

// TestResolveStepIDListAPIError proves a failure listing steps for name resolution surfaces the
// underlying error.
func TestResolveStepIDListAPIError(t *testing.T) {
	cmd := setupTest(t, func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusInternalServerError)
		_, _ = w.Write([]byte(`{"type":"error","error":{"message":"boom"}}`))
	}, false)

	_, err := resolveStepID(cmd.Context(), cmd, "42", "Build")
	if err == nil {
		t.Fatal("resolveStepID() expected an error, got nil")
	}
	if !strings.Contains(err.Error(), "boom") {
		t.Errorf("error = %q, want it to contain the BitBucket error message", err.Error())
	}
}
