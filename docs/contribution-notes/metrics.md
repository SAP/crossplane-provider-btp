# External Operation Duration Metrics

The BTP provider exposes the Prometheus histogram
`provider_btp_external_operation_duration_seconds` on its existing `/metrics`
endpoint. It records one observation per call from the Crossplane managed
reconciler to `ExternalConnector.Connect` or to the returned external client's
`Observe`, `Create`, `Update`, or `Delete` method. Each observation measures
elapsed time in seconds from method entry until return, including calls that
return an error.

| Label | Values |
|---|---|
| `kind` | The public Kubernetes Kind of the managed resource, such as `Subaccount` or `ServiceBinding`. |
| `operation` | `connect`, `observe`, `create`, `update`, or `delete`. |
| `outcome` | `success` if the method returned no error, or `error` if it returned an error. |

The `kind` label is bounded by the instrumented native, hybrid, and Upjet
controllers. Resource names, namespaces, account IDs, URLs, and error messages
are not labels. For a hybrid resource, time spent in internal Upjet or Terraform
calls is included in the public method's duration but is not counted again by
this histogram.

A successful return from an asynchronous method does not mean the operation
has completed in BTP. This metric measures the provider-side method call, not
remote completion time, individual HTTP requests, or the full reconciliation.
Existing reconcile and Upjet metrics are unchanged.

## Example PromQL queries

These queries look back five minutes (`[5m]`) and aggregate across scraped
provider instances. They group by `kind`, `operation`, and `outcome` so that
successful and failed calls can be compared separately. The percentile queries
estimate durations in seconds from histogram buckets and retain `le` (the bucket
upper bound) for `histogram_quantile`.

The provider does not expose zero-valued series for every label combination at
startup. For example, the series for `kind="Subaccount"`, `operation="delete"`,
and `outcome="success"` appears on `/metrics` only after the first `Subaccount`
`Delete` call returns without an error.
If Prometheus doesn't have a counter start timestamp, it can't count calls
already included in the first scraped value. For example, a one-off `delete`
can leave the raw `_count` at `1` on every scrape while `increase()` reports
zero and the percentile queries have no bucket increases to work with. A
missing or zero result does not prove that no calls occurred.

### p50 (median) duration

```promql
histogram_quantile(
  0.50,
  sum by (le, kind, operation, outcome) (
    rate(provider_btp_external_operation_duration_seconds_bucket[5m])
  )
)
```

### p95 duration

```promql
histogram_quantile(
  0.95,
  sum by (le, kind, operation, outcome) (
    rate(provider_btp_external_operation_duration_seconds_bucket[5m])
  )
)
```

### Number of calls in the last five minutes

```promql
sum by (kind, operation, outcome) (
  increase(provider_btp_external_operation_duration_seconds_count[5m])
)
```

`increase` estimates the number of calls in the window and can return fractional
values because it extrapolates from scrape samples. Use `rate` instead for the
average number of calls per second over the same window.
