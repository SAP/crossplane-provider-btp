package cli

import (
	"context"
	"errors"
	"log/slog"
	"os"

	"github.com/sap/crossplane-provider-btp/internal/exporttool/cli/configparam"
	"github.com/sap/crossplane-provider-btp/internal/exporttool/erratt"

	"github.com/spf13/cobra"
	"github.com/spf13/viper"
)

func makeCobraRun(fn func(context.Context) error) func(*cobra.Command, []string) {
	return func(_ *cobra.Command, _ []string) {
		defer cancel()
		if err := fn(rootCtx); err != nil {
			erratt.Slog(err)
			os.Exit(1)
		}
	}
}

// RegisterSubCommand registers a subcommand to the CLI application.
func RegisterSubCommand(command SubCommand) {
	registerConfigModule(func() error {
		cmd := &cobra.Command{
			Use:   command.GetName(),
			Short: command.GetShort(),
			Long:  command.GetLong(),
			PreRunE: func(cmd *cobra.Command, _ []string) error {
				if err := viper.BindPFlags(rootCommand.PersistentFlags()); err != nil {
					return err
				}
				for _, cp := range command.GetConfigParams() {
					cp.BindConfiguration(cmd)
				}
				configureConfigFile()
				if !command.MustIgnoreConfigFile() {
					slog.Debug("reading configfile", "config-path", viper.ConfigFileUsed())
					if err := viper.ReadInConfig(); err != nil {
						if !errors.Is(err, os.ErrNotExist) {
							return erratt.Errorf("cannot read config file: %w", err).With("configfile", viper.GetViper().ConfigFileUsed())
						}
					}
				}
				configureLogging()
				return nil
			},
			Run: makeCobraRun(command.GetRun()),
		}
		rootCommand.AddCommand(cmd)
		for _, cp := range command.GetConfigParams() {
			cp.AttachToCommand(cmd)
		}
		return nil
	})
}

// SubCommand interface defines the methods that the types which
// implement a subcommand must implement.
type SubCommand interface {
	// GetName returns the name of the subcommand, such as 'login' or 'cleanup'.
	GetName() string
	// GetShort returns the short description shown in the 'help' output.
	GetShort() string
	// GetLong returns the long message shown in the 'help <this-command>' output.
	GetLong() string
	// GetConfigParams returns the configuration parameters that can be applied for the subcommand.
	GetConfigParams() configparam.ParamList
	// MustIgnoreConfigFile returns true if the subcommand shall ignore the parameters set in the configuration file.
	MustIgnoreConfigFile() bool
	// GetRun returns the function that is to be executed when the subcommand is invoked.
	GetRun() func(context.Context) error
}

// BasicSubCommand implements the [SubCommand] interface.
type BasicSubCommand struct {
	Name             string
	Short            string
	Long             string
	ConfigParams     configparam.ParamList
	IgnoreConfigFile bool
	Run              func(context.Context) error
}

var _ SubCommand = &BasicSubCommand{}

func (s *BasicSubCommand) GetName() string {
	return s.Name
}

func (s *BasicSubCommand) GetShort() string {
	return s.Short
}

func (s *BasicSubCommand) GetLong() string {
	return s.Long
}

func (s *BasicSubCommand) GetConfigParams() configparam.ParamList {
	return s.ConfigParams
}

func (s *BasicSubCommand) MustIgnoreConfigFile() bool {
	return s.IgnoreConfigFile
}

func (s *BasicSubCommand) GetRun() func(context.Context) error {
	return s.Run
}
