package branch_test

import (
	"encoding/json"
	"os"
	"reflect"
	"strings"
	"testing"

	"github.com/gildas/bitbucket-cli/cmd/branch"
	"github.com/joho/godotenv"
	"github.com/stretchr/testify/suite"
)

type BranchSuite struct {
	suite.Suite
	Name string
}

func TestBranchSuite(t *testing.T) {
	suite.Run(t, new(BranchSuite))
}

// *****************************************************************************
// Suite Tools

func (suite *BranchSuite) SetupSuite() {
	_ = godotenv.Load()
	suite.Name = strings.TrimSuffix(reflect.TypeFor[BranchSuite]().Name(), "Suite")
}

func (suite *BranchSuite) TearDownSuite() {
	if suite.T().Failed() {
		suite.T().Log("At least one test failed, we are not cleaning")
	}
}

func (suite *BranchSuite) LoadTestData(filename string) []byte {
	data, err := os.ReadFile("../../testdata/" + filename)
	if err != nil {
		suite.T().Fatal(err)
	}
	return data
}

func (suite *BranchSuite) UnmarshalData(filename string, v any) error {
	data := suite.LoadTestData(filename)
	return json.Unmarshal(data, v)
}

// *****************************************************************************

func (suite *BranchSuite) TestCanUnmarshal() {
	payload := suite.LoadTestData("branch.json")
	var b branch.Branch
	err := json.Unmarshal(payload, &b)
	suite.Require().NoError(err)
	suite.Require().NotNil(b)
	data, err := json.Marshal(b)
	suite.Require().NoError(err)
	suite.JSONEq(string(payload), string(data))
}
