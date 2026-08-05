package workspace

import (
	"github.com/avitsrimer/bitbucket-cli/internal/common"
	"github.com/avitsrimer/bitbucket-cli/internal/user"
)

type Member struct {
	Type      string       `json:"type"`
	User      user.User    `json:"user"`
	Workspace Workspace    `json:"workspace"`
	Links     common.Links `json:"links"`
}
