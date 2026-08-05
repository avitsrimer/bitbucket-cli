package workspace

import (
	"github.com/gildas/bitbucket-cli/cmd/common"
	"github.com/gildas/bitbucket-cli/cmd/user"
)

type Member struct {
	Type      string       `json:"type"      mapstructure:"type"`
	User      user.User    `json:"user"      mapstructure:"user"`
	Workspace Workspace    `json:"workspace" mapstructure:"workspace"`
	Links     common.Links `json:"links"     mapstructure:"links"`
}
