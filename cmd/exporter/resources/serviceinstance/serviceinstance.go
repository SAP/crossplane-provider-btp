package serviceinstance

import (
	"context"

	"github.com/SAP/crossplane-provider-cloudfoundry/exporttool/cli/configparam"
	"github.com/SAP/crossplane-provider-cloudfoundry/exporttool/cli/export"
	"github.com/sap/crossplane-provider-btp/btp"
	"github.com/sap/crossplane-provider-btp/cmd/exporter/resources"
)

const KIND_NAME = "serviceinstance"

var (
	Exporter = serviceinstanceExporter{}
)

func init() {
	resources.RegisterKind(Exporter)
}

type serviceinstanceExporter struct{}

var _ resources.Kind = serviceinstanceExporter{}

func (e serviceinstanceExporter) Param() configparam.ConfigParam {
	return nil
}

func (e serviceinstanceExporter) KindName() string {
	return KIND_NAME
}

func (e serviceinstanceExporter) Export(ctx context.Context, btpClient *btp.Client, eventHandler export.EventHandler, _ bool) error {
	return nil
}
