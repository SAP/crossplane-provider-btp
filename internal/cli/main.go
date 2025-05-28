package main

import (
	"github.com/sap/crossplane-provider-btp/internal/cli/cmd"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/log/zap"
)

func main() {
	// Set up a new logger
	ctrl.SetLogger(zap.New(zap.UseDevMode(true)))
	cmd.Execute()
}
