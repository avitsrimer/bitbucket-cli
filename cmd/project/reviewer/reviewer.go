package reviewer

import (
	"github.com/gildas/bitbucket-cli/cmd/user"
)

type Reviewer struct {
	Type         string    `json:"type" mapstructure:"type"`
	ReviewerType string    `json:"reviewer_type" mapstructure:"reviewer_type"`
	User         user.User `json:"user" mapstructure:"user"`
}
