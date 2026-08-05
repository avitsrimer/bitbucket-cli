package pullrequest_test

import (
	"encoding/json"
	"os"
	"reflect"
	"strings"
	"testing"

	"github.com/gildas/bitbucket-cli/cmd/pullrequest"
	"github.com/joho/godotenv"
	"github.com/stretchr/testify/suite"
)

type PullRequestSuite struct {
	suite.Suite
	Name string
}

func TestPullRequestSuite(t *testing.T) {
	suite.Run(t, new(PullRequestSuite))
}

// *****************************************************************************
// Suite Tools

func (suite *PullRequestSuite) SetupSuite() {
	_ = godotenv.Load()
	suite.Name = strings.TrimSuffix(reflect.TypeOf(suite).Elem().Name(), "Suite")
}

func (suite *PullRequestSuite) TearDownSuite() {
	if suite.T().Failed() {
		suite.T().Log("At least one test failed, we are not cleaning")
	}
}

func (suite *PullRequestSuite) LoadTestData(filename string) []byte {
	data, err := os.ReadFile("../../testdata/" + filename)
	if err != nil {
		suite.T().Fatal(err)
	}
	return data
}

func (suite *PullRequestSuite) UnmarshalData(filename string, v any) error {
	data := suite.LoadTestData(filename)
	return json.Unmarshal(data, v)
}

// *****************************************************************************

func (suite *PullRequestSuite) TestCanUnmarshal() {
	payload := suite.LoadTestData("pullrequest.json")
	var pr pullrequest.PullRequest
	err := json.Unmarshal(payload, &pr)
	suite.Require().NoError(err)
	suite.Require().NotNil(pr)
	data, err := json.Marshal(pr)
	suite.Require().NoError(err)
	suite.JSONEq(string(payload), string(data))
}

func (suite *PullRequestSuite) TestCanUnmarshalWithNilDestinationRepository() {
	payload := suite.LoadTestData("pullrequest-no-dest-repo.json")
	var pr pullrequest.PullRequest
	err := json.Unmarshal(payload, &pr)
	suite.Require().NoError(err)
	suite.Require().NotNil(pr)
	suite.Nil(pr.Destination.Repository)
	suite.NotEmpty(pr.Destination.Branch.Name)
}

func (suite *PullRequestSuite) TestDestinationRepositoryIsNilAfterSettingNewDestination() {
	payload := suite.LoadTestData("pullrequest.json")
	var pr pullrequest.PullRequest
	err := json.Unmarshal(payload, &pr)
	suite.Require().NoError(err)
	suite.Require().NotNil(pr.Destination.Repository)

	pr.Destination = pullrequest.Endpoint{Branch: pullrequest.Branch{Name: "new-branch"}}

	suite.Nil(pr.Destination.Repository)
	suite.Equal("new-branch", pr.Destination.Branch.Name)
}

func (suite *PullRequestSuite) TestCanCreatePullRequestMergeStatus() {
	location := "https://api.bitbucket.org/2.0/repositories/workspace_slug/repo_slug/pullrequests/123/merge/task-status/b45ea563-edb0-4d1d-ba34-ffaac2a6e10b"
	mergeStatus, err := pullrequest.NewPullRequestMergeStatusFromLocation(location)
	suite.Require().NoError(err)
	suite.Require().NotNil(mergeStatus)
	suite.Equal("b45ea563-edb0-4d1d-ba34-ffaac2a6e10b", mergeStatus.ID)
	suite.Equal(uint64(123), mergeStatus.PullRequest.ID)
}

func (suite *PullRequestSuite) TestShouldFailCreatingPullRequestMergeStatusWithInvalidURL() {
	location := "invalid-url"
	mergeStatus, err := pullrequest.NewPullRequestMergeStatusFromLocation(location)
	suite.Require().Error(err)
	suite.Nil(mergeStatus)
	suite.T().Logf("Expected error: %s", err.Error())
}

func (suite *PullRequestSuite) TestShouldFailCreatingPullRequestMergeStatusWithEmptyURL() {
	location := ""
	mergeStatus, err := pullrequest.NewPullRequestMergeStatusFromLocation(location)
	suite.Require().Error(err)
	suite.Nil(mergeStatus)
	suite.T().Logf("Expected error: %s", err.Error())
}

func (suite *PullRequestSuite) TestShouldFailCreatingPullRequestMergeStatusWithShortURL() {
	location := "https://api.bitbucket.org/2.0/repositories/workspace_slug/repo_slug/pullrequests/123/merge/task-status"
	mergeStatus, err := pullrequest.NewPullRequestMergeStatusFromLocation(location)
	suite.Require().Error(err)
	suite.Nil(mergeStatus)
	suite.T().Logf("Expected error: %s", err.Error())
}
