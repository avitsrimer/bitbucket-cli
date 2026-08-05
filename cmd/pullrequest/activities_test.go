package pullrequest_test

import (
	"encoding/json"
	"fmt"
	"os"
	"reflect"
	"strings"
	"testing"

	"github.com/gildas/bitbucket-cli/cmd/pullrequest"
	"github.com/joho/godotenv"
	"github.com/stretchr/testify/suite"
)

type ActivitySuite struct {
	suite.Suite
	Name string
}

func TestActivitySuite(t *testing.T) {
	suite.Run(t, new(ActivitySuite))
}

// *****************************************************************************
// Suite Tools

func (suite *ActivitySuite) SetupSuite() {
	_ = godotenv.Load()
	suite.Name = strings.TrimSuffix(reflect.TypeFor[ActivitySuite]().Name(), "Suite")
}

func (suite *ActivitySuite) TearDownSuite() {
	if suite.T().Failed() {
		suite.T().Log("At least one test failed, we are not cleaning")
	}
}

func (suite *ActivitySuite) LoadTestData(filename string) []byte {
	data, err := os.ReadFile("../../testdata/" + filename)
	if err != nil {
		suite.T().Fatal(err)
	}
	return data
}

func (suite *ActivitySuite) UnmarshalData(filename string, v any) error {
	data := suite.LoadTestData(filename)
	if err := json.Unmarshal(data, v); err != nil {
		return fmt.Errorf("cannot unmarshal test data: %w", err)
	}
	return nil
}

// *****************************************************************************

func (suite *ActivitySuite) TestCanUnmarshalApproval() {
	payload := suite.LoadTestData("activity-approval.json")
	var activity pullrequest.Activity
	err := json.Unmarshal(payload, &activity)
	suite.Require().NoError(err)
	suite.Require().NotNil(activity)
	_, err = json.Marshal(activity)
	suite.Require().NoError(err)
	suite.NotEmpty(activity.Approval)
	suite.Empty(activity.Comment)
	suite.Empty(activity.Update)
}
func (suite *ActivitySuite) TestCanUnmarshalUpdate() {
	payload := suite.LoadTestData("activity-update.json")
	var activity pullrequest.Activity
	err := json.Unmarshal(payload, &activity)
	suite.Require().NoError(err)
	suite.Require().NotNil(activity)
	_, err = json.Marshal(activity)
	suite.Require().NoError(err)
	suite.Empty(activity.Approval)
	suite.Empty(activity.Comment)
	suite.NotEmpty(activity.Update)
}

func (suite *ActivitySuite) TestCanUnmarshalComment() {
	payload := suite.LoadTestData("activity-comment.json")
	var activity pullrequest.Activity
	err := json.Unmarshal(payload, &activity)
	suite.Require().NoError(err)
	suite.Require().NotNil(activity)
	_, err = json.Marshal(activity)
	suite.Require().NoError(err)
	suite.Empty(activity.Approval)
	suite.NotEmpty(activity.Comment)
	suite.Empty(activity.Update)
}

func (suite *ActivitySuite) TestShouldFailUnmarshalWithoutApprovalNorCommentNorUpdate() {
	payload := suite.LoadTestData("activity-missing.json")
	var activity pullrequest.Activity
	err := json.Unmarshal(payload, &activity)
	suite.Require().Error(err)
}
