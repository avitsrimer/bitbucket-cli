package profile

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/avitsrimer/bitbucket-cli/internal/common"
	"github.com/spf13/cobra"
)

// TestProfilesLoadReloadsWhenConfigFlagChangesToADifferentPath mitigates major finding #2: a
// dynamic EnumFlag (e.g. --default-workspace) can resolve its allowed values during pflag's
// left-to-right ParseFlags pass, reaching Load through GetAll/authorize possibly before a --config
// flag appearing later on the same command line has itself been parsed -- so Load's first call can
// run against the wrong (default) config, and the len(*profiles) > 0 short-circuit would then
// wrongly reuse that wrong config's profiles for the rest of the command. Once --config has
// actually been Changed to a path different from the one Profiles was loaded from, Load must
// reload rather than trusting the short-circuit -- which is exactly the state of affairs by the
// time RunE runs, since ParseFlags has fully completed by then.
func TestProfilesLoadReloadsWhenConfigFlagChangesToADifferentPath(t *testing.T) {
	oldProfiles, oldCurrent := Profiles, Current
	oldConfig := common.CurrentConfig()
	t.Cleanup(func() {
		Profiles = oldProfiles
		Current = oldCurrent
		common.SetCurrentConfig(oldConfig)
	})
	Profiles = nil
	Current = nil
	common.SetCurrentConfig(nil)

	firstPath := filepath.Join(t.TempDir(), "first-config.yml")
	if err := os.WriteFile(firstPath, []byte("profiles:\n  - name: from-first-config\n"), 0o600); err != nil {
		t.Fatalf("cannot write first config: %v", err)
	}
	secondPath := filepath.Join(t.TempDir(), "second-config.yml")
	if err := os.WriteFile(secondPath, []byte("profiles:\n  - name: from-second-config\n"), 0o600); err != nil {
		t.Fatalf("cannot write second config: %v", err)
	}

	cmd := &cobra.Command{Use: "test"}
	cmd.PersistentFlags().String("config", "", "")
	cmd.SetContext(context.Background())

	// Simulates a dynamic EnumFlag's premature Set()-time call, before --config has been parsed:
	// the "config" flag is still unset, so ConfigPath/Initialize resolve against whatever the
	// process's ambient default config would be. To make the first load deterministic in this
	// test, set --config to firstPath directly (as if it were the flag's baked-in default value,
	// which never marks Changed -- see common.ConfigPath's doc comment on BB_CONFIG).
	if err := cmd.PersistentFlags().Set("config", firstPath); err != nil {
		t.Fatalf("cannot set config flag: %v", err)
	}
	cmd.PersistentFlags().Lookup("config").Changed = false // simulate a default value, not an explicit flag

	if err := Profiles.Load(context.Background(), cmd); err != nil {
		t.Fatalf("first Load() error = %v", err)
	}
	if len(Profiles) != 1 || Profiles[0].Name != "from-first-config" {
		t.Fatalf("Profiles after first Load = %v, want [from-first-config]", Profiles.Names())
	}

	// Now simulate --config having actually been explicitly parsed later on the same command
	// line, pointing at a different file -- the exact ordering hazard the finding describes.
	if err := cmd.PersistentFlags().Set("config", secondPath); err != nil {
		t.Fatalf("cannot set config flag to secondPath: %v", err)
	}

	if err := Profiles.Load(context.Background(), cmd); err != nil {
		t.Fatalf("second Load() error = %v", err)
	}
	if len(Profiles) != 1 || Profiles[0].Name != "from-second-config" {
		t.Fatalf("Profiles after second Load = %v, want [from-second-config]: Load must reload once --config is explicitly changed to a different path", Profiles.Names())
	}
}
