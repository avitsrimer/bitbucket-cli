package profile_test

import (
	"testing"

	"github.com/avitsrimer/bitbucket-cli/internal/common"
	"github.com/avitsrimer/bitbucket-cli/internal/profile"
)

// TestProfileUpdateCopiesVaultKeyDefaultPageLengthAndErrorProcessing reproduces critical finding
// #4: updateSimpleFields (reached through Profile.Update) used to omit VaultKey, DefaultPageLength,
// and ErrorProcessing even though all three are bound to `profile update` flags -- so
// `profile update p --vault-key work --client-id ID --client-secret S` stored the secret in
// keyring service "work" but saved vaultKey: bitbucket-cli, breaking GetClientSecret on every
// later command against that profile, and --default-page-length/--error-processing were silent
// no-ops.
func TestProfileUpdateCopiesVaultKeyDefaultPageLengthAndErrorProcessing(t *testing.T) {
	target := profile.Profile{Name: "p", VaultKey: "bitbucket-cli", DefaultPageLength: 50}

	err := target.Update(profile.Profile{
		VaultKey:          "work",
		DefaultPageLength: 25,
		ErrorProcessing:   common.WarnOnError,
	})
	if err != nil {
		t.Fatalf("Update() error = %v", err)
	}

	if target.VaultKey != "work" {
		t.Errorf("VaultKey = %q, want %q", target.VaultKey, "work")
	}
	if target.DefaultPageLength != 25 {
		t.Errorf("DefaultPageLength = %d, want 25", target.DefaultPageLength)
	}
	if target.ErrorProcessing != common.WarnOnError {
		t.Errorf("ErrorProcessing = %v, want %v", target.ErrorProcessing, common.WarnOnError)
	}
}

// TestProfileUpdateLeavesFieldsUnchangedWhenOtherIsZero proves the fix does not regress
// updateSimpleFields' partial-update contract: fields other doesn't set (its zero value) must
// leave the target's existing value alone.
func TestProfileUpdateLeavesFieldsUnchangedWhenOtherIsZero(t *testing.T) {
	target := profile.Profile{Name: "p", VaultKey: "bitbucket-cli", DefaultPageLength: 50, ErrorProcessing: common.WarnOnError}

	err := target.Update(profile.Profile{})
	if err != nil {
		t.Fatalf("Update() error = %v", err)
	}

	if target.VaultKey != "bitbucket-cli" {
		t.Errorf("VaultKey = %q, want unchanged %q", target.VaultKey, "bitbucket-cli")
	}
	if target.DefaultPageLength != 50 {
		t.Errorf("DefaultPageLength = %d, want unchanged 50", target.DefaultPageLength)
	}
	if target.ErrorProcessing != common.WarnOnError {
		t.Errorf("ErrorProcessing = %v, want unchanged %v", target.ErrorProcessing, common.WarnOnError)
	}
}

// TestProfileUpdateCopiesDefaultRepository proves updateSimpleFields (reached through
// Profile.Update) copies DefaultRepository the same way it already copies DefaultWorkspace, and
// leaves it unchanged when other's value is the zero value (an absent --default-repository flag).
func TestProfileUpdateCopiesDefaultRepository(t *testing.T) {
	target := profile.Profile{Name: "p", DefaultRepository: "acme/old"}

	err := target.Update(profile.Profile{DefaultRepository: "acme/new"})
	if err != nil {
		t.Fatalf("Update() error = %v", err)
	}
	if target.DefaultRepository != "acme/new" {
		t.Errorf("DefaultRepository = %q, want %q", target.DefaultRepository, "acme/new")
	}

	err = target.Update(profile.Profile{})
	if err != nil {
		t.Fatalf("Update() error = %v", err)
	}
	if target.DefaultRepository != "acme/new" {
		t.Errorf("DefaultRepository = %q, want unchanged %q", target.DefaultRepository, "acme/new")
	}
}
