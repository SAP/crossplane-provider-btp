#!/bin/bash
set -e

echo "🚀 Setting up SAP BTP Provider development environment..."

# Function to check if command succeeded
check_command() {
  if [ $? -ne 0 ]; then
    echo "❌ Error: $1 failed"
    exit 1
  fi
}

# Wait for Docker to be ready (Docker-in-Docker takes a moment)
echo "⏳ Waiting for Docker to be ready..."
for i in {1..30}; do
  if docker ps &>/dev/null; then
    echo "✓ Docker is ready"
    break
  fi
  echo "   Waiting for Docker... ($i/30)"
  sleep 2
done

if ! docker ps &>/dev/null; then
  echo "❌ Docker failed to start after 60 seconds"
  exit 1
fi

# Install kind
if ! command -v kind &> /dev/null; then
  echo "📦 Installing kind..."
  curl -Lo ./kind https://kind.sigs.k8s.io/dl/latest/kind-linux-amd64
  chmod +x ./kind
  sudo mv ./kind /usr/local/bin/kind
else
  echo "✓ kind already installed"
fi

# Install crossplane CLI
if ! command -v crossplane &> /dev/null; then
  echo "📦 Installing Crossplane CLI..."
  curl -sL "https://raw.githubusercontent.com/crossplane/crossplane/master/install.sh" | sh
  sudo mv crossplane /usr/local/bin/
else
  echo "✓ Crossplane CLI already installed"
fi

# Install Terraform
if ! command -v terraform &> /dev/null; then
  echo "📦 Installing Terraform..."
  TERRAFORM_VERSION=$(curl -s https://checkpoint-api.hashicorp.com/v1/check/terraform | jq -r '.current_version')
  
  curl -Lo terraform.zip "https://releases.hashicorp.com/terraform/${TERRAFORM_VERSION}/terraform_${TERRAFORM_VERSION}_linux_amd64.zip"
  unzip terraform.zip
  sudo mv terraform /usr/local/bin/
  rm terraform.zip
  
  echo "✓ Terraform ${TERRAFORM_VERSION} installed"
else
  TERRAFORM_VERSION=$(terraform version -json | jq -r '.terraform_version')
  echo "✓ Terraform ${TERRAFORM_VERSION} already installed"
fi

# Install Terraform docs
if ! command -v terraform-docs &> /dev/null; then
  echo "📦 Installing terraform-docs..."
  TFDOCS_VERSION=$(curl -s https://api.github.com/repos/terraform-docs/terraform-docs/releases/latest | jq -r '.tag_name')
  curl -Lo terraform-docs.tar.gz "https://github.com/terraform-docs/terraform-docs/releases/download/${TFDOCS_VERSION}/terraform-docs-${TFDOCS_VERSION}-linux-amd64.tar.gz"
  tar -xzf terraform-docs.tar.gz
  sudo mv terraform-docs /usr/local/bin/
  rm terraform-docs.tar.gz
  echo "✓ terraform-docs installed"
else
  echo "✓ terraform-docs already installed"
fi

# Install jq
if ! command -v jq &> /dev/null; then
  echo "📦 Installing jq..."
  sudo apt-get update && sudo apt-get install -y jq unzip
else
  echo "✓ jq already installed"
fi

# Create kind cluster only if it doesn't exist
if kind get clusters 2>/dev/null | grep -q "^btp-dev$"; then
  echo "✓ kind cluster 'btp-dev' already exists"
  echo "📝 Exporting kubeconfig for existing cluster..."
  kind export kubeconfig --name btp-dev
  echo "✓ kubeconfig updated"
else
  echo "🎯 Creating kind cluster (this takes ~2 min)..."
  kind create cluster --name btp-dev --wait 3m
  echo "✓ kind cluster created"
  kind export kubeconfig --name btp-dev
fi

# Verify kubectl can see the cluster
echo "✓ Verifying kubectl configuration..."
kubectl config get-contexts
kubectl config current-context

# Verify cluster is accessible
echo "✓ Verifying cluster access..."
kubectl cluster-info
kubectl get nodes

# Check if Crossplane is already installed
if kubectl get namespace crossplane-system &>/dev/null; then
  echo "✓ Crossplane already installed, skipping"
else
  echo "⚙️  Installing Crossplane in cluster..."
  helm repo add crossplane-stable https://charts.crossplane.io/stable
  helm repo update
  helm install crossplane crossplane-stable/crossplane \
    --namespace crossplane-system \
    --create-namespace \
    --wait
  check_command "Crossplane installation"
fi

echo ""
echo "✅ Base setup complete!"
echo ""

# Handle BTP credentials
SECRETS_CREATED=false

if [ -n "$BTP_TECHNICAL_USER" ] && [ -n "$CIS_CENTRAL_BINDING" ]; then
  echo "🔐 BTP credentials detected - configuring..."
  
  # Check if secrets already exist
  if kubectl get secret cis-provider-secret -n crossplane-system &>/dev/null; then
    echo "✓ CIS secret already exists"
  else
    kubectl create secret generic cis-provider-secret \
      --from-literal=credentials="$CIS_CENTRAL_BINDING" \
      -n crossplane-system
    check_command "CIS secret creation"
    echo "✅ CIS credentials configured"
  fi
  
  if kubectl get secret sa-provider-secret -n crossplane-system &>/dev/null; then
    echo "✓ SA secret already exists"
  else
    kubectl create secret generic sa-provider-secret \
      --from-literal=credentials="$BTP_TECHNICAL_USER" \
      -n crossplane-system
    check_command "SA secret creation"
    echo "✅ SA credentials configured"
  fi
  
  # Clear from environment
  unset CIS_CENTRAL_BINDING BTP_TECHNICAL_USER
  
  SECRETS_CREATED=true
else
  echo "⚠️  No BTP credentials found"
  echo "   Set BTP_TECHNICAL_USER and CIS_CENTRAL_BINDING environment variables"
  echo "   Or add them to .env file for local development"
fi

# Create ProviderConfig
if kubectl get providerconfig account-provider-config &>/dev/null 2>&1; then
  echo "✓ ProviderConfig 'account-provider-config' already exists"
elif [ "$SECRETS_CREATED" = true ]; then
  echo "📝 Creating ProviderConfig..."
  
  # Check if BTP_GLOBAL_ACCOUNT is set
  if [ -z "$BTP_GLOBAL_ACCOUNT" ]; then
    echo "⚠️  Warning: BTP_GLOBAL_ACCOUNT not set"
    echo "   You need to set this environment variable"
    echo "   Skipping ProviderConfig creation"
  else
    echo "✓ Using globalAccount: $BTP_GLOBAL_ACCOUNT"
    
    cat > /tmp/providerconfig.yaml <<EOF
apiVersion: btp.sap.crossplane.io/v1alpha1
kind: ProviderConfig
metadata:
  name: account-provider-config
spec:
  globalAccount: "${BTP_GLOBAL_ACCOUNT}"
  cliServerUrl: https://canary.cli.btp.int.sap
  cisCredentials:
    secretRef:
      name: cis-provider-secret
      namespace: crossplane-system
      key: credentials
    source: Secret
  serviceAccountSecret:
    secretRef:
      key: credentials
      name: sa-provider-secret
      namespace: crossplane-system
    source: Secret
EOF
    
    kubectl apply -f /tmp/providerconfig.yaml
    check_command "ProviderConfig creation"
    echo "✅ ProviderConfig 'account-provider-config' created"
  fi
fi

echo ""
echo "🔧 Installed tools:"
echo "   - kind: $(kind version 2>/dev/null | head -n1 || echo 'N/A')"
echo "   - kubectl: $(kubectl version --client --short 2>/dev/null | head -n1 || echo 'N/A')"
echo "   - helm: $(helm version --short 2>/dev/null || echo 'N/A')"
echo "   - crossplane: $(crossplane --version 2>/dev/null || echo 'N/A')"
echo "   - terraform: $(terraform version -json 2>/dev/null | jq -r '.terraform_version' || echo 'N/A')"
echo "   - go: $(go version 2>/dev/null | awk '{print $3}' || echo 'N/A')"
echo ""
echo "📚 Next steps:"
echo ""
echo "   1. Install BTP provider:"
echo "      kubectl crossplane install provider ghcr.io/sap/crossplane-provider-btp:latest"
echo ""
echo "   2. Wait for provider:"
echo "      kubectl wait --for=condition=Healthy provider.pkg.crossplane.io --all --timeout=300s"
echo ""
echo "   3. Run controller locally:"
echo "      make run"
echo ""
echo "   4. Create a test subaccount:"
echo "      kubectl apply -f examples/subaccount.yaml"
echo ""
echo "✨ Happy coding!"