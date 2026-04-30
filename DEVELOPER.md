# Development Setup

If you want to contribute to this Crossplane BTP provider, be aware of the [contribution guidelines](CONTRIBUTING.md).

## Local Setup

Ensure you have the following tools installed:
- git
- go
- golangci-lint
- make
- docker
- helm
- kind
- kubectl

## Developing

Clone the repository:

```console
git clone https://github.com/SAP/crossplane-provider-btp.git
cd crossplane-provider-btp
```

Check out the branch you want to work on:

```console
git checkout -b <branch-name>
```

Init and update submodules:

```console
make submodules
```

Configure connection details to your local development environment in `examples/provider/` directory.

## Local Build

Run code-generation pipeline:

```console
make generate
```

To test the provider with a local kind cluster, first run:

```console
make dev-debug
```

This creates a dev cluster, installs the CRDs, and creates the crossplane-system namespace. You can then debug the provider with your IDE.

To run the controller directly without a debugger:

```console
make dev
```

## Local Kind Build

If you want to run the controller component and the Crossplane controller in the same local kind cluster, run:

```console
make build PLATFORM=linux_amd64
make local.xpkg.deploy.provider.provider-btp
```

## Local e2e test

Build binary:

```console
make build PLATFORM=linux_amd64
```

To start the local e2e test (requires BTP credentials in environment):

```console
export GLOBAL_ACCOUNT=<your-global-account-guid>
export CLI_SERVER_URL=<your-cli-server-url>
export CIS_CENTRAL_BINDING=<your-cis-binding-json>
export BTP_TECHNICAL_USER=<your-technical-user-json>
make test-acceptance
```

## Report a Bug

For filing bugs, suggesting improvements, or requesting new features, please
open an [issue](https://github.com/SAP/crossplane-provider-btp/issues).
