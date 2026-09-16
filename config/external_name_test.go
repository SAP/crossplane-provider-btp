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

// TestServiceInstanceFrameworkResourceContract pins what map membership does
// not. Moving btp_subaccount_service_instance back to the CLI-only map fires
// neither of upjet's NewProvider panics and just leaves
// TerraformPluginFrameworkResource nil, so the nil check is what catches a
// routing regression. The placeholder literal is this repo's choice, so it is
// pinned here too. Keep this the package's only GetProvider() call.
func TestServiceInstanceFrameworkResourceContract(t *testing.T) {
	const name = "btp_subaccount_service_instance"

	p := GetProvider()
	r, ok := p.Resources[name]
	if !ok {
		t.Fatalf("resource %q not found in provider.Resources", name)
	}
	if r.TerraformPluginFrameworkResource == nil {
		t.Errorf("resource %q must have a non-nil TerraformPluginFrameworkResource", name)
	}

	// Upjet writes the placeholder into the reconstructed prior Terraform state
	// as "id" while the external-name is still empty, so the pre-create Read
	// fails against a bogus id instead of matching an arbitrary resource.
	m := map[string]any{}
	r.ExternalName.SetIdentifierArgumentFn(m, "")
	if got, want := m["id"], "NOT_EMPTY_GUID"; got != want {
		t.Errorf("resource %q SetIdentifierArgumentFn(m, \"\"): id = %v, want %v", name, got, want)
	}

	m2 := map[string]any{}
	r.ExternalName.SetIdentifierArgumentFn(m2, "a-real-guid")
	if got, want := m2["id"], "a-real-guid"; got != want {
		t.Errorf("resource %q SetIdentifierArgumentFn(m2, \"a-real-guid\"): id = %v, want %v", name, got, want)
	}
}
