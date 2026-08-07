package common

import (
	"encoding/json"
	"fmt"
	"net/url"
	"strconv"
	"strings"
	"time"
)

// URL is a url.URL that marshals to/from JSON as its plain string form, including for its
// zero value (marshaled as "", never omitted).
type URL url.URL

// AsURL converts a URL into a url.URL.
func (u URL) AsURL() url.URL {
	return url.URL(u)
}

// MarshalJSON implements json.Marshaler.
func (u URL) MarshalJSON() ([]byte, error) {
	uu := url.URL(u)
	data, err := json.Marshal(uu.String())
	if err != nil {
		return nil, fmt.Errorf("cannot marshal url to json: %w", err)
	}
	return data, nil
}

// UnmarshalJSON implements json.Unmarshaler. An empty string returns nil without writing to u
// at all, leaving the receiver exactly as it was before the call -- not reset to its zero value.
func (u *URL) UnmarshalJSON(payload []byte) error {
	var inner string
	if err := json.Unmarshal(payload, &inner); err != nil {
		return fmt.Errorf("cannot unmarshal url: %w", err)
	}
	if inner == "" {
		return nil
	}
	parsed, err := url.Parse(inner)
	if err != nil {
		return fmt.Errorf("cannot unmarshal url: %w", err)
	}
	*u = *(*URL)(parsed)
	return nil
}

// Time is a time.Time that marshals to/from JSON as an RFC3339 UTC string, at second
// precision (sub-second components are dropped on marshal).
type Time time.Time

// MarshalJSON implements json.Marshaler.
//
// Time is stored as RFC3339 UTC.
func (t Time) MarshalJSON() ([]byte, error) {
	data, err := json.Marshal(time.Time(t).UTC().Format(time.RFC3339))
	if err != nil {
		return nil, fmt.Errorf("cannot marshal time to json: %w", err)
	}
	return data, nil
}

// UnmarshalJSON implements json.Unmarshaler.
//
// Time is read as RFC3339 UTC; an empty string unmarshals to the zero time.
func (t *Time) UnmarshalJSON(payload []byte) error {
	var inner string
	if err := json.Unmarshal(payload, &inner); err != nil {
		return fmt.Errorf("cannot unmarshal time: %w", err)
	}
	if inner == "" {
		*t = Time(time.Time{})
		return nil
	}
	parsed, err := time.Parse(time.RFC3339, inner)
	if err != nil {
		return fmt.Errorf("cannot unmarshal time: %w", err)
	}
	*t = Time(parsed)
	return nil
}

// Timestamp is a time.Time that marshals to/from JSON as a Unix epoch in milliseconds
// (Node.js-style), as a bare JSON number.
type Timestamp time.Time

// TimestampFromJSEpoch returns a Timestamp from a JS epoch (milliseconds since Unix epoch).
func TimestampFromJSEpoch(epoch int64) Timestamp {
	return Timestamp(time.Unix(epoch/1000, (epoch%1000)*1000000))
}

// JSEpoch returns the Unix epoch in milliseconds (Node.js-style).
func (t Timestamp) JSEpoch() int64 {
	return time.Time(t).UnixNano() / int64(time.Millisecond)
}

// MarshalJSON implements json.Marshaler.
//
// The epoch is emitted as a bare JSON number, in milliseconds.
func (t Timestamp) MarshalJSON() ([]byte, error) {
	return []byte(strconv.FormatInt(t.JSEpoch(), 10)), nil
}

// UnmarshalJSON implements json.Unmarshaler.
//
// The epoch, in milliseconds, may be a bare number (12345) or a quoted string ("12345").
func (t *Timestamp) UnmarshalJSON(payload []byte) error {
	unquoted := strings.ReplaceAll(string(payload), `"`, ``)
	value, err := strconv.ParseInt(unquoted, 10, 64)
	if err != nil {
		return fmt.Errorf("cannot unmarshal timestamp: %w", err)
	}
	*t = TimestampFromJSEpoch(value)
	return nil
}
