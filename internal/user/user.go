package user

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/avitsrimer/bitbucket-cli/internal/common"
	"github.com/avitsrimer/bitbucket-cli/internal/profile"
	"github.com/go-pkgz/lgr"
	"github.com/google/uuid"
	"github.com/spf13/cobra"
)

type User struct {
	Type          string       `json:"type"                     mapstructure:"type"`
	ID            common.UUID  `json:"uuid"                     mapstructure:"uuid"`
	AccountID     string       `json:"account_id"               mapstructure:"account_id"`
	Username      string       `json:"username,omitempty"       mapstructure:"username"`
	Name          string       `json:"display_name"             mapstructure:"display_name"`
	Nickname      string       `json:"nickname,omitempty"       mapstructure:"nickname"`
	Raw           string       `json:"raw,omitempty"            mapstructure:"raw"`
	Kind          string       `json:"kind,omitempty"           mapstructure:"kind"`
	Links         common.Links `json:"links"                    mapstructure:"links"`
	CreatedOn     time.Time    `json:"created_on"               mapstructure:"created_on"`
	AccountStatus string       `json:"account_status,omitempty" mapstructure:"account_status"`
}

var UserCache = common.NewCache[User]()

// Command represents this folder's command
var Command = &cobra.Command{
	Use:     "user",
	Aliases: []string{"account"},
	Short:   "Manage users",
	Run: func(cmd *cobra.Command, args []string) {
		fmt.Println("Issue requires a subcommand:")
		for _, command := range cmd.Commands() {
			fmt.Println(command.Name())
		}
	},
}

var columns = common.Columns[User]{
	{Name: "id", DefaultSorter: true, Compare: func(a, b User) bool {
		return strings.ToLower(a.ID.String()) < strings.ToLower(b.ID.String())
	}},
	{Name: "username", DefaultSorter: false, Compare: func(a, b User) bool {
		return strings.ToLower(a.Username) < strings.ToLower(b.Username)
	}},
	{Name: "name", DefaultSorter: false, Compare: func(a, b User) bool {
		return strings.ToLower(a.Name) < strings.ToLower(b.Name)
	}},
	{Name: "nickname", DefaultSorter: false, Compare: func(a, b User) bool {
		return strings.ToLower(a.Nickname) < strings.ToLower(b.Nickname)
	}},
	{Name: "account", DefaultSorter: false, Compare: func(a, b User) bool {
		return strings.ToLower(a.AccountID) < strings.ToLower(b.AccountID)
	}},
	{Name: "created_on", DefaultSorter: false, Compare: func(a, b User) bool {
		return a.CreatedOn.Before(b.CreatedOn)
	}},
	{Name: "account_status", DefaultSorter: false, Compare: func(a, b User) bool {
		return strings.ToLower(a.AccountStatus) < strings.ToLower(b.AccountStatus)
	}},
}

// GetID gets the ID of the user
//
// implements core.Identifiable
func (user User) GetID() uuid.UUID {
	return uuid.UUID(user.ID)
}

// GetName gets the name of the user
//
// implements core.Named
func (user User) GetName() string {
	return user.Username
}

// GetHeaders gets the header for a table
//
// implements common.Tableable
func (user User) GetHeaders(cmd *cobra.Command) []string {
	if cmd != nil && cmd.Flag("columns") != nil && cmd.Flag("columns").Changed {
		if columns, err := cmd.Flags().GetStringSlice("columns"); err == nil {
			return columns
		}
	}
	return []string{"ID", "Username", "Name"}
}

// GetRow gets the row for a table
//
// implements common.Tableable
func (user User) GetRow(headers []string) []string {
	var row []string

	for _, header := range headers {
		switch strings.ToLower(header) {
		case "id":
			row = append(row, user.ID.String())
		case "username":
			if user.Username != "" {
				row = append(row, user.Username)
			} else {
				row = append(row, user.Nickname)
			}
		case "name":
			row = append(row, user.Name)
		case "nickname":
			row = append(row, user.Nickname)
		case "account":
			row = append(row, user.AccountID)
		case "created on", "created_on":
			if user.CreatedOn.IsZero() {
				row = append(row, " ")
			} else {
				row = append(row, user.CreatedOn.Format("2006-01-02 15:04:05"))
			}
		case "account status":
			if user.AccountStatus == "" {
				row = append(row, " ")
			} else {
				row = append(row, user.AccountStatus)
			}
		}
	}
	return row
}

// IsEmpty checks if this User is empty
func (user User) IsEmpty() bool {
	return user.Type == "" &&
		user.ID.IsNil() &&
		user.AccountID == "" &&
		user.Username == "" &&
		user.Name == "" &&
		user.Nickname == "" &&
		user.Raw == "" &&
		user.Kind == "" &&
		user.Links.IsEmpty() &&
		user.CreatedOn.IsZero() &&
		user.AccountStatus == ""
}

// String gets the string representation of the user
//
// implements fmt.Stringer
func (user User) String() string {
	if user.Name == "" {
		return user.ID.String()
	}
	return user.Name
}

// MarshalJSON implements the json.Marshaler interface.
func (user User) MarshalJSON() (data []byte, err error) {
	type surrogate User
	var createdOn string

	if !user.CreatedOn.IsZero() {
		createdOn = user.CreatedOn.Format("2006-01-02T15:04:05.999999999-07:00")
	}
	data, err = json.Marshal(struct {
		surrogate
		CreatedOn string `json:"created_on,omitempty"`
	}{
		surrogate: surrogate(user),
		CreatedOn: createdOn,
	})
	if err != nil {
		return nil, fmt.Errorf("cannot marshal json: %w", err)
	}
	return data, nil
}

// GetMe gets the current user
func GetMe(context context.Context, cmd *cobra.Command) (user *User, err error) {
	profile, err := profile.GetProfileFromCommand(cmd.Context(), cmd)
	if err != nil {
		return nil, fmt.Errorf("cannot get profile: %w", err)
	}
	if user, err = UserCache.Get(profile.Name + ":me"); err == nil {
		lgr.Printf("[DEBUG] user found in cache")
		return user, nil
	}
	err = profile.Get(
		context,
		cmd,
		"/user",
		&user,
	)
	if err == nil {
		if user == nil {
			return nil, errors.New("received an empty response for the current user")
		}
		_ = UserCache.Set(profile.Name+":me", *user)
	}
	return
}

// GetUser gets a user
func GetUser(context context.Context, cmd *cobra.Command, userid string) (user *User, err error) {
	profile, err := profile.GetProfileFromCommand(cmd.Context(), cmd)
	if err != nil {
		return nil, fmt.Errorf("cannot get profile: %w", err)
	}
	if userid == "" || strings.EqualFold(userid, "me") || strings.EqualFold(userid, "myself") {
		me, meErr := GetMe(context, cmd)
		if meErr != nil {
			return nil, meErr
		}
		return me, nil
	}
	userUUID, err := common.ParseUUID(userid)
	if err != nil {
		return nil, fmt.Errorf("cannot parse user id %s: %w", userid, err)
	}
	if user, err = UserCache.Get(profile.Name + ":" + userUUID.String()); err == nil {
		return user, nil
	}
	err = profile.Get(
		context,
		cmd,
		"/users/"+userUUID.String(),
		&user,
	)
	if err != nil {
		return nil, fmt.Errorf("cannot get user %s: %w", userUUID.String(), err)
	}
	if user == nil {
		return nil, fmt.Errorf("received an empty response for user %s", userUUID.String())
	}
	_ = UserCache.Set(profile.Name+":"+userUUID.String(), *user)
	return user, nil
}
