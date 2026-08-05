package project

import (
	"github.com/avitsrimer/bitbucket-cli/internal/user"
)

// Reviewer represents a reviewer of a pullrequest or a default reviewer of a repository/project
type Reviewer struct {
	Type         string    `json:"type"`
	ReviewerType string    `json:"reviewer_type"`
	User         user.User `json:"user"`
}
