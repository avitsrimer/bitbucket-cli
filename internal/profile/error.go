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
	// StatusCode is the HTTP status of the response this error was mapped from. It is set by the
	// client (mapErrorResponse), not by the payload: Bitbucket's error body carries no status.
	StatusCode int `json:"-"`
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

// statusError is the error a non-2xx response whose body carries no usable BitBucket error payload
// maps to. It renders exactly the status text (the message every caller and test already sees) and
// additionally keeps the status code, so a caller can react to a specific status (see IsNotFound)
// without parsing that message.
type statusError struct {
	StatusCode int
	StatusText string
}

func (serr *statusError) Error() string {
	return "cannot send request: " + serr.StatusText
}

// IsNotFound reports whether err was mapped from an HTTP 404 response, for either of the two error
// shapes a non-2xx response produces: a *BitBucketError built from the API's own error payload, or
// the bare status error used when the body carries none.
func IsNotFound(err error) bool {
	var bberr *BitBucketError
	if errors.As(err, &bberr) {
		return bberr.StatusCode == http.StatusNotFound
	}
	var serr *statusError
	if errors.As(err, &serr) {
		return serr.StatusCode == http.StatusNotFound
	}
	return false
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
