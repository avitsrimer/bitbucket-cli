package step

import (
	"net/http"
	"testing"

	"github.com/spf13/cobra"
)

// TestStepValidArgsNoSuggestionsWhenPipelineUnset proves the documented behavior: with --pipeline
// unset, stepValidArgs resolves to no suggestions rather than an error, and never even attempts to
// resolve the step list (there is nothing to resolve it against).
func TestStepValidArgsNoSuggestionsWhenPipelineUnset(t *testing.T) {
	var requests int
	cmd := setupTest(t, func(http.ResponseWriter, *http.Request) { requests++ }, false)
	// --pipeline deliberately left unset ("")

	names, directive := stepValidArgs(cmd, nil, "")

	if len(names) != 0 {
		t.Errorf("stepValidArgs() = %v, want no suggestions when --pipeline is unset", names)
	}
	if directive != cobra.ShellCompDirectiveNoFileComp {
		t.Errorf("directive = %v, want ShellCompDirectiveNoFileComp", directive)
	}
	if requests != 0 {
		t.Errorf("expected no HTTP request when --pipeline is unset, got %d", requests)
	}
}

// TestStepValidArgsResolvesStepsWhenPipelineSet is the companion case: once --pipeline is set,
// stepValidArgs resolves the pipeline's steps and offers their ids as completions.
func TestStepValidArgsResolvesStepsWhenPipelineSet(t *testing.T) {
	cmd := setupTest(t, func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"values":[{"type":"pipeline_step","uuid":"{11111111-1111-1111-1111-111111111111}"}]}`))
	}, false)
	if err := cmd.Flags().Set("pipeline", "42"); err != nil {
		t.Fatalf("cannot set pipeline flag: %v", err)
	}

	names, directive := stepValidArgs(cmd, nil, "")

	if len(names) != 1 {
		t.Errorf("stepValidArgs() = %v, want exactly 1 suggestion", names)
	}
	if directive != cobra.ShellCompDirectiveNoFileComp {
		t.Errorf("directive = %v, want ShellCompDirectiveNoFileComp", directive)
	}
}

// TestGetStepIDsIgnoresLimitFlag proves a completion getter uses GetAllUnbounded, so a --limit
// flag registered on cmd (belonging to a different, unrelated output query) never truncates the
// enumeration.
func TestGetStepIDsIgnoresLimitFlag(t *testing.T) {
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

	ids, err := getStepIDs(cmd.Context(), cmd, "42")
	if err != nil {
		t.Fatalf("getStepIDs() error = %v", err)
	}
	if len(ids) != 2 {
		t.Errorf("ids = %v, want 2 ids despite --limit=1 on cmd", ids)
	}
}
