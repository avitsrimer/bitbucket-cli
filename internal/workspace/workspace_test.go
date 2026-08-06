package workspace_test

import (
	"encoding/json"
	"os"
	"reflect"
	"strings"
	"testing"

	"github.com/avitsrimer/bitbucket-cli/internal/workspace"
	"github.com/joho/godotenv"
	"github.com/stretchr/testify/suite"
)

type WorkspaceSuite struct {
	suite.Suite
	Name string
}

func TestWorkspaceSuite(t *testing.T) {
	suite.Run(t, new(WorkspaceSuite))
}

// *****************************************************************************
// Suite Tools

func (suite *WorkspaceSuite) SetupSuite() {
	_ = godotenv.Load()
	suite.Name = strings.TrimSuffix(reflect.TypeFor[WorkspaceSuite]().Name(), "Suite")
}

func (suite *WorkspaceSuite) TearDownSuite() {
	if suite.T().Failed() {
		suite.T().Log("At least one test failed, we are not cleaning")
	}
}

func (suite *WorkspaceSuite) LoadTestData(filename string) []byte {
	data, err := os.ReadFile("../../testdata/" + filename)
	if err != nil {
		suite.T().Fatal(err)
	}
	return data
}

// ***********************************************************************

func (suite *WorkspaceSuite) TestCanUnmarshal() {
	payload := suite.LoadTestData("workspace.json")
	var workspace workspace.Workspace
	err := json.Unmarshal(payload, &workspace)
	suite.Require().NoError(err)
	suite.Equal("myworkspace", workspace.Slug)
	suite.Equal("{12345678-9abc-def0-1234-56789abcdef0}", workspace.ID.String())
}

func (suite *WorkspaceSuite) TestCanUnmarshal_WithWorkspaceAccess() {
	payload := suite.LoadTestData("workspace-access.json")
	var workspace workspace.Workspace
	err := json.Unmarshal(payload, &workspace)
	suite.Require().NoError(err)
	suite.Equal("myworkspace", workspace.Slug)
	suite.True(workspace.Administrator)
	suite.Equal("{12345678-9abc-def0-1234-56789abcdef0}", workspace.ID.String())
}
