package export

import (
	"context"
	"fmt"
	"sync"

	"github.com/sap/crossplane-provider-btp/internal/exporttool/cli"
	"github.com/sap/crossplane-provider-btp/internal/exporttool/cli/configparam"
	"github.com/sap/crossplane-provider-btp/internal/exporttool/erratt"
)

func init() {
	cli.RegisterSubCommand(exportCmd)
}

type exportSubCommand struct {
	runCommand              func(context.Context, EventHandler) error
	configParams            configparam.ParamList
	exportableResourceKinds []string
}

var ResourceKindParam = configparam.StringSlice("exported kinds", "Resource kinds to export").
	WithShortName("k").
	WithFlagName("kind")

var OutputParam = configparam.String("output", "redirect the YAML output to a file").
	WithShortName("o").
	WithFlagName("output")

var (
	_         cli.SubCommand = &exportSubCommand{}
	exportCmd                = &exportSubCommand{
		runCommand: func(_ context.Context, _ EventHandler) error {
			return erratt.New("export subcommand is not set")
		},
		configParams: configparam.ParamList{
			ResourceKindParam,
			OutputParam,
		},
	}
)

func (c *exportSubCommand) GetName() string {
	return "export"
}

func (c *exportSubCommand) GetShort() string {
	return fmt.Sprintf("Export %s resources", cli.Configuration.CLIConfiguration.ObservedSystem)
}

func (c *exportSubCommand) GetLong() string {
	return fmt.Sprintf("Export %s resources and transform them into managed resources that the Crossplane provider can consume", cli.Configuration.CLIConfiguration.ObservedSystem)
}

func (c *exportSubCommand) GetConfigParams() configparam.ParamList {
	return c.configParams
}

func (c *exportSubCommand) MustIgnoreConfigFile() bool {
	return false
}

func (c *exportSubCommand) GetRun() func(context.Context) error {
	return func(ctx context.Context) error {
		evHandler := newEventHandler(ctx)
		wg := sync.WaitGroup{}
		wg.Add(1)
		go printErrors(ctx, &wg, evHandler.errorHandler.ch)

		wg.Add(1)
		go handleResources(ctx, &wg, evHandler.resourceHandler.ch, evHandler.errorHandler.ch)
		err := c.runCommand(ctx, evHandler)
		if err != nil {
			return err
		}
		wg.Wait()
		return nil
	}
}

func SetCommand(cmd func(context.Context, EventHandler) error) {
	exportCmd.runCommand = cmd
}

func AddConfigParams(param ...configparam.ConfigParam) {
	exportCmd.configParams = append(exportCmd.configParams, param...)
}

func GetConfigParams() configparam.ParamList {
	return exportCmd.configParams
}

func AddResourceKinds(kinds ...string) {
	exportCmd.exportableResourceKinds = append(exportCmd.exportableResourceKinds, kinds...)
	ResourceKindParam.WithPossibleValues(exportCmd.exportableResourceKinds)
}
