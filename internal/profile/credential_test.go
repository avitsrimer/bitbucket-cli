package profile_test

import "github.com/avitsrimer/bitbucket-cli/internal/profile"

func (suite *ProfileSuite) TestGetClientSecretReturnsErrorWhenVaultLookupFails() {
	target := profile.Profile{
		Name:     "test-profile-without-client-secret",
		ClientID: "test-client-id-that-does-not-exist-in-the-vault",
		VaultKey: "bitbucket-cli-test-vault-key-that-does-not-exist",
	}

	_, err := target.GetClientSecret(suite.Context)
	suite.Require().Error(err, "GetClientSecret should propagate the vault lookup error instead of returning nil")
	suite.Contains(err.Error(), "does not have a client secret")
}

func (suite *ProfileSuite) TestGetPasswordReturnsErrorWhenVaultLookupFails() {
	target := profile.Profile{
		Name:     "test-profile-without-password",
		User:     "test-user-that-does-not-exist-in-the-vault",
		VaultKey: "bitbucket-cli-test-vault-key-that-does-not-exist",
	}

	_, err := target.GetPassword(suite.Context)
	suite.Require().Error(err, "GetPassword should propagate the vault lookup error instead of returning nil")
	suite.Contains(err.Error(), "does not have a password")
}
