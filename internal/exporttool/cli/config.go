package cli

import (
	"reflect"
	"runtime"
	"strings"

	"github.com/sap/crossplane-provider-btp/internal/exporttool/erratt"
)

type ConfigSchema struct {
	CLIConfiguration
	// ExportSubcommandConfiguration
}

type configModule func() error

var (
	Configuration *ConfigSchema = &ConfigSchema{}
	configModules               = []configModule{}
)

func registerConfigModule(module configModule) {
	configModules = append(configModules, module)
}

func configModuleName(module configModule) string {
	s := strings.Split(runtime.FuncForPC(reflect.ValueOf(module).Pointer()).Name(), ".")
	if len(s) == 0 {
		panic("invalid configModule name")
	}
	return s[len(s)-1]
}

func configure() error {
	for _, fn := range configModules {
		if err := fn(); err != nil {
			return erratt.Errorf("configuration failed: %w", err).
				With("module", configModuleName(fn))
		}
	}
	return nil
}
