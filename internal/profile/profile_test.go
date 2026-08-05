package profile_test

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"reflect"
	"strings"
	"testing"

	"github.com/joho/godotenv"
	"github.com/stretchr/testify/suite"
)

type ProfileSuite struct {
	suite.Suite
	Name    string
	Context context.Context
}

func TestProfileSuite(t *testing.T) {
	suite.Run(t, new(ProfileSuite))
}

// *****************************************************************************
// Suite Tools

func (suite *ProfileSuite) SetupSuite() {
	_ = godotenv.Load()
	suite.Name = strings.TrimSuffix(reflect.TypeFor[ProfileSuite]().Name(), "Suite")
	suite.Context = context.Background()
}

func (suite *ProfileSuite) TearDownSuite() {
	if suite.T().Failed() {
		suite.T().Log("At least one test failed, we are not cleaning")
	}
}

func (suite *ProfileSuite) LoadTestData(filename string) []byte {
	data, err := os.ReadFile("../../testdata/" + filename)
	if err != nil {
		suite.T().Fatal(err)
	}
	return data
}

func (suite *ProfileSuite) UnmarshalData(filename string, v any) error {
	data := suite.LoadTestData(filename)
	if err := json.Unmarshal(data, v); err != nil {
		return fmt.Errorf("cannot unmarshal test data: %w", err)
	}
	return nil
}

// *****************************************************************************
