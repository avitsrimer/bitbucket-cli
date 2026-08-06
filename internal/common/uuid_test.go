package common_test

import (
	"encoding/json"

	"github.com/avitsrimer/bitbucket-cli/internal/common"
)

func (suite *CommonSuite) TestCanMarshalUUID() {
	expected := "{c32f719b-6c8a-4c87-93e2-9ba8f5cd90dd}" // a Bitbucket String for UUIDs
	uuid, err := common.ParseUUID(expected)
	suite.Require().NoError(err)
	suite.Require().NotNil(uuid)
	suite.False(uuid.IsNil())
	payload, err := json.Marshal(uuid)
	suite.Require().NoError(err)
	suite.Require().NotNil(payload)
	suite.Equal(`"`+expected+`"`, string(payload))
}

func (suite *CommonSuite) TestCanUnmarshalUUID() {
	expected := "{c32f719b-6c8a-4c87-93e2-9ba8f5cd90dd}" // a Bitbucket String for UUIDs
	var uuid common.UUID
	err := json.Unmarshal([]byte(`"`+expected+`"`), &uuid)
	suite.Require().NoError(err)
	suite.Require().NotNil(uuid)
	suite.False(uuid.IsNil())
	suite.Equal(expected, uuid.String())

	err = json.Unmarshal([]byte(`"`+expected[1:len(expected)-1]+`"`), &uuid)
	suite.Require().NoError(err)
	suite.Require().NotNil(uuid)
	suite.False(uuid.IsNil())
	suite.Equal(expected, uuid.String())

	err = json.Unmarshal([]byte(`""`), &uuid)
	suite.Require().NoError(err)
	suite.Require().NotNil(uuid)
	suite.True(uuid.IsNil())
}

// TestCanUnmarshalNullUUID reproduces major finding #3: UnmarshalJSON sliced payload[1:len-1]
// unconditionally, so a JSON null (the 4-byte literal "null", not a quoted empty string) became
// "ul" once sliced, which fails to parse as a UUID -- so any API response with a null uuid field
// (common.UUID is the uuid field on Repository, User, Workspace, Project) failed to decode
// entirely instead of yielding a nil UUID the same way an empty string does.
func (suite *CommonSuite) TestCanUnmarshalNullUUID() {
	var uuid common.UUID
	err := json.Unmarshal([]byte(`null`), &uuid)
	suite.Require().NoError(err)
	suite.True(uuid.IsNil())
}
