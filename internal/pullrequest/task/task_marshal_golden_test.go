package task

import (
	"encoding/json"
	"testing"
	"time"

	"github.com/avitsrimer/bitbucket-cli/internal/common"
	"github.com/avitsrimer/bitbucket-cli/internal/user"
)

// golden byte fixtures captured from gildas/go-core v0.6.4's JSON marshaling (via a throwaway
// worktree and temporary capture tests, never committed), pinned here so Task.MarshalJSON's
// surrogate CreatedOn/UpdatedOn/ResolvedOn fields keep producing byte-identical output on the
// locally-ported common.Time, with and without resolved_on.
const (
	goldenTaskNoResolved   = `{"id":1,"content":{"raw":"do the thing"},"creator":{"type":"","uuid":"{00000000-0000-0000-0000-000000000000}","account_id":"","display_name":"alice","links":{}},"pending":true,"state":"OPEN","created_on":"2026-01-02T03:04:05Z","updated_on":"2026-01-03T04:05:06Z"}`
	goldenTaskWithResolved = `{"id":2,"content":{"raw":"do the thing"},"creator":{"type":"","uuid":"{00000000-0000-0000-0000-000000000000}","account_id":"","display_name":"alice","links":{}},"pending":false,"state":"RESOLVED","created_on":"2026-01-02T03:04:05Z","updated_on":"2026-01-03T04:05:06Z","resolved_on":"2026-01-04T05:06:07Z"}`
)

func TestTaskMarshalJSONGoldenWithoutResolvedOn(t *testing.T) {
	task := Task{
		ID:        1,
		Content:   common.RenderedText{Raw: "do the thing"},
		Creator:   user.User{Name: "alice"},
		IsPending: true,
		State:     "OPEN",
		CreatedOn: time.Date(2026, 1, 2, 3, 4, 5, 0, time.UTC),
		UpdatedOn: time.Date(2026, 1, 3, 4, 5, 6, 0, time.UTC),
	}

	data, err := json.Marshal(task)
	if err != nil {
		t.Fatal(err)
	}
	if got, want := string(data), goldenTaskNoResolved; got != want {
		t.Errorf("MarshalJSON() = %s, want %s", got, want)
	}
}

func TestTaskMarshalJSONGoldenWithResolvedOn(t *testing.T) {
	resolved := time.Date(2026, 1, 4, 5, 6, 7, 0, time.UTC)
	task := Task{
		ID:         2,
		Content:    common.RenderedText{Raw: "do the thing"},
		Creator:    user.User{Name: "alice"},
		IsPending:  false,
		State:      "RESOLVED",
		CreatedOn:  time.Date(2026, 1, 2, 3, 4, 5, 0, time.UTC),
		UpdatedOn:  time.Date(2026, 1, 3, 4, 5, 6, 0, time.UTC),
		ResolvedOn: &resolved,
	}

	data, err := json.Marshal(task)
	if err != nil {
		t.Fatal(err)
	}
	if got, want := string(data), goldenTaskWithResolved; got != want {
		t.Errorf("MarshalJSON() = %s, want %s", got, want)
	}
}
