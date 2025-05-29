# xpbtp CLI Tool

## Overview

`xpbtp` is a command-line interface tool designed to import pre-existing SAP BTP resources (Subaccounts, Directories, and Entitlements) into a Kubernetes cluster managed by Crossplane. The tool reads a local configuration file to determine which BTP resources to target and then generates the corresponding Crossplane manifest files.

The CLI simplifies the process of bringing existing BTP infrastructure under Crossplane management by automatically discovering resources based on your filtering criteria and creating the appropriate Kubernetes custom resources.

## Prerequisites

- `kubectl` installed and configured to access your Kubernetes cluster where Crossplane and the `crossplane-provider-btp` are running.
- A `ProviderConfig` custom resource for `crossplane-provider-btp` must be deployed and correctly configured in your Kubernetes cluster. This `ProviderConfig` contains the necessary credentials for the tool to communicate with your BTP global account.
- The `xpbtp` CLI binary (built from the `internal/cli` directory, e.g., `go build -o xpbtp`).

## Configuration

### 1. Identifying Your ProviderConfig

The CLI needs to know which `ProviderConfig` to use. You can list your available `ProviderConfig` resources with:

```bash
kubectl get providerconfigs.btp.sap.crossplane.io -A
```

Note the `NAME` of your target `ProviderConfig`. Since `ProviderConfig` is a cluster-scoped resource, it does not have a namespace.

### 2. CLI Configuration File (`config.yaml`)

`xpbtp` uses a YAML configuration file (default name: `config.yaml` in the same directory as the `xpbtp` executable, or specified with the `--config` flag) to define which BTP resources to import. This file contains the `providerConfigRef.name` which specifies the name of your cluster-scoped `ProviderConfig` Kubernetes resource, as well as filters for resources to import.

The configuration file structure supports filtering for three types of resources:

#### Subaccounts
- `displayName`: (String, Primary filter) The display name of the subaccount. Supports regex patterns.
- `region`: (String, Optional) The region of the subaccount (e.g., "eu10", "us20").
- `subdomain`: (String, Optional) The subdomain of the subaccount.
- `description`: (String, Optional) The description of the subaccount.
- `managementPolicies`: (List of Strings, e.g., `[Observe]`) How Crossplane should manage the resource.

#### Directories
- `displayName`: (String, Primary filter) The display name of the directory. Supports regex patterns.
- `description`: (String, Optional) The description of the directory.
- `directoryFeatures`: (List of Strings, Optional) Features enabled for the directory.
- `managementPolicies`: (List of Strings) How Crossplane should manage the resource.

#### Entitlements
- `subaccountGuid`: (String, Required) The GUID of the subaccount to which the entitlement belongs.
- `serviceName`: (String, Required) The name of the entitled service (e.g., "alert-notification").
- `servicePlanName`: (String, Required) The name of the service plan (e.g., "standard", "free").
- `managementPolicies`: (List of Strings) How Crossplane should manage the resource.

**Important:** Fields other than `displayName` for subaccounts/directories, and the three required fields for entitlements, act as additional "AND" filters. All specified criteria must match for a resource to be imported.

#### ProviderConfig Reference

The configuration file must also include a `providerConfigRef` section that specifies which `ProviderConfig` resource to use:

```yaml
providerConfigRef:
  name: "your-provider-config-name"
```

This `providerConfigRef.name` field is used by the `init` command to identify the `ProviderConfig` CR in the Kubernetes cluster.

For a complete configuration example, see [`config-example.yaml`](config-example.yaml) in this directory.

## Usage

### 1. Initialize Environment (Optional but Recommended)

The `init` command creates a local environment file (`.xpbtp_env.yaml` by default) that stores the `ProviderConfig` details and kubeconfig path, so they don't have to be specified for every `import` command.

```bash
./xpbtp init --config <path-to-your-xpbtp-config.yaml> [--kubeconfig <path-to-your-kubeconfig>] [--env-file <path-to-env-file>]
```

**Flags:**
- `--config <path-to-your-xpbtp-config.yaml>`: Path to the `xpbtp` CLI's own configuration file (default: `./config.yaml`). This file contains the `providerConfigRef.name` which specifies the name of your cluster-scoped `ProviderConfig` Kubernetes resource, as well as filters for resources to import.
- `--kubeconfig <path-to-your-kubeconfig>`: Optional. Path to your Kubernetes kubeconfig file. If not provided, the CLI will attempt to use the default kubeconfig path (e.g., `~/.kube/config`) or the `KUBECONFIG` environment variable.
- `--env-file <path-to-env-file>`: Optional. Path where the fetched BTP credentials and environment settings will be stored (default: `./.xpbtp_env.yaml`).

**Example:**
```bash
./xpbtp init --config ./my-config.yaml --kubeconfig ~/.kube/config
```

### 2. Import Resources

The `import` command fetches BTP resource details based on your configuration and generates Crossplane manifest YAML files.

```bash
./xpbtp import [--config <path-to-config.yaml>] [--preview]
```

**Flags:**
- `--config`, `-c`: Path to the CLI's resource filter configuration file (defaults to `./config.yaml`)
- `--preview`, `-p`: Get a detailed overview of importable resources before creating them

**Examples:**
```bash
# Basic import using default config.yaml
./xpbtp import

# Import with preview to see what will be created
./xpbtp import --preview

# Import using a specific configuration file
./xpbtp import --config ./production-config.yaml
```

The tool will:
1. Discover BTP resources matching your filter criteria
2. Generate corresponding Crossplane manifest files
3. Prompt for confirmation before creating resources in your cluster
4. Apply the manifests to your Kubernetes cluster

After successful import, you can manage the imported resources using standard `kubectl` commands. The tool provides a transaction ID that can be used to revert the import if needed:

```bash
kubectl delete <RESOURCE_TYPE> -l import-ID=<transaction-id>
```

## Building the CLI

To build the `xpbtp` CLI tool from source:

```bash
cd internal/cli
go build -o xpbtp
```

This will create the `xpbtp` executable in the current directory.