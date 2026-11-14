package main

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

const (
	generatedCredentialConnector = `managed.WithExternalConnecter(
			tjcontroller.NewTerraformPluginFrameworkConnector(mgr.GetClient(), o.SetupFn, o.Provider.Resources["btp_subaccount_api_credential"], o.OperationTrackerStore,
				tjcontroller.WithTerraformPluginFrameworkLogger(o.Logger),
				tjcontroller.WithTerraformPluginFrameworkMetricRecorder(metrics.NewMetricRecorder(v1alpha1.SubaccountApiCredential_GroupVersionKind, mgr, o.PollInterval)),
				tjcontroller.WithTerraformPluginFrameworkManagementPolicies(o.Features.Enabled(features.EnableBetaManagementPolicies)))),`
	adoptionGuardedCredentialConnector = `managed.WithExternalConnecter(
			&adoptionRejectingConnector{ExternalConnector: tjcontroller.NewTerraformPluginFrameworkConnector(mgr.GetClient(), o.SetupFn, o.Provider.Resources["btp_subaccount_api_credential"], o.OperationTrackerStore,
				tjcontroller.WithTerraformPluginFrameworkLogger(o.Logger),
				tjcontroller.WithTerraformPluginFrameworkMetricRecorder(metrics.NewMetricRecorder(v1alpha1.SubaccountApiCredential_GroupVersionKind, mgr, o.PollInterval)),
				tjcontroller.WithTerraformPluginFrameworkManagementPolicies(o.Features.Enabled(features.EnableBetaManagementPolicies))), Kube: mgr.GetClient()}),`
)

// patchSubaccountApiCredentialController applies the handwritten adoption guard
// after Upjet generation. The patch is intentionally strict and idempotent so
// an upstream template change cannot silently remove this resource safeguard.
func patchSubaccountApiCredentialController(root string) error {
	path := filepath.Join(root, "internal/controller/security/subaccountapicredential/zz_controller.go")
	content, err := os.ReadFile(path)
	if err != nil {
		return fmt.Errorf("read generated SubaccountApiCredential controller: %w", err)
	}
	source := string(content)
	switch {
	case strings.Contains(source, adoptionGuardedCredentialConnector):
		return nil
	case strings.Count(source, generatedCredentialConnector) != 1:
		return fmt.Errorf("expected exactly one uncustomized SubaccountApiCredential connector in %s", path)
	default:
		source = strings.Replace(source, generatedCredentialConnector, adoptionGuardedCredentialConnector, 1)
		info, err := os.Stat(path)
		if err != nil {
			return fmt.Errorf("stat generated SubaccountApiCredential controller: %w", err)
		}
		if err := os.WriteFile(path, []byte(source), info.Mode().Perm()); err != nil {
			return fmt.Errorf("write generated SubaccountApiCredential controller: %w", err)
		}
		return nil
	}
}
