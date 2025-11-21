package configparam

import (
	"context"

	"github.com/sap/crossplane-provider-btp/internal/exporttool/cli/widget"
	"github.com/sap/crossplane-provider-btp/internal/exporttool/erratt"

	"github.com/spf13/cobra"
	"github.com/spf13/viper"
)

type possibleValuesFnType func() ([]string, error)

// StringSliceParam type represents a configuration parameterkxo that
// holds a []string value.
type StringSliceParam struct {
	*configWithDefaultValue[StringSliceParam, []string]
	sensitive        bool
	possibleValues   []string
	possibleValuesFn possibleValuesFnType
}

var _ ConfigParam = &StringSliceParam{}

func stringSliceGenerator(name, description string, sensitive bool) *StringSliceParam {
	p := &StringSliceParam{
		sensitive:      sensitive,
		possibleValues: []string{},
	}
	p.configWithDefaultValue = newConfigWithDefaultValue(p, name, description, []string{})
	return p
}

// StringSlice creates a [StringSliceParam] value. The mandatory name
// and description parameters must be set.
func StringSlice(name, description string) *StringSliceParam {
	return stringSliceGenerator(name, description, false)
}

// SensitiveStringSlice returns a new [StringSliceParam] whose
// contents are masked when the parameter is printed to the
// console. Both name and description are required.
func SensitiveStringSlice(name, description string) *StringSliceParam {
	return stringSliceGenerator(name, description, true)
}

// WithPossibleValues restricts the interactive selection to the
// supplied slice. When the CLI prompts the user, only these strings
// are offered as choices.
func (p *StringSliceParam) WithPossibleValues(values []string) *StringSliceParam {
	p.possibleValues = values
	return p
}

// WithPossibleValuesFn lazily supplies the valid choices for
// interactive selection. The given function is called when the CLI
// prompts the user. The returned strings are presented as options.
func (p *StringSliceParam) WithPossibleValuesFn(fn func() ([]string, error)) *StringSliceParam {
	p.possibleValuesFn = fn
	return p
}

// AttachToCommand registers the persistent string-slice flag (long form and
// optional short form) with the supplied [cobra.Command].
func (p *StringSliceParam) AttachToCommand(command *cobra.Command) {
	p.attachToCommand(command.PersistentFlags().StringSlice, command.PersistentFlags().StringSliceP)
}

// Value returns the user configured string slice. If the user has not
// configured any values, the default value is returned.
func (p *StringSliceParam) Value() []string {
	return p.value(viper.GetStringSlice)
}

// ValueOrAsk returns the configured slice or prompts the user to
// choose from the values supplied via [WithPossibleValues] or
// [WithPossibleValuesFn].  It fails if neither has been set.
//
// After successful selection the chosen slice is stored and the
// parameter is considered set.
func (p *StringSliceParam) ValueOrAsk(ctx context.Context) ([]string, error) {
	if p.configParam.IsSet() {
		return p.Value(), nil
	}
	if len(p.possibleValues) == 0 && p.possibleValuesFn == nil {
		return nil, erratt.New("StringSliceParam ValueOrAsk invoked but possibleValues are not set", "name", p.configParam.Name)
	}
	possibleValues := p.possibleValues
	if len(possibleValues) == 0 {
		var err error
		possibleValues, err = p.possibleValuesFn()
		if err != nil {
			return nil, erratt.Errorf("cannot get possible values: %w", err)
		}
		if len(possibleValues) == 0 {
			return []string{}, nil
		}
	}
	values, err := widget.MultiInput(ctx,
		p.configParam.Description,
		possibleValues,
	)
	if err != nil {
		return nil, err
	}
	viper.Set(p.Name, values)
	return values, nil
}
