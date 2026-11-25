# BTP Provider Metrics

The BTP provider exposes these provider-specific Prometheus metrics on its
existing `/metrics` endpoint alongside the standard runtime and reconcile
metrics. No additional provider configuration is required to expose them.

| Metric | Type | What it measures |
|---|---|---|
| [`provider_btp_external_operation_duration_seconds`](#external-operation-duration) | Histogram | Duration and count of Crossplane external operation calls. |
| [`provider_btp_api_request_attempts_total`](#entitlements-api-request-attempts) | Counter | Entitlements HTTP request attempts by result, including HTTP 429. |
| [`provider_btp_api_request_duration_seconds`](#entitlements-api-response-latency) | Histogram | Time to receive headers for matching Entitlements HTTP responses. |

## External operation duration

The histogram `provider_btp_external_operation_duration_seconds` records one
observation per call from the Crossplane managed reconciler to
`ExternalConnector.Connect` or to the returned external client's `Observe`,
`Create`, `Update`, or `Delete` method. Each observation measures elapsed time in
seconds from method entry until return, including calls that return an error.

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

### Example PromQL queries

These queries look back five minutes (`[5m]`) and aggregate across scraped
provider instances. They group by `kind`, `operation`, and `outcome` so that
successful and failed calls can be compared separately. The percentile queries
estimate durations in seconds from histogram buckets and retain `le` (the bucket
upper bound) for `histogram_quantile`.

#### p50 (median) duration

```promql
histogram_quantile(
  0.50,
  sum by (le, kind, operation, outcome) (
    rate(provider_btp_external_operation_duration_seconds_bucket[5m])
  )
)
```

#### p95 duration

```promql
histogram_quantile(
  0.95,
  sum by (le, kind, operation, outcome) (
    rate(provider_btp_external_operation_duration_seconds_bucket[5m])
  )
)
```

#### Number of calls in the last five minutes

```promql
sum by (kind, operation, outcome) (
  increase(provider_btp_external_operation_duration_seconds_count[5m])
)
```

## Entitlements API request attempts

The counter `provider_btp_api_request_attempts_total` tracks request volume and
HTTP 429 responses, not reconciles or external operation duration. The
Entitlements client wraps its base HTTP transport below OAuth. For each matching
request, the wrapper increments the counter once, when the base transport
returns. It matches the exact HTTP method and path, including any path prefix
configured on the Entitlements endpoint. Query parameters don't affect matching.

Assignment cache hits, invalid requests rejected before dispatch (such as a PUT
with no payload), and OAuth token acquisition failures don't reach the wrapper
and don't count. OAuth token requests, other Entitlements routes, and traffic
from Accounts, Provisioning, and Upjet/Terraform clients are excluded.

A caller retry counts separately only if it makes another matching call through
the wrapper. Retries within the base transport aren't individually visible. A
returned redirect counts as `3xx`; the follow-up counts separately only if it
also matches and goes through the wrapper. The counter doesn't change retry or
redirect behavior or require any managed resource migration.

### Labels and results

| Label | Values |
|---|---|
| `service` | `entitlements` |
| `operation` | `get_directory_assignments` for `GET /entitlements/v1/assignments`; `set_service_plans` for `PUT /entitlements/v1/subaccountServicePlans` |
| `result` | `2xx`, `3xx`, `429`, `other_4xx`, `5xx`, `other_http`, `timeout`, `transport_error` |

If the transport returns a response, its HTTP status determines the result, even
if it also returns an error. Status 429 has its own bucket; the remaining 400–499
statuses are `other_4xx`. The `2xx`, `3xx`, and `5xx` buckets cover 200–299,
300–399, and 500–599 respectively. All other statuses, including 1xx, are
`other_http`.

Without a response, `context.DeadlineExceeded` or a `net.Error` reporting a timeout
anywhere in the error chain yields `timeout`. Other outcomes without a response,
including ordinary cancellation, are `transport_error`. The result doesn't change
if reading or decoding the response later fails. A `2xx` does not prove the SDK
decoded the response successfully or that BTP completed the operation.

Hosts, paths, query values, resource identifiers, and error messages are not labels.

### Querying attempt counts

#### Total attempts in the last five minutes

```promql
sum(increase(provider_btp_api_request_attempts_total{service="entitlements"}[5m]))
```

#### HTTP 429s by operation in the last five minutes

```promql
sum by (operation) (increase(provider_btp_api_request_attempts_total{service="entitlements",result="429"}[5m]))
```

## Entitlements API response latency

The histogram `provider_btp_api_request_duration_seconds` measures seconds from
just before the underlying HTTP transport's `RoundTrip` call until it returns
response headers. It doesn't include debugger logging, reading or decoding the
response body, or work outside that HTTP attempt, such as reconciliation or
Crossplane external operations. A response counts once, including HTTP 429,
5xx, or a response returned alongside an error. Failures without a response
(such as a timeout before receiving headers) increase the attempt counter but
not the histogram's `_count`. A later body-read or decoding failure doesn't
remove the observation.

| Label | Values |
|---|---|
| `service` | `entitlements` |
| `operation` | `get_directory_assignments` for `GET /entitlements/v1/assignments`; `set_service_plans` for `PUT /entitlements/v1/subaccountServicePlans` |

Matching uses the exact method and escaped path, including any configured
endpoint prefix, and ignores query parameters. OAuth token requests, other API
clients, cache hits, and calls rejected before dispatch do not contribute. When
callers share one in-flight request, that request contributes at most one
observation. A caller retry or redirect follow-up contributes separately only
if it matches and returns a response; retries inside the underlying transport
aren't observed individually.

The classic histogram has only the labels above, plus Prometheus's `le` label
on `_bucket` series. The upper bounds in seconds are `0.005`, `0.01`, `0.025`,
`0.05`, `0.1`, `0.25`, `0.5`, `1`, `2.5`, `5`, `10`, and `+Inf`. Responses
slower than 10 seconds still contribute to `_sum` and `_count`, but bucket-based
percentiles can't resolve how far beyond 10 seconds they took. No configuration
or resource migration is needed. Older provider versions don't expose this
histogram, and new instances don't backfill observations.

### Response latency by operation

These queries aggregate across provider instances over five minutes. Percentiles
are estimates in seconds from bucket rates; retain `le` for `histogram_quantile`.

#### p50 (median) response latency

```promql
histogram_quantile(0.50, sum by (le, operation) (rate(provider_btp_api_request_duration_seconds_bucket{service="entitlements"}[5m])))
```

#### p95 response latency

```promql
histogram_quantile(0.95, sum by (le, operation) (rate(provider_btp_api_request_duration_seconds_bucket{service="entitlements"}[5m])))
```

#### Number of measured responses in the last five minutes

```promql
sum by (operation) (increase(provider_btp_api_request_duration_seconds_count{service="entitlements"}[5m]))
```

## Interpreting counters and histogram counts

All three metrics are process-local and reset when the provider restarts. Apply
`increase()` or `rate()` to each scraped series before aggregating across provider
instances so Prometheus can account for resets in each series. `increase()`
estimates counts from scrape samples and can return fractional values because it
extrapolates to the edges of the window. Use `rate()` instead for the average
number per second over the same window.

A series appears only after its first matching observation or result. For
example, the histogram series for `kind="Subaccount"`, `operation="delete"`, and
`outcome="success"` appears after the first successful `Subaccount` `Delete` call.
A five-minute query can return no data if a series hasn't appeared or has too few
samples. Without a counter start timestamp, events already included in the first
scraped value can be missed: a one-off `Delete` or HTTP 429 before the first
scrape can leave a raw histogram `_count` or attempt counter at `1` while
`increase()` later reports zero. The duration percentile queries then have no
bucket increases to work with for that series. A missing or zero result doesn't
prove that no events occurred.
