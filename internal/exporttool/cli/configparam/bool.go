package configparam

import (
	"github.com/spf13/cobra"
	"github.com/spf13/viper"
)

// BoolParam type represents a configuration parameter that holds a
// bool value.
type BoolParam struct {
	*configWithDefaultValue[BoolParam, bool]
}

var _ ConfigParam = &BoolParam{}

// Bool creates [BoolParam] value. The mandatory name and description
// parameters must be set.
func Bool(name, description string) *BoolParam {
	p := &BoolParam{}
	p.configWithDefaultValue = newConfigWithDefaultValue(p, name, description, false)
	return p
}

// AttachToCommand registers the persistent bool flag (long form and
// optional short form) with the supplied [cobra.Command].
func (p *BoolParam) AttachToCommand(command *cobra.Command) {
	p.attachToCommand(command.PersistentFlags().Bool, command.PersistentFlags().BoolP)
}

// Value returns the user configured string slice. If the user has not
// configured any values, the default value is returned.
func (p *BoolParam) Value() bool {
	return p.value(viper.GetBool)
}
