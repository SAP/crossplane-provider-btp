# BTP Resource Exporter User Guide

The BTP Resource Exporter is a CLI tool that generates Crossplane-compatible resource manifests from SAP Business Technology Platform (BTP). It allows you to export existing BTP resources into YAML manifests that can be managed by Crossplane.

## Table of Contents

- [Overview](#overview)
- [Prerequisites](#prerequisites)
- [Environment Variables](#environment-variables)
- [Global Flags](#global-flags)
- [Supported Resource Kinds](#supported-resource-kinds)
  - [Subaccount](#subaccount)
  - [Entitlement](#entitlement)
  - [Service Instance](#service-instance)
  - [Cloud Foundry Environment](#cloud-foundry-environment)
- [Usage Examples](#usage-examples)
  - [Interactive Mode](#interactive-mode)
  - [Non-Interactive Mode](#non-interactive-mode)
  - [Exporting Multiple Kinds](#exporting-multiple-kinds)
  - [Output to File](#output-to-file)

## Overview

The exporter connects to your SAP BTP global account using the BTP CLI and retrieves resource information. It then converts this information into Crossplane resource manifests that can be applied to a Managed Control Plane cluster with the Crossplane BTP provider installed. The approach is based on the "[Import Existing Resources](https://docs.crossplane.io/v2.2/guides/import-existing-resources/)" Crossplane documentation.

**Key features:**
- Interactive and non-interactive modes
- Support for regex-based resource selection
- Inter-resource reference resolution
- Export to stdout or file

> [!TIP]
> While the exporter significantly accelerates the process of creating Crossplane manifests, the generated output should be reviewed before applying it to your cluster. Pay special attention to fields under `spec.forProvider`, as they define the desired state of your resources. Depending on your requirements, you may need to correct values or add additional configuration that the exporter could not infer from the source system.

## Prerequisites

- **BTP CLI**: The `btp` command-line tool must be installed and available in your `$PATH` (or specify a custom path via `--btp-cli` or `$BTP_EXPORT_BTP_CLI_PATH`)
- **BTP Global Account**: Access credentials for a SAP BTP global account
- **Go** (if running from source): Go 1.25 or later

## Environment Variables

The following environment variables can be used to configure the exporter:

| Variable | Description | Required |
|----------|-------------|----------|
| `BTP_EXPORT_USER_NAME` | User name to log in to a global account of SAP BTP | Yes* |
| `BTP_EXPORT_PASSWORD` | User password to log in to a global account of SAP BTP | Yes* |
| `BTP_EXPORT_GLOBAL_ACCOUNT` | The subdomain of the global account to export resources from | Yes* |
| `BTP_EXPORT_BTP_CLI_PATH` | Path to the BTP CLI binary. Default: `btp` in your `$PATH` | No |
| `BTP_EXPORT_BTP_CLI_SERVER_URL` | The URL of the BTP CLI server. Default: `https://cli.btp.cloud.sap` | No |
| `BTP_EXPORT_IDP` | Origin of the custom identity provider, if configured for the global account | No |
| `RESOLVE_REFERENCES` | Enable inter-resource reference resolution (true/false) | No |

*\* Required unless provided via command-line flags or in interactive mode*

## Global Flags

### Authentication & Connection

| Flag | Short | Description |
|------|-------|-------------|
| `--username <user>` | `-u` | User name to log in to a global account of SAP BTP |
| `--password <password>` | `-p` | User password to log in to a global account of SAP BTP |
| `--subdomain <subdomain>` | | The subdomain of the global account to export resources from |
| `--btp-cli <path>` | | Path to the BTP CLI binary. Default: `btp` in your `$PATH` |
| `--url <url>` | | The URL of the BTP CLI server. Default: `https://cli.btp.cloud.sap` |
| `--idp <idp>` | | Origin of the custom identity provider, if configured for the global account |

### Export Options

| Flag | Short | Description |
|------|-------|-------------|
| `--kind <kinds>` | | Comma-separated list of resource kinds to export |
| `--resolve-references` | `-r` | Resolve inter-resource references (use resource names instead of IDs) |
| `-o <file>` | | Output file path. If not specified, output is written to stdout |
| `--verbose` | | Enable verbose logging output |

## Supported Resource Kinds

### Subaccount

Exports BTP subaccount resources.

**Kind name:** `subaccount`

**CLI flag:** `--subaccount <value>`

**Selection criteria:**
- BTP subaccount ID (exact match)
- Regex expression matching subaccount display name

**Example:**
```bash
# Export a specific subaccount by ID
go run github.com/sap/crossplane-provider-btp/cmd/exporter export --kind subaccount --subaccount ec1cde20-1411-44b9-b092-9da6d7ebf99f

# Export all subaccounts matching a pattern
go run github.com/sap/crossplane-provider-btp/cmd/exporter export --kind subaccount --subaccount '.*dev.*'
```

---

### Entitlement

Exports BTP service plan entitlements assigned to subaccounts.

**Kind name:** `entitlement`

**CLI flags:**
| Flag | Description |
|------|-------------|
| `--entitlement <value>` | Service plan name or regex expression to match |
| `--entitlement-auto-assigned` | Include service plans that are automatically assigned to all subaccounts |

**Selection criteria:**
- Service plan name (exact match or regex)
- Requires subaccount selection first

**Example:**
```bash
# Export entitlements matching a pattern from all subaccounts
go run github.com/sap/crossplane-provider-btp/cmd/exporter export --kind entitlement --subaccount '.*' --entitlement '.*postgre.*'

# Include auto-assigned entitlements
go run github.com/sap/crossplane-provider-btp/cmd/exporter export --kind entitlement --entitlement-auto-assigned
```

---

### Service Instance

Exports BTP service instances.

**Kind name:** `serviceinstance`

**CLI flag:** `--serviceinstance <value>`

**Selection criteria:**
- Service instance ID (exact match)
- Regex expression matching service instance name

**Notes:**
- When exporting service instances, the exporter automatically exports prerequisite resources such as Service Manager instances
- Special handling is applied for Cloud Management and Service Manager instances

**Example:**
```bash
# Export service instances interactively with reference resolution
go run github.com/sap/crossplane-provider-btp/cmd/exporter export --kind serviceinstance --resolve-references

# Export service instances matching a pattern
go run github.com/sap/crossplane-provider-btp/cmd/exporter export --kind serviceinstance --serviceinstance '.*-prod-.*'
```

---

### Cloud Foundry Environment

Exports Cloud Foundry environment instances from BTP subaccounts.

**Kind name:** `cloudfoundry-environment`

**CLI flag:** `--cloudfoundry-environment <value>`

**Selection criteria:**
- CF environment ID (exact match)
- Regex expression matching CF environment name

**Notes:**
- Requires subaccount selection first
- When exported, prerequisite Cloud Management resources are automatically included

**Example:**
```bash
# Export Cloud Foundry environments
go run github.com/sap/crossplane-provider-btp/cmd/exporter export --kind cloudfoundry-environment

# Export CF environments from a specific subaccount
go run github.com/sap/crossplane-provider-btp/cmd/exporter export --kind cloudfoundry-environment --subaccount ec1cde20-1411-44b9-b092-9da6d7ebf99f
```

## Usage Examples

### Interactive Mode

In interactive mode, the exporter prompts you to select resources from available options. This is useful for exploring available resources.

```bash
# Export subaccounts interactively (prompts for selection)
go run github.com/sap/crossplane-provider-btp/cmd/exporter --verbose export --kind subaccount

# Export entitlements interactively
go run github.com/sap/crossplane-provider-btp/cmd/exporter --verbose export --kind entitlement

# Export service instances interactively with reference resolution
go run github.com/sap/crossplane-provider-btp/cmd/exporter --verbose export --kind serviceinstance --resolve-references
```

### Non-Interactive Mode

In non-interactive mode, you specify selection criteria via command-line flags or regex patterns.

```bash
# Export a specific subaccount by ID
go run github.com/sap/crossplane-provider-btp/cmd/exporter --verbose export \
  --kind subaccount \
  --subaccount ec1cde20-1411-44b9-b092-9da6d7ebf99f

# Export all entitlements matching a pattern from all subaccounts
go run github.com/sap/crossplane-provider-btp/cmd/exporter --verbose export \
  --kind entitlement \
  --subaccount '.*' \
  --entitlement '.*postgre.*' \
  --resolve-references
```

### Exporting Multiple Kinds

You can export multiple resource kinds in a single command by specifying a comma-separated list.

```bash
# Export subaccounts and Cloud Foundry environments together
go run github.com/sap/crossplane-provider-btp/cmd/exporter --verbose export \
  --kind subaccount,cloudfoundry-environment \
  --resolve-references \
  -o demo-cf-env.yaml
```

### Output to File

Use the `-o` flag to write the generated manifests to a file instead of stdout.

```bash
# Export service instances to a file
go run github.com/sap/crossplane-provider-btp/cmd/exporter --verbose export \
  --kind serviceinstance \
  --resolve-references \
  -o demo-si.yaml

# Export multiple kinds to a single file
go run github.com/sap/crossplane-provider-btp/cmd/exporter --verbose export \
  --kind subaccount,entitlement,serviceinstance \
  --resolve-references \
  -o full-export.yaml
```

## Reference Resolution

When the `--resolve-references` (or `-r`) flag is enabled, the exporter generates manifests that use Kubernetes resource references instead of hardcoded IDs. This is recommended for:

- Maintaining relationships between resources
- Enabling Crossplane to manage resource dependencies
- Making manifests more portable and readable

**Without reference resolution:**
```yaml
spec:
  forProvider:
    subaccountId: ec1cde20-1411-44b9-b092-9da6d7ebf99f
```

**With reference resolution:**
```yaml
spec:
  forProvider:
    subaccountIdRef:
      name: my-subaccount
```

## Tips and Best Practices

1. **Start with verbose mode**: Use `--verbose` to see detailed logging and understand what the exporter is doing.

2. **Use interactive mode first**: When exploring a new BTP account, start with interactive mode to discover available resources.

3. **Enable reference resolution**: Use `--resolve-references` for production exports to maintain proper resource relationships.

4. **Export in stages**: For complex environments, export resources kind by kind to review and customize each manifest.

5. **Use regex patterns carefully**: Regex patterns are applied to resource display names. Test your patterns to ensure they match expected resources.

## Troubleshooting

### Authentication Issues

If you encounter login failures:
- Verify your username and password
- Check that the global account subdomain is correct
- If using a custom IDP, ensure the `--idp` flag is set correctly

### Missing BTP CLI

If the exporter cannot find the BTP CLI:
- Install the BTP CLI from [SAP Development Tools](https://tools.hana.ondemand.com/#cloud)
- Add the BTP CLI to your `$PATH`
- Or specify the path explicitly with `--btp-cli /path/to/btp`

### No Resources Found

If no resources are returned:
- Verify you have access to the specified subaccounts
- Check that your regex patterns are correct
- Ensure the resource kind exists in your BTP account

