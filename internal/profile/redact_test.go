package profile_test

import (
	"fmt"
	"net/url"

	"github.com/avitsrimer/bitbucket-cli/internal/profile"
)

// TestProfileRedactSurvivesStringerFormatting is a regression test for Redact's output being
// discarded by fmt: Profile implements fmt.Stringer (returning just the profile name), and fmt
// prefers Stringer over field-by-field struct formatting for %v/%+v. Since Redact used to return
// a plain Profile value, logging it with %+v printed only the name, silently dropping every
// redacted field - this asserts the redacted secret's masked form is visible, not the name-only
// Stringer output.
func (suite *ProfileSuite) TestProfileRedactSurvivesStringerFormatting() {
	target := profile.Profile{Name: "test-profile", ClientID: "super-secret-client-id"}

	rendered := fmt.Sprintf("%+v", target.Redact())

	suite.NotEqual("test-profile", rendered, "Redact's output must not collapse to Profile.String()'s bare name")
	suite.Contains(rendered, "REDACTED-", "the redacted ClientID should still be visible in its masked form")
	suite.NotContains(rendered, "super-secret-client-id", "the raw secret must never appear in the redacted rendering")
}

// TestProfileRedactMasksAPIRootUserinfoPassword is a regression test for review-iter4 finding 4:
// Redact masked ClientID/ClientSecret/User/Password/AccessToken/CloneUser but left APIRoot
// untouched, so a profile configured with userinfo credentials in its apiRoot (e.g.
// "https://alice:s3cr3t@api.bitbucket.org", preserved verbatim by MarshalYAML/UnmarshalYAML's
// string-form round trip) leaked its password in plain text through every debug log line that
// formats Redact()'s result with %+v (APIRoot's *url.URL implements fmt.Stringer, so fmt prints
// its full URL string instead of dumping the struct field-by-field).
func (suite *ProfileSuite) TestProfileRedactMasksAPIRootUserinfoPassword() {
	apiRoot, err := url.Parse("https://alice:s3cr3t@api.bitbucket.org")
	suite.Require().NoError(err)
	target := profile.Profile{Name: "test-profile", APIRoot: apiRoot}

	rendered := fmt.Sprintf("%+v", target.Redact())

	suite.NotContains(rendered, "s3cr3t", "the apiRoot password must never appear unredacted in Redact's output")
	suite.Contains(rendered, "alice", "the apiRoot username should still be visible for context")
}

// TestProfileRedactDoesNotMutateOriginalAPIRoot is a regression test: Redact copies Profile by
// value, but APIRoot is a pointer, so masking its userinfo in place (rather than through a cloned
// *url.URL) would corrupt the live profile's APIRoot for every other command still using it after
// this one logs it.
func (suite *ProfileSuite) TestProfileRedactDoesNotMutateOriginalAPIRoot() {
	apiRoot, err := url.Parse("https://alice:s3cr3t@api.bitbucket.org")
	suite.Require().NoError(err)
	target := profile.Profile{Name: "test-profile", APIRoot: apiRoot}

	_ = target.Redact()

	suite.Equal("https://alice:s3cr3t@api.bitbucket.org", target.APIRoot.String(), "Redact must not mutate the original profile's APIRoot")
}
