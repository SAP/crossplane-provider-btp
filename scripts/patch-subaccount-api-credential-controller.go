// Command patch-subaccount-api-credential-controller switches only the
// SubaccountApiCredential generated controller to its ownership-aware connector.
package main

import (
	"bytes"
	"fmt"
	"os"
)

const old = `tjcontroller.NewTerraformPluginFrameworkConnector(mgr.GetClient(), o.SetupFn, o.Provider.Resources["btp_subaccount_api_credential"], o.OperationTrackerStore,
				tjcontroller.WithTerraformPluginFrameworkLogger(o.Logger),
				tjcontroller.WithTerraformPluginFrameworkMetricRecorder(metrics.NewMetricRecorder(v1alpha1.SubaccountApiCredential_GroupVersionKind, mgr, o.PollInterval)),
				tjcontroller.WithTerraformPluginFrameworkManagementPolicies(o.Features.Enabled(features.EnableBetaManagementPolicies)))`
const replacement = `NewGuardedConnector(mgr.GetClient(), o.SetupFn, o.Provider.Resources["btp_subaccount_api_credential"], o.OperationTrackerStore,
				o.Logger,
				metrics.NewMetricRecorder(v1alpha1.SubaccountApiCredential_GroupVersionKind, mgr, o.PollInterval),
				o.Features.Enabled(features.EnableBetaManagementPolicies))`

func main() {
	args := os.Args[1:]
	if len(args) > 0 && args[0] == "--" {
		args = args[1:]
	}
	if len(args) != 1 {
		panic("usage: patch-subaccount-api-credential-controller <generated-controller.go>")
	}
	path := args[0]
	data, err := os.ReadFile(path)
	must(err)
	if bytes.Contains(data, []byte("NewGuardedConnector(")) {
		return
	}
	if bytes.Count(data, []byte(old)) != 1 {
		panic("expected exactly one standard Terraform framework connector expression in generated credential controller")
	}
	data = bytes.Replace(data, []byte(old), []byte(replacement), 1)
	must(os.WriteFile(path, data, 0o644))
}
func must(err error) {
	if err != nil {
		panic(fmt.Errorf("patch generated credential controller: %w", err))
	}
}
