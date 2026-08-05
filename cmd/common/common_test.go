package common_test

import (
	"encoding/json"
	"os"
	"reflect"
	"strings"
	"testing"

	"github.com/joho/godotenv"
	"github.com/stretchr/testify/suite"
)

type CommonSuite struct {
	suite.Suite
	Name string
}

func TestPullRequestSuite(t *testing.T) {
	suite.Run(t, new(CommonSuite))
}

// *****************************************************************************
// Suite Tools

func (suite *CommonSuite) SetupSuite() {
	_ = godotenv.Load()
	suite.Name = strings.TrimSuffix(reflect.TypeFor[CommonSuite]().Name(), "Suite")
}

func (suite *CommonSuite) TearDownSuite() {
	if suite.T().Failed() {
		suite.T().Log("At least one test failed, we are not cleaning")
	}
}

func (suite *CommonSuite) LoadTestData(filename string) []byte {
	data, err := os.ReadFile("../../testdata/" + filename)
	if err != nil {
		suite.T().Fatal(err)
	}
	return data
}

func (suite *CommonSuite) UnmarshalData(filename string, v any) error {
	data := suite.LoadTestData(filename)
	return json.Unmarshal(data, v)
}

// *****************************************************************************
