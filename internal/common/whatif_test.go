package common_test

import (
	"strings"
	"testing"

	"github.com/avitsrimer/bitbucket-cli/internal/common"
	"github.com/avitsrimer/bitbucket-cli/internal/testutil"
	"github.com/spf13/cobra"
)

// newWhatIfCmd returns a bare command carrying the "dry-run" flag WhatIf/WhatIfPayload read.
func newWhatIfCmd(t *testing.T, dryRun bool) *cobra.Command {
	t.Helper()
	cmd := &cobra.Command{Use: "test"}
	cmd.Flags().Bool("dry-run", dryRun, "")
	return cmd
}

func TestWhatIfProceedsWhenNotDryRun(t *testing.T) {
	cmd := newWhatIfCmd(t, false)

	var proceed bool
	stderr := testutil.CaptureStderr(t, func() {
		proceed = common.WhatIf(cmd, "doing %s", "the-thing")
	})

	if !proceed {
		t.Error("WhatIf() = false, want true when --dry-run is not set")
	}
	if stderr != "" {
		t.Errorf("stderr = %q, want empty when --dry-run is not set", stderr)
	}
}

func TestWhatIfStopsAndReportsWhenDryRun(t *testing.T) {
	cmd := newWhatIfCmd(t, true)

	var proceed bool
	stderr := testutil.CaptureStderr(t, func() {
		proceed = common.WhatIf(cmd, "doing %s", "the-thing")
	})

	if proceed {
		t.Error("WhatIf() = true, want false when --dry-run is set")
	}
	if !strings.Contains(stderr, "Dry run: doing the-thing") {
		t.Errorf("stderr = %q, want it to report the formatted dry-run message", stderr)
	}
}

func TestWhatIfPayloadProceedsWhenNotDryRun(t *testing.T) {
	cmd := newWhatIfCmd(t, false)
	type payload struct {
		Title string `json:"title"`
	}

	var proceed bool
	stderr := testutil.CaptureStderr(t, func() {
		proceed = common.WhatIfPayload(cmd, "/repositories/acme/widgets/pullrequests", payload{Title: "should-not-print"}, "Creating pullrequest")
	})

	if !proceed {
		t.Error("WhatIfPayload() = false, want true when --dry-run is not set")
	}
	if stderr != "" {
		t.Errorf("stderr = %q, want empty when --dry-run is not set", stderr)
	}
}

// TestWhatIfPayloadEchoesTargetAndPayloadWhenDryRun verifies the input-dependent echo FR-6
// requires: the target path and the resolved payload's JSON encoding both appear on stderr.
func TestWhatIfPayloadEchoesTargetAndPayloadWhenDryRun(t *testing.T) {
	cmd := newWhatIfCmd(t, true)
	type payload struct {
		Title string `json:"title"`
	}

	var proceed bool
	stderr := testutil.CaptureStderr(t, func() {
		proceed = common.WhatIfPayload(cmd, "/repositories/acme/widgets/pullrequests", payload{Title: "Add feature"}, "Creating pullrequest")
	})

	if proceed {
		t.Error("WhatIfPayload() = true, want false when --dry-run is set")
	}
	if !strings.Contains(stderr, "Dry run: Creating pullrequest") {
		t.Errorf("stderr = %q, want it to report the formatted dry-run message", stderr)
	}
	if !strings.Contains(stderr, "/repositories/acme/widgets/pullrequests") {
		t.Errorf("stderr = %q, want it to echo the target path", stderr)
	}
	if !strings.Contains(stderr, `"title": "Add feature"`) {
		t.Errorf("stderr = %q, want it to echo the resolved JSON payload", stderr)
	}
}

// TestWhatIfPayloadSkipsPayloadLineWhenNil verifies that a nil payload (e.g. a pullrequest action
// with no request body) still echoes the target path, but prints no payload block.
func TestWhatIfPayloadSkipsPayloadLineWhenNil(t *testing.T) {
	cmd := newWhatIfCmd(t, true)

	var proceed bool
	stderr := testutil.CaptureStderr(t, func() {
		proceed = common.WhatIfPayload(cmd, "/repositories/acme/widgets/pullrequests/42/approve", nil, "Approving pullrequest 42")
	})

	if proceed {
		t.Error("WhatIfPayload() = true, want false when --dry-run is set")
	}
	if !strings.Contains(stderr, "/repositories/acme/widgets/pullrequests/42/approve") {
		t.Errorf("stderr = %q, want it to echo the target path", stderr)
	}
	if strings.Contains(stderr, "Dry run: payload") {
		t.Errorf("stderr = %q, want no payload block for a nil payload", stderr)
	}
}
