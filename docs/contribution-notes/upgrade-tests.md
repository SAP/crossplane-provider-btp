# Upgrade Tests

## Overview

Upgrade tests verify that the provider can be successfully upgraded from one version to another while maintaining resource health. The test framework works as follows:

1. Creates a Kind cluster with Crossplane installed
2. Installs the provider at the "from" version
3. Applies test resources
4. Verifies resources are healthy before upgrade *(can be disabled)*
5. *(optionally)* Runs any pre-upgrade assessments
6. Upgrades the provider to the "to" version
7. Verifies resources remain healthy after upgrade *(can be disabled)*
8. *(optionally)* Runs any post-upgrade assessments
9. Cleans up resources and the provider

While the base upgrade test (`base_upgrade_test.go`) only verifies resource health by checking whether the resources are synced and ready, custom upgrade tests may be added, e.g. to verify external name upgradability.

## Make Targets

### Running Upgrade Tests

#### `make upgrade-test`

The main target for running upgrade tests.

```bash
# Required environment variables
export UPGRADE_TEST_FROM_TAG=v1.4.0
export UPGRADE_TEST_TO_TAG=v1.5.0

# Run all upgrade tests
make upgrade-test

# Run a specific test by name (e.g. only baseline tests)
make upgrade-test testFilter=Test_BaselineUpgradeProvider
```

#### `make upgrade-test-debug`

Run upgrade tests with Delve debugger attached for debugging.

```bash
export UPGRADE_TEST_FROM_TAG=v1.4.0
export UPGRADE_TEST_TO_TAG=v1.5.0

make upgrade-test-debug testFilter=Test_BaselineUpgradeProvider
```

The debugger listens on port `:2345`.

### Environment Variables

| Variable                                   | Required | Default                                                                  | Description                                           |
|--------------------------------------------|----------|--------------------------------------------------------------------------|-------------------------------------------------------|
| `UPGRADE_TEST_FROM_TAG`                    | Yes      | -                                                                        | Source provider version (e.g., `v1.4.0` or `local`)   |
| `UPGRADE_TEST_TO_TAG`                      | Yes      | -                                                                        | Target provider version (e.g., `v1.5.0` or `local`)   |
| `UPGRADE_TEST_CRS_TAG`                     | No       | `$UPGRADE_TEST_FROM_TAG`                                                 | Git tag to pull baseline CRs from                     |
| `UPGRADE_TEST_VERIFY_TIMEOUT`              | No       | `30`                                                                     | Timeout in minutes for resource verification          |
| `UPGRADE_TEST_WAIT_FOR_PAUSE`              | No       | `1`                                                                      | Minutes to wait for resources to pause during upgrade |
| `UPGRADE_TEST_FROM_PROVIDER_REPOSITORY`    | No       | `ghcr.io/sap/crossplane-provider-btp/crossplane/provider-btp`            | Source provider image repository                      |
| `UPGRADE_TEST_TO_PROVIDER_REPOSITORY`      | No       | `ghcr.io/sap/crossplane-provider-btp/crossplane/provider-btp`            | Target provider image repository                      |
| `UPGRADE_TEST_FROM_CONTROLLER_REPOSITORY`  | No       | `ghcr.io/sap/crossplane-provider-btp/crossplane/provider-btp-controller` | Source controller image repository                    |
| `UPGRADE_TEST_TO_CONTROLLER_REPOSITORY`    | No       | `ghcr.io/sap/crossplane-provider-btp/crossplane/provider-btp-controller` | Target controller image repository                    |

#### Using Local Builds

Set `UPGRADE_TEST_FROM_TAG=local` or `UPGRADE_TEST_TO_TAG=local` to use locally built provider images:

```bash
# Test upgrading FROM local build TO a released version
export UPGRADE_TEST_FROM_TAG=local
export UPGRADE_TEST_TO_TAG=v1.5.0
make upgrade-test

# Test upgrading FROM a released version TO local build
export UPGRADE_TEST_FROM_TAG=v1.4.0
export UPGRADE_TEST_TO_TAG=local
make upgrade-test
```

When using `local`, the Makefile automatically builds the provider images before running tests.

### Cleanup Targets

#### `make upgrade-test-clean`

Cleanup that:
- Restores test CRs to their original state
- Removes upgrade test log files
- Deletes Kind clusters created by upgrade tests
- Cleans up BTP artifacts

```bash
make upgrade-test-clean
```

#### `make upgrade-test-restore-crs`

Restores only the `test/upgrade/testdata/baseCRs` directory to the git state:

```bash
make upgrade-test-restore-crs
```

### Helper Targets

#### `make pull-upgrade-test-version-crs`

Pulls baseline CRs from a specific git tag:

```bash
export UPGRADE_TEST_CRS_TAG=v1.4.0
make pull-upgrade-test-version-crs
```

#### `make generate-upgrade-test-crs`

Generates test CRs by substituting environment variables in YAML files:

```bash
make generate-upgrade-test-crs
```

#### `make build-upgrade-test-images`

Builds local provider images when `local` tag is used:

```bash
make build-upgrade-test-images
```

## Test Resources Structure

```
test/upgrade/testdata/
├── baseCRs/           # Baseline resources for standard upgrade tests
│   └── subaccount/
│       ├── directory.yaml
│       └── subaccount.yaml
└── customCRs/         # Resources for custom upgrade tests
    └── subaccountExternalName/
        └── subaccount.yaml
```

### Baseline CRs (`baseCRs/`)

Resources in `baseCRs/` are used by the baseline upgrade test (`Test_BaselineUpgradeProvider`). These resources are:
- Automatically discovered from subdirectories
- Applied before upgrade
- Verified before and after upgrade
- Cleaned up after the test

### Custom CRs (`customCRs/`)

Resources in `customCRs/` are used by custom upgrade tests that need specific resources or configurations.

## Writing Custom Upgrade Tests

The `CustomUpgradeTestBuilder` framework provides an API for creating custom upgrade tests with minimal boilerplate.

```go
//go:build upgrade

package upgrade

import (
    "testing"
)

func Test_MyCustomUpgrade(t *testing.T) {
    upgradeTest := NewCustomUpgradeTest("my-custom-test").
        FromVersion("v1.4.0").
        ToVersion("v1.5.0").
        WithResourceDirectories([]string{
            "./testdata/customCRs/myResources",
        })

    testenv.Test(t, upgradeTest.Feature())
}
```

### Timeout Configuration

```go
// Set verification timeout (default: 30 minutes from env or global default)
builder.WithVerifyTimeout(45 * time.Minute)

// Set pause wait duration (default: 1 minute from env or global default)
builder.WithWaitForPause(2 * time.Minute)
```

### Custom Assessments

Add custom verification logic before or after the upgrade:

```go
builder.WithCustomPreUpgradeAssessment(
    "Verify state before upgrade",
    func(ctx context.Context, t *testing.T, cfg *envconf.Config) context.Context {
        // Custom verification logic
        return ctx
    },
)

builder.WithCustomPostUpgradeAssessment(
    "Verify state after upgrade",
    func(ctx context.Context, t *testing.T, cfg *envconf.Config) context.Context {
        // Custom verification logic
        return ctx
    },
)
```

### Skip Default Verification

If you want complete control over verification:

```go
builder.SkipDefaultResourceVerification()
```

### Test Execution Flow

When `upgradeTest.Feature()` is called, the test executes in this order:

1. **Setup Phase**
   - Install provider at `fromVersion`
   - Apply ProviderConfig
   - Import resources from specified directories

2. **Pre-Upgrade Assessment**
   - Verify resources are synced and ready (unless skipped)
   - Execute custom pre-upgrade assessments

3. **Upgrade Phase**
   - Pause all managed resources
   - Upgrade provider to `toVersion`
   - Resume all managed resources

4. **Post-Upgrade Assessment**
   - Verify resources are synced and ready (unless skipped)
   - Execute custom post-upgrade assessments

5. **Teardown Phase**
   - Delete test resources gracefully
   - Delete ProviderConfig
   - Delete provider

## Best Practices

### 1. Use Meaningful Test Names

```go
NewCustomUpgradeTest("subaccount-external-name-migration-test")
```

### 2. Store State in Context

When comparing pre/post upgrade states, use the context to pass data:

```go
// Pre-upgrade: store value
return context.WithValue(ctx, "myKey", value)

// Post-upgrade: retrieve value
storedValue := ctx.Value("myKey").(string)
```

### 3. Use Proper Error Handling

```go
if err != nil {
    t.Fatalf("Failed to get resource: %v", err)
}
```

### 4. Add Logging for Debugging

```go
import "k8s.io/klog/v2"

klog.V(4).Infof("Pre-upgrade value: %s", value)
```

### 5. Create Dedicated Test Resources

Place custom test resources in `testdata/customCRs/<test-name>/` to avoid conflicts with other tests.

### 6. Clean Up After Tests

The framework handles cleanup automatically, but ensure your custom assessments don't leave orphaned resources.

## Troubleshooting

### Tests Timing Out

Increase the verification timeout:

```bash
export UPGRADE_TEST_VERIFY_TIMEOUT=60  # 60 minutes
make upgrade-test
```

Or in code:

```go
builder.WithVerifyTimeout(60 * time.Minute)
```

### Resources Not Found

Ensure the resource directories exist and contain valid YAML files:

```bash
ls -la test/upgrade/testdata/customCRs/myResources/
```

### Kind Cluster Issues

Clean up orphaned clusters:

```bash
make upgrade-test-clean
```

### Viewing Test Logs

Test output is written to `upgrade-test-output.log`. Kind cluster logs are exported to `test/upgrade/logs/post-tests/` after test completion.

```bash
# View test output
cat upgrade-test-output.log

# View Kind cluster logs
ls test/upgrade/logs/post-tests/
```
