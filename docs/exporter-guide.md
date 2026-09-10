# btp-exporter User Guide

## Overview

`btp-exporter` reads your existing SAP BTP landscape and writes it out as Crossplane YAML — ready to apply to a cluster with `kubectl apply`.

## Installation

See [exporter-install.md](exporter-install.md).

---

## Authentication

The exporter reuses a BTP CLI session. Log in first:

```bash
btp login --sso
```

Or use the built-in login command:

```bash
btp-exporter login
```

Then target your global account:

```bash
btp target --global-account <your-global-account-subdomain>
```

---

## Usage

```
btp-exporter export [flags]
```

### Flags

| Flag | Description |
|---|---|
| `--all` | Export all supported resource kinds |
| `--kind strings` | Limit to specific resource kinds (see below) |
| `--subaccount strings` | Subaccount ID or name regex |
| `--serviceinstance strings` | Service instance ID or name regex |
| `--servicebinding strings` | Service binding ID or name regex |
| `--cloudfoundry-environment strings` | CF environment ID or name regex |
| `--entitlement strings` | Service plan name or name fragment (regex) |
| `--entitlement-auto-assigned` | Include auto-assigned entitlements (use with `--kind entitlement`) |
| `-r, --resolve-references` | Resolve inter-resource references by name instead of GUID |
| `-o, --output string` | Write output to a file instead of stdout |
| `--btp-cli string` | Path to BTP CLI binary (default: `btp` from `$PATH`) |
| `-v, --verbose` | Verbose output |

### Supported resource kinds

- `subaccount`
- `entitlement`
- `serviceinstance`
- `servicebinding`
- `cloudfoundryenvironment`
- `servicemanager`
- `cloudmanagement`

---

## Examples

### Export everything from all subaccounts

```bash
btp-exporter export --all \
  --resolve-references \
  --entitlement-auto-assigned \
  --output ./my-landscape.yaml
```

### Export a specific subaccount by name

```bash
btp-exporter export --all \
  --subaccount "my-subaccount" \
  --resolve-references \
  --output ./my-subaccount.yaml
```

### Export only service instances and bindings

```bash
btp-exporter export \
  --kind serviceinstance \
  --kind servicebinding \
  --resolve-references \
  --output ./services.yaml
```

### Export a single service instance by name regex

```bash
btp-exporter export \
  --kind serviceinstance \
  --serviceinstance "my-postgres.*" \
  --resolve-references \
  --output ./postgres.yaml
```

### Export entitlements (including auto-assigned)

```bash
btp-exporter export \
  --kind entitlement \
  --entitlement-auto-assigned \
  --output ./entitlements.yaml
```

---

## Output format

The output is a multi-document YAML file — one Crossplane resource per `---` block:

```yaml
---
apiVersion: account.btp.sap.crossplane.io/v1alpha1
kind: Subaccount
metadata:
  annotations:
    crossplane.io/external-name: <guid>
  name: my-subaccount
spec:
  managementPolicies:
  - Observe
...
---
apiVersion: account.btp.sap.crossplane.io/v1alpha1
kind: Entitlement
...
```

By default `managementPolicies` is set to `Observe` — Crossplane will look up existing resources by their BTP GUID without creating or deleting anything.

`--resolve-references` replaces raw GUIDs in cross-resource references (e.g. `subaccountRef`) with the Crossplane resource name, making the output portable.

---

## Applying to a cluster

```bash
kubectl apply -f my-landscape.yaml
```

Watch resources come up:

```bash
watch --color kubecolor --force-colors get managed
```

Once all resources show `READY=True` and `SYNCED=True`, the landscape is under Crossplane management.
