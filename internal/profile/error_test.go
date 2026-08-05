package profile_test

import (
	"errors"
	"fmt"

	"github.com/avitsrimer/bitbucket-cli/internal/profile"
)

func (suite *ProfileSuite) TestCanUnmarshalErrorAboutPrivileges() {
	var bberr profile.BitBucketError

	err := suite.UnmarshalData("error-privileges.json", &bberr)
	suite.Require().NoError(err)
	suite.Equal("error", bberr.Type)
	suite.Equal("Your credentials lack one or more required privilege scopes.", bberr.Message)
	suite.Require().Len(bberr.Fields, 2)
	suite.Require().Contains(bberr.Fields, "required")
	suite.Contains(bberr.Fields["required"], "project")
	suite.Require().Contains(bberr.Fields, "granted")
	suite.Contains(bberr.Fields["granted"], "account")
	suite.T().Logf("Expected Error string: %s", bberr.Error())
}

func (suite *ProfileSuite) TestCanUnmarshalErrorAboutNoAPI() {
	var bberr profile.BitBucketError

	err := suite.UnmarshalData("error-noapi.json", &bberr)
	suite.Require().NoError(err)
	suite.Equal("error", bberr.Type)
	suite.Equal("Resource not found", bberr.Message)
	suite.Equal("There is no API hosted at this URL", bberr.Detail)
	suite.T().Logf("Expected Error string: %s", bberr.Error())
}

func (suite *ProfileSuite) TestCanUnmarshalErrorAboutBadRequest() {
	var bberr profile.BitBucketError

	err := suite.UnmarshalData("error-badrequest.json", &bberr)
	suite.Require().NoError(err)
	suite.Equal("error", bberr.Type)
	suite.Equal("Bad request", bberr.Message)
	suite.Require().Len(bberr.Fields, 1)
	suite.Require().Contains(bberr.Fields, "links.avatar")
	suite.Contains(bberr.Fields["links.avatar"], "required key not provided")
	suite.T().Logf("Expected Error string: %s", bberr.Error())
}

func (suite *ProfileSuite) TestErrNoProfilesMatchesWrappedError() {
	wrapped := fmt.Errorf("context: %w", profile.ErrNoProfiles)
	suite.ErrorIs(wrapped, profile.ErrNoProfiles, "errors.Is should match a wrapped ErrNoProfiles")
}

func (suite *ProfileSuite) TestErrNoProfilesDoesNotMatchUnrelatedError() {
	other := errors.New("some other error")
	suite.NotErrorIs(other, profile.ErrNoProfiles, "errors.Is should not match an unrelated error")
}

func (suite *ProfileSuite) TestErrUnmarshalJSONMatchesMalformedTokenData() {
	_, err := profile.UnmarshalTokenFromBitbucketData([]byte("not json"))
	suite.Require().Error(err)
	suite.ErrorIs(err, profile.ErrUnmarshalJSON, "errors.Is should match a wrapped ErrUnmarshalJSON")
}

func (suite *ProfileSuite) TestErrUnmarshalJSONDoesNotMatchValidTokenData() {
	token, err := profile.UnmarshalTokenFromBitbucketData([]byte(`{"access_token": "abc", "token_type": "bearer"}`))
	suite.Require().NoError(err)
	suite.Require().NotNil(token)
	suite.Equal("abc", token.AccessToken)
}
