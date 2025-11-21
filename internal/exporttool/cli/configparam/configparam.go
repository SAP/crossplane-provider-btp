package configparam

import (
	"github.com/spf13/cobra"
	"github.com/spf13/viper"
)

type configParam[T any] struct {
	Name        string
	Description string
	ShortName   *string
	FlagName    string
	EnvVarName  string
	Example     string
	parent      *T
}

func newConfigParam[T any](parent *T, name, description string) *configParam[T] {
	return &configParam[T]{
		parent:      parent,
		Name:        name,
		FlagName:    name,
		Description: description,
	}
}

// Name returns with the configured name of the configparam value.
func (p *configParam[T]) GetName() string {
	return p.Name
}

// WithShortName sets the short name of the command line flag that can
// be used to configure the parameter value.
func (p *configParam[T]) WithShortName(shortName string) *T {
	p.ShortName = &shortName
	return p.parent
}

// WithFlagName sets the name of the command line flag that can be
// used to configure the parameter value.
func (p *configParam[T]) WithFlagName(flagName string) *T {
	p.FlagName = flagName
	return p.parent
}

// WithEnvVarName sets the name of the environment variable that can
// be used to configure the parameter value.
func (p *configParam[T]) WithEnvVarName(envVarName string) *T {
	p.EnvVarName = envVarName
	return p.parent
}

// WithExample sets an example value for the parameter.
func (p *configParam[T]) WithExample(example string) *T {
	p.Example = example
	return p.parent
}

// IsSet returns true of the configuration parameter is set.
func (p *configParam[T]) IsSet() bool {
	return viper.IsSet(p.Name)
}

func (p *configParam[T]) BindConfiguration(command *cobra.Command) {
	if p.EnvVarName != "" {
		if err := viper.BindEnv(p.Name, p.EnvVarName); err != nil {
			panic(err)
		}
	}
	if err := viper.BindPFlag(p.Name, command.PersistentFlags().Lookup(p.FlagName)); err != nil {
		panic(err)
	}

}

type defaultValue[CP, T any] struct {
	defaultValue T
	parent       *CP
}

func newDefaultValue[CP, T any](parent *CP, defaultDefault T) *defaultValue[CP, T] {
	return &defaultValue[CP, T]{
		parent:       parent,
		defaultValue: defaultDefault,
	}
}

// WithDefaultValue sets the default value of the configured
// configuration parameter.
func (v *defaultValue[CP, T]) WithDefaultValue(value T) *CP {
	v.defaultValue = value
	return v.parent
}

type configWithDefaultValue[CP, T any] struct {
	*defaultValue[CP, T]
	*configParam[CP]
}

func newConfigWithDefaultValue[CP, T any](parent *CP, name, description string, defaultDefault T) *configWithDefaultValue[CP, T] {
	return &configWithDefaultValue[CP, T]{
		defaultValue: newDefaultValue(parent, defaultDefault),
		configParam:  newConfigParam(parent, name, description),
	}
}

func (p *configWithDefaultValue[CP, T]) attachToCommand(flagFn func(name string, value T, usage string) *T, flagShortFn func(name, shorthand string, value T, usage string) *T) {
	if p.ShortName != nil {
		flagShortFn(p.configParam.FlagName, *p.configParam.ShortName, p.defaultValue.defaultValue, p.configParam.Description)
	} else {
		flagFn(p.configParam.FlagName, p.defaultValue.defaultValue, p.configParam.Description)
	}
}

func (p *configWithDefaultValue[CP, T]) value(valueGetter func(string) T) T {
	if p.configParam.IsSet() {
		return valueGetter(p.configParam.Name)
	}
	return p.defaultValue.defaultValue
}

// ConfigParam interface defines the methods that a configuration
// parameter type must implement.
type ConfigParam interface {
	GetName() string
	AttachToCommand(cmd *cobra.Command)
	BindConfiguration(cmd *cobra.Command)
}

type ParamList []ConfigParam
