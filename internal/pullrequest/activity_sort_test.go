package pullrequest

import (
	"testing"
	"time"

	"github.com/avitsrimer/bitbucket-cli/internal/pullrequest/comment"
	"github.com/avitsrimer/bitbucket-cli/internal/user"
	"github.com/gildas/go-core"
)

// TestActivityColumnsSortByDateOrdersMixedVariantsChronologically proves the "date" comparator
// orders a feed mixing every activity variant chronologically. Each comparison here is
// necessarily cross-variant (approval vs comment vs update): a comparator that only compares
// same-variant pairs (returning false for every cross-variant pair) leaves an out-of-order input
// slice unchanged, since sort.Slice never swaps two elements whose comparator returns false in
// both directions.
func TestActivityColumnsSortByDateOrdersMixedVariantsChronologically(t *testing.T) {
	early := time.Date(2024, 1, 1, 0, 0, 0, 0, time.UTC)
	middle := time.Date(2024, 6, 1, 0, 0, 0, 0, time.UTC)
	late := time.Date(2024, 12, 1, 0, 0, 0, 0, time.UTC)

	activities := []Activity{
		{Update: &ActivityUpdate{Date: late}},
		{Comment: &comment.Comment{CreatedOn: early}},
		{Approval: &ActivityApproval{Date: middle}},
	}

	core.Sort(activities, activityColumns.SortBy("date"))

	if activities[0].Comment == nil || !activities[0].Comment.CreatedOn.Equal(early) {
		t.Fatalf("expected the comment activity (earliest) first, got %+v", activities)
	}
	if activities[1].Approval == nil || !activities[1].Approval.Date.Equal(middle) {
		t.Fatalf("expected the approval activity (middle) second, got %+v", activities)
	}
	if activities[2].Update == nil || !activities[2].Update.Date.Equal(late) {
		t.Fatalf("expected the update activity (latest) third, got %+v", activities)
	}
}

// TestActivityColumnsSortByUserOrdersMixedVariantsAlphabetically is TestActivityColumnsSortByDateOrdersMixedVariantsChronologically
// for the "user" comparator: it must resolve each activity's actor (approver, changes-requester,
// commenter, or update author) regardless of variant, so cross-variant pairs order correctly
// instead of the sort silently no-opping.
func TestActivityColumnsSortByUserOrdersMixedVariantsAlphabetically(t *testing.T) {
	activities := []Activity{
		{Update: &ActivityUpdate{Author: user.User{Name: "Zed"}}},
		{ChangesRequested: &ActivityApproval{User: user.User{Name: "Alice"}}},
		{Comment: &comment.Comment{User: user.User{Name: "Mallory"}}},
	}

	core.Sort(activities, activityColumns.SortBy("user"))

	if activities[0].ChangesRequested == nil || activities[0].ChangesRequested.User.Name != "Alice" {
		t.Fatalf("expected Alice (changes_requested) first, got %+v", activities)
	}
	if activities[1].Comment == nil || activities[1].Comment.User.Name != "Mallory" {
		t.Fatalf("expected Mallory (comment) second, got %+v", activities)
	}
	if activities[2].Update == nil || activities[2].Update.Author.Name != "Zed" {
		t.Fatalf("expected Zed (update) third, got %+v", activities)
	}
}
