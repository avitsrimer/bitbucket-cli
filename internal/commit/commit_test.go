package commit_test

import (
	"encoding/json"
	"fmt"
	"os"
	"reflect"
	"strings"
	"testing"

	"github.com/avitsrimer/bitbucket-cli/internal/commit"
	"github.com/joho/godotenv"
	"github.com/stretchr/testify/suite"
)

type CommitSuite struct {
	suite.Suite
	Name string
}

func TestCommitSuite(t *testing.T) {
	suite.Run(t, new(CommitSuite))
}

// *****************************************************************************
// Suite Tools

func (suite *CommitSuite) SetupSuite() {
	_ = godotenv.Load()
	suite.Name = strings.TrimSuffix(reflect.TypeFor[CommitSuite]().Name(), "Suite")
}

func (suite *CommitSuite) TearDownSuite() {
	if suite.T().Failed() {
		suite.T().Log("At least one test failed, we are not cleaning")
	}
}

func (suite *CommitSuite) LoadTestData(filename string) []byte {
	data, err := os.ReadFile("../../testdata/" + filename)
	if err != nil {
		suite.T().Fatal(err)
	}
	return data
}

func (suite *CommitSuite) UnmarshalData(filename string, v any) error {
	data := suite.LoadTestData(filename)
	if err := json.Unmarshal(data, v); err != nil {
		return fmt.Errorf("cannot unmarshal test data: %w", err)
	}
	return nil
}

// *****************************************************************************

func (suite *CommitSuite) TestCanUnmarshal() {
	payload := suite.LoadTestData("commit.json")
	var c commit.Commit
	err := json.Unmarshal(payload, &c)
	suite.Require().NoError(err)
	suite.Require().NotNil(c)
	data, err := json.Marshal(c)
	suite.Require().NoError(err)
	suite.JSONEq(string(payload), string(data))
}

func (suite *CommitSuite) TestCanMarshalCommitReference() {
	expected := `{"type": "commit", "hash": "026560720168aa12820a01e8262f6bb60f0639d1"}`
	reference := commit.CommitReference{Hash: "026560720168aa12820a01e8262f6bb60f0639d1"}

	data, err := json.Marshal(reference)
	suite.Require().NoError(err)
	suite.JSONEq(expected, string(data))
}
