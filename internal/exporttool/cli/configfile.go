package cli

import (
	"fmt"
	"log/slog"
	"os"
	"path/filepath"

	"github.com/sap/crossplane-provider-btp/internal/exporttool/cli/configparam"

	"github.com/spf13/viper"
)

func init() {
	registerConfigModule(configSetup)
}

func defaultConfigFileName() string {
	return fmt.Sprintf("export-cli-config-%s", Configuration.CLIConfiguration.ShortName)
}

var ConfigFileParam = configparam.String("config", "Configuration file").WithShortName("c")

func configSetup() error {
	ConfigFileParam.AttachToCommand(rootCommand)
	return nil
}

func configureConfigFile() {
	configFilePath := "."
	if ConfigFileParam.IsSet() {
		configFilePath = ConfigFileParam.Value()
	} else if v := os.Getenv("XDG_CONFIG_HOME"); v != "" {
		configFilePath = filepath.Join(v, defaultConfigFileName())
	} else if v := os.Getenv("HOME"); v != "" {
		configFilePath = filepath.Join(v, defaultConfigFileName())
	}
	ConfigFileParam.WithDefaultValue(configFilePath)
	viper.SetConfigFile(configFilePath)
	viper.SetConfigType("yaml")
}

type ConfigFileSettings map[string]any

func (cfs ConfigFileSettings) Set(key string, value any) {
	cfs[key] = value
}

func (cfs ConfigFileSettings) StoreConfig(configFile string) error {
	vip := viper.New()
	for k, v := range cfs {
		vip.Set(k, v)
	}
	vip.SetConfigFile(configFile)
	slog.Info("writing config file", "config-file", configFile)
	vip.SetConfigType("yaml")
	return vip.WriteConfig()
}
