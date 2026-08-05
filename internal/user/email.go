package user

import (
	"encoding/json"
	"fmt"
	"strconv"
	"strings"

	"github.com/avitsrimer/bitbucket-cli/internal/common"
	"github.com/gildas/go-core"
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
	if cmd != nil && cmd.Flag("columns") != nil && cmd.Flag("columns").Changed {
		if columns, err := cmd.Flags().GetStringSlice("columns"); err == nil {
			return core.Map(columns, func(column string) string { return strings.ReplaceAll(column, "_", " ") })
		}
	}
	return []string{"Email", "Is Primary", "Is Confirmed"}
}

// GetRow gets the row for a table
//
// implements common.Tableable
func (email Email) GetRow(headers []string) []string {
	row := make([]string, 0, len(headers))
	for _, header := range headers {
		switch header {
		case "Email":
			row = append(row, email.Email)
		case "Is Primary":
			row = append(row, strconv.FormatBool(email.IsPrimary))
		case "Is Confirmed":
			row = append(row, strconv.FormatBool(email.IsConfirmed))
		default:
			row = append(row, "")
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
