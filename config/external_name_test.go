package config

import "testing"

// TestExternalNameConfigMapsDisjoint guards the contract that each resource is
// reconciled by exactly one connector. If a resource lands in both maps,
// ExternalNameConfigurations() silently applies the CLI config and the
// plugin-framework include-list is wrong.
func TestExternalNameConfigMapsDisjoint(t *testing.T) {
	for name := range CLIReconciledExternalNameConfigs {
		if _, ok := TerraformPluginFrameworkReconciledExternalNameConfigs[name]; ok {
			t.Errorf("resource %q is in both CLIReconciled and TerraformPluginFramework external-name maps; it must be reconciled by exactly one connector", name)
		}
	}
}
