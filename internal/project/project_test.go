package project

import (
	"encoding/json"
	"testing"
	"time"

	"github.com/avitsrimer/bitbucket-cli/internal/common"
	"github.com/avitsrimer/bitbucket-cli/internal/user"
	"github.com/avitsrimer/bitbucket-cli/internal/workspace"
)

func TestProjectString(t *testing.T) {
	target := Project{Name: "Open Source"}
	if got := target.String(); got != "Open Source" {
		t.Errorf("String() = %q, want %q", got, "Open Source")
	}
}

func TestProjectMarshalJSONOmitsNilOwnerAndWorkspace(t *testing.T) {
	target := Project{Key: "OS", Name: "Open Source"}

	data, err := json.Marshal(target)
	if err != nil {
		t.Fatalf("Marshal() error = %v", err)
	}

	var decoded map[string]any
	if err := json.Unmarshal(data, &decoded); err != nil {
		t.Fatalf("cannot unmarshal marshaled project: %v", err)
	}
	if _, present := decoded["owner"]; present {
		t.Errorf("expected owner to be omitted for a nil owner, got %v", decoded["owner"])
	}
	if _, present := decoded["workspace"]; present {
		t.Errorf("expected workspace to be omitted for a nil workspace, got %v", decoded["workspace"])
	}
	if _, present := decoded["created_on"]; present {
		t.Errorf("expected created_on to be omitted for a zero time, got %v", decoded["created_on"])
	}
}

func TestProjectMarshalJSONIncludesOwnerWorkspaceAndDates(t *testing.T) {
	createdOn := time.Date(2024, 1, 2, 3, 4, 5, 0, time.UTC)
	target := Project{
		Key:       "OS",
		Name:      "Open Source",
		Owner:     user.User{ID: common.NewUUID(), Name: "Jane Doe"},
		Workspace: workspace.Workspace{ID: common.NewUUID(), Slug: "acme"},
		CreatedOn: createdOn,
	}

	data, err := json.Marshal(target)
	if err != nil {
		t.Fatalf("Marshal() error = %v", err)
	}

	var decoded map[string]any
	if err := json.Unmarshal(data, &decoded); err != nil {
		t.Fatalf("cannot unmarshal marshaled project: %v", err)
	}
	if _, present := decoded["owner"]; !present {
		t.Error("expected owner to be present for a non-nil owner")
	}
	if _, present := decoded["workspace"]; !present {
		t.Error("expected workspace to be present for a non-nil workspace")
	}
	if _, present := decoded["created_on"]; !present {
		t.Error("expected created_on to be present for a non-zero time")
	}
}

func TestReviewerMarshalUnmarshalJSON(t *testing.T) {
	target := Reviewer{Type: "default_reviewer", ReviewerType: "user", User: user.User{Name: "Jane Doe"}}

	data, err := json.Marshal(target)
	if err != nil {
		t.Fatalf("Marshal() error = %v", err)
	}

	var decoded Reviewer
	if err := json.Unmarshal(data, &decoded); err != nil {
		t.Fatalf("Unmarshal() error = %v", err)
	}
	if decoded.User.Name != "Jane Doe" {
		t.Errorf("decoded.User.Name = %q, want %q", decoded.User.Name, "Jane Doe")
	}
	if decoded.ReviewerType != "user" {
		t.Errorf("decoded.ReviewerType = %q, want %q", decoded.ReviewerType, "user")
	}
}
