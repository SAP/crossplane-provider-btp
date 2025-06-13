package importer

import (
	"context"
	"fmt"

	"github.com/google/uuid"
	"github.com/sap/crossplane-provider-btp/internal/crossplaneimport/config"
	"github.com/sap/crossplane-provider-btp/internal/crossplaneimport/resource"
	"k8s.io/apimachinery/pkg/runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

// Importer is the main struct for importing BTP resources based on configuration.
// It orchestrates the import process by using registered adapters to fetch BTP resources
// and convert them to Crossplane managed resources.
type Importer struct {
	BTPClient resource.BTPClientInterface
	K8sClient client.Client // from sigs.k8s.io/controller-runtime/pkg/client
	// Logger could be added here if needed
}

// NewImporter creates a new Importer instance with the provided clients.
//
// Parameters:
//   - btpClient: Interface to interact with BTP APIs
//   - k8sClient: Kubernetes client for creating Crossplane resources
//
// Returns:
//   - A new Importer instance
func NewImporter(btpClient resource.BTPClientInterface, k8sClient client.Client) *Importer {
	return &Importer{
		BTPClient: btpClient,
		K8sClient: k8sClient,
	}
}

// RunImportProcess orchestrates the import of BTP resources based on the provided configuration.
// It iterates through the configured resources, fetches them from BTP using the appropriate adapters,
// and either previews or creates the corresponding Crossplane resources.
//
// Parameters:
//   - ctx: Context for the operation
//   - cfg: Import configuration containing resource definitions and settings
//   - previewOnly: If true, only generates previews without creating resources
//
// Returns:
//   - An error if any critical failures occurred, or nil on success
func (i *Importer) RunImportProcess(ctx context.Context, cfg *config.ImportConfig, previewOnly bool) error {
	if cfg == nil {
		return fmt.Errorf("import configuration cannot be nil")
	}
	if i.BTPClient == nil {
		return fmt.Errorf("BTP client is not initialized in Importer")
	}
	if !previewOnly && i.K8sClient == nil {
		return fmt.Errorf("Kubernetes client is not initialized in Importer for non-preview mode")
	}

	// Generate a unique transaction ID for this import run
	transactionID := uuid.New().String()

	var encounteredErrors []error

	fmt.Printf("Starting import process. Transaction ID: %s, ProviderConfigRef: %s, Default ManagementPolicy: %s\n", transactionID, cfg.ProviderConfigRefName, cfg.ManagementPolicy)

	for _, resConfig := range cfg.Resources {
		fmt.Printf("Processing resource type: %s (Name: %s)\n", resConfig.Type, resConfig.Name)
		adapter, found := GetAdapter(resConfig.Type) // GetAdapter is from registry.go
		if !found {
			err := fmt.Errorf("no adapter found for resource type '%s' configured under name '%s'", resConfig.Type, resConfig.Name)
			fmt.Println(err.Error())
			encounteredErrors = append(encounteredErrors, err)
			continue
		}

		effectiveManagementPolicy := cfg.ManagementPolicy
		if resConfig.ManagementPolicy != "" {
			effectiveManagementPolicy = resConfig.ManagementPolicy
		}
		fmt.Printf("  Using management policy: %s\n", effectiveManagementPolicy)

		// Type assertion for filters if ResourceFilterConfig is an interface.
		// For now, assuming it's map[string]interface{} and adapters handle it.
		// If ResourceFilterConfig is a concrete type, direct pass is fine.
		// The plan suggests ResourceFilterConfig is an interface with GetCriteria(),
		// so the adapter will call that.
		fmt.Printf("  Fetching BTP resources with filters: %+v\n", resConfig.Filters)
		btpResourceRepresentations, err := adapter.FetchBTPResources(ctx, i.BTPClient, resConfig.Filters)
		if err != nil {
			err = fmt.Errorf("failed to fetch BTP resources for type '%s' (Name: %s): %w", resConfig.Type, resConfig.Name, err)
			fmt.Println(err.Error())
			encounteredErrors = append(encounteredErrors, err)
			continue
		}
		fmt.Printf("  Fetched %d BTP resource(s) of type '%s'\n", len(btpResourceRepresentations), resConfig.Type)

		for idx, btpRepresentation := range btpResourceRepresentations {
			fmt.Printf("    Processing BTP resource #%d\n", idx+1)
			if previewOnly {
				previewOutput, err := adapter.PreviewResource(ctx, btpRepresentation, cfg.ProviderConfigRefName, effectiveManagementPolicy, transactionID)
				if err != nil {
					err = fmt.Errorf("failed to generate preview for BTP resource #%d of type '%s': %w", idx+1, resConfig.Type, err)
					fmt.Println(err.Error())
					encounteredErrors = append(encounteredErrors, err)
					continue
				}
				fmt.Printf("      [PREVIEW] %s\n", previewOutput)
			} else {
				crossplaneResource, err := adapter.ConvertToCrossplaneResource(ctx, btpRepresentation, cfg.ProviderConfigRefName, effectiveManagementPolicy, transactionID)
				if err != nil {
					err = fmt.Errorf("failed to convert BTP resource #%d of type '%s' to Crossplane resource: %w", idx+1, resConfig.Type, err)
					fmt.Println(err.Error())
					encounteredErrors = append(encounteredErrors, err)
					continue
				}

				fmt.Printf("      Attempting to create Crossplane resource: %s/%s\n", crossplaneResource.GetNamespace(), crossplaneResource.GetName())
				if err := i.K8sClient.Create(ctx, crossplaneResource); err != nil {
					// TODO: Handle 'AlreadyExists' more gracefully if needed (e.g. update or skip with a flag)
					err = fmt.Errorf("failed to create Crossplane resource %s/%s for BTP resource #%d of type '%s': %w",
						crossplaneResource.GetNamespace(), crossplaneResource.GetName(), idx+1, resConfig.Type, err)
					fmt.Println(err.Error())
					encounteredErrors = append(encounteredErrors, err)
					continue
				}
				fmt.Printf("      Successfully created Crossplane resource: %s/%s\n", crossplaneResource.GetNamespace(), crossplaneResource.GetName())
				// add to state
				cfg.AddImported(resConfig.Type, crossplaneResource.GetName())
			}
		}
	}

	if len(encounteredErrors) > 0 {
		return fmt.Errorf("import process completed with %d error(s)", len(encounteredErrors))
	}
	fmt.Println("Import process completed successfully.")
	return nil
}

// ImportResources is a CLI-compatible method that imports BTP resources from a configuration file.
// This method bridges the gap between the CLI expectations and the current importer structure.
//
// Parameters:
//   - ctx: Context for the operation
//   - configPath: Path to the configuration file
//   - kubeConfigPath: Path to the kubeconfig file
//   - scheme: Kubernetes scheme for resource creation
//
// Returns:
//   - A slice of imported resources and an error if any critical failures occurred
func (i *Importer) ImportResources(ctx context.Context, configPath string, kubeConfigPath string, scheme *runtime.Scheme) ([]interface{}, error) {
	// This is a placeholder implementation that would need to be completed
	// based on the actual CLI requirements and configuration parsing logic
	return nil, fmt.Errorf("ImportResources method not yet implemented - use RunImportProcess instead")
}

// PreviewResources displays a preview of the resources that would be created.
// This method is expected by the CLI but not yet implemented.
//
// Parameters:
//   - resources: Slice of resources to preview
func (i *Importer) PreviewResources(resources []interface{}) {
	fmt.Println("PreviewResources method not yet implemented")
}

// CreateResources creates the specified resources in the Kubernetes cluster.
// This method is expected by the CLI but not yet implemented.
//
// Parameters:
//   - ctx: Context for the operation
//   - resources: Slice of resources to create
//   - kubeConfigPath: Path to the kubeconfig file
//   - transactionID: Transaction ID for tracking
//
// Returns:
//   - An error if any critical failures occurred
func (i *Importer) CreateResources(ctx context.Context, resources []interface{}, kubeConfigPath string, transactionID string) error {
	return fmt.Errorf("CreateResources method not yet implemented - use RunImportProcess instead")
}
