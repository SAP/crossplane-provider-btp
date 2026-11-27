# SAP BTP Crossplane Provider — Metrics Operator

This directory contains [metrics-operator](https://github.com/openmcp-project/metrics-operator) resources for observing the state of crossplane-provider-btp managed resources and shipping them to an OpenTelemetry endpoint.

## Prerequisites & Setup

For installation, DataSink configuration, and OTEL endpoint setup, follow the upstream documentation:

- **Install the operator:** [metrics-operator — Getting Started](https://github.com/openmcp-project/metrics-operator?tab=readme-ov-file#getting-started)
- **Configure a DataSink:** [DataSink Configuration Guide](https://github.com/openmcp-project/metrics-operator/blob/main/docs/datasink-configuration.md)
- **Configure dimensions:** [Dimensions Configuration Guide](https://github.com/openmcp-project/metrics-operator/blob/main/docs/dimensions-configuration.md)

## Apply BTP Metrics

The resources in this directory are plain Kubernetes manifests and are best managed via **GitOps** — point your GitOps tool at this directory so that metric definitions are version-controlled, auditable, and automatically reconciled.

**Flux v2 example** — create a `Kustomization` that watches this path:

```yaml
apiVersion: kustomize.toolkit.fluxcd.io/v1
kind: Kustomization
metadata:
  name: btp-metrics
  namespace: flux-system
spec:
  interval: 10m
  sourceRef:
    kind: GitRepository
    name: crossplane-provider-btp   # your existing GitRepository source
  path: ./examples/metrics-operator
  prune: true
  wait: true
```

See the [Flux Kustomization documentation](https://fluxcd.io/flux/components/kustomize/kustomizations/) for full reference.

If you prefer to apply manually, once the operator is running and a DataSink named `default` exists in `metrics-operator-system`:

```bash
kubectl apply -f managed-metric-subaccounts.yaml
kubectl apply -f managed-metric-entitlements-subscriptions.yaml
kubectl apply -f managed-metric-service-instances.yaml
kubectl apply -f managed-metric-environments.yaml
kubectl apply -f managed-metric-security.yaml
kubectl apply -f metric-resource-age.yaml   # optional: age/drift tracking
```

## Recommended Metrics

| File | Metric name | What it tracks |
|---|---|---|
| `managed-metric-subaccounts.yaml` | `btp_subaccount_count` | All Subaccount CRs — ready/synced state, region, ID |
| `managed-metric-entitlements-subscriptions.yaml` | `btp_entitlement_count` | Entitlements per service plan |
| `managed-metric-entitlements-subscriptions.yaml` | `btp_subscription_count` | SaaS subscriptions per app/plan |
| `managed-metric-service-instances.yaml` | `btp_service_instance_count` | ServiceInstance CRs per offering |
| `managed-metric-service-instances.yaml` | `btp_service_binding_count` | ServiceBinding CRs |
| `managed-metric-environments.yaml` | `btp_cf_environment_count` | CloudFoundry environments |
| `managed-metric-environments.yaml` | `btp_kyma_environment_count` | Kyma environments per plan |
| `managed-metric-security.yaml` | `btp_role_collection_count` | RoleCollections |
| `managed-metric-security.yaml` | `btp_trust_configuration_count` | Trust configs per IdP origin |
| `metric-resource-age.yaml` | `btp_subaccount_creation_timestamp_seconds` | Subaccount age (drift detection) |
| `metric-resource-age.yaml` | `btp_service_instance_creation_timestamp_seconds` | ServiceInstance age |

## Key Dimensions

All `ManagedMetric` resources expose these dimensions by default:

| Dimension | Description |
|---|---|
| `cluster` | Kubernetes cluster name |
| `group` | CRD API group |
| `version` | CRD version |
| `kind` | Resource kind |
| `condition_ready` | Full `Ready` condition object (JSON) |
| `condition_synced` | Full `Synced` condition object (JSON) |

Resource-specific dimensions (name, region, plan, offering, …) are defined per metric.

## Why ManagedMetric over Metric?

`ManagedMetric` is purpose-built for Crossplane — it automatically includes `ready` and `synced` convenience dimensions when no custom `dimensions` block is specified. Once you add a custom `dimensions` block (as done here), you take full control and only the `cluster` base dimension is added automatically.

## OTEL Collector tip

For production, place an OTel Collector in-cluster and point the DataSink at it over gRPC (`grpc://<collector-svc>:4317`). The collector can then parse the `condition_ready`/`condition_synced` JSON strings, flatten them into individual attributes, and route to multiple backends.
