package profile

import (
	"fmt"

	"github.com/zalando/go-keyring"
)

// Credential represents a user credential for authentication.
type Credential struct {
	Username string
	Password string
}

// GetCredentialFromVault retrieves the credential for the given key from the Windows Credential Manager or Linux/macOS keychain.
func (profile Profile) GetCredentialFromVault(service, username string) (credential *Credential, err error) {
	secret, err := keyring.Get(service, username)
	if err != nil {
		return nil, fmt.Errorf("failed to get secret from keyring: %w", err)
	}
	if secret == "" {
		return nil, fmt.Errorf("key %s not found", service)
	}
	return &Credential{Username: username, Password: secret}, nil
}

// SetCredentialInVault stores the credential in the Windows Credential Manager or Linux/macOS keychain.
func (profile Profile) SetCredentialInVault(service, username, password string) error {
	if err := keyring.Set(service, username, password); err != nil {
		return fmt.Errorf("failed to set secret in keyring: %w", err)
	}
	return nil
}

// DeleteCredentialFromVault removes the credential from the Windows Credential Manager or Linux/macOS keychain.
func (profile Profile) DeleteCredentialFromVault(service, username string) error {
	if err := keyring.Delete(service, username); err != nil {
		return fmt.Errorf("failed to delete secret from keyring: %w", err)
	}
	return nil
}
