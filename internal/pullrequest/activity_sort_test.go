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

// TestActivityColumnsSortByApprovedOrdersMixedVariantsByBoolean is
// TestActivityColumnsSortByDateOrdersMixedVariantsChronologically for the "approved" comparator:
// it must resolve each activity's approved boolean (via summarize) regardless of variant, so the
// sole approved entry orders relative to the unapproved comment/update instead of the sort
// silently no-opping across variants.
func TestActivityColumnsSortByApprovedOrdersMixedVariantsByBoolean(t *testing.T) {
	activities := []Activity{
		{Approval: &ActivityApproval{User: user.User{Name: "Zed"}}},
		{Comment: &comment.Comment{User: user.User{Name: "Alice"}}},
		{Update: &ActivityUpdate{Author: user.User{Name: "Bob"}}},
	}

	core.Sort(activities, activityColumns.SortBy("approved"))

	if activities[2].Approval == nil {
		t.Fatalf("expected the approval activity last (approved=true sorts after approved=false), got %+v", activities)
	}
	for i, activity := range activities[:2] {
		if activity.summarize().approved {
			t.Fatalf("expected activities[%d] to be unapproved, got %+v", i, activity)
		}
	}
}

// TestActivityColumnsSortByApprovedIgnoresUserName proves the "approved" comparator orders solely
// by the boolean GetRow renders, not by the approver's name: two approvals must compare equal
// (order-preserving) regardless of their User.Name.
func TestActivityColumnsSortByApprovedIgnoresUserName(t *testing.T) {
	compare := activityColumns.SortBy("approved")

	alice := Activity{Approval: &ActivityApproval{User: user.User{Name: "Alice"}}}
	zed := Activity{Approval: &ActivityApproval{User: user.User{Name: "Zed"}}}

	if compare(alice, zed) {
		t.Errorf("compare(alice, zed) = true, want false: both approved, ordering must not depend on User.Name")
	}
	if compare(zed, alice) {
		t.Errorf("compare(zed, alice) = true, want false: both approved, ordering must not depend on User.Name")
	}
}

// TestActivityColumnsSortByStateOrdersMixedVariantsAlphabetically is
// TestActivityColumnsSortByDateOrdersMixedVariantsChronologically for the "state" comparator: it
// must resolve each activity's state (via summarize) regardless of variant, so cross-variant
// pairs order correctly instead of the sort silently no-opping.
func TestActivityColumnsSortByStateOrdersMixedVariantsAlphabetically(t *testing.T) {
	activities := []Activity{
		{Update: &ActivityUpdate{State: "ZZZ"}},
		{Approval: &ActivityApproval{}},
		{ChangesRequested: &ActivityApproval{}},
	}

	core.Sort(activities, activityColumns.SortBy("state"))

	if activities[0].ChangesRequested == nil {
		t.Fatalf("expected the changes_requested activity (state CHANGES_REQUESTED) first, got %+v", activities)
	}
	if activities[1].Approval == nil {
		t.Fatalf("expected the approval activity (state N/A) second, got %+v", activities)
	}
	if activities[2].Update == nil || activities[2].Update.State != "ZZZ" {
		t.Fatalf("expected the update activity (state ZZZ) third, got %+v", activities)
	}
}
