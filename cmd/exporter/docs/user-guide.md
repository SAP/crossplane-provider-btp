# BTP Resource Exporter User Guide

The BTP Resource Exporter is a CLI tool that generates Crossplane-compatible resource manifests from SAP Business Technology Platform (BTP). It allows you to export existing BTP resources into YAML manifests that can be managed by Crossplane.

## Table of Contents

- [Overview](#overview)
- [Prerequisites](#prerequisites)
- [Environment Variables](#environment-variables)
- [Global Flags](#global-flags)
- [Commands](#commands)
  - [Completion Command](#completion-command)
  - [Login Command](#login-command)
  - [Export Command](#export-command)
- [Supported Resource Kinds](#supported-resource-kinds)
  - [Subaccount](#subaccount)
  - [Entitlement](#entitlement)
  - [Service Instance](#service-instance)
  - [Cloud Foundry Environment](#cloud-foundry-environment)
- [Usage Examples](#usage-examples)
  - [Building the Exporter](#building-the-exporter)
  - [Interactive Mode](#interactive-mode)
  - [Non-Interactive Mode](#non-interactive-mode)
  - [Exporting Multiple Kinds](#exporting-multiple-kinds)
  - [Output to File](#output-to-file)
- [Reference Resolution](#reference-resolution)
- [Tips and Best Practices](#tips-and-best-practices)
- [Troubleshooting](#troubleshooting)
  - [Authentication Issues](#authentication-issues)
  - [Missing BTP CLI](#missing-btp-cli)
  - [No Resources Found](#no-resources-found)

## Overview

The exporter connects to your SAP BTP global account using the BTP CLI and retrieves resource information. It then converts this information into Crossplane resource manifests that can be applied to a Managed Control Plane cluster with the Crossplane BTP provider installed. The approach is based on the "[Import Existing Resources](https://docs.crossplane.io/v2.2/guides/import-existing-resources/)" Crossplane documentation.

**Key features:**
- Interactive and non-interactive modes
- Support for regex-based resource selection
- Inter-resource reference resolution
- Export to stdout or file

> [!TIP]
> While the exporter significantly accelerates the process of creating Crossplane manifests, **the generated output should be carefully reviewed before changing the management policy from `Observe` to something more permissive**. Pay special attention to fields under `spec.forProvider`, as they define the desired state of your resources. Depending on your requirements, to avoid faulty updates, you may need to correct values or add additional configuration that the exporter could not infer from the source system.

## Prerequisites

**Running the export tool**:

- **BTP CLI**: The `btp` command-line tool must be [installed](#missing-btp-cli) and available in your `$PATH` (or specify a custom path via `--btp-cli` or `$BTP_EXPORT_BTP_CLI_PATH`)
- **BTP Global Account**: Access credentials for a SAP BTP global account
- **Go**: Go 1.25 or later

> [!NOTE]
> Most command examples in this guide run the tool directly from source using `go run`. This is the recommended approach, as the tool is in an early stage of development and is being actively enhanced. Once the tool stabilizes, an officially released binary will be provided.

**Applying the generated manifests**:

- **Crossplane BTP Provider**: A Crossplane BTP provider must be installed in your cluster
- **ProviderConfig**: Ensure that a default `ProviderConfig` is configured. The exported manifests do not explicitly reference a specific `ProviderConfig`, so the default one will be used when applying the generated manifests.
- **Permissions**: Ensure that the user configured in the `ProviderConfig` has the necessary permissions in the affected BTP subaccounts 

> [!NOTE]
> A remark on namespaces: The exporter does not assign namespaces to the generated manifests. But, some resources require an explicit namespace for `WriteConnectionSecretToReference`, which is currently set to "*default*". You may need to adjust this value before applying the manifests.

## Environment Variables

The following environment variables apply to all commands:

| Variable | Description | Required |
|----------|-------------|----------|
| `BTP_EXPORT_BTP_CLI_PATH` | Path to the BTP CLI binary. Default: `btp` in your `$PATH` | No |

## Global Flags

The following flags are available for all commands:

| Flag | Short | Description |
|------|-------|-------------|
| `--btp-cli <path>` | | Path to the BTP CLI binary. Default: `btp` in your `$PATH` |
| `--config <file>` | `-c` | Configuration file |
| `--help` | `-h` | Help for btp-exporter |
| `--verbose` | | Enable verbose logging output |

## Commands

### Completion Command

The `completion` command generates shell autocompletion scripts for btp-exporter for the specified shell.

> [!NOTE]
> See each subcommand's help for detailed instructions on how to use the generated script for your specific shell (e.g., `btp-exporter completion zsh --help`).

> [!NOTE]
> The completion scripts only work with binaries. The name of the binary must be "btp-exporter".

**Usage:**
```bash
btp-exporter completion [shell]
```

#### Available Subcommands

| Subcommand | Description |
|------------|-------------|
| `bash` | Generate the autocompletion script for bash |
| `fish` | Generate the autocompletion script for fish |
| `powershell` | Generate the autocompletion script for powershell |
| `zsh` | Generate the autocompletion script for zsh |

#### Completion Flags

| Flag | Short | Description |
|------|-------|-------------|
| `--help` | `-h` | Help for completion |

---

### Login Command

The `login` command authenticates against a SAP BTP global account. Authentication is required before running the `export` command.

**Usage:**
```bash
go run github.com/sap/crossplane-provider-btp/cmd/exporter login [flags]
```

#### Login Environment Variables

| Variable | Description | Required |
|----------|-------------|----------|
| `BTP_EXPORT_USER_NAME` | User name to log in to a global account of SAP BTP | Yes*     |
| `BTP_EXPORT_PASSWORD` | User password to log in to a global account of SAP BTP | Yes*     |
| `BTP_EXPORT_GLOBAL_ACCOUNT` | The subdomain of the global account to export resources from | Yes**    |
| `BTP_EXPORT_BTP_CLI_SERVER_URL` | The URL of the BTP CLI server. Default: `https://cli.btp.cloud.sap` | No       |
| `BTP_EXPORT_IDP` | Origin of the custom identity provider, if configured for the global account | No       |

*\* Required in non-interactive mode unless provided via command-line flags*

*\*\* Required unless provided via command-line flags. Same applies for use with `--sso`*

#### Login Flags

| Flag | Short | Description |
|------|-------|-------------|
| `--username <user>` | `-u` | User name to log in to a global account of SAP BTP, usually an e-mail address |
| `--password <password>` | `-p` | User password (see [BTP CLI documentation](https://help.sap.com/docs/btp/btp-cli-command-reference/btp-login) for recommendations) |
| `--subdomain <subdomain>` | | The subdomain of the global account to export resources from. Can be found in BTP Cockpit |
| `--url <url>` | | The URL of the BTP CLI server. Default: `https://cli.btp.cloud.sap` |
| `--idp <idp>` | | Origin of the custom identity provider, if configured for the global account |
| `--sso` | | Opens a browser for single sign-on |

#### Login Examples

```bash
# Login with username and password
go run github.com/sap/crossplane-provider-btp/cmd/exporter login --username user@example.com --password mypassword --subdomain my-global-account

# Login with SSO (opens browser)
go run github.com/sap/crossplane-provider-btp/cmd/exporter login --subdomain my-global-account --sso

# Login with custom identity provider
go run github.com/sap/crossplane-provider-btp/cmd/exporter login --username user@example.com --subdomain my-global-account --idp my-custom-idp

# Login interactively (prompts for credentials)
go run github.com/sap/crossplane-provider-btp/cmd/exporter login
```

> [!NOTE]
> The login session is managed by the BTP CLI. Once authenticated, subsequent `export` commands will use the existing session until it expires or you log out.

---

### Export Command

The `export` command generates Crossplane-compatible resource manifests from the connected BTP global account.

**Usage:**
```bash
go run github.com/sap/crossplane-provider-btp/cmd/exporter export [flags]
```

#### Export Environment Variables

| Variable | Description | Required |
|----------|-------------|----------|
| `RESOLVE_REFERENCES` | Enable inter-resource reference resolution (true/false) | No |

#### Export Flags

| Flag | Short | Description |
|------|-------|-------------|
| `--kind <kinds>` | | Comma-separated list of resource kinds to export |
| `--resolve-references` | `-r` | Resolve inter-resource references (use resource names instead of IDs) |
| `-o <file>` | | Output file path. If not specified, output is written to stdout |

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
- Service plan name pattern (regex)

**Example:**
```bash
# Export entitlements matching a pattern from all subaccounts
go run github.com/sap/crossplane-provider-btp/cmd/exporter export --kind entitlement --subaccount '.*' --entitlement '.*postgre.*'

# Interactive selection, includes auto-assigned entitlements
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
- If a service instance is an instance of Cloud Management or Service Manager, it will be exported as a `CloudManagement` or `ServiceManager` resource respectively

**Example:**
```bash
# Export service instances interactively with reference resolution
go run github.com/sap/crossplane-provider-btp/cmd/exporter export --kind serviceinstance --resolve-references

# Export service instances matching a pattern
go run github.com/sap/crossplane-provider-btp/cmd/exporter export --kind serviceinstance --serviceinstance '.*-destination-.*'
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
- When exported, prerequisite resources (Cloud Management etc.) are automatically included

**Example:**
```bash
# Export Cloud Foundry environments
go run github.com/sap/crossplane-provider-btp/cmd/exporter export --kind cloudfoundry-environment
```

## Usage Examples

### Building the Exporter

To build the exporter binary locally, you can use one of the following methods:

**Using Go directly:**
```bash
# Build the exporter binary
go build -o btp-exporter github.com/sap/crossplane-provider-btp/cmd/exporter
```

**Using Make:**
```bash
# Build all project binaries (including the exporter)
make go.build

# The exporter binary will be located at:
# _output/bin/<platform>/exporter
# 
# Rename and move to your desired location:
mv _output/bin/$(go env GOOS)_$(go env GOARCH)/exporter ./btp-exporter
```

After building, you can run the exporter using:
```bash
./btp-exporter --help
```

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
  -o output.yaml
```

### Output to File

Use the `-o` flag to write the generated manifests to a file instead of stdout.

```bash
# Export service instances to a file
go run github.com/sap/crossplane-provider-btp/cmd/exporter --verbose export \
  --kind serviceinstance \
  --resolve-references \
  -o output.yaml

# Export multiple kinds to a single file
go run github.com/sap/crossplane-provider-btp/cmd/exporter --verbose export \
  --kind subaccount,entitlement,serviceinstance \
  --resolve-references \
  -o output.yaml
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

4. **Export in stages**: For complex environments, export resources subaccount by subaccount and kind by kind to review and customize each manifest.

5. **Use regex patterns carefully**: Regex patterns are applied to resource display names. Test your patterns to ensure they match expected resources.

## Troubleshooting

### Authentication Issues

If you encounter unclear failures with the `export` command:
- Access BTP with BTP CLI and check, if there is an active session
- Log in with the `login` command
- Check if the user has necessary permissions in affected subaccounts, Cloud Foundry organizations etc.

If you encounter failures with the `login` command:
- Check the values of the respective environment variables or command-line flags
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
