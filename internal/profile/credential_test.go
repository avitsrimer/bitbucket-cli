package profile_test

import (
	"github.com/avitsrimer/bitbucket-cli/internal/profile"
	"github.com/zalando/go-keyring"
)

func (suite *ProfileSuite) TestGetClientSecretReturnsErrorWhenVaultLookupFails() {
	target := profile.Profile{
		Name:     "test-profile-without-client-secret",
		ClientID: "test-client-id-that-does-not-exist-in-the-vault",
		VaultKey: "bitbucket-cli-test-vault-key-that-does-not-exist",
	}

	_, err := target.GetClientSecret(suite.Context)
	suite.Require().Error(err, "GetClientSecret should propagate the vault lookup error instead of returning nil")
	// getSecretOrFromVault wraps *any* vault failure in the same "does not have a ..." message
	// (absent credential, locked keychain, unavailable backend all read identically), so assert
	// the wrapped cause instead of the message: this is specifically the not-found case.
	suite.ErrorIs(err, keyring.ErrNotFound, "should wrap the vault's own not-found sentinel")
}

func (suite *ProfileSuite) TestGetPasswordReturnsErrorWhenVaultLookupFails() {
	target := profile.Profile{
		Name:     "test-profile-without-password",
		User:     "test-user-that-does-not-exist-in-the-vault",
		VaultKey: "bitbucket-cli-test-vault-key-that-does-not-exist",
	}

	_, err := target.GetPassword(suite.Context)
	suite.Require().Error(err, "GetPassword should propagate the vault lookup error instead of returning nil")
	suite.ErrorIs(err, keyring.ErrNotFound, "should wrap the vault's own not-found sentinel")
}

// TestGetClientSecretReturnsStoredSecret is the positive-path counterpart to the not-found
// cases above: a successful vault lookup should return the stored credential's password, not
// just an error path.
func (suite *ProfileSuite) TestGetClientSecretReturnsStoredSecret() {
	const vaultKey = "bitbucket-cli-test-vault-key-positive-case"
	target := profile.Profile{
		Name:     "test-profile-with-client-secret",
		ClientID: "test-client-id-positive-case",
		VaultKey: vaultKey,
	}
	suite.Require().NoError(target.SetCredentialInVault(vaultKey, target.ClientID, "s3cr3t-from-vault"))
	suite.T().Cleanup(func() { _ = target.DeleteCredentialFromVault(vaultKey, target.ClientID) })

	secret, err := target.GetClientSecret(suite.Context)
	suite.Require().NoError(err)
	suite.Equal("s3cr3t-from-vault", secret)
}
