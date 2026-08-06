package user

import (
	"encoding/json"
	"fmt"
	"strconv"

	"github.com/avitsrimer/bitbucket-cli/internal/common"
	"github.com/spf13/cobra"
)

type Email struct {
	Email       string       `json:"email"`
	IsPrimary   bool         `json:"is_primary"`
	IsConfirmed bool         `json:"is_confirmed"`
	Links       common.Links `json:"links"`
}

// GetType gets the type of the email
//
// implements common.Typeable
func (email Email) GetType() string {
	return "email"
}

// GetHeaders gets the header for a table
//
// implements common.Tableable
func (email Email) GetHeaders(cmd *cobra.Command) []string {
	return common.HeadersFromFlag(cmd, "Email", "Is Primary", "Is Confirmed")
}

// GetRow gets the row for a table
//
// implements common.Tableable
//
// headers is normalized via common.NormalizeColumnKey before matching, so a header of any case
// and using either spaces, hyphens, or underscores as separators (e.g. "Is Primary", "is-primary",
// "is_primary") resolves to the same cell.
func (email Email) GetRow(headers []string) []string {
	simple := map[string]string{
		"email":        email.Email,
		"is_primary":   strconv.FormatBool(email.IsPrimary),
		"is_confirmed": strconv.FormatBool(email.IsConfirmed),
	}

	row := make([]string, 0, len(headers))
	for _, header := range headers {
		key := common.NormalizeColumnKey(header)
		if value, found := simple[key]; found {
			row = append(row, value)
		} else {
			row = append(row, " ")
		}
	}
	return row
}

// MarshalJSON implements the json.Marshaler interface.
func (email Email) MarshalJSON() ([]byte, error) {
	type surrogate Email

	data, err := json.Marshal(struct {
		Type string `json:"type"`
		surrogate
	}{
		Type:      email.GetType(),
		surrogate: surrogate(email),
	})
	if err != nil {
		return nil, fmt.Errorf("cannot marshal json: %w", err)
	}
	return data, nil
}

// UnmarshalJSON implements the json.Unmarshaler interface.
func (email *Email) UnmarshalJSON(data []byte) error {
	type surrogate Email

	var inner struct {
		Type string `json:"type"`
		surrogate
	}

	err := json.Unmarshal(data, &inner)
	if err != nil {
		return fmt.Errorf("cannot unmarshal json: %w", err)
	}
	if inner.Type != email.GetType() {
		return fmt.Errorf("invalid type %s, expected %s", inner.Type, email.GetType())
	}

	*email = Email(inner.surrogate)
	return nil
}
