package comment_test

import (
	"encoding/json"
	"reflect"
	"strings"
	"testing"

	"github.com/gildas/bitbucket-cli/cmd/pullrequest/comment"
	"github.com/joho/godotenv"
	"github.com/stretchr/testify/suite"
)

type CommentCreateSuite struct {
	suite.Suite
	Name string
}

func TestCommentCreateSuite(t *testing.T) {
	suite.Run(t, new(CommentCreateSuite))
}

// *****************************************************************************
// Suite Tools

func (suite *CommentCreateSuite) SetupSuite() {
	_ = godotenv.Load()
	suite.Name = strings.TrimSuffix(reflect.TypeOf(suite).Elem().Name(), "Suite")
}

func (suite *CommentCreateSuite) TearDownSuite() {
	if suite.T().Failed() {
		suite.T().Log("At least one test failed, we are not cleaning")
	}
}

// *****************************************************************************

func (suite *CommentCreateSuite) TestCanMarshalCommentCreatorWithParent() {
	creator := comment.CommentCreator{
		Content: comment.ContentCreator{Raw: "This is a reply"},
		Parent:  &comment.ParentReference{ID: 759578390},
	}

	data, err := json.Marshal(creator)
	suite.Require().NoError(err)

	var result map[string]any
	err = json.Unmarshal(data, &result)
	suite.Require().NoError(err)

	content, ok := result["content"].(map[string]any)
	suite.Require().True(ok, "content should be present")
	suite.Equal("This is a reply", content["raw"])

	parent, ok := result["parent"].(map[string]any)
	suite.Require().True(ok, "parent should be present")
	suite.Assert().Equal(float64(759578390), parent["id"])
}

func (suite *CommentCreateSuite) TestCanMarshalCommentCreatorWithoutParent() {
	creator := comment.CommentCreator{
		Content: comment.ContentCreator{Raw: "This is a top-level comment"},
	}

	data, err := json.Marshal(creator)
	suite.Require().NoError(err)

	var result map[string]any
	err = json.Unmarshal(data, &result)
	suite.Require().NoError(err)

	content, ok := result["content"].(map[string]any)
	suite.Require().True(ok, "content should be present")
	suite.Equal("This is a top-level comment", content["raw"])

	_, ok = result["parent"]
	suite.False(ok, "parent should not be present when nil")
}

func (suite *CommentCreateSuite) TestCommentCreatorJSONMatchesBitbucketAPIFormat() {
	creator := comment.CommentCreator{
		Content: comment.ContentCreator{Raw: "Done!"},
		Parent:  &comment.ParentReference{ID: 759578390},
	}

	data, err := json.Marshal(creator)
	suite.Require().NoError(err)

	expected := `{"content":{"raw":"Done!"},"parent":{"id":759578390}}`
	suite.JSONEq(expected, string(data))
}

func (suite *CommentCreateSuite) TestCanMarshalCommentCreatorWithPending() {
	creator := comment.CommentCreator{
		Content: comment.ContentCreator{Raw: "This is a top-level comment"},
		Pending: new(true),
	}

	data, err := json.Marshal(creator)
	suite.Require().NoError(err)

	var result map[string]any
	err = json.Unmarshal(data, &result)
	suite.Require().NoError(err)

	content, ok := result["content"].(map[string]any)
	suite.Require().True(ok, "content should be present")
	suite.Equal("This is a top-level comment", content["raw"])

	_, ok = result["parent"]
	suite.False(ok, "parent should not be present when nil")

	pending, ok := result["pending"].(bool)
	suite.Require().True(ok, "pending should be present")
	suite.True(pending, "pending should be true")
}
