package pipeline

import (
	"encoding/json"
	"os"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
)

// loadTestData reads a fixture from the repo-root testdata directory (two levels up from
// internal/pipeline).
func loadTestData(t *testing.T, filename string) []byte {
	t.Helper()
	data, err := os.ReadFile("../../testdata/" + filename)
	if err != nil {
		t.Fatalf("cannot read testdata/%s: %v", filename, err)
	}
	return data
}

// TestPipelineMarshalUnmarshalRoundTrip proves Pipeline.UnmarshalJSON/MarshalJSON round-trip a
// branch-target pipeline byte-for-byte (structurally): unmarshaling testdata/pipeline.json and
// re-marshaling it must reproduce the same JSON, catching a MarshalJSON field disagreeing with
// the field UnmarshalJSON actually reads back.
func TestPipelineMarshalUnmarshalRoundTrip(t *testing.T) {
	expected := loadTestData(t, "pipeline.json")

	var p Pipeline
	if err := json.Unmarshal(expected, &p); err != nil {
		t.Fatalf("cannot unmarshal pipeline.json: %v", err)
	}

	if p.ID.String() != "{a1b2c3d4-e5f6-7890-abcd-ef1234567890}" {
		t.Errorf("ID = %s, want {a1b2c3d4-e5f6-7890-abcd-ef1234567890}", p.ID.String())
	}
	if p.BuildNumber != 42 {
		t.Errorf("BuildNumber = %d, want 42", p.BuildNumber)
	}
	if p.State.Name != "COMPLETED" || p.State.Result == nil || p.State.Result.Name != "SUCCESSFUL" {
		t.Errorf("State = %+v, want COMPLETED/SUCCESSFUL", p.State)
	}
	if p.Duration != 330*time.Second {
		t.Errorf("Duration = %s, want 330s", p.Duration)
	}
	if p.Target.GetDestination() != "main" {
		t.Errorf("Target.GetDestination() = %q, want %q", p.Target.GetDestination(), "main")
	}
	if p.Target.Commit == nil || p.Target.Commit.Hash != "abc123def456" {
		t.Errorf("Target.Commit = %+v, want hash abc123def456", p.Target.Commit)
	}
	if p.Target.Selector == nil || p.Target.Selector.Type != "default" {
		t.Errorf("Target.Selector = %+v, want type default", p.Target.Selector)
	}
	if p.Repository.FullName != "myworkspace/my-repo" {
		t.Errorf("Repository.FullName = %q, want %q", p.Repository.FullName, "myworkspace/my-repo")
	}
	if p.Creator.Name != "John Developer" || p.Creator.Nickname != "johnd" {
		t.Errorf("Creator = %+v, want name John Developer / nickname johnd", p.Creator)
	}

	data, err := json.Marshal(p)
	if err != nil {
		t.Fatalf("cannot marshal pipeline: %v", err)
	}
	assert.JSONEq(t, string(expected), string(data))
}

// TestTargetUnmarshalEmptyPullRequestIDsMarshalsAbsent proves an empty "pullrequest_ids": [] from
// the server re-marshals as an absent key rather than an empty list.
func TestTargetUnmarshalEmptyPullRequestIDsMarshalsAbsent(t *testing.T) {
	var target Target
	if err := json.Unmarshal([]byte(`{"type":"pipeline_merge_queue_ref_target","ref_name":"bitbucket-merge-queue-62","target_branch":"master","pullrequest_ids":[]}`), &target); err != nil {
		t.Fatalf("cannot unmarshal target: %v", err)
	}

	data, err := json.Marshal(target)
	if err != nil {
		t.Fatalf("cannot marshal target: %v", err)
	}
	var got map[string]any
	if err := json.Unmarshal(data, &got); err != nil {
		t.Fatalf("cannot unmarshal marshaled target: %v", err)
	}
	if _, present := got["pullrequest_ids"]; present {
		t.Errorf("got pullrequest_ids key %v, want it absent", got["pullrequest_ids"])
	}
	if got["target_branch"] != "master" {
		t.Errorf("target_branch = %v, want master", got["target_branch"])
	}
}

// TestPipelineUnmarshalPullRequestTarget proves a pull-request-shaped target unmarshals into the
// flat Target correctly: Source/Destination/Commit come straight from the top-level fields, and
// PullRequest is the nested "pullrequest" object (only its id is kept -- see Target's doc
// comment).
func TestPipelineUnmarshalPullRequestTarget(t *testing.T) {
	var p Pipeline
	if err := json.Unmarshal(loadTestData(t, "pipeline-pullrequest.json"), &p); err != nil {
		t.Fatalf("cannot unmarshal pipeline-pullrequest.json: %v", err)
	}

	if p.Target.Type != "pipeline_pullrequest_target" {
		t.Errorf("Target.Type = %q, want %q", p.Target.Type, "pipeline_pullrequest_target")
	}
	if p.Target.Source != "frontend-develop-non-delete-key" {
		t.Errorf("Target.Source = %q, want %q", p.Target.Source, "frontend-develop-non-delete-key")
	}
	if p.Target.GetDestination() != "main" {
		t.Errorf("Target.GetDestination() = %q, want %q", p.Target.GetDestination(), "main")
	}
	if p.Target.Commit == nil || p.Target.Commit.Hash != "3c80cde6b371" {
		t.Errorf("Target.Commit = %+v, want hash 3c80cde6b371", p.Target.Commit)
	}
	if p.Target.PullRequest == nil || p.Target.PullRequest.ID != 62 {
		t.Errorf("Target.PullRequest = %+v, want id 62", p.Target.PullRequest)
	}
	if p.State.Result == nil || p.State.Result.Name != "FAILED" {
		t.Errorf("State.Result = %+v, want FAILED", p.State.Result)
	}
}

// TestPipelineUnmarshalMergeQueueTarget proves a merge-queue-shaped target decodes every field it
// carries into the flat Target, reports target_branch (not the queue ref) as its destination, and
// re-marshals to structurally equal JSON so target_branch/pullrequest_ids survive -o json.
func TestPipelineUnmarshalMergeQueueTarget(t *testing.T) {
	expected := loadTestData(t, "pipeline-merge-queue.json")

	var p Pipeline
	if err := json.Unmarshal(expected, &p); err != nil {
		t.Fatalf("cannot unmarshal pipeline-merge-queue.json: %v", err)
	}

	assert.Equal(t, "pipeline_merge_queue_ref_target", p.Target.Type)
	if assert.NotNil(t, p.Target.Selector) {
		assert.Equal(t, "merge-queues", p.Target.Selector.Type)
		assert.Equal(t, "mergequeue-main", p.Target.Selector.Pattern)
	}
	assert.Equal(t, "branch", p.Target.RefType)
	assert.Equal(t, "bitbucket-merge-queue-62", p.Target.RefName)
	assert.Equal(t, "master", p.Target.TargetBranch)
	assert.Equal(t, []uint64{62, 63}, p.Target.PullRequestIDs)
	if assert.NotNil(t, p.Target.Commit) {
		assert.Equal(t, "abcdef1234567890abcdef1234567890abcdef12", p.Target.Commit.Hash)
	}
	assert.Equal(t, "master", p.Target.GetDestination())

	data, err := json.Marshal(p)
	if err != nil {
		t.Fatalf("cannot marshal pipeline: %v", err)
	}
	assert.JSONEq(t, string(expected), string(data))
}

func TestTargetMarshalReferenceTarget(t *testing.T) {
	target := Target{Type: "pipeline_ref_target", RefType: "branch", RefName: "master"}

	data, err := json.Marshal(target)
	if err != nil {
		t.Fatalf("cannot marshal target: %v", err)
	}
	var got map[string]any
	if err := json.Unmarshal(data, &got); err != nil {
		t.Fatalf("cannot unmarshal marshaled target: %v", err)
	}
	if _, present := got["pullrequest"]; present {
		t.Errorf("got pullrequest key %v, want it absent for a reference target", got["pullrequest"])
	}
	if _, present := got["source"]; present {
		t.Errorf("got source key %v, want it absent (omitempty, zero value)", got["source"])
	}
	if got["ref_name"] != "master" {
		t.Errorf("ref_name = %v, want master", got["ref_name"])
	}
	assertMergeQueueKeysAbsent(t, got)
}

func TestTargetMarshalPullRequestTarget(t *testing.T) {
	target := Target{Type: "pipeline_pullrequest_target", Source: "release", Destination: "main", PullRequest: &pullRequestReference{Type: "pullrequest", ID: 62}}

	data, err := json.Marshal(target)
	if err != nil {
		t.Fatalf("cannot marshal target: %v", err)
	}
	var got map[string]any
	if err := json.Unmarshal(data, &got); err != nil {
		t.Fatalf("cannot unmarshal marshaled target: %v", err)
	}
	pr, ok := got["pullrequest"].(map[string]any)
	if !ok {
		t.Fatalf("got pullrequest = %v, want a nested object", got["pullrequest"])
	}
	if pr["type"] != "pullrequest" || pr["id"] != float64(62) {
		t.Errorf("pullrequest = %v, want type pullrequest / id 62", pr)
	}
	assertMergeQueueKeysAbsent(t, got)
}

// assertMergeQueueKeysAbsent requires a marshaled non-merge-queue target to carry neither
// target_branch nor pullrequest_ids, so the pipeline trigger payload keeps its exact shape.
func assertMergeQueueKeysAbsent(t *testing.T, got map[string]any) {
	t.Helper()
	for _, key := range []string{"target_branch", "pullrequest_ids"} {
		if _, present := got[key]; present {
			t.Errorf("got %s key %v, want it absent (omitempty, zero value)", key, got[key])
		}
	}
}

func TestTargetGetDestination(t *testing.T) {
	cases := []struct {
		name   string
		target Target
		want   string
	}{
		{"reference target uses RefName", Target{RefName: "main"}, "main"},
		{"pull request target uses Destination", Target{Destination: "main"}, "main"},
		{"commit target has no destination", Target{}, ""},
		{"merge queue target uses TargetBranch", Target{RefName: "bitbucket-merge-queue-62", TargetBranch: "master"}, "master"},
		{"TargetBranch beats Destination", Target{TargetBranch: "master", Destination: "develop"}, "master"},
		{"RefName beats Destination", Target{RefName: "main", Destination: "develop"}, "main"},
		{"merge queue target without target_branch falls back to the queue ref", Target{Type: "pipeline_merge_queue_ref_target", RefName: "bitbucket-merge-queue-62"}, "bitbucket-merge-queue-62"},
	}
	for _, testCase := range cases {
		t.Run(testCase.name, func(t *testing.T) {
			if got := testCase.target.GetDestination(); got != testCase.want {
				t.Errorf("GetDestination() = %q, want %q", got, testCase.want)
			}
		})
	}
}

func TestVariableMarshalOmitsNilID(t *testing.T) {
	variable := Variable{Key: "env", Value: "production"}

	data, err := json.Marshal(variable)
	if err != nil {
		t.Fatalf("cannot marshal variable: %v", err)
	}
	var got map[string]any
	if err := json.Unmarshal(data, &got); err != nil {
		t.Fatalf("cannot unmarshal marshaled variable: %v", err)
	}
	if _, present := got["uuid"]; present {
		t.Errorf("got uuid key %v, want it absent for a nil ID", got["uuid"])
	}
	if got["key"] != "env" || got["value"] != "production" || got["secured"] != false {
		t.Errorf("variable = %v, want key/value/secured all present", got)
	}
}

func TestPipelineStageAndResultString(t *testing.T) {
	if got := (PipelineStage{Name: "building"}).String(); got != "building" {
		t.Errorf("PipelineStage.String() = %q, want %q", got, "building")
	}
	if got := (PipelineResult{Name: "SUCCESSFUL"}).String(); got != "SUCCESSFUL" {
		t.Errorf("PipelineResult.String() = %q, want %q", got, "SUCCESSFUL")
	}
}
