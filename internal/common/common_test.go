package common_test

import (
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

func TestCommonSuite(t *testing.T) {
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

// *****************************************************************************
