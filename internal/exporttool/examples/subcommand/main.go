package main

import (
	"context"
	"log/slog"

	"github.com/sap/crossplane-provider-btp/internal/exporttool/cli"
	"github.com/sap/crossplane-provider-btp/internal/exporttool/cli/configparam"
	_ "github.com/sap/crossplane-provider-btp/internal/exporttool/cli/export"
	"github.com/sap/crossplane-provider-btp/internal/exporttool/cli/widget"
)

var subcommand = &cli.BasicSubCommand{
	Name:         "widget",
	Short:        "widget testing",
	Long:         "demo widget capabilities",
	ConfigParams: []configparam.ConfigParam{},
	Run:          widgetTesting,
}

func widgetTesting(ctx context.Context) error {
	slog.Info("widget testing")

	text, err := widget.TextInput(ctx, "Testing TextInput", "enter text", false)
	if err != nil {
		return err
	}
	slog.Debug("Text entered", "value", text)

	sensitive, err := widget.TextInput(ctx, "Testing sensitive TextInput", "", true)
	if err != nil {
		return err
	}
	slog.Debug("Sensitive text entered", "value", sensitive)

	options, err := widget.MultiInput(ctx, "Testing MultiInput", []string{"option A", "option B", "option C"})
	if err != nil {
		return err
	}
	slog.Debug("Options selected", "values", options)
	return nil
}

func main() {
	cli.Configuration.ShortName = "test"
	cli.Configuration.ObservedSystem = "test system"
	cli.RegisterSubCommand(subcommand)
	cli.Execute()
}
