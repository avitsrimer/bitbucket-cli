package profile_test

import (
	"fmt"

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
