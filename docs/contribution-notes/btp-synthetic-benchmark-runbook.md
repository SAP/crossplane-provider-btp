# BTP synthetic benchmark operations

The workflow `BTP synthetic benchmark` is report-only. It builds the exact reviewed provider revision and runs one Subaccount create/delete iteration against a **dedicated, approved SAP BTP test account**. It is not a performance gate. A healthy report has `status: not_evaluated`; a missing lifecycle measurement or operational failure fails the job.

## Required setup before enabling runs

1. Provision a dedicated BTP test account with sufficient Subaccount quota in region `eu10`. Confirm the account owner, quota, billing responsibility, and a human cleanup contact. Do not use a developer or production account.
2. Configure GitHub environment `pr-e2e-approval` with required reviewers. Every PR run, including fork PRs, waits for explicit review before checkout/build or secret access. A changed PR head fails the stale run; the new push must receive fresh review.
3. Configure `pr-e2e-no-approval` secrets `BTP_TECHNICAL_USER`, `CIS_CENTRAL_BINDING`, and `XP_DIADROMOS_RELEASE_TOKEN`. Use the provider's established JSON credential formats. The release token should be a short-lived GitHub App installation token or fine-grained credential with only Contents: read on the private xp-diadromos repository. Never put credential material into config, scripts, artifacts, or logs.
4. Confirm private action access and authenticated v0.7.1 asset installation in a trusted manually approved default-branch dispatch. A successful local syntax check does not prove the private action can be fetched or the release token works.

## Resource ownership and discovery

Every workload resource is recognizable by the `xp-btp-bench-<run-id>-<attempt>-` Subaccount name prefix and matching `xpbtpbench-<run-id>-<attempt>-` subdomain prefix. The run also labels the workload through its unique generated resource name. Find the GitHub Actions run ID/attempt in the workflow URL or run summary. The workload always attempts deletion after creation, including when readiness times out, and waits for Kubernetes observation of deletion. `cleanup: auto` removes the action-owned kind environment after normal completion/failure.

If a run fails or is cancelled, inspect both sides before cleaning:

- In the dedicated BTP account, list Subaccounts and match the exact `xp-btp-bench-<run-id>-<attempt>-` ownership prefix. Verify the matching subdomain and run timing. Do not delete resources based on a broad `xp` prefix or delete unrelated account contents.
- In the workflow run, use its logs to identify the generated prefix; logs and artifacts must not contain credentials. If a kind environment is retained during ordinary runner cleanup, inspect Subaccount objects in namespace `default` and their Crossplane conditions/finalizers before deleting anything.
- A requested Kubernetes delete is not proof that the remote BTP Subaccount is gone. Verify deletion in BTP after finalizers complete.

## Trusted fallback cleanup

Hard cancellation, runner loss, or a stuck cloud finalizer can bypass workload cleanup. An account owner or other trusted operator must perform fallback cleanup from a trusted workstation or a separately approved workflow whose cleanup implementation comes from the protected default branch, never from the PR checkout. Authenticate with the dedicated account credentials, enumerate resources, and delete **only** Subaccounts whose full ownership prefix and run identity have been independently confirmed. Record the target IDs before deletion; wait for the BTP API/account listing to show absence. If the remote finalizer is stuck, escalate to the account owner/provider support rather than removing unrelated Kubernetes state. Do not automatically delete by a user-supplied arbitrary prefix.

Never upload kubeconfigs, the provider package, archives, diagnostics, credentials, or output directories as part of cleanup. The workflow uploads only the report action's explicit JSON and Markdown report files (7-day retention). Review any downloaded report before sharing it; labels and context can identify private infrastructure.

## Operational interpretation

The v0.7.1 k6 image and CLI are explicitly selected, and the package comes from the reviewed SHA. One VU/one iteration provides only a smoke-level observation; CPU, memory, and Prometheus series are informational. No latency threshold, baseline, or policy is configured. Benchmark creation/deletion failures are not repaired by report publication: the execution and reporting steps remain separate and either failure must remain visible.
