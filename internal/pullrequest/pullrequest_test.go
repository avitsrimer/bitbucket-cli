package pullrequest_test

import (
	"encoding/json"
	"os"
	"reflect"
	"strings"
	"testing"

	"github.com/avitsrimer/bitbucket-cli/internal/pullrequest"
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

// ***********************************************************************

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

// TestCanUnmarshalParticipants is FR-9's regression: the API returns a "participants" array
// carrying each reviewer's approval state, but PullRequest had no field for it, making
// approval-state-per-reviewer unreachable in every output format. This proves the field decodes
// and that the resolved approval state survives round-tripping back out to json.
func (suite *PullRequestSuite) TestCanUnmarshalParticipants() {
	payload := suite.LoadTestData("pullrequest-with-participants.json")
	var pr pullrequest.PullRequest
	err := json.Unmarshal(payload, &pr)
	suite.Require().NoError(err)
	suite.Require().Len(pr.Participants, 2)
	suite.Equal("jane_doe", pr.Participants[0].User.Nickname)
	suite.Equal("approved", pr.Participants[0].State)
	suite.True(pr.Participants[0].Approved)
	suite.Equal("john_smith", pr.Participants[1].User.Nickname)
	suite.Equal("changes_requested", pr.Participants[1].State)
	suite.False(pr.Participants[1].Approved)

	data, err := json.Marshal(pr)
	suite.Require().NoError(err)
	var raw map[string]any
	suite.Require().NoError(json.Unmarshal(data, &raw))
	participants, ok := raw["participants"].([]any)
	suite.Require().True(ok, "marshaled pullrequest is missing a \"participants\" array")
	suite.Require().Len(participants, 2)
	first, ok := participants[0].(map[string]any)
	suite.Require().True(ok)
	suite.Equal("approved", first["state"], "approval state per reviewer must be reachable in json output")
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
