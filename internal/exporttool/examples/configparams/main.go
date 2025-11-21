package main

import (
	"context"
	"log/slog"

	"github.com/sap/crossplane-provider-btp/internal/exporttool/cli"
	"github.com/sap/crossplane-provider-btp/internal/exporttool/cli/configparam"
	_ "github.com/sap/crossplane-provider-btp/internal/exporttool/cli/export"
	"github.com/sap/crossplane-provider-btp/internal/exporttool/cli/widget"
	"github.com/sap/crossplane-provider-btp/internal/exporttool/erratt"
)

var subcommand = &cli.BasicSubCommand{
	Name:  "widget",
	Short: "widget testing",
	Long:  "demo widget capabilities",
	ConfigParams: []configparam.ConfigParam{
		selectorParam,
	},
	Run: widgetTesting,
}

var selectorParam = configparam.StringSlice("select", "Which widget to test. Possible values: text, sensitive or multi").
	WithShortName("s").
	WithPossibleValues([]string{"text", "sensitive", "multi"}).
	WithEnvVarName("SELECT")

func widgetTesting(ctx context.Context) error {
	slog.Info("widget testing")
	selectedWidgets, err := selectorParam.ValueOrAsk(ctx)
	if err != nil {
		return err
	}
	for _, selectedWidget := range selectedWidgets {
		slog.Debug("processing selection parameter", "parameter", selectedWidget)
		switch selectedWidget {
		case "text":
			text, err := widget.TextInput(ctx, "Testing TextInput", "enter text", false)
			if err != nil {
				return err
			}
			slog.Debug("Text entered", "value", text)
		case "sensitive":
			sensitive, err := widget.TextInput(ctx, "Testing sensitive TextInput", "", true)
			if err != nil {
				return err
			}
			slog.Debug("Sensitive text entered", "value", sensitive)
		case "multi":
			options, err := widget.MultiInput(ctx, "Testing MultiInput", []string{"option A", "option B", "option C"})
			if err != nil {
				return err
			}
			slog.Debug("Options selected", "values", options)
		default:
			return erratt.New("invalid parameter value", "parameter", selectorParam.Name, "value", selectedWidget)
		}
	}
	return nil
}

func main() {
	cli.Configuration.ShortName = "test"
	cli.Configuration.ObservedSystem = "test system"
	cli.RegisterSubCommand(subcommand)
	cli.Execute()
}
