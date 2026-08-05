package pullrequest

import (
	"github.com/avitsrimer/bitbucket-cli/internal/commit"
	"github.com/avitsrimer/bitbucket-cli/internal/repository"
)

type Endpoint struct {
	Branch     Branch                  `json:"branch"               mapstructure:"branch"`
	Commit     *commit.CommitReference `json:"commit,omitempty"     mapstructure:"commit"`
	Repository *repository.Repository  `json:"repository,omitempty" mapstructure:"repository"`
}
