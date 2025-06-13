[![Slack](https://img.shields.io/badge/Slack-4A154B?logo=slack)](https://crossplane.slack.com/archives/C07UZ3UJY7Q)
![Golang](https://img.shields.io/badge/Go-1.23-informational)
[![REUSE status](https://api.reuse.software/badge/github.com/SAP/crossplane-provider-btp)](https://api.reuse.software/info/github.com/SAP/crossplane-provider-btp)

# Crossplane Provider for SAP BTP

## About this project

`crossplane-provider-btp` is a [Crossplane](https://crossplane.io/) provider that handles the orchestration of account related resources on [SAP Business Technology Platform](https://www.sap.com/products/technology-platform.html):

- Subaccount
- User Management
- Entitlements
- Service Manager
- Cloud Management
- Environments

Have a look on all available CRDs in the [API reference](https://doc.crds.dev/github.com/SAP/crossplane-provider-btp).
Check the documentation for more detailed information on available capabilities for different kinds.

## Roadmap

We have a lot of exciting new features and improvements in our backlogs for you to expect and even contribute yourself! The major part of this roadmap will be publicly managed in github very soon.

Until then here are the top 3 features we are working on:

### Serviceinstances and ServiceBindings

We are working on the implementation of the ServiceInstance and ServiceBinding resources. This will allow you to create and manage service instances and bindings in your BTP account without requiring another tool for that.

### Automation of xsuaa credential management

While it already is possible today to orchestrate your role collections and assignments using the provider, usage up to this point, still requires you to manually create and inject API credentials for the xsuaa API. This is subject to change. We will add new CRDs for managing the API credentials using the newly added <https://registry.terraform.io/providers/SAP/btp/latest/docs/resources/subaccount_api_credential>.

### More complex resource imports

We know a lot of you would like to use crossplane for observation of (previously unmanaged) landscapes. Importing resources for observation is already possible, but requires manual process for importing each resource individually. We are working on a more automated way to import resources in bulk.

## 📊 Installation

To install this provider in a kubernetes cluster running crossplane, you can use the provider custom resource, replacing the `<version>`placeholder with the current version of this provider:

```yaml
apiVersion: pkg.crossplane.io/v1
kind: Provider
metadata:
  name: provider-btp
spec:
  package: ghcr.io/sap/crossplane-provider-btp/crossplane/provider-btp:<VERSION>
```

Crossplane will take care to create a deployment for this provider. Once it becomes healthy, you can configure your provider using proper credentials and start orchestrating :rocket:.

## 🔬 Developing

### Initial Setup

The provider comes with some tooling to ease a local setup for development. As initial setup you can follow these steps:

1. Clone the repository
2. Run `make submodules` to initialize the "build" submodule provided by crossplane
3. Run `make dev-debug` to create a kind cluster and install the CRDs

:warning: Please note that you are required to have [kind](https://kind.sigs.k8s.io) and [docker](https://www.docker.com/get-started/) installed on your local machine in order to run dev debug.

Those steps will leave you with a local cluster and your KUBECONFIG being configured to connect to it via e.g. [kubectl](https://kubernetes.io/docs/reference/kubectl/) or [k9s](https://k9scli.io). You can already apply manifests to that cluster at this point.

### Running the Controller

To run the controller locally, you can use the following command:

```bash
make run
```

This will compile your controller as executable and run it locally (outside of your cluster).
It will connect to your cluster using your KUBECONFIG configuration and start watching for resources.

### Cleaning up

For deleting the cluster again, run

```bash
make dev-clean
```

### E2E Tests

The provider comes with a set of end-to-end tests that can be run locally. To run them, you can use the following command:

```bash
make test-acceptance
```

This will spin up a specific kind cluster which runs the provider as docker container in it. The e2e tests will run kubectl commands against that cluster to test the provider's functionality.

:warning:
Please be aware that as part of the e2e tests a script will be executed which injects the environment configuration (see below) into the test data. Therefor you will see a lot of changes in the directory `test/e2e/testdata`after running the command. Make sure to not commit those changes into git.

Please note that when running multiple times you might want to delete the kind cluster again to avoid conflicts:

```bash
kind delete cluster <cluster-name>
```

#### Required Configuration

In order for the tests to perform successfully some configuration need to be present as environment variables:

**BTP_TECHNICAL_USER**

User credentials for a user that is Global Account Administrator in the configured globalaccount, structure:

```json
{
  "email": "email",
  "username": "PuserId",
  "password": "mypass"
}
```

**CIS_CENTRAL_BINDING**

Contents from the service binding of a `cis-central` service in the same globalaccount, structure:

```json
{
  "endpoints": {
    "accounts_service_url": "...",
    "cloud_automation_url": "...",
    "entitlements_service_url": "...",
    "events_service_url": "...",
    "external_provider_registry_url": "...",
    "metadata_service_url": "...",
    "order_processing_url": "...",
    "provisioning_service_url": "...",
    "saas_registry_service_url": "..."
  },
  "grant_type": "client_credentials",
  "sap.cloud.service": "com.sap.core.commercial.service.central",
  "uaa": {
      …
  }
}
```

**CLI_SERVER_URL**

Contains the CLI server URL, for example:

```
https://canary.cli.btp.int.sap/
```

**GLOBAL_ACCOUNT**

Contains the subdomain of the global account.

**IDP_URL**

Contains the URL of an IDP that can be connected to the global account as trustconfiguration.

**SECOND_DIRECTORY_ADMIN_EMAIL**

Contains a second email (different from the technical user's email) for the directory admin field.

**TECHNICAL_USER_EMAIL**

Contains the email of the BTP_TECHNICAL_USER.

#### Optional Configuration

**BUILD_ID**

ID that is injected in resource names to relate them to a specific test run.

**CLUSTER_NAME**

Name of created kind cluster, if not set will be randomly generated

**TEST_REUSE_CLUSTER**

0 or 1, default is 0

## Setting up the Provider COnfiguration

### About

This Python script automates the following SAP BTP operations using the BTP CLI:

- Logs into BTP using the CLI.
- Creates a new subaccount.
- Assigns entitlements to the subaccount.
- Creates a cis service instance and binding.
- Retrieves binding credentials for secure use (e.g., upload to Vault).

### Prerequisites

- SAP BTP CLI must be installed
- Python 3.6+ installed

### Invoke Python Script

Invoke the python script `provider-config-setup.py` as below and ensure to pass the required parameters. Also, ensure that you are using technical username and password to setup the provider subaccount as a recommendation.

```python
python3 provider-config-setup.py \
  --btpEnvName live \
  --userName <btp-username> \
  --password <btp-password> \
  --subDomain <btp-subdomain> \
  --subDomainAlias <friendly-subdomain-alias> \
  --region <btp-region>
```

### Output

The credentials section of the service binding, which can be securely stored in SAP Vault or similar secrets manager.

## 👐 Support, Feedback, Contributing

If you have a question always feel free to reach out on our official crossplane slack channel:

:rocket: [**#provider-sap-btp**](https://crossplane.slack.com/archives/C07UZ3UJY7Q).

This project is open to feature requests/suggestions, bug reports etc. via [GitHub issues](https://github.com/SAP/crossplane-provider-btp/issues). Contribution and feedback are encouraged and always welcome.

For more information about how to contribute, the project structure, as well as additional contribution information, see our [Contribution Guidelines](CONTRIBUTING.md).

## 🔒 Security / Disclosure

If you find any bug that may be a security problem, please follow our instructions at [in our security policy](https://github.com/SAP/crossplane-provider-btp/security/policy) on how to report it. Please do not create GitHub issues for security-related doubts or problems.

## 🙆‍♀️ Code of Conduct

We as members, contributors, and leaders pledge to make participation in our community a harassment-free experience for everyone. By participating in this project, you agree to abide by its [Code of Conduct](https://github.com/SAP/.github/blob/main/CODE_OF_CONDUCT.md) at all times.

## 📋 Licensing

Copyright 2024 SAP SE or an SAP affiliate company and crossplane-provider-btp contributors. Please see our [LICENSE](LICENSE) for copyright and license information. Detailed information including third-party components and their licensing/copyright information is available [via the REUSE tool](https://api.reuse.software/info/github.com/SAP/crossplane-provider-btp).
