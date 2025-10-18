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

4. **Test it works**:
   ```bash
   # Install the BTP provider
   kubectl crossplane install provider ghcr.io/sap/crossplane-provider-btp:latest
   
   # Create a test subaccount
   kubectl apply -f examples/subaccount.yaml
   
   # Watch it get created
   kubectl get btpsubaccount -w
   ```

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