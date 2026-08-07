package pullrequest

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/avitsrimer/bitbucket-cli/internal/commit"
	"github.com/avitsrimer/bitbucket-cli/internal/common"
	"github.com/avitsrimer/bitbucket-cli/internal/pullrequest/comment"
	"github.com/avitsrimer/bitbucket-cli/internal/user"
	"github.com/spf13/cobra"
)

// activityKnownVariants are the top-level JSON keys Activity's UnmarshalJSON recognizes, besides
// "pull_request" (present on every entry). Per the Bitbucket Cloud REST API's documentation for
// GET /repositories/{workspace}/{repo_slug}/pullrequests/{pull_request_id}/activity, the feed
// emits three entry kinds -- "update" (state/description/reviewer changes), "approval", and
// "comment" -- plus "changes_requested", added for the request-changes review action. Removing an
// approval or a changes-requested review does not add its own feed entry (it just disappears from
// the pull request's participants), so there is no separate "removal" variant to model.
var activityKnownVariants = map[string]struct{}{
	"pull_request":      {},
	"approval":          {},
	"changes_requested": {},
	"comment":           {},
	"update":            {},
}

// Activity describes an activity on a PullRequest: an approval, a request-for-changes, a comment,
// or an update. A variant this type does not recognize is tolerated rather than rejected (see
// UnmarshalJSON): decoding still succeeds, and unknownVariant records the raw JSON key so the
// caller can skip the entry and warn about it instead of the whole feed failing to decode.
type Activity struct {
	PullRequest      PullRequestReference `json:"pull_request"`
	Approval         *ActivityApproval    `json:"approval,omitempty"`
	ChangesRequested *ActivityApproval    `json:"changes_requested,omitempty"`
	Comment          *comment.Comment     `json:"comment,omitempty"`
	Update           *ActivityUpdate      `json:"update,omitempty"`

	// unknownVariant is the raw JSON key of an activity-feed entry kind this type does not
	// recognize (e.g. a new kind BitBucket adds later), set by UnmarshalJSON. It is deliberately
	// unexported: MarshalJSON's surrogate copy only carries exported fields into json.Marshal, so
	// this never reaches json/yaml/table output; activitiesProcess reads it directly since it is
	// in the same package.
	unknownVariant string
}

// ActivityApproval describes an approval activity on a PullRequest
type ActivityApproval struct {
	Date        time.Time             `json:"date"`
	User        user.User             `json:"user"`
	PullRequest *PullRequestReference `json:"pullrequest"`
}

// ActivityUpdate describes an update activity on a PullRequest
type ActivityUpdate struct {
	Date              time.Time           `json:"date"`
	Type              string              `json:"type"`
	ID                uint64              `json:"id"`
	Title             string              `json:"title"`
	Description       string              `json:"description"`
	Summary           common.RenderedText `json:"summary"`
	State             string              `json:"state"`
	MergeCommit       *commit.Commit      `json:"merge_commit,omitempty"`
	CloseSourceBranch bool                `json:"close_source_branch"`
	ClosedBy          user.User           `json:"closed_by"`
	Author            user.User           `json:"author"`
	Reason            string              `json:"reason"`
	Destination       Endpoint            `json:"destination"`
	Source            Endpoint            `json:"source"`
	Links             common.Links        `json:"links"`
	CommentCount      uint64              `json:"comment_count"`
	TaskCount         uint64              `json:"task_count"`
	CreatedOn         time.Time           `json:"created_on"`
	UpdatedOn         time.Time           `json:"updated_on"`
}

// MarshalJSON implements the json.Marshaler interface.
//
// Activity.MarshalJSON (below) marshals its embedded *ActivityUpdate through the surrogate
// wrapper unchanged, which would otherwise fall back to time.Time's own zero-value marshaling
// for CreatedOn/UpdatedOn -- "0001-01-01T00:00:00Z", a year-1 timestamp with no meaning to a
// caller scripting against it -- on any ActivityUpdate built by hand rather than decoded from a
// real API response. CreatedOn/UpdatedOn are only formatted (and only included at all, via
// omitempty) when non-zero, matching the pattern already used by PullRequest/Comment/Resolution.
func (update ActivityUpdate) MarshalJSON() (data []byte, err error) {
	type surrogate ActivityUpdate

	data, err = json.Marshal(struct {
		surrogate
		CreatedOn *string `json:"created_on,omitempty"`
		UpdatedOn *string `json:"updated_on,omitempty"`
	}{
		surrogate: surrogate(update),
		CreatedOn: common.FormatOptionalTime(update.CreatedOn),
		UpdatedOn: common.FormatOptionalTime(update.UpdatedOn),
	})
	if err != nil {
		return nil, fmt.Errorf("cannot marshal activity update to json: %w", err)
	}
	return data, nil
}

var activityColumns = common.Columns[Activity]{
	{Name: "pull_request", DefaultSorter: true, Compare: func(a, b Activity) bool {
		return a.PullRequest.ID < b.PullRequest.ID
	}},
	{Name: "date", DefaultSorter: false, Compare: func(a, b Activity) bool {
		return a.summarize().date.Before(b.summarize().date)
	}},
	{Name: "approved", DefaultSorter: false, Compare: func(a, b Activity) bool {
		return !a.summarize().approved && b.summarize().approved
	}},
	{Name: "description", DefaultSorter: false, Compare: func(a, b Activity) bool {
		return strings.ToLower(a.summarize().description) < strings.ToLower(b.summarize().description)
	}},
	{Name: "state", DefaultSorter: false, Compare: func(a, b Activity) bool {
		return strings.ToLower(a.summarize().state) < strings.ToLower(b.summarize().state)
	}},
	{Name: "author", DefaultSorter: false, Compare: func(a, b Activity) bool {
		return strings.ToLower(a.summarize().author) < strings.ToLower(b.summarize().author)
	}},
	{Name: "closed_by", DefaultSorter: false, Compare: func(a, b Activity) bool {
		return strings.ToLower(a.summarize().closedBy) < strings.ToLower(b.summarize().closedBy)
	}},
	{Name: "reason", DefaultSorter: false, Compare: func(a, b Activity) bool {
		return strings.ToLower(a.summarize().reason) < strings.ToLower(b.summarize().reason)
	}},
	{Name: "user", DefaultSorter: false, Compare: func(a, b Activity) bool {
		return strings.ToLower(a.summarize().actor.Name) < strings.ToLower(b.summarize().actor.Name)
	}},
	{Name: "destination", DefaultSorter: false, Compare: func(a, b Activity) bool {
		return strings.ToLower(a.summarize().destination) < strings.ToLower(b.summarize().destination)
	}},
	{Name: "source", DefaultSorter: false, Compare: func(a, b Activity) bool {
		return strings.ToLower(a.summarize().source) < strings.ToLower(b.summarize().source)
	}},
	{Name: "created_on", DefaultSorter: false, Compare: func(a, b Activity) bool {
		return a.summarize().createdOn.Before(b.summarize().createdOn)
	}},
	{Name: "updated_on", DefaultSorter: false, Compare: func(a, b Activity) bool {
		return a.summarize().updatedOn.Before(b.summarize().updatedOn)
	}},
}

// GetHeaders gets the header for a table
//
// implements common.Tableable
func (activity Activity) GetHeaders(cmd *cobra.Command) []string {
	return common.HeadersFromFlag(cmd, "Date", "Approved", "State", "User")
}

// activitySummary holds every field activityColumns' comparators and GetRow's per-column switch
// need, resolved once by summarize (instead of inline in either) so both stay variant-agnostic:
// a field this activity's variant has no value for (e.g. description on an Approval) simply
// resolves to its zero value, which sorts and renders consistently against every other variant
// instead of requiring a same-variant guard. GetRow still renders description/author/closed_by/
// reason/destination/source/created_on/updated_on as common.EmptyCell for a non-Update activity
// (via updateField) -- summarize's zero-valued fields only drive ordering, not that cell text.
type activitySummary struct {
	date        time.Time
	approved    bool
	state       string
	actor       user.User
	description string
	author      string
	closedBy    string
	reason      string
	destination string
	source      string
	createdOn   time.Time
	updatedOn   time.Time
}

// summarize resolves every activitySummary field from whichever variant activity carries. A
// variant this type does not recognize (unknownVariant set) resolves to the zero activitySummary.
// Only ActivityUpdate populates description/author/closed_by/reason/destination/source/
// created_on/updated_on -- every other variant leaves them at their zero value, which is exactly
// what makes the comparators in activityColumns that sort by those fields variant-agnostic.
func (activity Activity) summarize() activitySummary {
	switch {
	case activity.Approval != nil:
		return activitySummary{date: activity.Approval.Date, approved: true, actor: activity.Approval.User, state: "N/A"}
	case activity.ChangesRequested != nil:
		return activitySummary{date: activity.ChangesRequested.Date, actor: activity.ChangesRequested.User, state: "CHANGES_REQUESTED"}
	case activity.Comment != nil:
		return activitySummary{date: activity.Comment.CreatedOn, actor: activity.Comment.User, state: "N/A"}
	case activity.Update != nil:
		return activitySummary{
			date:        activity.Update.Date,
			state:       activity.Update.State,
			actor:       activity.Update.Author,
			description: activity.Update.Description,
			author:      activity.Update.Author.Name,
			closedBy:    activity.Update.ClosedBy.Name,
			reason:      activity.Update.Reason,
			destination: endpointRepositoryName(activity.Update.Destination),
			source:      endpointRepositoryName(activity.Update.Source),
			createdOn:   activity.Update.CreatedOn,
			updatedOn:   activity.Update.UpdatedOn,
		}
	default:
		return activitySummary{}
	}
}

// endpointRepositoryName returns endpoint.Repository's Name, or "" when endpoint carries no
// repository (e.g. a source/destination whose repository was deleted after the pull request was
// opened).
func endpointRepositoryName(endpoint Endpoint) string {
	if endpoint.Repository == nil {
		return ""
	}
	return endpoint.Repository.Name
}

// GetRow gets the row for a table
//
// implements common.Tableable
func (activity Activity) GetRow(headers []string) []string {
	summary := activity.summarize()

	row := make([]string, 0, len(headers))
	for _, header := range headers {
		switch common.NormalizeColumnKey(header) {
		case "pull_request":
			row = append(row, strconv.FormatUint(activity.PullRequest.ID, 10))
		case "date":
			row = append(row, common.TimeCell(summary.date))
		case "approved":
			row = append(row, strconv.FormatBool(summary.approved))
		case "description":
			row = append(row, activity.updateField(func(update *ActivityUpdate) string { return update.Description }))
		case "state":
			row = append(row, summary.state)
		case "author":
			row = append(row, activity.updateField(func(update *ActivityUpdate) string { return update.Author.Name }))
		case "closed_by":
			row = append(row, activity.updateField(func(update *ActivityUpdate) string { return update.ClosedBy.Name }))
		case "reason":
			row = append(row, activity.updateField(func(update *ActivityUpdate) string { return update.Reason }))
		case "user":
			row = append(row, summary.actor.Name)
		case "destination":
			row = append(row, activity.updateField(func(update *ActivityUpdate) string {
				if update.Destination.Repository == nil {
					return common.EmptyCell
				}
				return update.Destination.Repository.Name
			}))
		case "source":
			row = append(row, activity.updateField(func(update *ActivityUpdate) string {
				if update.Source.Repository == nil {
					return common.EmptyCell
				}
				return update.Source.Repository.Name
			}))
		case "created_on", "created":
			row = append(row, activity.updateField(func(update *ActivityUpdate) string { return common.TimeCell(update.CreatedOn) }))
		case "updated_on", "updated":
			row = append(row, activity.updateField(func(update *ActivityUpdate) string { return common.TimeCell(update.UpdatedOn) }))
		default:
			row = append(row, common.EmptyCell)
		}
	}
	return row
}

// updateField returns common.EmptyCell when activity has no Update, otherwise the value returned
// by get.
func (activity Activity) updateField(get func(*ActivityUpdate) string) string {
	if activity.Update == nil {
		return common.EmptyCell
	}
	return get(activity.Update)
}

// String gets a string representation of this pullrequest
//
// implements fmt.Stringer
func (activity Activity) String() string {
	return activity.PullRequest.Title
}

// MarshalJSON implements the json.Marshaler interface.
//
// implements json.Marshaler
func (activity Activity) MarshalJSON() (data []byte, err error) {
	type surrogate Activity

	data, err = json.Marshal(struct {
		surrogate
	}{
		surrogate: surrogate(activity),
	})
	if err != nil {
		return nil, fmt.Errorf("cannot marshal activity to json: %w", err)
	}
	return data, nil
}

// UnmarshalJSON implements the json.Unmarshaler interface.
//
// A malformed JSON payload still errors. The switch below tries the known variant keys (approval,
// changes_requested, comment, update) in that fixed order and decodes only the FIRST one present
// on the entry -- real feed entries only ever carry one variant, so this is not a practical
// limitation, but it does mean a hypothetical entry carrying two known variant keys would only
// ever decode the earlier one in that order. A key whose raw value is the JSON literal null is
// treated as though it were absent (see decodeActivityVariant), so {"approval":null,"comment":{...}}
// still decodes the comment instead of fabricating a zero-valued Approval from the null and never
// reaching comment. Among the keys actually tried, one that IS present but carries the wrong shape
// or an unparsable field (e.g. {"approval":[]}, an array where an object is expected, or
// {"update":{"id":"abc"}}, a string where ActivityUpdate.ID expects a number) is tolerated exactly
// like an unrecognized kind -- unknownVariant records that key and decoding succeeds -- instead of
// erroring out of the whole entry the way decoding straight into a single surrogate struct would.
// An entry that decodes cleanly but carries none of the known variants (including one whose only
// known-variant key was null) is likewise tolerated as long as it carries some other, unrecognized
// variant key (see unrecognizedActivityVariant): unknownVariant records that key and decoding
// succeeds, so a new activity kind BitBucket adds later cannot blind the whole feed. An entry with
// no variant key at all (recognized or not, or present only as null) is a genuinely malformed
// activity and still errors: read paths tolerate unrecognized variant VALUES, malformed shapes and
// null among them, not missing required content.
//
// implements json.Unmarshaler
func (activity *Activity) UnmarshalJSON(data []byte) (err error) {
	var raw map[string]json.RawMessage
	if err = json.Unmarshal(data, &raw); err != nil {
		return fmt.Errorf("cannot unmarshal activity: %w", err)
	}

	if pullRequestRaw, ok := raw["pull_request"]; ok {
		if err = json.Unmarshal(pullRequestRaw, &activity.PullRequest); err != nil {
			return fmt.Errorf("cannot unmarshal activity: cannot unmarshal pull_request: %w", err)
		}
	}

	switch {
	case decodeActivityVariant(activity, raw, "approval", &activity.Approval):
	case decodeActivityVariant(activity, raw, "changes_requested", &activity.ChangesRequested):
	case decodeActivityVariant(activity, raw, "comment", &activity.Comment):
	case decodeActivityVariant(activity, raw, "update", &activity.Update):
	default:
		variant, isUnrecognizedVariant := unrecognizedActivityVariant(data)
		if !isUnrecognizedVariant {
			return errors.New("cannot unmarshal activity: argument approval, changes_requested, comment, or update is missing")
		}
		activity.unknownVariant = variant
	}
	return nil
}

// decodeActivityVariant handles raw's key entry, if present and non-null: on success it unmarshals
// it into a fresh *T, points *target at it, and returns true. On failure (key present but the
// wrong shape or an unparsable field) it records key as activity's unknownVariant instead of
// returning an error, and still returns true -- a malformed KNOWN variant is tolerated exactly
// like a genuinely unrecognized one, per UnmarshalJSON's own doc comment. An absent key, or one
// whose raw value is the JSON literal null, returns false without touching unknownVariant, letting
// the caller's switch fall through to the next known variant (or, if none matched at all, to
// unrecognizedActivityVariant): unmarshaling JSON null into T (a value type, not a pointer) is a
// silent no-op that leaves decoded at its zero value and reports no error, so treating a present
// null the same as an absent key avoids *target ending up pointed at a fabricated zero-valued T
// the source payload never actually sent.
func decodeActivityVariant[T any](activity *Activity, raw map[string]json.RawMessage, key string, target **T) bool {
	value, ok := raw[key]
	if !ok || isJSONNullValue(value) {
		return false
	}
	var decoded T
	if err := json.Unmarshal(value, &decoded); err != nil {
		activity.unknownVariant = key
		return true
	}
	*target = &decoded
	return true
}

// unrecognizedActivityVariant looks for a top-level JSON key on an activity entry that
// activityKnownVariants does not recognize, regardless of the shape of its value -- a future
// activity kind BitBucket adds could just as easily serialize as an array or a scalar as an
// object, and tolerating only the object-shaped case would still blind the whole feed on those,
// exactly the failure mode this exists to prevent. Returns the key (and true) when found; an
// entry carrying no key besides "pull_request" reports not found, which is what makes decoding
// still error for a genuinely malformed activity. When more than one key qualifies, an
// object-valued key is preferred over a scalar/array-valued one: every activity kind BitBucket has
// ever added serializes as an object, so a real new kind riding alongside incidental scalar/array
// metadata on the same entry (e.g. {"pull_request":…,"id":7,"some_new_kind":{...}}) is far more
// likely to be the object-valued candidate than the scalar one -- reporting "id" there instead of
// "some_new_kind" would name the wrong key in the resulting [WARN]. Ties within the same shape
// class are broken lexicographically for deterministic behavior. data is assumed to already be
// valid JSON: the caller's own json.Unmarshal into map[string]json.RawMessage already succeeded on
// these same bytes before this is ever called, so the re-unmarshal below cannot fail.
func unrecognizedActivityVariant(data []byte) (variant string, found bool) {
	var raw map[string]json.RawMessage
	_ = json.Unmarshal(data, &raw)

	var unrecognized []string
	var unrecognizedObjects []string
	for key, value := range raw {
		if _, known := activityKnownVariants[key]; known {
			continue
		}
		unrecognized = append(unrecognized, key)
		if isJSONObjectValue(value) {
			unrecognizedObjects = append(unrecognizedObjects, key)
		}
	}
	if len(unrecognizedObjects) > 0 {
		sort.Strings(unrecognizedObjects)
		return unrecognizedObjects[0], true
	}
	if len(unrecognized) == 0 {
		return "", false
	}
	sort.Strings(unrecognized)
	return unrecognized[0], true
}

// isJSONObjectValue reports whether value's first non-whitespace byte is '{', i.e. value is a JSON
// object rather than an array, string, number, bool, or null. value is assumed already-valid JSON
// (see unrecognizedActivityVariant, its only caller).
func isJSONObjectValue(value json.RawMessage) bool {
	trimmed := bytes.TrimLeft(value, " \t\r\n")
	return len(trimmed) > 0 && trimmed[0] == '{'
}

// isJSONNullValue reports whether value is, ignoring surrounding whitespace, the JSON literal
// null. value is assumed already-valid JSON (see decodeActivityVariant, its only caller).
func isJSONNullValue(value json.RawMessage) bool {
	return string(bytes.TrimSpace(value)) == "null"
}
