package common

import (
	"errors"
	"os"
	"path/filepath"

	"github.com/go-pkgz/lgr"
	"github.com/spf13/cobra"
	"github.com/spf13/viper"
)

// Initialize configure the logger and load the Viper Configuration
func Initialize(cmd *cobra.Command) (err error) {
	initializeLogger(cmd)
	return initializeConfiguration(cmd)
}

// initializeLogger configures the logger based on the command line flags and environment variables
func initializeLogger(cmd *cobra.Command) {
	options := []lgr.Option{lgr.Out(os.Stderr), lgr.Err(os.Stderr)}
	if cmd.Root().PersistentFlags().Changed("debug") && cmd.Root().PersistentFlags().Lookup("debug").Value.String() == "true" {
		options = append(options, lgr.Debug, lgr.CallerFile, lgr.CallerFunc, lgr.Msec, lgr.LevelBraces)
	}
	lgr.Setup(options...)
}

// initializeConfiguration loads the configuration file and profiles
func initializeConfiguration(cmd *cobra.Command) (err error) {
	viper.SetConfigType("yaml")
	if cmd.Root().PersistentFlags().Changed("config") {
		viper.SetConfigFile(cmd.Root().PersistentFlags().Lookup("config").Value.String())
	} else if configDir, _ := os.UserConfigDir(); configDir != "" {
		viper.AddConfigPath(filepath.Join(configDir, "bitbucket"))
		viper.SetConfigName("config-cli.yml")
	} else {
		homeDir, err := os.UserHomeDir()
		cobra.CheckErr(err)
		viper.AddConfigPath(homeDir)
		viper.SetConfigName(".bitbucket-cli")
	}

	err = viper.ReadInConfig()
	if verr, ok := errors.AsType[viper.ConfigFileNotFoundError](err); ok {
		lgr.Printf("[WARN] config file not found: %s", verr)
	} else if err != nil {
		return errors.Join(errors.New("failed to read config file"), err)
	} else {
		lgr.Printf("[DEBUG] config file: %s", viper.ConfigFileUsed())
	}
	return nil
}
