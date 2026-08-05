package common_test

import (
	"testing"

	"github.com/avitsrimer/bitbucket-cli/internal/common"
	"github.com/stretchr/testify/assert"
)

func TestLinksIsEmptyWhenNoLinkIsSet(t *testing.T) {
	links := common.Links{}
	assert.True(t, links.IsEmpty())
}

func TestLinksIsNotEmptyWhenALinkIsSet(t *testing.T) {
	links := common.Links{Self: &common.Link{Name: "self"}}
	assert.False(t, links.IsEmpty())
}

func TestLinksIsNotEmptyWhenCloneIsSet(t *testing.T) {
	links := common.Links{Clone: []common.Link{{Name: "self"}}}
	assert.False(t, links.IsEmpty())
}
