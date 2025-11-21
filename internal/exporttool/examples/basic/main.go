package main

import (
	"github.com/sap/crossplane-provider-btp/internal/exporttool/cli"
	_ "github.com/sap/crossplane-provider-btp/internal/exporttool/cli/export"
)

func main() {
	cli.Configuration.ShortName = "test"
	cli.Configuration.ObservedSystem = "test system"
	cli.Execute()
}
