package common_test

import (
	"encoding/json"
	"net/url"
	"testing"
	"time"

	"github.com/avitsrimer/bitbucket-cli/internal/common"
)

// golden byte fixtures captured from gildas/go-core v0.6.4's JSON marshaling (via a throwaway
// worktree and temporary capture tests, never committed), pinned here so the locally-ported
// URL/Time/Timestamp types cannot silently change wire behavior.
const (
	goldenTimeRFC3339      = "2026-01-02T03:04:05Z"
	goldenTimestampEpochMS = int64(1767225600000) // 2026-01-01T00:00:00Z in ms
	goldenURLPopulated     = "https://api.bitbucket.org/2.0"
	goldenURLZero          = ""
)

func TestTimeMarshalJSON(t *testing.T) {
	parsed, err := time.Parse(time.RFC3339, goldenTimeRFC3339)
	if err != nil {
		t.Fatal(err)
	}

	data, err := json.Marshal(common.Time(parsed))
	if err != nil {
		t.Fatal(err)
	}
	if got, want := string(data), `"`+goldenTimeRFC3339+`"`; got != want {
		t.Errorf("MarshalJSON() = %s, want %s", got, want)
	}
}

func TestTimeMarshalJSONDropsSubSecondPrecision(t *testing.T) {
	withNanos := time.Date(2026, 1, 2, 3, 4, 5, 123456789, time.UTC)

	data, err := json.Marshal(common.Time(withNanos))
	if err != nil {
		t.Fatal(err)
	}
	if got, want := string(data), `"2026-01-02T03:04:05Z"`; got != want {
		t.Errorf("MarshalJSON() = %s, want %s (sub-second precision must be dropped)", got, want)
	}
}

func TestTimeMarshalJSONConvertsToUTC(t *testing.T) {
	loc := time.FixedZone("test", 2*60*60) // UTC+2
	local := time.Date(2026, 1, 2, 5, 4, 5, 0, loc)

	data, err := json.Marshal(common.Time(local))
	if err != nil {
		t.Fatal(err)
	}
	if got, want := string(data), `"2026-01-02T03:04:05Z"`; got != want {
		t.Errorf("MarshalJSON() = %s, want %s (must convert to UTC)", got, want)
	}
}

func TestTimeUnmarshalJSON(t *testing.T) {
	var got common.Time
	if err := json.Unmarshal([]byte(`"`+goldenTimeRFC3339+`"`), &got); err != nil {
		t.Fatal(err)
	}
	want, err := time.Parse(time.RFC3339, goldenTimeRFC3339)
	if err != nil {
		t.Fatal(err)
	}
	if !time.Time(got).Equal(want) {
		t.Errorf("UnmarshalJSON() = %v, want %v", time.Time(got), want)
	}
}

func TestTimeUnmarshalJSONEmptyStringYieldsZeroTime(t *testing.T) {
	var got common.Time
	if err := json.Unmarshal([]byte(`""`), &got); err != nil {
		t.Fatal(err)
	}
	if !time.Time(got).IsZero() {
		t.Errorf("UnmarshalJSON(\"\") = %v, want the zero time", time.Time(got))
	}
}

func TestTimeRoundTrip(t *testing.T) {
	original := time.Date(2026, 1, 2, 3, 4, 5, 0, time.UTC)

	data, err := json.Marshal(common.Time(original))
	if err != nil {
		t.Fatal(err)
	}
	var roundTripped common.Time
	if err := json.Unmarshal(data, &roundTripped); err != nil {
		t.Fatal(err)
	}
	if !time.Time(roundTripped).Equal(original) {
		t.Errorf("round-trip = %v, want %v", time.Time(roundTripped), original)
	}
}

func TestTimestampMarshalJSONEmitsBareNumber(t *testing.T) {
	when := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)

	data, err := json.Marshal(common.Timestamp(when))
	if err != nil {
		t.Fatal(err)
	}
	if got, want := string(data), "1767225600000"; got != want {
		t.Errorf("MarshalJSON() = %s, want bare number %s (not quoted, not UnixMilli-cleaned-up)", got, want)
	}
}

func TestTimestampUnmarshalJSONAcceptsBareNumber(t *testing.T) {
	var got common.Timestamp
	if err := json.Unmarshal([]byte(`1767225600000`), &got); err != nil {
		t.Fatal(err)
	}
	if got.JSEpoch() != goldenTimestampEpochMS {
		t.Errorf("JSEpoch() = %d, want %d", got.JSEpoch(), goldenTimestampEpochMS)
	}
}

func TestTimestampUnmarshalJSONAcceptsQuotedString(t *testing.T) {
	var got common.Timestamp
	if err := json.Unmarshal([]byte(`"1767225600000"`), &got); err != nil {
		t.Fatal(err)
	}
	if got.JSEpoch() != goldenTimestampEpochMS {
		t.Errorf("JSEpoch() = %d, want %d", got.JSEpoch(), goldenTimestampEpochMS)
	}
}

func TestTimestampRoundTrip(t *testing.T) {
	original := common.TimestampFromJSEpoch(goldenTimestampEpochMS)

	data, err := json.Marshal(original)
	if err != nil {
		t.Fatal(err)
	}
	var roundTripped common.Timestamp
	if err := json.Unmarshal(data, &roundTripped); err != nil {
		t.Fatal(err)
	}
	if roundTripped.JSEpoch() != original.JSEpoch() {
		t.Errorf("round-trip JSEpoch() = %d, want %d", roundTripped.JSEpoch(), original.JSEpoch())
	}
}

func TestURLMarshalJSONPopulated(t *testing.T) {
	parsed, err := url.Parse(goldenURLPopulated)
	if err != nil {
		t.Fatal(err)
	}

	data, err := json.Marshal(common.URL(*parsed))
	if err != nil {
		t.Fatal(err)
	}
	if got, want := string(data), `"`+goldenURLPopulated+`"`; got != want {
		t.Errorf("MarshalJSON() = %s, want %s", got, want)
	}
}

func TestURLMarshalJSONZeroValue(t *testing.T) {
	var zero common.URL

	data, err := json.Marshal(zero)
	if err != nil {
		t.Fatal(err)
	}
	if got, want := string(data), `"`+goldenURLZero+`"`; got != want {
		t.Errorf("MarshalJSON() on zero URL = %s, want %s (must marshal as \"\", not be omitted)", got, want)
	}
}

func TestURLUnmarshalJSONEmptyStringLeavesReceiverUntouched(t *testing.T) {
	got := common.URL(*mustParseURL(t, goldenURLPopulated))
	if err := json.Unmarshal([]byte(`""`), &got); err != nil {
		t.Fatal(err)
	}
	// UnmarshalJSON returns nil on an empty string without writing to the receiver at all: a
	// populated receiver stays populated, it is not reset to the zero value.
	asURL := got.AsURL()
	if got := asURL.String(); got != goldenURLPopulated {
		t.Errorf("UnmarshalJSON(\"\") mutated the receiver: got %s, want unchanged %s", got, goldenURLPopulated)
	}
}

func TestURLRoundTrip(t *testing.T) {
	parsed, err := url.Parse(goldenURLPopulated)
	if err != nil {
		t.Fatal(err)
	}
	original := common.URL(*parsed)

	data, err := json.Marshal(original)
	if err != nil {
		t.Fatal(err)
	}
	var roundTripped common.URL
	if err := json.Unmarshal(data, &roundTripped); err != nil {
		t.Fatal(err)
	}
	gotURL, wantURL := roundTripped.AsURL(), original.AsURL()
	if got, want := gotURL.String(), wantURL.String(); got != want {
		t.Errorf("round-trip = %s, want %s", got, want)
	}
}

func mustParseURL(t *testing.T, raw string) *url.URL {
	t.Helper()
	parsed, err := url.Parse(raw)
	if err != nil {
		t.Fatal(err)
	}
	return parsed
}
