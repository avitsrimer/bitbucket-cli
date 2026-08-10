package profile

import (
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"strings"
)

// BitBucketError represents an error returned by the BitBucket API
//
// See: https://developer.atlassian.com/cloud/bitbucket/rest/intro/#standardized-error-responses
type BitBucketError struct {
	Type    string              `json:"type"`
	Message string              `json:"-"`
	Detail  string              `json:"-"`
	Fields  map[string][]string `json:"-"`
}

func (bberr *BitBucketError) Error() string {
	var buffer strings.Builder

	buffer.WriteString(bberr.Message)
	if bberr.Detail != "" {
		buffer.WriteString(": ")
		buffer.WriteString(bberr.Detail)
	}
	if len(bberr.Fields) > 0 {
		buffer.WriteString(" (")
		for field, messages := range bberr.Fields {
			buffer.WriteString(field)
			buffer.WriteString(": ")
			buffer.WriteString(strings.Join(messages, ", "))
			buffer.WriteString("; ")
		}
		buffer.WriteString(")")
	}
	return buffer.String()
}

// statusError is the single carrier of the HTTP status a non-2xx response was mapped from: every
// error mapErrorResponse builds is wrapped in one, whether the body carried a BitBucket error
// payload (*BitBucketError) or nothing usable (the bare "cannot send request: <status text>"
// message). It adds nothing to the message it wraps -- Error() renders the wrapped error verbatim,
// so every caller and test sees exactly the text it always did -- and unwraps to it, so an
// errors.As for *BitBucketError still finds the payload underneath. Callers reacting to a specific
// status go through IsNotFound (or an errors.As of their own) rather than parsing the message.
type statusError struct {
	StatusCode int
	err        error
}

func (serr *statusError) Error() string {
	return serr.err.Error()
}

func (serr *statusError) Unwrap() error {
	return serr.err
}

// IsNotFound reports whether err was mapped from an HTTP 404 response.
func IsNotFound(err error) bool {
	var serr *statusError
	return errors.As(err, &serr) && serr.StatusCode == http.StatusNotFound
}

// UnmarshalJSON unmarshals the JSON
func (bberr *BitBucketError) UnmarshalJSON(data []byte) (err error) {
	type surrogate BitBucketError

	var innerType1 struct {
		surrogate
		Error struct {
			Message string              `json:"message"`
			Fields  map[string][]string `json:"detail"`
		} `json:"error"`
	}
	if err = json.Unmarshal(data, &innerType1); err == nil && len(innerType1.Error.Fields) > 0 {
		*bberr = BitBucketError(innerType1.surrogate)
		bberr.Message = innerType1.Error.Message
		bberr.Fields = innerType1.Error.Fields
		return nil
	}

	var innerType2 struct {
		surrogate
		Error struct {
			Message string            `json:"message"`
			Detail  string            `json:"detail"`
			Fields  map[string]string `json:"fields"`
		} `json:"error"`
	}

	if err = json.Unmarshal(data, &innerType2); err == nil && len(innerType2.Error.Fields) > 0 {
		*bberr = BitBucketError(innerType2.surrogate)
		bberr.Message = innerType2.Error.Message
		bberr.Detail = innerType2.Error.Detail
		if len(innerType2.Error.Fields) > 0 {
			bberr.Fields = make(map[string][]string)
			for field, message := range innerType2.Error.Fields {
				bberr.Fields[field] = []string{message}
			}
		}
		return nil
	}

	var innerType3 struct {
		surrogate
		Error struct {
			Message string              `json:"message"`
			Detail  string              `json:"detail"`
			Fields  map[string][]string `json:"fields"`
		} `json:"error"`
	}

	if err = json.Unmarshal(data, &innerType3); err != nil {
		return fmt.Errorf("cannot unmarshal bitbucket error: %w", err)
	}
	*bberr = BitBucketError(innerType3.surrogate)
	bberr.Message = innerType3.Error.Message
	bberr.Detail = innerType3.Error.Detail
	bberr.Fields = innerType3.Error.Fields
	return nil
}
