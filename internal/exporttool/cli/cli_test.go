package cli_test

import "github.com/sap/crossplane-provider-btp/internal/exporttool/cli"

func ExampleExecute() {
	cli.Configuration.ShortName = "ts"
	cli.Configuration.ObservedSystem = "test system"
	cli.Execute()
}
