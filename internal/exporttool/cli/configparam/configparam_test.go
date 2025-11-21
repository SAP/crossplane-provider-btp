package configparam_test

import (
	"github.com/sap/crossplane-provider-btp/internal/exporttool/cli/configparam"

	"github.com/spf13/cobra"
)

func ExampleString() {
	nameParam := configparam.String("name", "Name of the user").
		WithDefaultValue("anonymous").
		WithEnvVarName("USER").
		WithExample("user1").
		WithShortName("u")
	passwordParam := configparam.SensitiveString("password", "Password of the user").
		WithEnvVarName("PASSWORD").
		WithShortName("p")
	cmd := &cobra.Command{
		PreRun: func(cmd *cobra.Command, _ []string) {
			nameParam.BindConfiguration(cmd)
			passwordParam.BindConfiguration(cmd)
		},
	}
	nameParam.AttachToCommand(cmd)
	passwordParam.AttachToCommand(cmd)
}

func ExampleStringSlice() {
	groupParam := configparam.StringSlice("group", "Group that the user is member of").
		WithShortName("g")
	cmd := &cobra.Command{
		PreRun: func(cmd *cobra.Command, _ []string) {
			groupParam.BindConfiguration(cmd)
		},
	}
	groupParam.AttachToCommand(cmd)
}

func ExampleBool() {
	boolParam := configparam.Bool("verbose", "Turn verbose messages on").
		WithEnvVarName("VERBOSE").
		WithShortName("v").
		WithDefaultValue(false)
	cmd := &cobra.Command{
		PreRun: func(cmd *cobra.Command, _ []string) {
			boolParam.BindConfiguration(cmd)
		},
	}
	boolParam.AttachToCommand(cmd)
}
