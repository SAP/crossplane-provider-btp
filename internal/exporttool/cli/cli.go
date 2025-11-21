package cli

import (
	"fmt"
	"os"

	"github.com/sap/crossplane-provider-btp/internal/exporttool/cli/configparam"
	"github.com/sap/crossplane-provider-btp/internal/exporttool/erratt"

	"github.com/spf13/cobra"
)

func init() {
	registerConfigModule(configureCLI)
	Configuration.CLIConfiguration = defaultCLIConfiguration()
}

var rootCommand *cobra.Command

// Execute function is the main entrypoint for the CLI tool.
func Execute() {
	if err := configure(); err != nil {
		erratt.Slog(err)
		os.Exit(1)
	}
	if err := rootCommand.Execute(); err != nil {
		erratt.Slog(err)
		os.Exit(1)
	}
}

type CLIConfiguration struct {
	ConfiguratorCLI

	// ShortName is the abbreviated name of the observed system
	// that does not contain spaces, like "cf" for CloudFoundry
	// provider
	ShortName string

	// ObservedSystem is the full name of the external system that
	// may contain spaces, like "Cloud Foundry"
	ObservedSystem string

	// HasVerboseFlag indicates whether the CLI tool shall support
	// the --verbose/-v flag.
	HasVerboseFlag bool
}

func defaultCLIConfiguration() CLIConfiguration {
	return CLIConfiguration{
		ConfiguratorCLI: DefaultConfiguratorCLI{},
		ShortName:       "SHORTNAME_NOT_SET",
		ObservedSystem:  "OBSERVED_SYSTEM_NOT_SET",
		HasVerboseFlag:  true,
	}
}

// ConfiguratorCLI defines the methods that a value has to support to
// provider lazy CLI configuration parameters.
type ConfiguratorCLI interface {
	// CommandUse method returns the one-line usage message.
	CommandUse(config *ConfigSchema) string
	// CommandShort method returns the short description shown in the 'help' output.
	CommandShort(config *ConfigSchema) string
	// CommandLong method returns the long message shown in the 'help <this-command>' output.
	CommandLong(config *ConfigSchema) string
}

// DefaultConfiguratorCLI type satisfies the [ConfiguratorCLI]
// interface. It provides a basic method implementations based on the
// values set in the [ConfigSchema].
type DefaultConfiguratorCLI struct{}

var _ ConfiguratorCLI = DefaultConfiguratorCLI{}

func (c DefaultConfiguratorCLI) CommandUse(config *ConfigSchema) string {
	return fmt.Sprintf("%s-exporter [command] [flags...]", config.ShortName)
}

func (c DefaultConfiguratorCLI) CommandShort(config *ConfigSchema) string {
	return fmt.Sprintf("%s exporting tool", config.ObservedSystem)
}

func (c DefaultConfiguratorCLI) CommandLong(config *ConfigSchema) string {
	return fmt.Sprintf("%s exporting tool is a CLI tool for exporting existing resources as Crossplane managed resources",
		config.ObservedSystem)
}

func configureCLI() error {
	config := Configuration.CLIConfiguration
	verboseFlag := configparam.Bool("verbose", "Verbose output").
		WithShortName("v")
	rootCommand = &cobra.Command{
		Use:   config.CommandUse(Configuration),
		Short: config.CommandShort(Configuration),
		Long:  config.CommandLong(Configuration),
		PreRun: func(cmd *cobra.Command, _ []string) {
			if config.HasVerboseFlag {
				verboseFlag.BindConfiguration(cmd)
			}
		},
	}
	if config.HasVerboseFlag {
		verboseFlag.AttachToCommand(rootCommand)
	}

	return nil
}
