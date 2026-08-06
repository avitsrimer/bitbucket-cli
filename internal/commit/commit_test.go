package commit_test

import (
	"encoding/json"
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

// ***********************************************************************

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

// TestCommitReferenceGetShortHashLongHash proves the normal case still returns a 7-character
// short hash.
func (suite *CommitSuite) TestCommitReferenceGetShortHashLongHash() {
	reference := commit.CommitReference{Hash: "026560720168aa12820a01e8262f6bb60f0639d1"}
	suite.Equal("0265607", reference.GetShortHash())
}

// TestCommitReferenceGetShortHashShortHashDoesNotPanic proves a hash shorter than 7 characters is
// returned as-is instead of being sliced out of range.
func (suite *CommitSuite) TestCommitReferenceGetShortHashShortHashDoesNotPanic() {
	reference := commit.CommitReference{Hash: "abc"}
	suite.NotPanics(func() {
		suite.Equal("abc", reference.GetShortHash())
	})
}

// TestCommitReferenceGetShortHashEmptyHash covers the boundary case of an empty hash.
func (suite *CommitSuite) TestCommitReferenceGetShortHashEmptyHash() {
	reference := commit.CommitReference{}
	suite.Empty(reference.GetShortHash())
}

// TestLongHashSorterComparesHash proves the "longhash" column's Compare sorts by Hash, matching
// its purpose as a hash-based sorter exposed via "bb pr commits --sort longhash".
func (suite *CommitSuite) TestLongHashSorterComparesHash() {
	compare := commit.Columns().SortBy("longhash")

	a := commit.Commit{Hash: "aaaaaaa", Message: "zzz message"}
	b := commit.Commit{Hash: "bbbbbbb", Message: "aaa message"}

	suite.True(compare(a, b), "expected a (hash aaaaaaa) to sort before b (hash bbbbbbb)")
	suite.False(compare(b, a), "expected b (hash bbbbbbb) not to sort before a (hash aaaaaaa)")
}
