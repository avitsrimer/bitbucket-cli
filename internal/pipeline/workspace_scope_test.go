package pipeline

import (
	"encoding/json"
	"net/http"
	"strings"
	"testing"

	"github.com/avitsrimer/bitbucket-cli/internal/testutil"
)

// TestListProcessResolvesRepositoryWithoutWorkspaceRequest is field report FR-3's end-to-end
// regression test for "bb pipeline list --repository <slug> --workspace <slug>": unlike
// setupTest, this does not call testutil.PrimeFixtureCaches, so a regression that fell back to
// fetching a Workspace object cannot silently pass by resolving from the cache instead of the
// network, and any request to /workspaces/{slug} fails with the exact scope error the field
// report reproduced.
func TestListProcessResolvesRepositoryWithoutWorkspaceRequest(t *testing.T) {
	const workspaceSlug = "fr3-pipeline-ws"
	const repositorySlug = "fr3-pipeline-repo"
	repositoryPath := "/2.0/repositories/" + workspaceSlug + "/" + repositorySlug
	pipelinesPath := repositoryPath + "/pipelines"

	var requests []*http.Request
	cmd := testutil.SetupProfile(t, "fr3-pipeline-list", func(w http.ResponseWriter, r *http.Request) {
		requests = append(requests, r)
		if strings.Contains(r.URL.Path, "/workspaces/") {
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusForbidden)
			_, _ = w.Write([]byte(`{"type":"error","error":{"message":"Your credentials lack one or more required privilege scopes. (required: read:workspace:bitbucket)"}}`))
			return
		}
		w.Header().Set("Content-Type", "application/json")
		switch r.URL.Path {
		case repositoryPath:
			_, _ = w.Write([]byte(`{"type":"repository","uuid":"{11111111-1111-1111-1111-111111111111}","name":"FR3","full_name":"` + workspaceSlug + `/` + repositorySlug + `","slug":"` + repositorySlug + `","workspace":{"type":"workspace","uuid":"{33333333-3333-3333-3333-333333333333}","slug":"` + workspaceSlug + `","name":"FR3 Workspace"}}`))
		case pipelinesPath:
			_, _ = w.Write([]byte(`{"values":[{"type":"pipeline","uuid":"{22222222-2222-2222-2222-222222222222}","build_number":1,"state":{"type":"pipeline_state_completed","name":"COMPLETED"},"target":{"type":"pipeline_ref_target"},"created_on":"2026-01-01T00:00:00+00:00","duration_in_seconds":0}]}`))
		default:
			t.Errorf("unexpected request to %s", r.URL.Path)
		}
	})
	cmd.Flags().String("workspace", "", "")
	cmd.Flags().String("query", "", "")
	cmd.Flags().StringSlice("columns", []string{}, "")
	cmd.Flags().String("sort", "build_number", "")
	if err := cmd.Flags().Set("repository", repositorySlug); err != nil {
		t.Fatalf("cannot set repository flag: %v", err)
	}
	if err := cmd.Flags().Set("workspace", workspaceSlug); err != nil {
		t.Fatalf("cannot set workspace flag: %v", err)
	}

	stdout := testutil.CaptureStdout(t, func() {
		if err := listProcess(cmd, nil); err != nil {
			t.Fatalf("listProcess() error = %v (must succeed on a token that lacks read:workspace)", err)
		}
	})

	for _, req := range requests {
		if strings.Contains(req.URL.Path, "/workspaces/") {
			t.Errorf("unexpected request to %s: resolving --workspace/--repository must never fetch a Workspace object", req.URL.Path)
		}
	}

	var pipelines []Pipeline
	if err := json.Unmarshal([]byte(stdout), &pipelines); err != nil {
		t.Fatalf("cannot unmarshal printed output %q: %v", stdout, err)
	}
	if len(pipelines) != 1 || pipelines[0].BuildNumber != 1 {
		t.Errorf("pipelines = %+v, want one pipeline with build_number 1", pipelines)
	}
}
