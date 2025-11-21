package cli

import (
	"strings"

	"github.com/spf13/viper"
)

func init() {
	registerConfigModule(configureViper)
}

func configureViper() error {
	viper.SetEnvKeyReplacer(strings.NewReplacer(".", "_"))
	viper.AutomaticEnv()
	return nil
}
