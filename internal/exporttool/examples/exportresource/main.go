package main

import (
	"context"
	"fmt"
	"log/slog"

	"github.com/sap/crossplane-provider-btp/internal/exporttool/cli"
	"github.com/sap/crossplane-provider-btp/internal/exporttool/cli/export"
	"github.com/sap/crossplane-provider-btp/internal/exporttool/erratt"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
)

func exportLogic(_ context.Context, events export.EventHandler) error {
	slog.Info("export invoked", "kind", export.ResourceKindParam.Value())
	for i := 0; i < 20; i++ {
		slog.Debug("exporting resource", "i", i)
		events.Resource(&unstructured.Unstructured{
			Object: map[string]interface{}{
				"user":     fmt.Sprintf("test-%d", i),
				"password": "secret",
			},
		})
		if i%5 == 0 {
			events.Warn(erratt.New("test warning", "reason", "test"))
		}
	}
	events.Stop()
	return nil
}

func main() {
	cli.Configuration.ShortName = "test"
	cli.Configuration.ObservedSystem = "test system"
	export.SetCommand(exportLogic)
	cli.Execute()
}
