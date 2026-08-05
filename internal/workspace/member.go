package workspace

import (
	"github.com/avitsrimer/bitbucket-cli/internal/common"
	"github.com/avitsrimer/bitbucket-cli/internal/user"
)

type Member struct {
	Type      string       `json:"type"      mapstructure:"type"`
	User      user.User    `json:"user"      mapstructure:"user"`
	Workspace Workspace    `json:"workspace" mapstructure:"workspace"`
	Links     common.Links `json:"links"     mapstructure:"links"`
}
