package main

import (
	"context"
	"log/slog"

	"github.com/sap/crossplane-provider-btp/internal/exporttool/cli"
	"github.com/sap/crossplane-provider-btp/internal/exporttool/cli/export"
)

func exportLogic(_ context.Context, events export.EventHandler) error {
	slog.Info("export command invoked")
	events.Stop()
	return nil
}

func main() {
	cli.Configuration.ShortName = "test"
	cli.Configuration.ObservedSystem = "test system"
	export.SetCommand(exportLogic)
	cli.Execute()
}
