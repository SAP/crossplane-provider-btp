package main

import (
	"errors"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"gopkg.in/yaml.v3"
)

func TestTransformCreatesCloneReadyManifest(t *testing.T) {
	directory := t.TempDir()
	input := filepath.Join(directory, "raw.yaml")
	output := filepath.Join(directory, "clone.yaml")
	raw := `apiVersion: account.btp.sap.crossplane.io/v1alpha1
kind: Subaccount
metadata:
  name: source
  annotations:
    crossplane.io/external-name: source-guid
    keep: annotation
spec:
  managementPolicies:
    - Observe
  forProvider:
    displayName: Source name
    subdomain: source-subdomain
    subaccountAdmins:
      - source-admin@example.test
---
apiVersion: account.btp.sap.crossplane.io/v1alpha1
kind: Entitlement
metadata:
  name: entitlement
  annotations:
    crossplane.io/external-name: entitlement-id
spec:
  managementPolicies:
    - Observe
  forProvider:
    amount: 10
    serviceName: cis
    servicePlanName: central
    subaccountId: source-guid
    subaccountIdRef:
      name: source
---
apiVersion: services.btp.sap.crossplane.io/v1alpha1
kind: ServiceBinding
metadata:
  name: binding
  annotations:
    crossplane.io/external-name: binding-id
spec:
  managementPolicies:
    - Observe
  forProvider:
    name: my—cis-binding
    serviceInstanceId: instance-id
    serviceInstanceRef:
      name: instance
---
apiVersion: environment.btp.sap.crossplane.io/v1alpha1
kind: CloudFoundryEnvironment
metadata:
  name: source-environment
spec:
  forProvider:
    environmentName: source-environment
    landscape: cf-eu12-001
    orgName: source-org
`
	if err := os.WriteFile(input, []byte(raw), 0o600); err != nil {
		t.Fatal(err)
	}

	var progress strings.Builder
	if err := transformWithProgress([]string{
		"--input", input,
		"--output", output,
		"--target-subdomain", "source-subdomain-build-123",
		"--technical-user-email", "technical@example.test",
	}, &progress); err != nil {
		t.Fatalf("transform() error = %v", err)
	}
	for _, expected := range []string{
		"clone subdomain",
		"technical user is a subaccount administrator",
		"removing adoption external names",
		"normalizing clone-specific BTP values: using the target subdomain for Cloud Foundry organization names, making CIS Central entitlements enable-only, and converting service-binding names to valid Terraform identifiers",
		"full management policies",
		"stale generated-reference IDs",
	} {
		if !strings.Contains(progress.String(), expected) {
			t.Errorf("transformation progress does not mention %q:\n%s", expected, progress.String())
		}
	}

	content, err := os.ReadFile(output)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(content), externalNameAnnotation) {
		t.Fatalf("clone manifest contains external name annotation:\n%s", content)
	}

	decoder := yaml.NewDecoder(strings.NewReader(string(content)))
	var documents []yaml.Node
	for {
		var document yaml.Node
		if err := decoder.Decode(&document); err != nil {
			if errors.Is(err, io.EOF) {
				break
			}
			t.Fatal(err)
		}
		documents = append(documents, document)
	}
	if len(documents) != 4 {
		t.Fatalf("got %d documents, want 4", len(documents))
	}
	if !strings.HasPrefix(string(content), "---\n") {
		t.Errorf("clone manifest must begin with a YAML document-start separator:\n%s", content)
	}
	if got := strings.Count(string(content), "\n...\n"); got != len(documents) {
		t.Errorf("clone manifest has %d YAML document-end separators, want %d:\n%s", got, len(documents), content)
	}

	subaccount := documents[0].Content[0]
	forProvider, ok := nestedMapping(subaccount, "spec", "forProvider")
	if !ok {
		t.Fatal("Subaccount has no spec.forProvider")
	}
	if got, _ := stringValue(forProvider, "subdomain"); got != "source-subdomain-build-123" {
		t.Errorf("subdomain = %q", got)
	}
	if got, _ := stringValue(forProvider, "displayName"); got != "source-subdomain-build-123" {
		t.Errorf("displayName = %q, want target subdomain", got)
	}
	admins, _ := mappingValue(forProvider, "subaccountAdmins")
	if !sequenceContains(admins, "technical@example.test") {
		t.Errorf("technical user not added to subaccount admins")
	}

	for _, document := range documents {
		resource := document.Content[0]
		if !isManagedBTPResource(resource) {
			continue
		}
		kind, _ := stringValue(resource, "kind")
		forProvider, _ := nestedMapping(resource, "spec", "forProvider")
		policies, _ := nestedValue(resource, "spec", "managementPolicies")
		if len(policies.Content) != 1 || policies.Content[0].Value != "*" {
			t.Errorf("%s policies = %#v, want [*]", kind, policies.Content)
		}
		if hasStaleIDWithReference(resource) {
			t.Errorf("%s retains a stale ID", kind)
		}
		switch kind {
		case "Entitlement":
			if _, hasAmount := mappingValue(forProvider, "amount"); hasAmount {
				t.Error("CIS central entitlement retains a numeric amount")
			}
			if enable, _ := stringValue(forProvider, "enable"); enable != "true" {
				t.Errorf("CIS central entitlement enable = %q, want true", enable)
			}
		case "ServiceBinding":
			if name, _ := stringValue(forProvider, "name"); name != "my-cis-binding" {
				t.Errorf("service binding name = %q, want ASCII Terraform identifier", name)
			}
			if _, found := mappingValue(forProvider, "serviceInstanceId"); found {
				t.Error("service binding retains the source serviceInstanceId")
			}
		case "CloudFoundryEnvironment":
			if orgName, _ := stringValue(forProvider, "orgName"); orgName != "source-subdomain-build-123" {
				t.Errorf("Cloud Foundry org name = %q, want target-specific org name", orgName)
			}
		}
	}
}

func TestRedactLoginOutput(t *testing.T) {
	user := technicalUser{Email: "user@example.invalid", Username: "p-user", Password: "very-secret"}
	output := redactLoginOutput(`login failed for user@example.invalid: password=very-secret --token abc authorization: "Bearer xyz"`, user, "global-account", "https://server.invalid")
	for _, sensitive := range []string{"user@example.invalid", "very-secret", "abc", "Bearer xyz"} {
		if strings.Contains(output, sensitive) {
			t.Errorf("diagnostic leaks %q: %s", sensitive, output)
		}
	}
}

func TestWriteExportConfigUsesExportAll(t *testing.T) {
	path := filepath.Join(t.TempDir(), "export.yaml")
	if err := writeExportConfig([]string{
		"--output", path,
		"--raw-output", ".work/raw.yaml",
	}); err != nil {
		t.Fatal(err)
	}
	content, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	var config struct {
		All               bool   `yaml:"all"`
		ResolveReferences bool   `yaml:"resolve-references"`
		Output            string `yaml:"output"`
	}
	if err := yaml.Unmarshal(content, &config); err != nil {
		t.Fatal(err)
	}
	if !config.All || !config.ResolveReferences || config.Output != ".work/raw.yaml" {
		t.Errorf("unexpected exporter config: %#v", config)
	}
	if strings.Contains(string(content), "subaccount:") {
		t.Errorf("exporter config must not contain the source subaccount selector:\n%s", content)
	}
}

func TestDeriveSubdomainKeepsBuildIDWithinLimit(t *testing.T) {
	target, err := deriveSubdomain(strings.Repeat("source", 12), "BUILD_123")
	if err != nil {
		t.Fatal(err)
	}
	if len(target) > 63 || !strings.HasSuffix(target, "-build-123") || !validSubdomain(target) {
		t.Errorf("invalid derived target %q", target)
	}
}
