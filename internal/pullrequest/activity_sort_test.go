package pullrequest

import (
	"strings"
	"testing"
	"time"

	"github.com/avitsrimer/bitbucket-cli/internal/common"
	"github.com/avitsrimer/bitbucket-cli/internal/pullrequest/comment"
	"github.com/avitsrimer/bitbucket-cli/internal/repository"
	"github.com/avitsrimer/bitbucket-cli/internal/user"
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

	common.Sort(activities, activityColumns.SortBy("date"))

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

	common.Sort(activities, activityColumns.SortBy("user"))

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

	common.Sort(activities, activityColumns.SortBy("approved"))

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

	common.Sort(activities, activityColumns.SortBy("state"))

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

// activityMixedVariantFixtures registers, for every sortable column in activityColumns, a
// (low, high) pair of Activity values of DIFFERENT variants where low's summarize() field for
// that column sorts strictly before high's. Both carry a distinguishing PullRequest.ID (1 for
// low, 2 for high) as an identity marker independent of the column under test, since Activity
// itself is not comparable and every field this map varies belongs to a different pointer variant
// (Approval/ChangesRequested/Comment/Update).
//
// TestActivityColumnsAllSortersOrderMixedVariantsCorrectly iterates activityColumns.Sorters() and
// requires every one of them to have an entry here: a newly added comparator with no fixture
// registered fails that test immediately, and a comparator that regresses to a same-variant-only
// guard (the recurring bug this test exists to close) fails it too, since low/high are always two
// different variants.
var activityMixedVariantFixtures = map[string]struct{ low, high Activity }{
	"pull_request": {
		low:  Activity{PullRequest: PullRequestReference{ID: 1}},
		high: Activity{PullRequest: PullRequestReference{ID: 2}},
	},
	"date": {
		low:  Activity{PullRequest: PullRequestReference{ID: 1}, Comment: &comment.Comment{CreatedOn: time.Date(2024, 1, 1, 0, 0, 0, 0, time.UTC)}},
		high: Activity{PullRequest: PullRequestReference{ID: 2}, Update: &ActivityUpdate{Date: time.Date(2024, 12, 1, 0, 0, 0, 0, time.UTC)}},
	},
	"approved": {
		low:  Activity{PullRequest: PullRequestReference{ID: 1}, Comment: &comment.Comment{}},
		high: Activity{PullRequest: PullRequestReference{ID: 2}, Approval: &ActivityApproval{}},
	},
	"description": {
		low:  Activity{PullRequest: PullRequestReference{ID: 1}, Comment: &comment.Comment{}},
		high: Activity{PullRequest: PullRequestReference{ID: 2}, Update: &ActivityUpdate{Description: "zzz"}},
	},
	"state": {
		low:  Activity{PullRequest: PullRequestReference{ID: 1}, Approval: &ActivityApproval{}},
		high: Activity{PullRequest: PullRequestReference{ID: 2}, Update: &ActivityUpdate{State: "ZZZ"}},
	},
	"author": {
		low:  Activity{PullRequest: PullRequestReference{ID: 1}, Comment: &comment.Comment{}},
		high: Activity{PullRequest: PullRequestReference{ID: 2}, Update: &ActivityUpdate{Author: user.User{Name: "Zed"}}},
	},
	"closed_by": {
		low:  Activity{PullRequest: PullRequestReference{ID: 1}, Comment: &comment.Comment{}},
		high: Activity{PullRequest: PullRequestReference{ID: 2}, Update: &ActivityUpdate{ClosedBy: user.User{Name: "Zed"}}},
	},
	"reason": {
		low:  Activity{PullRequest: PullRequestReference{ID: 1}, Comment: &comment.Comment{}},
		high: Activity{PullRequest: PullRequestReference{ID: 2}, Update: &ActivityUpdate{Reason: "zzz"}},
	},
	"user": {
		low:  Activity{PullRequest: PullRequestReference{ID: 1}, Comment: &comment.Comment{User: user.User{Name: "Alice"}}},
		high: Activity{PullRequest: PullRequestReference{ID: 2}, Update: &ActivityUpdate{Author: user.User{Name: "Zed"}}},
	},
	"destination": {
		low:  Activity{PullRequest: PullRequestReference{ID: 1}, Comment: &comment.Comment{}},
		high: Activity{PullRequest: PullRequestReference{ID: 2}, Update: &ActivityUpdate{Destination: Endpoint{Repository: &repository.Repository{Name: "zzz"}}}},
	},
	"source": {
		low:  Activity{PullRequest: PullRequestReference{ID: 1}, Comment: &comment.Comment{}},
		high: Activity{PullRequest: PullRequestReference{ID: 2}, Update: &ActivityUpdate{Source: Endpoint{Repository: &repository.Repository{Name: "zzz"}}}},
	},
	"created_on": {
		low:  Activity{PullRequest: PullRequestReference{ID: 1}, Comment: &comment.Comment{}},
		high: Activity{PullRequest: PullRequestReference{ID: 2}, Update: &ActivityUpdate{CreatedOn: time.Date(2024, 12, 1, 0, 0, 0, 0, time.UTC)}},
	},
	"updated_on": {
		low:  Activity{PullRequest: PullRequestReference{ID: 1}, Comment: &comment.Comment{}},
		high: Activity{PullRequest: PullRequestReference{ID: 2}, Update: &ActivityUpdate{UpdatedOn: time.Date(2024, 12, 1, 0, 0, 0, 0, time.UTC)}},
	},
}

// TestActivityColumnsAllSortersOrderMixedVariantsCorrectly is the structural regression test for
// the mixed-feed sort bug that recurred across three earlier fixer rounds, each of which only
// patched the specific comparators a prior review named and left the rest broken: it drives every
// sortable column in activityColumns.Sorters() through activityMixedVariantFixtures, so a
// comparator added later (or one still gated on a specific variant) fails automatically instead of
// needing another review pass to notice it.
func TestActivityColumnsAllSortersOrderMixedVariantsCorrectly(t *testing.T) {
	for _, sorter := range activityColumns.Sorters() {
		name := strings.TrimPrefix(sorter, "+")
		t.Run(name, func(t *testing.T) {
			fixture, ok := activityMixedVariantFixtures[name]
			if !ok {
				t.Fatalf("no mixed-variant fixture registered for sortable column %q in activityMixedVariantFixtures -- add one so this comparator is proven variant-agnostic", name)
			}

			mixed := []Activity{fixture.high, fixture.low}
			common.Sort(mixed, activityColumns.SortBy(name))

			if mixed[0].PullRequest.ID != 1 || mixed[1].PullRequest.ID != 2 {
				t.Errorf("SortBy(%q) left [high, low] unordered (got PullRequest.ID %d, %d, want 1, 2): comparator likely still gates on a specific variant instead of resolving through summarize()", name, mixed[0].PullRequest.ID, mixed[1].PullRequest.ID)
			}
		})
	}
}
