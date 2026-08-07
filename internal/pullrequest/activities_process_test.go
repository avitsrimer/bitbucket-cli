package pullrequest

import (
	"encoding/json"
	"net/http"
	"os"
	"strings"
	"testing"

	"github.com/avitsrimer/bitbucket-cli/internal/profile"
	"github.com/avitsrimer/bitbucket-cli/internal/testutil"
)

// TestFilterUnknownActivityKindsWarnsOncePerDistinctKind drives filterUnknownActivityKinds
// directly against a mix of known and unknown-kind activities decoded from a real fixture feed
// (FR-5's pinned design), proving: known entries pass through untouched, unknown-kind entries are
// dropped, and exactly one [WARN] is emitted per DISTINCT unknown kind no matter how many entries
// carry it.
func TestFilterUnknownActivityKindsWarnsOncePerDistinctKind(t *testing.T) {
	fixture, err := os.ReadFile("../../testdata/activity-feed-mixed.json")
	if err != nil {
		t.Fatalf("cannot read testdata: %v", err)
	}
	var page struct {
		Values []Activity `json:"values"`
	}
	if err := json.Unmarshal(fixture, &page); err != nil {
		t.Fatalf("cannot unmarshal fixture feed: %v", err)
	}
	if len(page.Values) != 7 {
		t.Fatalf("fixture feed decoded %d activities, want 7", len(page.Values))
	}

	logBuf := testutil.CaptureLog(t)
	known := filterUnknownActivityKinds("1650", page.Values)

	if len(known) != 4 {
		t.Fatalf("filterUnknownActivityKinds returned %d known activities, want 4 (approval, changes_requested, comment, update)", len(known))
	}
	for _, activity := range known {
		if activity.Approval == nil && activity.ChangesRequested == nil && activity.Comment == nil && activity.Update == nil {
			t.Errorf("filterUnknownActivityKinds kept an activity with no known variant: %+v", activity)
		}
	}

	logOutput := logBuf.String()
	for _, wantKind := range []string{"some_future_activity_kind", "another_new_activity_kind"} {
		count := strings.Count(logOutput, wantKind)
		if count != 1 {
			t.Errorf("log output mentions unknown kind %q %d times, want exactly 1 (deduped): %s", wantKind, count, logOutput)
		}
	}
	if warnCount := strings.Count(logOutput, "WARN"); warnCount != 2 {
		t.Errorf("log output has %d WARN lines, want exactly 2 (one per distinct unknown kind): %s", warnCount, logOutput)
	}
}

// TestActivitiesProcessRendersKnownEntriesFromAMixedFeed is the RunE-level regression test for
// FR-5: before the fix, an activity feed containing even one activity Activity.Validate did not
// recognize aborted json.Unmarshal for the WHOLE page (profile.GetAll decodes one page in a single
// json.Unmarshal call), so activitiesProcess returned an error and rendered nothing at all. After
// the fix, it must succeed, print every known entry, and warn once per distinct unknown kind.
func TestActivitiesProcessRendersKnownEntriesFromAMixedFeed(t *testing.T) {
	fixture, err := os.ReadFile("../../testdata/activity-feed-mixed.json")
	if err != nil {
		t.Fatalf("cannot read testdata: %v", err)
	}

	cmd := setupTest(t, func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write(fixture)
	}, false)

	// profile.GetProfileFromCommand's first call in the whole test binary resolves the local
	// config via common.Initialize, which unconditionally resets the global lgr logger to
	// os.Stderr. Warming that resolution up here, before CaptureLog installs its buffer, keeps
	// activitiesProcess's own (later) call to it a no-op that leaves the buffer redirect intact
	// for the [WARN] lines this test asserts on.
	if _, err := profile.GetProfileFromCommand(cmd.Context(), cmd); err != nil {
		t.Fatalf("cannot warm up profile resolution: %v", err)
	}

	logBuf := testutil.CaptureLog(t)
	var runErr error
	stdout := testutil.CaptureStdout(t, func() {
		runErr = activitiesProcess(cmd, []string{"1650"})
	})
	if runErr != nil {
		t.Fatalf("activitiesProcess() error = %v, want nil (a mixed feed with unknown kinds must not fail the command)", runErr)
	}

	var activities []Activity
	if err := json.Unmarshal([]byte(stdout), &activities); err != nil {
		t.Fatalf("cannot unmarshal printed output %q: %v", stdout, err)
	}
	if len(activities) != 4 {
		t.Fatalf("printed %d activities, want 4 (the unknown-kind entries must be skipped, not printed)", len(activities))
	}

	logOutput := logBuf.String()
	if warnCount := strings.Count(logOutput, "WARN"); warnCount != 2 {
		t.Errorf("log output has %d WARN lines, want exactly 2 (one per distinct unknown kind): %s", warnCount, logOutput)
	}
}

// TestActivitiesProcessLimitAppliesAfterFilteringUnknownKinds is a regression test: --limit must
// count only KNOWN activities, not the raw, unfiltered feed. Before the fix, profile.GetAll
// applied --limit to the raw page (known and unknown kinds together), so a feed containing
// unrecognized entries could silently return fewer than --limit known activities. The fixture
// here has 3 unknown-kind entries interleaved before 2 known ones; --limit 2 must still return
// both known entries.
func TestActivitiesProcessLimitAppliesAfterFilteringUnknownKinds(t *testing.T) {
	const fixture = `{"values":[
		{"pull_request":{"id":1650},"some_kind":{"note":"unknown 1"}},
		{"pull_request":{"id":1650},"some_kind":{"note":"unknown 2"}},
		{"pull_request":{"id":1650},"some_kind":{"note":"unknown 3"}},
		{"pull_request":{"id":1650},"approval":{"date":"2026-08-01T00:00:00+00:00","user":{"display_name":"A"}}},
		{"pull_request":{"id":1650},"approval":{"date":"2026-08-02T00:00:00+00:00","user":{"display_name":"B"}}}
	],"next":""}`

	cmd := setupTest(t, func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(fixture))
	}, false)
	if err := cmd.Flags().Set("limit", "2"); err != nil {
		t.Fatalf("cannot set limit flag: %v", err)
	}

	if _, err := profile.GetProfileFromCommand(cmd.Context(), cmd); err != nil {
		t.Fatalf("cannot warm up profile resolution: %v", err)
	}

	var runErr error
	stdout := testutil.CaptureStdout(t, func() {
		runErr = activitiesProcess(cmd, []string{"1650"})
	})
	if runErr != nil {
		t.Fatalf("activitiesProcess() error = %v", runErr)
	}

	var activities []Activity
	if err := json.Unmarshal([]byte(stdout), &activities); err != nil {
		t.Fatalf("cannot unmarshal printed output %q: %v", stdout, err)
	}
	if len(activities) != 2 {
		t.Fatalf("printed %d activities, want 2 known activities despite --limit=2 also matching 3 unknown-kind entries in the raw feed", len(activities))
	}
}

// unmarshalActivityRaw decodes raw into the map[string]json.RawMessage form Activity.UnmarshalJSON
// itself decodes an entry into, since unrecognizedActivityVariant now takes that decoded map
// rather than re-parsing the entry's raw bytes.
func unmarshalActivityRaw(t *testing.T, raw string) map[string]json.RawMessage {
	t.Helper()
	var decoded map[string]json.RawMessage
	if err := json.Unmarshal([]byte(raw), &decoded); err != nil {
		t.Fatalf("cannot unmarshal test fixture %q: %v", raw, err)
	}
	return decoded
}

// TestUnrecognizedActivityVariantPrefersObjectValuedKey proves that when an entry carries multiple
// unrecognized keys of mixed shapes, an object-valued one is preferred over a scalar-valued one,
// since a real new activity kind is expected to serialize as an object (like every existing kind)
// -- so the [WARN] this drives names the actual new kind ("some_future_activity_kind") instead of
// incidental scalar metadata riding along on the same entry ("id").
func TestUnrecognizedActivityVariantPrefersObjectValuedKey(t *testing.T) {
	raw := unmarshalActivityRaw(t, `{"pull_request":{"id":1650},"id":999,"some_future_activity_kind":{"date":"2026-08-01T00:00:00+00:00"}}`)

	variant, found := unrecognizedActivityVariant(raw)
	if !found {
		t.Fatal("unrecognizedActivityVariant() found = false, want true")
	}
	if variant != "some_future_activity_kind" {
		t.Errorf("unrecognizedActivityVariant() = %q, want %q (object-valued key preferred over scalar)", variant, "some_future_activity_kind")
	}
}

// TestUnrecognizedActivityVariantFallsBackToLexicographicallyFirstWhenNoObjectCandidate proves the
// pre-existing lexicographically-first tiebreak still applies when none of the unrecognized keys
// are object-valued.
func TestUnrecognizedActivityVariantFallsBackToLexicographicallyFirstWhenNoObjectCandidate(t *testing.T) {
	raw := unmarshalActivityRaw(t, `{"pull_request":{"id":1650},"id":999,"links":"not an object"}`)

	variant, found := unrecognizedActivityVariant(raw)
	if !found {
		t.Fatal("unrecognizedActivityVariant() found = false, want true")
	}
	if variant != "id" {
		t.Errorf("unrecognizedActivityVariant() = %q, want %q (lexicographically first among non-object candidates)", variant, "id")
	}
}

// TestUnrecognizedActivityVariantNoKeyBesidesPullRequest proves an entry carrying no key besides
// "pull_request" reports not found -- this is the genuinely malformed case that must still error,
// per the read-paths-tolerate-unrecognized-VALUES-not-missing-content rule.
func TestUnrecognizedActivityVariantNoKeyBesidesPullRequest(t *testing.T) {
	raw := unmarshalActivityRaw(t, `{"pull_request":{"id":1650}}`)

	_, found := unrecognizedActivityVariant(raw)
	if found {
		t.Error("unrecognizedActivityVariant() found = true, want false (no key besides pull_request)")
	}
}

// TestUnrecognizedActivityVariantAcceptsArrayValuedVariant proves an unrecognized top-level key
// whose value is a JSON array is tolerated, not just object-shaped ones: a future activity kind
// serialized as an array must not fail decoding of the whole entry (and therefore the whole
// page).
func TestUnrecognizedActivityVariantAcceptsArrayValuedVariant(t *testing.T) {
	raw := unmarshalActivityRaw(t, `{"pull_request":{"id":1650},"reviewers_changed":[]}`)

	variant, found := unrecognizedActivityVariant(raw)
	if !found {
		t.Fatal("unrecognizedActivityVariant() found = false, want true (array-valued unknown variant)")
	}
	if variant != "reviewers_changed" {
		t.Errorf("unrecognizedActivityVariant() = %q, want %q", variant, "reviewers_changed")
	}
}

// TestUnrecognizedActivityVariantAcceptsScalarValuedVariant proves an unrecognized top-level key
// whose value is a JSON scalar is tolerated, not just object-shaped ones.
func TestUnrecognizedActivityVariantAcceptsScalarValuedVariant(t *testing.T) {
	raw := unmarshalActivityRaw(t, `{"pull_request":{"id":1650},"some_scalar_kind":"just a string"}`)

	variant, found := unrecognizedActivityVariant(raw)
	if !found {
		t.Fatal("unrecognizedActivityVariant() found = false, want true (scalar-valued unknown variant)")
	}
	if variant != "some_scalar_kind" {
		t.Errorf("unrecognizedActivityVariant() = %q, want %q", variant, "some_scalar_kind")
	}
}

// TestActivityUnmarshalTreatsArrayValuedKnownVariantAsUnknown proves a known variant key
// ("approval" here) whose value has the wrong SHAPE (an array, not an object) is tolerated exactly
// like a genuinely unrecognized kind, instead of erroring the whole entry's decode: decoding
// succeeds and unknownVariant records "approval".
func TestActivityUnmarshalTreatsArrayValuedKnownVariantAsUnknown(t *testing.T) {
	data := []byte(`{"pull_request":{"id":1650},"approval":[]}`)

	var activity Activity
	if err := json.Unmarshal(data, &activity); err != nil {
		t.Fatalf("json.Unmarshal() error = %v, want nil (a malformed known variant must be tolerated, not error the whole entry)", err)
	}
	if activity.Approval != nil {
		t.Errorf("activity.Approval = %+v, want nil (array-valued approval must not populate the struct)", activity.Approval)
	}
	if activity.unknownVariant != "approval" {
		t.Errorf("activity.unknownVariant = %q, want %q", activity.unknownVariant, "approval")
	}
}

// TestActivityUnmarshalTreatsWrongFieldTypeInKnownVariantAsUnknown is
// TestActivityUnmarshalTreatsArrayValuedKnownVariantAsUnknown for a subtler malformation: the
// variant key itself is object-shaped as expected, but one of ITS fields has the wrong JSON type
// (ActivityUpdate.ID is numeric; "abc" is a string).
func TestActivityUnmarshalTreatsWrongFieldTypeInKnownVariantAsUnknown(t *testing.T) {
	data := []byte(`{"pull_request":{"id":1650},"update":{"id":"abc"}}`)

	var activity Activity
	if err := json.Unmarshal(data, &activity); err != nil {
		t.Fatalf("json.Unmarshal() error = %v, want nil", err)
	}
	if activity.Update != nil {
		t.Errorf("activity.Update = %+v, want nil", activity.Update)
	}
	if activity.unknownVariant != "update" {
		t.Errorf("activity.unknownVariant = %q, want %q", activity.unknownVariant, "update")
	}
}

// TestActivityUnmarshalTreatsUnparsableDateInKnownVariantAsUnknown is
// TestActivityUnmarshalTreatsWrongFieldTypeInKnownVariantAsUnknown for a non-RFC3339 date string,
// which fails time.Time's own UnmarshalJSON.
func TestActivityUnmarshalTreatsUnparsableDateInKnownVariantAsUnknown(t *testing.T) {
	data := []byte(`{"pull_request":{"id":1650},"update":{"date":"08/01/2026"}}`)

	var activity Activity
	if err := json.Unmarshal(data, &activity); err != nil {
		t.Fatalf("json.Unmarshal() error = %v, want nil", err)
	}
	if activity.Update != nil {
		t.Errorf("activity.Update = %+v, want nil", activity.Update)
	}
	if activity.unknownVariant != "update" {
		t.Errorf("activity.unknownVariant = %q, want %q", activity.unknownVariant, "update")
	}
}

// TestActivityUnmarshalNullKnownVariantAloneMatchesNoVariantAtAllError pins that a null known
// variant is treated exactly like an absent one, never decoded into a fabricated non-nil,
// zero-valued struct (json.Unmarshal of JSON null into a value-typed destination behind a pointer
// field is a silent no-op that would otherwise leave activity.Approval != nil with every field
// zero, which summarize() would render as a false "approved" row and -o json would emit as an
// "approval":{} the real response never sent): with nothing else on the entry, decoding must
// produce the same hard error as an entry carrying no variant key at all.
func TestActivityUnmarshalNullKnownVariantAloneMatchesNoVariantAtAllError(t *testing.T) {
	nullApproval := []byte(`{"pull_request":{"id":1650},"approval":null}`)
	noVariantAtAll := []byte(`{"pull_request":{"id":1650}}`)

	var withNullApproval Activity
	errNullApproval := json.Unmarshal(nullApproval, &withNullApproval)
	if errNullApproval == nil {
		t.Fatalf("json.Unmarshal(%s) error = nil, want an error (null approval leaves the entry with no decodable variant)", nullApproval)
	}
	if withNullApproval.Approval != nil {
		t.Errorf("activity.Approval = %+v, want nil (a null approval must not fabricate a zero-valued struct)", withNullApproval.Approval)
	}

	var withNoVariantAtAll Activity
	errNoVariantAtAll := json.Unmarshal(noVariantAtAll, &withNoVariantAtAll)
	if errNoVariantAtAll == nil {
		t.Fatalf("json.Unmarshal(%s) error = nil, want an error", noVariantAtAll)
	}

	if errNullApproval.Error() != errNoVariantAtAll.Error() {
		t.Errorf("null-approval error = %q, no-variant-at-all error = %q, want identical (a null known variant must be treated exactly like an absent one)", errNullApproval, errNoVariantAtAll)
	}
}

// TestActivityUnmarshalNullKnownVariantLetsSiblingVariantDecode is
// TestActivityUnmarshalNullKnownVariantAloneMatchesNoVariantAtAllError's positive counterpart:
// before the fix, {"approval":null,"comment":{…}} lost the real comment entirely, because the
// switch in UnmarshalJSON matched the "approval" case first (raw["approval"] is present, even
// though its value is null) and never even reached the "comment" case. After the fix, a null
// known variant is treated as absent, so the switch falls through to the comment that IS present.
func TestActivityUnmarshalNullKnownVariantLetsSiblingVariantDecode(t *testing.T) {
	data := []byte(`{"pull_request":{"id":1650},"approval":null,"comment":{"id":42,"content":{"raw":"hi"}}}`)

	var activity Activity
	if err := json.Unmarshal(data, &activity); err != nil {
		t.Fatalf("json.Unmarshal() error = %v, want nil (a valid sibling variant must still decode)", err)
	}
	if activity.Approval != nil {
		t.Errorf("activity.Approval = %+v, want nil", activity.Approval)
	}
	if activity.Comment == nil {
		t.Fatal("activity.Comment = nil, want the comment to decode despite the null sibling \"approval\" key")
	}
	if activity.Comment.ID != 42 {
		t.Errorf("activity.Comment.ID = %d, want 42", activity.Comment.ID)
	}
	if activity.unknownVariant != "" {
		t.Errorf("activity.unknownVariant = %q, want empty (a decoded comment is a known variant, not an unknown one)", activity.unknownVariant)
	}
}

// TestActivitiesProcessTreatsMalformedKnownVariantAsUnknownAcrossThePage is the RunE-level
// regression test proving a page containing malformed known-variant entries (wrong shape, wrong
// field type, unparsable date) must not abort the whole page's decode; the one well-formed entry
// must still render, and the malformed ones must be skipped, not printed.
func TestActivitiesProcessTreatsMalformedKnownVariantAsUnknownAcrossThePage(t *testing.T) {
	const fixture = `{"values":[
		{"pull_request":{"id":1650},"approval":{"date":"2026-08-01T00:00:00+00:00","user":{"display_name":"A"}}},
		{"pull_request":{"id":1650},"approval":[]},
		{"pull_request":{"id":1650},"update":{"id":"abc"}},
		{"pull_request":{"id":1650},"update":{"date":"08/01/2026"}}
	],"next":""}`

	cmd := setupTest(t, func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(fixture))
	}, false)

	if _, err := profile.GetProfileFromCommand(cmd.Context(), cmd); err != nil {
		t.Fatalf("cannot warm up profile resolution: %v", err)
	}

	var runErr error
	stdout := testutil.CaptureStdout(t, func() {
		runErr = activitiesProcess(cmd, []string{"1650"})
	})
	if runErr != nil {
		t.Fatalf("activitiesProcess() error = %v, want nil (a malformed known variant must not fail the whole page)", runErr)
	}

	var activities []Activity
	if err := json.Unmarshal([]byte(stdout), &activities); err != nil {
		t.Fatalf("cannot unmarshal printed output %q: %v", stdout, err)
	}
	if len(activities) != 1 {
		t.Fatalf("printed %d activities, want 1 (the malformed known-variant entries must be skipped, not printed)", len(activities))
	}
}

// TestActivitiesProcessToleratesArrayAndScalarUnknownVariants is the RunE-level regression test
// proving an activity feed containing an unknown variant serialized as an array or a scalar must
// not fail json.Unmarshal on that entry (and therefore the whole page, decoded in a single
// json.Unmarshal call by profile.GetAll), per FR-5's "never blind the feed" guarantee: the page
// must still decode, the known entry must render, and the array/scalar-valued unknown entries
// must be skipped, not printed.
func TestActivitiesProcessToleratesArrayAndScalarUnknownVariants(t *testing.T) {
	const fixture = `{"values":[
		{"pull_request":{"id":1650},"approval":{"date":"2026-08-01T00:00:00+00:00","user":{"display_name":"A"}}},
		{"pull_request":{"id":1650},"reviewers_changed":[]},
		{"pull_request":{"id":1650},"some_scalar_kind":"just a string"}
	],"next":""}`

	cmd := setupTest(t, func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(fixture))
	}, false)

	if _, err := profile.GetProfileFromCommand(cmd.Context(), cmd); err != nil {
		t.Fatalf("cannot warm up profile resolution: %v", err)
	}

	var runErr error
	stdout := testutil.CaptureStdout(t, func() {
		runErr = activitiesProcess(cmd, []string{"1650"})
	})
	if runErr != nil {
		t.Fatalf("activitiesProcess() error = %v, want nil (array/scalar-valued unknown variants must not fail the whole page)", runErr)
	}

	var activities []Activity
	if err := json.Unmarshal([]byte(stdout), &activities); err != nil {
		t.Fatalf("cannot unmarshal printed output %q: %v", stdout, err)
	}
	if len(activities) != 1 {
		t.Fatalf("printed %d activities, want 1 (the array/scalar-valued unknown-kind entries must be skipped, not printed)", len(activities))
	}
}
