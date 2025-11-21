package configparam

import (
	"context"
	"fmt"

	"github.com/sap/crossplane-provider-btp/internal/exporttool/cli/widget"

	"github.com/spf13/cobra"
	"github.com/spf13/viper"
)

// StringParam type represents a configuration parameter that
// holds a []string value.
type StringParam struct {
	sensitive bool
	*configWithDefaultValue[StringParam, string]
}

var _ ConfigParam = &StringParam{}

func stringGenerator(name, description string, sensitive bool) *StringParam {
	p := &StringParam{
		sensitive: sensitive,
	}
	p.configWithDefaultValue = newConfigWithDefaultValue(p, name, description, "")
	return p
}

// String creates a [StringParam] value. The mandatory name and
// description parameters must be set.
func String(name, description string) *StringParam {
	return stringGenerator(name, description, false)
}

// SensitiveString returns a new [StringParam] whose contents are
// masked when the parameter is printed to the console. Both name and
// description are required.
func SensitiveString(name, description string) *StringParam {
	return stringGenerator(name, description, true)
}

// AttachToCommand registers the persistent string flag (long form and
// optional short form) with the supplied cobra.Command.
func (p *StringParam) AttachToCommand(command *cobra.Command) {
	p.attachToCommand(command.PersistentFlags().String, command.PersistentFlags().StringP)
}

// Value returns the user configured string. If the user has not
// configured any values, the default value is returned.
func (p *StringParam) Value() string {
	return p.value(viper.GetString)
}

// ValueOrAsk returns the configured slice or prompts the user to
// enter a value value.
//
// After successful selection the entered string is stored and the
// parameter is considered set.
func (p *StringParam) ValueOrAsk(ctx context.Context) (string, error) {
	if p.configParam.IsSet() {
		return p.Value(), nil
	}
	return p.AskValue(ctx)
}

func (p *StringParam) inputPrompt() string {
	return fmt.Sprintf("%s [%s]: ", p.configParam.Description, p.configParam.Name)
}

func (p *StringParam) askValue(ctx context.Context, sensitive bool) (string, error) {
	return widget.TextInput(ctx,
		p.inputPrompt(),
		p.configParam.Example,
		sensitive,
	)
}

func (p *StringParam) AskValue(ctx context.Context) (string, error) {
	value, err := p.askValue(ctx, p.sensitive)
	if err != nil {
		return "", err
	}
	viper.Set(p.configParam.Name, value)
	return value, nil
}
