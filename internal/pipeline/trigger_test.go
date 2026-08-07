package pipeline

import (
	"bytes"
	"encoding/json"
	"io"
	"net/http"
	"os"
	"os/exec"
	"strings"
	"testing"

	"github.com/avitsrimer/bitbucket-cli/internal/testutil"
	"github.com/spf13/cobra"
)

// setupTriggerTest extends setupTest with the flags triggerProcess reads directly off cmd:
// --branch, --commit, --pullrequest, --variable, --force.
func setupTriggerTest(t *testing.T, handler http.HandlerFunc, dryRun bool) *cobra.Command {
	t.Helper()
	cmd := setupTest(t, handler, dryRun)
	cmd.Flags().String("branch", "", "")
	cmd.Flags().String("commit", "", "")
	cmd.Flags().Uint64("pullrequest", 0, "")
	cmd.Flags().String("pattern", "", "")
	cmd.Flags().StringArray("variable", []string{}, "")
	cmd.Flags().Bool("force", false, "")
	return cmd
}

const pipelineTriggeredResponse = `{"type":"pipeline","uuid":"{a1b2c3d4-e5f6-7890-abcd-ef1234567890}","build_number":7,"state":{"type":"pipeline_state_pending","name":"PENDING"},"target":{"type":"pipeline_ref_target","ref_type":"branch","ref_name":"main"},"created_on":"2026-01-01T00:00:00+00:00","duration_in_seconds":0}`

func TestTriggerProcessConfirmYesProceeds(t *testing.T) {
	var requests []*http.Request
	cmd := setupTriggerTest(t, func(w http.ResponseWriter, r *http.Request) {
		requests = append(requests, r)
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(pipelineTriggeredResponse))
	}, false)
	_ = cmd.Flags().Set("branch", "main")
	cmd.SetIn(strings.NewReader("y\n"))

	stdout := testutil.CaptureStdout(t, func() {
		if err := triggerProcess(cmd, nil); err != nil {
			t.Fatalf("triggerProcess() error = %v", err)
		}
	})

	if len(requests) != 1 {
		t.Fatalf("expected exactly 1 request, got %d", len(requests))
	}
	wantPath := "/2.0/repositories/" + testutil.FixtureRepositoryFlag + "/pipelines"
	if requests[0].URL.Path != wantPath {
		t.Errorf("path = %s, want %s", requests[0].URL.Path, wantPath)
	}

	var got Pipeline
	if err := json.Unmarshal([]byte(stdout), &got); err != nil {
		t.Fatalf("cannot unmarshal printed output %q: %v", stdout, err)
	}
	if got.BuildNumber != 7 {
		t.Errorf("printed pipeline build number = %d, want 7", got.BuildNumber)
	}
}

func TestTriggerProcessConfirmNoZeroHandlerHits(t *testing.T) {
	var requestCount int
	cmd := setupTriggerTest(t, func(http.ResponseWriter, *http.Request) { requestCount++ }, false)
	_ = cmd.Flags().Set("branch", "main")
	cmd.SetIn(strings.NewReader("n\n"))

	stdout := testutil.CaptureStdout(t, func() {
		if err := triggerProcess(cmd, nil); err != nil {
			t.Fatalf("triggerProcess() error = %v", err)
		}
	})

	if requestCount != 0 {
		t.Errorf("expected no HTTP request when confirmation is declined, got %d", requestCount)
	}
	if !strings.Contains(stdout, "canceled") {
		t.Errorf("stdout = %q, want it to mention the trigger was canceled", stdout)
	}
}

func TestTriggerProcessForceSkipsPrompt(t *testing.T) {
	var requestCount int
	cmd := setupTriggerTest(t, func(w http.ResponseWriter, _ *http.Request) {
		requestCount++
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(pipelineTriggeredResponse))
	}, false)
	_ = cmd.Flags().Set("branch", "main")
	_ = cmd.Flags().Set("force", "true")
	cmd.SetIn(poisonStdin{t})

	testutil.CaptureStdout(t, func() {
		if err := triggerProcess(cmd, nil); err != nil {
			t.Fatalf("triggerProcess() error = %v", err)
		}
	})

	if requestCount != 1 {
		t.Errorf("expected exactly 1 HTTP request with --force, got %d", requestCount)
	}
}

func TestTriggerProcessDryRunWithoutForceOrTTYDoesNotError(t *testing.T) {
	var requestCount int
	cmd := setupTriggerTest(t, func(http.ResponseWriter, *http.Request) { requestCount++ }, true)
	_ = cmd.Flags().Set("branch", "main")
	// cmd.InOrStdin() is left as the real os.Stdin (no SetIn); swapped to a pipe that is never
	// written to, so a hang (rather than a clean failure) would prove the dry-run short-circuit
	// did not fire before Confirm attempted a read.
	swapStdinToNonInteractivePipe(t)

	if err := triggerProcess(cmd, nil); err != nil {
		t.Fatalf("triggerProcess() error = %v, want nil in dry-run mode", err)
	}
	if requestCount != 0 {
		t.Errorf("expected no HTTP request in dry-run mode, got %d", requestCount)
	}
}

func TestTriggerProcessNonInteractiveWithoutForceErrors(t *testing.T) {
	var requestCount int
	cmd := setupTriggerTest(t, func(http.ResponseWriter, *http.Request) { requestCount++ }, false)
	_ = cmd.Flags().Set("branch", "main")
	swapStdinToNonInteractivePipe(t)

	err := triggerProcess(cmd, nil)
	if err == nil {
		t.Fatal("triggerProcess() expected an error for non-interactive stdin without --force, got nil")
	}
	if requestCount != 0 {
		t.Errorf("expected no HTTP request, got %d", requestCount)
	}
}

func TestTriggerProcessPayloadIncludesVariablesButNeverLogsValues(t *testing.T) {
	const secretValue = "super-secret-token"

	logBuf := testutil.CaptureLog(t)

	var body []byte
	cmd := setupTriggerTest(t, func(w http.ResponseWriter, r *http.Request) {
		body, _ = io.ReadAll(r.Body)
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(pipelineTriggeredResponse))
	}, false)
	_ = cmd.Flags().Set("branch", "main")
	_ = cmd.Flags().Set("force", "true")
	_ = cmd.Flags().Set("variable", "DEPLOY_TOKEN="+secretValue)

	var stderr string
	stdout := testutil.CaptureStdout(t, func() {
		stderr = testutil.CaptureStderr(t, func() {
			if err := triggerProcess(cmd, nil); err != nil {
				t.Fatalf("triggerProcess() error = %v", err)
			}
		})
	})

	if !bytes.Contains(body, []byte(secretValue)) {
		t.Errorf("POST body = %s, want it to contain the variable value sent to BitBucket", body)
	}
	var sent triggerBody
	if err := json.Unmarshal(body, &sent); err != nil {
		t.Fatalf("cannot unmarshal POST body: %v", err)
	}
	if len(sent.Variables) != 1 || sent.Variables[0].Key != "DEPLOY_TOKEN" || sent.Variables[0].Value != secretValue {
		t.Errorf("POST body variables = %+v, want [{DEPLOY_TOKEN %s}]", sent.Variables, secretValue)
	}

	logged := logBuf.String()
	if strings.Contains(logged, secretValue) {
		t.Errorf("log output leaked a variable value: %q", logged)
	}
	if !strings.Contains(logged, "DEPLOY_TOKEN") {
		t.Errorf("log output = %q, want it to still mention the variable key", logged)
	}
	if strings.Contains(stdout, secretValue) {
		t.Errorf("stdout leaked a variable value: %q", stdout)
	}
	if strings.Contains(stderr, secretValue) {
		t.Errorf("stderr leaked a variable value: %q", stderr)
	}
}

func TestTriggerProcessRejectsEmptyVariableKey(t *testing.T) {
	var requestCount int
	cmd := setupTriggerTest(t, func(http.ResponseWriter, *http.Request) { requestCount++ }, false)
	_ = cmd.Flags().Set("branch", "main")
	_ = cmd.Flags().Set("force", "true")
	_ = cmd.Flags().Set("variable", "=value-without-a-key")

	if err := triggerProcess(cmd, nil); err == nil {
		t.Fatal("triggerProcess() expected an error for an empty variable key, got nil")
	}
	if requestCount != 0 {
		t.Errorf("expected no HTTP request for an invalid variable, got %d", requestCount)
	}
}

// TestTriggerProcessInvalidVariableErrorNeverEchoesTheSecretValue reproduces review-iter-7 finding
// #3: the error for a malformed --variable used to echo the raw, unsplit entry verbatim
// (fmt.Errorf("invalid --variable %q: ...", entry)), so a value with no "=" at all -- e.g.
// `--variable "$DEPLOY_TOKEN"` when the caller forgot the "KEY=" prefix -- leaked the secret
// straight to stderr/CI logs. The fix reports the --variable's POSITION, never its content.
func TestTriggerProcessInvalidVariableErrorNeverEchoesTheSecretValue(t *testing.T) {
	const secretValue = "s3cr3t-token-value"
	var requestCount int
	cmd := setupTriggerTest(t, func(http.ResponseWriter, *http.Request) { requestCount++ }, false)
	_ = cmd.Flags().Set("branch", "main")
	_ = cmd.Flags().Set("force", "true")
	_ = cmd.Flags().Set("variable", secretValue) // no "=" at all -- the missing-"KEY=" shape

	err := triggerProcess(cmd, nil)
	if err == nil {
		t.Fatal("triggerProcess() expected an error for a variable with no \"=\", got nil")
	}
	if requestCount != 0 {
		t.Errorf("expected no HTTP request for an invalid variable, got %d", requestCount)
	}
	if strings.Contains(err.Error(), secretValue) {
		t.Errorf("error = %q, want it to never contain the secret value %q", err.Error(), secretValue)
	}
}

// TestTriggerProcessEmptyKeyVariableErrorNeverEchoesTheSecretValue is
// TestTriggerProcessInvalidVariableErrorNeverEchoesTheSecretValue for the empty-key shape
// ("=s3cr3t"), the other way a real secret ends up on the wrong side of a malformed --variable.
func TestTriggerProcessEmptyKeyVariableErrorNeverEchoesTheSecretValue(t *testing.T) {
	const secretValue = "s3cr3t-token-value"
	var requestCount int
	cmd := setupTriggerTest(t, func(http.ResponseWriter, *http.Request) { requestCount++ }, false)
	_ = cmd.Flags().Set("branch", "main")
	_ = cmd.Flags().Set("force", "true")
	_ = cmd.Flags().Set("variable", "="+secretValue)

	err := triggerProcess(cmd, nil)
	if err == nil {
		t.Fatal("triggerProcess() expected an error for an empty variable key, got nil")
	}
	if requestCount != 0 {
		t.Errorf("expected no HTTP request for an invalid variable, got %d", requestCount)
	}
	if strings.Contains(err.Error(), secretValue) {
		t.Errorf("error = %q, want it to never contain the secret value %q", err.Error(), secretValue)
	}
}

func TestTriggerProcessAPIError(t *testing.T) {
	cmd := setupTriggerTest(t, func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusBadRequest)
		_, _ = w.Write([]byte(`{"type":"error","error":{"message":"invalid branch"}}`))
	}, false)
	_ = cmd.Flags().Set("branch", "main")
	_ = cmd.Flags().Set("force", "true")

	err := triggerProcess(cmd, nil)
	if err == nil {
		t.Fatal("triggerProcess() expected an error, got nil")
	}
	if !strings.Contains(err.Error(), "invalid branch") {
		t.Errorf("error = %q, want it to contain the BitBucket error message", err.Error())
	}
}

func TestTriggerProcessPullRequestTarget(t *testing.T) {
	var body []byte
	cmd := setupTriggerTest(t, func(w http.ResponseWriter, r *http.Request) {
		body, _ = io.ReadAll(r.Body)
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(pipelineTriggeredResponse))
	}, false)
	_ = cmd.Flags().Set("pullrequest", "62")
	_ = cmd.Flags().Set("force", "true")

	testutil.CaptureStdout(t, func() {
		if err := triggerProcess(cmd, nil); err != nil {
			t.Fatalf("triggerProcess() error = %v", err)
		}
	})

	var sent triggerBody
	if err := json.Unmarshal(body, &sent); err != nil {
		t.Fatalf("cannot unmarshal POST body: %v", err)
	}
	if sent.Target.Type != "pipeline_pullrequest_target" || sent.Target.PullRequest == nil || sent.Target.PullRequest.ID != 62 {
		t.Errorf("target = %+v, want type pipeline_pullrequest_target with pull request id 62", sent.Target)
	}
}

// TestTriggerProcessCommitPinsBranchTarget pins the payload shape --commit produces on a branch
// target: buildTriggerTarget attaches a Commit reference alongside the branch ref, not in place of
// it.
func TestTriggerProcessCommitPinsBranchTarget(t *testing.T) {
	var body []byte
	cmd := setupTriggerTest(t, func(w http.ResponseWriter, r *http.Request) {
		body, _ = io.ReadAll(r.Body)
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(pipelineTriggeredResponse))
	}, false)
	_ = cmd.Flags().Set("branch", "main")
	_ = cmd.Flags().Set("commit", "abc1234")
	_ = cmd.Flags().Set("force", "true")

	testutil.CaptureStdout(t, func() {
		if err := triggerProcess(cmd, nil); err != nil {
			t.Fatalf("triggerProcess() error = %v", err)
		}
	})

	var sent triggerBody
	if err := json.Unmarshal(body, &sent); err != nil {
		t.Fatalf("cannot unmarshal POST body: %v", err)
	}
	if sent.Target.Type != "pipeline_ref_target" || sent.Target.RefName != "main" {
		t.Errorf("target = %+v, want a branch ref target for main", sent.Target)
	}
	if sent.Target.Commit == nil || sent.Target.Commit.Hash != "abc1234" {
		t.Errorf("target.Commit = %+v, want a commit reference pinning hash abc1234", sent.Target.Commit)
	}
}

// TestTriggerProcessPatternSelectsCustomPipeline proves --pattern is restored and wired: it is the
// only way to trigger one of a repository's *custom* pipeline definitions (as opposed to its
// default/branch pipelines), and BitBucket selects one via a Selector{Type:"custom",Pattern:...}
// attached to the ref target, alongside (not instead of) the branch.
func TestTriggerProcessPatternSelectsCustomPipeline(t *testing.T) {
	var body []byte
	cmd := setupTriggerTest(t, func(w http.ResponseWriter, r *http.Request) {
		body, _ = io.ReadAll(r.Body)
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(pipelineTriggeredResponse))
	}, false)
	_ = cmd.Flags().Set("branch", "main")
	_ = cmd.Flags().Set("pattern", "deploy-to-prod")
	_ = cmd.Flags().Set("force", "true")

	testutil.CaptureStdout(t, func() {
		if err := triggerProcess(cmd, nil); err != nil {
			t.Fatalf("triggerProcess() error = %v", err)
		}
	})

	var sent triggerBody
	if err := json.Unmarshal(body, &sent); err != nil {
		t.Fatalf("cannot unmarshal POST body: %v", err)
	}
	if sent.Target.Type != "pipeline_ref_target" || sent.Target.RefName != "main" {
		t.Errorf("target = %+v, want a branch ref target for main", sent.Target)
	}
	if sent.Target.Selector == nil || sent.Target.Selector.Type != "custom" || sent.Target.Selector.Pattern != "deploy-to-prod" {
		t.Errorf("target.Selector = %+v, want type custom / pattern deploy-to-prod", sent.Target.Selector)
	}
}

// TestTriggerCmdRejectsPatternWithPullRequest proves the real triggerCmd rejects --pattern
// combined with --pullrequest: a custom pipeline selector has no meaning for a pull-request
// target.
func TestTriggerCmdRejectsPatternWithPullRequest(t *testing.T) {
	if err := triggerCmd.Flags().Set("pattern", "deploy-to-prod"); err != nil {
		t.Fatalf("cannot set --pattern: %v", err)
	}
	t.Cleanup(func() { _ = triggerCmd.Flags().Set("pattern", "") })
	if err := triggerCmd.Flags().Set("pullrequest", "1"); err != nil {
		t.Fatalf("cannot set --pullrequest: %v", err)
	}
	t.Cleanup(func() { _ = triggerCmd.Flags().Set("pullrequest", "0") })

	if err := triggerCmd.ValidateFlagGroups(); err == nil {
		t.Error("ValidateFlagGroups() expected an error for --pattern with --pullrequest, got nil")
	}
}

// TestTriggerCmdRejectsPullRequestWithBranchOrCommit proves the real triggerCmd (not a standalone
// test command) rejects --branch/--pullrequest and --commit/--pullrequest together at flag-parse
// time: BitBucket derives a pull request target's branch/commit server-side, so combining them
// with --pullrequest is rejected outright rather than silently discarding one of them.
func TestTriggerCmdRejectsPullRequestWithBranchOrCommit(t *testing.T) {
	t.Run("branch and pullrequest", func(t *testing.T) {
		if err := triggerCmd.Flags().Set("branch", "main"); err != nil {
			t.Fatalf("cannot set --branch: %v", err)
		}
		t.Cleanup(func() { _ = triggerCmd.Flags().Set("branch", "") })
		if err := triggerCmd.Flags().Set("pullrequest", "1"); err != nil {
			t.Fatalf("cannot set --pullrequest: %v", err)
		}
		t.Cleanup(func() { _ = triggerCmd.Flags().Set("pullrequest", "0") })

		if err := triggerCmd.ValidateFlagGroups(); err == nil {
			t.Error("ValidateFlagGroups() expected an error for --branch with --pullrequest, got nil")
		}
	})

	t.Run("commit and pullrequest", func(t *testing.T) {
		if err := triggerCmd.Flags().Set("commit", "abc1234"); err != nil {
			t.Fatalf("cannot set --commit: %v", err)
		}
		t.Cleanup(func() { _ = triggerCmd.Flags().Set("commit", "") })
		if err := triggerCmd.Flags().Set("pullrequest", "1"); err != nil {
			t.Fatalf("cannot set --pullrequest: %v", err)
		}
		t.Cleanup(func() { _ = triggerCmd.Flags().Set("pullrequest", "0") })

		if err := triggerCmd.ValidateFlagGroups(); err == nil {
			t.Error("ValidateFlagGroups() expected an error for --commit with --pullrequest, got nil")
		}
	})
}

func TestTriggerProcessDefaultBranchUsesCurrentGitBranch(t *testing.T) {
	dir := t.TempDir()
	runGit(t, dir, "init")
	runGit(t, dir, "commit", "--allow-empty", "-m", "init")
	runGit(t, dir, "checkout", "-b", "feature-under-test")
	chdirForTest(t, dir)

	var body []byte
	cmd := setupTriggerTest(t, func(w http.ResponseWriter, r *http.Request) {
		body, _ = io.ReadAll(r.Body)
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(pipelineTriggeredResponse))
	}, false)
	_ = cmd.Flags().Set("force", "true")

	testutil.CaptureStdout(t, func() {
		if err := triggerProcess(cmd, nil); err != nil {
			t.Fatalf("triggerProcess() error = %v", err)
		}
	})

	var sent triggerBody
	if err := json.Unmarshal(body, &sent); err != nil {
		t.Fatalf("cannot unmarshal POST body: %v", err)
	}
	if sent.Target.RefName != "feature-under-test" {
		t.Errorf("target.ref_name = %q, want %q (the current git branch)", sent.Target.RefName, "feature-under-test")
	}
}

// runGit runs a real git command with dir as its working directory, failing the test on error.
func runGit(t *testing.T, dir string, args ...string) {
	t.Helper()
	cmd := exec.Command("git", args...)
	cmd.Dir = dir
	cmd.Env = append(os.Environ(),
		"GIT_AUTHOR_NAME=test", "GIT_AUTHOR_EMAIL=test@example.com",
		"GIT_COMMITTER_NAME=test", "GIT_COMMITTER_EMAIL=test@example.com",
	)
	if output, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("git %v failed: %v\n%s", args, err, output)
	}
}

// chdirForTest changes the process's working directory to dir for the duration of the calling
// test, restoring the original directory via t.Cleanup.
func chdirForTest(t *testing.T, dir string) {
	t.Helper()
	original, err := os.Getwd()
	if err != nil {
		t.Fatalf("cannot get working directory: %v", err)
	}
	if err := os.Chdir(dir); err != nil {
		t.Fatalf("cannot chdir to %s: %v", dir, err)
	}
	t.Cleanup(func() {
		if err := os.Chdir(original); err != nil {
			t.Fatalf("cannot restore working directory: %v", err)
		}
	})
}

// TestTriggerProcessExplicitPullRequestZeroErrors reproduces the FINAL CRITICAL GATE's priority-5
// finding: buildTriggerTarget used to branch on "pullRequestID != 0", so an explicit
// `--pullrequest 0` (e.g. a scripted `--pullrequest $PR_ID` with an empty PR_ID) silently fell
// through to the branch target instead of erroring -- POSTing against the current branch when
// the caller clearly meant to target a pull request. It must be a hard error naming the flag
// instead, and no request may be sent.
func TestTriggerProcessExplicitPullRequestZeroErrors(t *testing.T) {
	var requestCount int
	cmd := setupTriggerTest(t, func(http.ResponseWriter, *http.Request) { requestCount++ }, false)
	if err := cmd.Flags().Set("pullrequest", "0"); err != nil {
		t.Fatalf("cannot set --pullrequest: %v", err)
	}
	if err := cmd.Flags().Set("force", "true"); err != nil {
		t.Fatalf("cannot set --force: %v", err)
	}

	err := triggerProcess(cmd, nil)
	if err == nil {
		t.Fatal("triggerProcess() expected an error for an explicit --pullrequest 0, got nil")
	}
	if !strings.Contains(err.Error(), "pullrequest") {
		t.Errorf("error = %q, want it to name the pullrequest argument", err.Error())
	}
	if requestCount != 0 {
		t.Errorf("expected no HTTP request, got %d", requestCount)
	}
}

// TestTriggerProcessUnsetPullRequestFallsBackToBranch proves the unset default (--pullrequest
// never passed, still carrying its zero value) is unaffected by the fix above: it must still
// fall back to the branch target exactly as before, not be mistaken for an explicit 0.
func TestTriggerProcessUnsetPullRequestFallsBackToBranch(t *testing.T) {
	var body []byte
	cmd := setupTriggerTest(t, func(w http.ResponseWriter, r *http.Request) {
		body, _ = io.ReadAll(r.Body)
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(pipelineTriggeredResponse))
	}, false)
	// --pullrequest is deliberately left unset here.
	if err := cmd.Flags().Set("branch", "main"); err != nil {
		t.Fatalf("cannot set --branch: %v", err)
	}
	if err := cmd.Flags().Set("force", "true"); err != nil {
		t.Fatalf("cannot set --force: %v", err)
	}

	testutil.CaptureStdout(t, func() {
		if err := triggerProcess(cmd, nil); err != nil {
			t.Fatalf("triggerProcess() error = %v", err)
		}
	})

	var sent triggerBody
	if err := json.Unmarshal(body, &sent); err != nil {
		t.Fatalf("cannot unmarshal POST body: %v", err)
	}
	if sent.Target.Type != "pipeline_ref_target" || sent.Target.RefName != "main" {
		t.Errorf("target = %+v, want a branch target for main", sent.Target)
	}
}

// TestTriggerProcessDryRunRedactsVariableValues is FR-6's binding secret-redaction test: under
// --dry-run, WhatIfPayload prints the resolved payload to stderr, and dryRunTriggerPayload must
// have replaced every --variable's real value with "***" in that copy before it ever reaches
// there. Without this test, reverting dryRunTriggerPayload's redaction back to the real value
// would pass the rest of the suite (which only exercises the non-dry-run path) while printing a
// live secret to stderr on every dry-run invocation.
func TestTriggerProcessDryRunRedactsVariableValues(t *testing.T) {
	const secretValue = "super-secret-deploy-token"

	var requestCount int
	cmd := setupTriggerTest(t, func(http.ResponseWriter, *http.Request) { requestCount++ }, true)
	_ = cmd.Flags().Set("branch", "main")
	_ = cmd.Flags().Set("force", "true")
	_ = cmd.Flags().Set("variable", "DEPLOY_TOKEN="+secretValue)

	var stderr string
	stdout := testutil.CaptureStdout(t, func() {
		stderr = testutil.CaptureStderr(t, func() {
			if err := triggerProcess(cmd, nil); err != nil {
				t.Fatalf("triggerProcess() error = %v", err)
			}
		})
	})

	if requestCount != 0 {
		t.Errorf("expected no HTTP request in dry-run mode, got %d", requestCount)
	}
	if strings.Contains(stderr, secretValue) {
		t.Fatalf("dry-run stderr leaked the variable value: %q", stderr)
	}
	if !strings.Contains(stderr, "DEPLOY_TOKEN") {
		t.Errorf("dry-run stderr = %q, want it to still mention the variable key", stderr)
	}
	if !strings.Contains(stderr, "***") {
		t.Errorf("dry-run stderr = %q, want the redacted placeholder in the echoed payload", stderr)
	}
	if strings.Contains(stdout, secretValue) {
		t.Errorf("dry-run stdout leaked the variable value: %q", stdout)
	}
}
