// Package config provides configuration parsing and validation functionality
// for the crossplane import system.
//
// This package handles loading and validating CLI configuration files,
// including dynamic filter validation using adapter schemas.
package config

import (
	"fmt"
	"os"

	"gopkg.in/yaml.v3"

	"github.com/sap/crossplane-provider-btp/internal/crossplaneimport/resource"
)

// AdapterRegistry defines the interface for retrieving resource adapters.
// This interface is used to avoid import cycles between config and importer packages.
type AdapterRegistry interface {
	GetAdapter(resourceTypeName string) (resource.BTPResourceAdapter, bool)
}

// BTPConfigParser implements configuration parsing and validation for BTP resources.
// It loads configuration from YAML files and validates resource-specific filters
// against schemas provided by registered BTPResourceAdapters.
type BTPConfigParser struct {
	registry AdapterRegistry
}

// NewBTPConfigParser creates a new BTPConfigParser with the provided adapter registry.
//
// Parameters:
//   - registry: The adapter registry for retrieving resource adapters
//
// Returns:
//   - A new BTPConfigParser instance
func NewBTPConfigParser(registry AdapterRegistry) *BTPConfigParser {
	return &BTPConfigParser{
		registry: registry,
	}
}

// rawImportConfig is used for initial YAML unmarshaling before validation.
// It uses map[string]interface{} for filters to capture the raw YAML data.
type rawImportConfig struct {
	ProviderConfigRefName string                  `yaml:"providerConfigRefName"`
	ManagementPolicy      string                  `yaml:"managementPolicy"`
	Resources             []rawConfiguredResource `yaml:"resources"`
	Tooling               []SubaccountTooling     `yaml:"tooling,omitempty"` //TODO: probably add more elegant parsing here as for the others
}

// rawConfiguredResource represents a resource configuration with raw filter data.
type rawConfiguredResource struct {
	Type             string                 `yaml:"type"`
	Name             string                 `yaml:"name,omitempty"`
	ManagementPolicy string                 `yaml:"managementPolicy,omitempty"`
	Filters          map[string]interface{} `yaml:"filters"`
}

// LoadAndValidateCLIConfig loads the CLI configuration from the given file path,
// unmarshals it into ImportConfig, and validates resource-specific filters
// against schemas provided by registered BTPResourceAdapters.
//
// The function performs the following operations:
// 1. Reads and parses the YAML configuration file
// 2. Validates global configuration settings
// 3. Iterates through each resource configuration
// 4. Retrieves the corresponding adapter for each resource type
// 5. Validates filters against the adapter's schema
// 6. Returns comprehensive error messages for any validation failures
//
// Parameters:
//   - filePath: Path to the configuration YAML file
//
// Returns:
//   - *ImportConfig: The parsed and validated configuration
//   - error: Any parsing or validation errors encountered
//
// Example usage:
//
//	parser := NewBTPConfigParser(registry)
//	config, err := parser.LoadAndValidateCLIConfig("config.yaml")
//	if err != nil {
//	    log.Fatalf("Configuration validation failed: %v", err)
//	}
func (p *BTPConfigParser) LoadAndValidateCLIConfig(filePath string) (*ImportConfig, error) {
	if filePath == "" {
		return nil, fmt.Errorf("configuration file path cannot be empty")
	}

	if p.registry == nil {
		return nil, fmt.Errorf("adapter registry is not initialized")
	}

	yamlFile, err := os.ReadFile(filePath)
	if err != nil {
		return nil, fmt.Errorf("failed to read config file '%s': %w", filePath, err)
	}

	// First, unmarshal into raw structure to get the filter data as map[string]interface{}
	var rawCfg rawImportConfig
	err = yaml.Unmarshal(yamlFile, &rawCfg)
	if err != nil {
		return nil, fmt.Errorf("failed to unmarshal config file '%s': %w", filePath, err)
	}

	// Validate global configuration
	if rawCfg.ProviderConfigRefName == "" {
		return nil, fmt.Errorf("global 'providerConfigRefName' must be set in the config file")
	}
	// ManagementPolicy can be empty, to be resolved by importer or adapter

	// Create the final ImportConfig structure
	cfg := &ImportConfig{
		ProviderConfigRefName: rawCfg.ProviderConfigRefName,
		ManagementPolicy:      rawCfg.ManagementPolicy,
		Resources:             make([]ConfiguredResource, len(rawCfg.Resources)),
	}

	// Validate each resource configuration and convert to final structure
	for i, rawResConfig := range rawCfg.Resources {
		if rawResConfig.Type == "" {
			return nil, fmt.Errorf("resource entry at index %d (Name: '%s') is missing 'type'", i, rawResConfig.Name)
		}

		// Get the adapter for this resource type
		adapter, found := p.registry.GetAdapter(rawResConfig.Type)
		if !found {
			return nil, fmt.Errorf("no adapter registered for resource type '%s' (Name: '%s')", rawResConfig.Type, rawResConfig.Name)
		}

		// Get the filter schema from the adapter
		schema := adapter.GetFilterSchema()

		// Validate filters against schema if schema is defined
		if schema.Fields != nil && len(schema.Fields) > 0 && rawResConfig.Filters != nil {
			validationErrors := validateFiltersAgainstSchema(rawResConfig.Filters, schema, rawResConfig.Type, rawResConfig.Name)
			if len(validationErrors) > 0 {
				// Aggregate errors for better reporting
				var errorMessages string
				for _, valErr := range validationErrors {
					errorMessages += fmt.Sprintf("\n  - %s", valErr.Error())
				}
				return nil, fmt.Errorf("filter validation failed for resource type '%s' (Name: '%s'):%s", rawResConfig.Type, rawResConfig.Name, errorMessages)
			}
		}

		// Create a simple ResourceFilterConfig implementation that holds the raw filter data
		var filterConfig resource.ResourceFilterConfig
		if rawResConfig.Filters != nil {
			filterConfig = &SimpleResourceFilterConfig{Filters: rawResConfig.Filters}
		}

		// Build the final ConfiguredResource
		cfg.Resources[i] = ConfiguredResource{
			Type:             rawResConfig.Type,
			Name:             rawResConfig.Name,
			ManagementPolicy: rawResConfig.ManagementPolicy,
			Filters:          filterConfig,
		}

		cfg.Tooling = rawCfg.Tooling
	}

	return cfg, nil
}

func (p *BTPConfigParser) WriteCLIConfig(filePath string, config *ImportConfig) error {
	if filePath == "" {
		return fmt.Errorf("configuration file path cannot be empty")
	}

	if config == nil {
		return fmt.Errorf("configuration cannot be nil")
	}

	rawCfg := importConfigToRawImportConfig(config)

	yamlData, err := yaml.Marshal(rawCfg)
	if err != nil {
		return fmt.Errorf("failed to marshal configuration to YAML: %w", err)
	}

	err = os.WriteFile(filePath, yamlData, 0644)
	if err != nil {
		return fmt.Errorf("failed to write configuration file '%s': %w", filePath, err)
	}

	return nil
}

// SimpleResourceFilterConfig is a basic implementation of ResourceFilterConfig
// that wraps the raw filter data from YAML unmarshaling.
type SimpleResourceFilterConfig struct {
	Filters map[string]interface{}
}

// GetCriteria converts the filter data to a map[string]string as expected by the interface.
// This performs basic string conversion for the filter values.
func (s *SimpleResourceFilterConfig) GetCriteria() map[string]string {
	if s.Filters == nil {
		return nil
	}

	criteria := make(map[string]string)
	for k, v := range s.Filters {
		criteria[k] = fmt.Sprintf("%v", v)
	}
	return criteria
}

// validateFiltersAgainstSchema validates the provided filters map against the adapter's schema.
// It performs the following validations:
// 1. Checks for required fields defined in the schema
// 2. Validates basic type compatibility (string, integer, boolean)
// 3. Reports unknown/extraneous filter fields
// 4. Collects and returns all validation errors
//
// Parameters:
//   - filters: The filter configuration as a map[string]interface{}
//   - schema: The filter schema definition from the adapter
//   - resType: Resource type name for error reporting
//   - resName: Resource name for error reporting
//
// Returns:
//   - []error: A slice of validation errors, empty if validation passes
func validateFiltersAgainstSchema(filters map[string]interface{}, schema resource.FilterSchemaDefinition, resType, resName string) []error {
	var errors []error

	// Check for required fields
	for fieldName, fieldDef := range schema.Fields {
		if fieldDef.Required {
			if _, exists := filters[fieldName]; !exists {
				errors = append(errors, fmt.Errorf("required filter field '%s' is missing", fieldName))
			}
		}
	}

	// Check for unknown fields and validate basic types
	for filterKey, filterValue := range filters {
		fieldDef, exists := schema.Fields[filterKey]
		if !exists {
			errors = append(errors, fmt.Errorf("unknown filter field '%s'", filterKey))
			continue
		}

		// Basic type checking - can be expanded for more sophisticated validation
		switch fieldDef.Type {
		case "string":
			if _, ok := filterValue.(string); !ok {
				errors = append(errors, fmt.Errorf("filter field '%s' expected type string, got %T", filterKey, filterValue))
			}
		case "integer":
			// YAML might parse integers as int, int64, or float64 depending on the value
			switch v := filterValue.(type) {
			case int, int32, int64:
				// Valid integer types
			case float64:
				// Check if it's actually an integer value
				if v != float64(int64(v)) {
					errors = append(errors, fmt.Errorf("filter field '%s' expected type integer, got float value %v", filterKey, v))
				}
			default:
				errors = append(errors, fmt.Errorf("filter field '%s' expected type integer, got %T", filterKey, filterValue))
			}
		case "boolean":
			if _, ok := filterValue.(bool); !ok {
				errors = append(errors, fmt.Errorf("filter field '%s' expected type boolean, got %T", filterKey, filterValue))
			}
		case "array":
			// Check if it's a slice or array
			switch filterValue.(type) {
			case []interface{}, []string, []int, []bool:
				// Valid array types
			default:
				errors = append(errors, fmt.Errorf("filter field '%s' expected type array, got %T", filterKey, filterValue))
			}
		default:
			// Unknown type in schema - this is a schema definition issue, not a user config issue
			errors = append(errors, fmt.Errorf("filter field '%s' has unknown type '%s' in schema definition", filterKey, fieldDef.Type))
		}
	}

	return errors
}

func importConfigToRawImportConfig(cfg *ImportConfig) rawImportConfig {
	var rawCfg rawImportConfig = rawImportConfig{
		ProviderConfigRefName: cfg.ProviderConfigRefName,
		ManagementPolicy:      cfg.ManagementPolicy,
		Resources:             make([]rawConfiguredResource, len(cfg.Resources)),
		Tooling:               cfg.Tooling,
	}
	for i, res := range cfg.Resources {
		rawCfg.Resources[i] = rawConfiguredResource{
			Type:             res.Type,
			Name:             res.Name,
			ManagementPolicy: res.ManagementPolicy,
			Filters:          make(map[string]interface{}),
		}
		if res.Filters != nil {
			// Convert ResourceFilterConfig to map[string]interface{}
			criteria := res.Filters.GetCriteria()
			filters := make(map[string]interface{}, len(criteria))
			for k, v := range criteria {
				filters[k] = v
			}
			rawCfg.Resources[i].Filters = filters
		}
	}
	return rawCfg
}

// ParseConfig provides a simplified interface for parsing configuration files.
// This method maintains compatibility with existing interfaces while leveraging
// the new validation logic.
//
// Parameters:
//   - configPath: Path to the configuration file
//
// Returns:
//   - *ImportConfig: The parsed configuration
//   - error: Any parsing or validation errors
func (p *BTPConfigParser) ParseConfig(configPath string) (*ImportConfig, error) {
	return p.LoadAndValidateCLIConfig(configPath)
}

// LoadAndValidateCLIConfigWithRegistry is a convenience function that creates a parser
// with the provided registry and loads the configuration.
//
// Parameters:
//   - filePath: Path to the configuration file
//   - registry: The adapter registry for retrieving resource adapters
//
// Returns:
//   - *ImportConfig: The parsed configuration
//   - error: Any parsing or validation errors
func LoadAndValidateCLIConfigWithRegistry(filePath string, registry AdapterRegistry) (*ImportConfig, error) {
	parser := NewBTPConfigParser(registry)
	return parser.LoadAndValidateCLIConfig(filePath)
}

func SaveCLIConfig(filePath string, config *ImportConfig) error {
	parser := NewBTPConfigParser(nil)
	return parser.WriteCLIConfig(filePath, config)
}
