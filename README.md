![Golang](https://img.shields.io/badge/Go-1.23-informational)
[![REUSE status](https://api.reuse.software/badge/github.com/SAP/crossplane-provider-btp)](https://api.reuse.software/info/github.com/SAP/crossplane-provider-btp)

# Crossplane Provider for SAP BTP

## About this project

`crossplane-provider-btp` is a [Crossplane](https://crossplane.io/) Provider that handles the orchestration of account related resources on [SAP Business Technology Platform](https://www.sap.com/products/technology-platform.html):

- Subaccount
- User Management
- Entitlements
- Service Manager
- Cloud Management
- Environments

Check the documentation for more detailed information on available capabilities for different kinds.

## Roadmap
We have a lot of exciting new features and improvements in our backlogs for you to expect and even contribute yourself! The major part of this roadmap will be publicly managed in github very soon.

Until then here are the top 3 features we are working on:

#### 1. Serviceinstances and ServiceBindings
We are working on the implementation of the ServiceInstance and ServiceBinding resources. This will allow you to create and manage service instances and bindings in your BTP account without requiring another tool for that.

#### 2. Automation of xsuaa credential management

While it already is possible today to orchestrate your role collections and assignments using the provider, usage up to this point, still requires you to manually create and inject API credentials for the xsuaa API. This is subject to change. We will add new CRDs for managing the API credentials using the newly added https://registry.terraform.io/providers/SAP/btp/latest/docs/resources/subaccount_api_credential.

#### 3. More complex resource imports

We know a lot of you would like to use crossplane for observation of (previously unmanaged) landscapes. Importing resources for observation is already possible, but requires manual process for importing each resource individually. We are working on a more automated way to import resources in bulk.

## Requirements and Setup

To install this provider in a kubernetes cluster running crossplane, you can use the provider custom resource, replacing the `<version>`placeholder with the current version of this provider:

```yaml
apiVersion: pkg.crossplane.io/v1
kind: Provider
metadata:
  name: provider-btp
spec:
  package: ghcr.io/sap/crossplane-provider-btp/crossplane/provider-btp:<VERSION>
```

You should then see the crossplane controller create a deployment for this provider. Once it becomes healthy, you can connect the provider to you BTP global account following the documentation.

## Developing

For local development, clone this repo and run `make` to initialize the "build" Make submodule we use for running, building and testing first.

Running the provider locally requires kind and docker to be installed on the system.

There are two different make commands for local development:
1. `make dev` creates a kind cluster, installs the CRDs and runs the provider directly.
1. `make dev-debug` only creates a kind cluster and installs the CRDs. You then run/debug the provider with any tool by using the `KUBECONFIG` environment variable set to the kind clusters kubeconfig.

For deleting the cluster again, run `make dev-clean`.

The [Crossplane Provider Development][provider-dev] guide may be of use to add new types to the controller.

[provider-dev]: https://github.com/crossplane/crossplane/blob/master/docs/contributing/provider_development_guide.md


### E2E Tests

To run the end2end tests, a technical user within the BTP is necessary for creation of environments (Kyma & CF). `.username` & `.password` is necessary for futher actions on `CloudFoundryEnvironment`.

BTP_TECHNICAL_USER
```json
{
  "email": "email",
  "username": "PuserId",
  "password": "mypass"
}
```

CIS_CENTRAL_BINDING
Contents from the service binding of a `cis-central` service, like
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
Contains the CLI server URL, for example:
```
https://cli.btp.cloud.sap/
```

GLOBAL_ACCOUNT
Contains the subdomain of the global account.

IDP_URL
Contains the URL of an IDP that can be connected to the global account.

SECOND_DIRECTORY_ADMIN_EMAIL
Contains a second email (different from the technical user's email) for the directory admin field.

TECHNICAL_USER_EMAIL
Contains the email of the BTP_TECHNICAL_USER.

## Support, Feedback, Contributing
If you have a question always feel free to reach out on our official crossplane slack channel: 

:rocket: [**#provider-sap-btp**](https://crossplane.slack.com/archives/C07UZ3UJY7Q).

This project is open to feature requests/suggestions, bug reports etc. via [GitHub issues](https://github.com/SAP/crossplane-provider-btp/issues). Contribution and feedback are encouraged and always welcome.

For more information about how to contribute, the project structure, as well as additional contribution information, see our [Contribution Guidelines](CONTRIBUTING.md).

## Security / Disclosure
If you find any bug that may be a security problem, please follow our instructions at [in our security policy](https://github.com/SAP/crossplane-provider-btp/security/policy) on how to report it. Please do not create GitHub issues for security-related doubts or problems.

## Code of Conduct

We as members, contributors, and leaders pledge to make participation in our community a harassment-free experience for everyone. By participating in this project, you agree to abide by its [Code of Conduct](https://github.com/SAP/.github/blob/main/CODE_OF_CONDUCT.md) at all times.

## Licensing

Copyright 2024 SAP SE or an SAP affiliate company and crossplane-provider-btp contributors. Please see our [LICENSE](LICENSE) for copyright and license information. Detailed information including third-party components and their licensing/copyright information is available [via the REUSE tool](https://api.reuse.software/info/github.com/SAP/crossplane-provider-btp).
