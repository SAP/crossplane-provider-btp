# SAP BTP Provider - Dev Container

Get started developing the SAP BTP Crossplane Provider in under 5 minutes.

## 🚀 Quick Start with GitHub Codespaces

1. **Set up credentials** (one-time):
   - Go to GitHub Settings → Codespaces → Secrets
   - Add these secrets:
     - `BTP_USERNAME`: Your BTP technical user
     - `BTP_PASSWORD`: Your BTP password
     - `BTP_GLOBAL_ACCOUNT_ID`: Your global account ID

2. **Launch**: Click the "Code" button → "Create codespace on main"

3. **Wait ~3 minutes** for automatic setup

## 🚀 Quick Start Development

### Option 1: GitHub Codespaces (Fastest)
[![Open in GitHub Codespaces](https://github.com/codespaces/badge.svg)](https://codespaces.new/SAP/crossplane-provider-btp)

Get a complete development environment in your browser in ~3 minutes. Perfect for:
- 👋 New contributors getting started
- 📝 Documentation updates
- 🧪 Testing examples
- 🐛 Quick bug fixes

See [.devcontainer/README.md](.devcontainer/README.md) for setup instructions.

### Option 2: Local Development (Full Power)
For intensive development, performance testing, or running the full E2E suite, follow our [detailed setup guide](docs/DEVELOPMENT.md).

## 🧪 Two Ways to Test

Once your devcontainer is running, choose your workflow:

### A) Test with Published Provider (Quick validation)

Use this to **test examples** or **validate functionality** without building code:
```bash
# Install the latest published provider
kubectl crossplane install provider ghcr.io/sap/crossplane-provider-btp:latest

# Wait for it to be ready
kubectl wait --for=condition=Healthy provider.pkg.crossplane.io/crossplane-provider-btp --timeout=300s

# Apply your test resources
kubectl apply -f examples/subaccount.yaml

# Watch it work
kubectl get btpsubaccount -w
```

**Use this when:**
- Testing examples from documentation
- Validating your ProviderConfig setup
- Quick functionality checks
- You don't need to modify provider code

### B) Run Local Dev Version (Active development)

Use this to **develop features** or **debug issues** with your local code changes:
```bash
# Build and run your local code
make run

# In another terminal, apply test resources
kubectl apply -f examples/subaccount.yaml

# Watch logs in the make run terminal
# See your code changes in action!
```

**Use this when:**
- Developing new features
- Debugging controller logic
- Testing uncommitted changes
- Need breakpoint debugging (with delve)

**Pro tip:** The devcontainer setup script already created your kind cluster and installed Crossplane, so you can jump straight to either workflow!