#!/bin/bash
set -e

echo "🚀 Setting up SAP BTP Provider development environment..."

# =============================================================================
# CONFIGURATION
# =============================================================================

TERRAFORM_VERSION="1.10.3"
TFDOCS_VERSION="v0.19.0"
K9S_VERSION="v0.32.5"

# =============================================================================
# HELPER FUNCTIONS
# =============================================================================

check_command() {
  if [ $? -ne 0 ]; then
    echo "❌ Error: $1 failed"
    exit 1
  fi
}

# =============================================================================
# LOAD ENVIRONMENT
# =============================================================================

if [ -f .env ]; then
  echo "📋 Loading environment variables from .env file..."
  set -a
  source .env 2>/dev/null || echo "⚠️ Could not load .env"
  set +a
  echo "✓ Environment variables loaded"
else
  echo "⚠️  No .env file found in workspace root"
fi

# =============================================================================
# INSTALL CERTIFICATES
# =============================================================================

install_sap_certificates() {
  echo "🔒 Installing SAP root certificates..."
  sudo mkdir -p /usr/local/share/ca-certificates/sap
  
  if [ -z "$(ls -A /usr/local/share/ca-certificates/sap/ 2>/dev/null)" ]; then
    echo "  Extracting certificates from SAP server..."
    
    if echo | timeout 10 openssl s_client -showcerts \
      -connect authentication.sap.hana.ondemand.com:443 2>/dev/null | \
      awk '/BEGIN CERTIFICATE/,/END CERTIFICATE/ {print}' > /tmp/sap-certs.pem 2>/dev/null && \
      [ -s /tmp/sap-certs.pem ]; then
      
      cd /tmp
      csplit -s -f sap-cert- sap-certs.pem '/-----BEGIN CERTIFICATE-----/' '{*}' 2>/dev/null || true
      
      cert_count=0
      for file in sap-cert-*; do
        if [ -f "$file" ] && [ -s "$file" ] && grep -q "BEGIN CERTIFICATE" "$file"; then
          sudo cp "$file" "/usr/local/share/ca-certificates/sap/sap-cert-${cert_count}.crt"
          cert_count=$((cert_count + 1))
        fi
        rm -f "$file"
      done
      
      rm -f sap-certs.pem
      cd - > /dev/null
      echo "  ✓ Installed ${cert_count} SAP certificates"
    else
      echo "  ⚠️  Could not extract certificates (network issue?)"
    fi
  else
    echo "  ✓ SAP certificates already installed"
  fi
  
  sudo update-ca-certificates --fresh > /dev/null 2>&1 || \
    sudo update-ca-certificates > /dev/null 2>&1
  echo "✓ Certificate configuration complete"
}

install_sap_certificates

# =============================================================================
# WAIT FOR DOCKER
# =============================================================================

echo "⏳ Waiting for Docker to be ready..."
for i in {1..30}; do
  if docker ps &>/dev/null; then
    echo "✓ Docker is ready"
    break
  fi
  [ $i -eq 30 ] && { echo "❌ Docker failed to start"; exit 1; }
  sleep 2
done

# =============================================================================
# INSTALL CLI TOOLS
# =============================================================================

# kind
if ! command -v kind &> /dev/null; then
  echo "📦 Installing kind..."
  curl -Lo ./kind https://kind.sigs.k8s.io/dl/latest/kind-linux-amd64
  chmod +x ./kind
  sudo mv ./kind /usr/local/bin/kind
  echo "✓ kind installed"
else
  echo "✓ kind already installed"
fi

# Crossplane CLI
if ! command -v crossplane &> /dev/null; then
  echo "📦 Installing Crossplane CLI..."
  curl -sL "https://raw.githubusercontent.com/crossplane/crossplane/master/install.sh" | sh
  sudo mv crossplane /usr/local/bin/
  echo "✓ Crossplane CLI installed"
else
  echo "✓ Crossplane CLI already installed"
fi

# jq
if ! command -v jq &> /dev/null; then
  echo "📦 Installing jq..."
  sudo apt-get update -qq && sudo apt-get install -y jq unzip > /dev/null
  echo "✓ jq installed"
else
  echo "✓ jq already installed"
fi

# Terraform
if ! command -v terraform &> /dev/null; then
  echo "📦 Installing Terraform ${TERRAFORM_VERSION}..."
  curl -sSLo terraform.zip \
    "https://releases.hashicorp.com/terraform/${TERRAFORM_VERSION}/terraform_${TERRAFORM_VERSION}_linux_amd64.zip"
  if [ -s terraform.zip ]; then
    unzip -q terraform.zip && sudo mv terraform /usr/local/bin/ && rm terraform.zip
    echo "✓ Terraform installed"
  fi
else
  echo "✓ Terraform already installed"
fi

# terraform-docs
if ! command -v terraform-docs &> /dev/null; then
  echo "📦 Installing terraform-docs ${TFDOCS_VERSION}..."
  curl -sSLo terraform-docs.tar.gz \
    "https://github.com/terraform-docs/terraform-docs/releases/download/${TFDOCS_VERSION}/terraform-docs-${TFDOCS_VERSION}-linux-amd64.tar.gz"
  if [ -s terraform-docs.tar.gz ]; then
    tar -xzf terraform-docs.tar.gz terraform-docs
    sudo mv terraform-docs /usr/local/bin/
    rm -f terraform-docs.tar.gz LICENSE README.md
    echo "✓ terraform-docs installed"
  fi
else
  echo "✓ terraform-docs already installed"
fi

# k9s
if ! command -v k9s &> /dev/null; then
  echo "📦 Installing k9s ${K9S_VERSION}..."
  curl -sSLo k9s.tar.gz \
    "https://github.com/derailed/k9s/releases/download/${K9S_VERSION}/k9s_Linux_amd64.tar.gz"
  if [ -s k9s.tar.gz ]; then
    tar -xzf k9s.tar.gz k9s
    sudo mv k9s /usr/local/bin/
    rm -f k9s.tar.gz LICENSE README.md
    echo "✓ k9s installed"
  fi
else
  echo "✓ k9s already installed"
fi

# =============================================================================
# SETUP KUBERNETES CLUSTER
# =============================================================================

if kind get clusters 2>/dev/null | grep -q "^btp-dev$"; then
  echo "✓ kind cluster 'btp-dev' already exists"
  kind export kubeconfig --name btp-dev
else
  echo "🎯 Creating kind cluster (this takes ~2 min)..."
  kind create cluster --name btp-dev --wait 3m
  kind export kubeconfig --name btp-dev
  echo "✓ kind cluster created"
fi

echo "✓ Verifying cluster..."
kubectl cluster-info > /dev/null

# =============================================================================
# INSTALL CROSSPLANE
# =============================================================================

if kubectl get namespace crossplane-system &>/dev/null; then
  echo "✓ Crossplane already installed"
else
  echo "⚙️  Installing Crossplane..."
  helm repo add crossplane-stable https://charts.crossplane.io/stable > /dev/null 2>&1
  helm repo update > /dev/null 2>&1
  helm install crossplane crossplane-stable/crossplane \
    --namespace crossplane-system \
    --create-namespace \
    --wait > /dev/null 2>&1
  check_command "Crossplane installation"
  echo "✓ Crossplane installed"
fi

# =============================================================================
# INSTALL BTP PROVIDER CRDs
# =============================================================================

if [ -d "package/crds" ]; then
  echo "📦 Installing BTP Provider CRDs..."
  kubectl apply -f package/crds/ > /dev/null 2>&1
  check_command "CRD installation"
  sleep 5
  echo "✓ BTP Provider CRDs installed"
else
  echo "⚠️  package/crds not found - run 'kubectl apply -f package/crds/' before 'make run'"
fi

# =============================================================================
# CONFIGURE BTP CREDENTIALS
# =============================================================================

configure_credentials() {
  local namespace="default"
  
  if [ -n "$BTP_TECHNICAL_USER" ] && [ -n "$CIS_CENTRAL_BINDING" ]; then
    echo "🔐 Configuring BTP credentials..."
    
    kubectl get secret cis-provider-secret -n $namespace &>/dev/null || \
      kubectl create secret generic cis-provider-secret \
        --from-literal=data="$CIS_CENTRAL_BINDING" -n $namespace
    
    kubectl get secret sa-provider-secret -n $namespace &>/dev/null || \
      kubectl create secret generic sa-provider-secret \
        --from-literal=credentials="$BTP_TECHNICAL_USER" -n $namespace
    
    unset CIS_CENTRAL_BINDING BTP_TECHNICAL_USER
    echo "✓ Credentials configured"
    return 0
  else
    echo "⚠️  No credentials found"
    echo "   Add to .env file:"
    echo "   BTP_TECHNICAL_USER='{...}'"
    echo "   CIS_CENTRAL_BINDING='{...}'"
    echo "   BTP_GLOBAL_ACCOUNT='...'"
    return 1
  fi
}

if configure_credentials; then
  # Create ProviderConfig
  if ! kubectl get providerconfig account-provider-config &>/dev/null && [ -n "$BTP_GLOBAL_ACCOUNT" ]; then
    echo "📝 Creating ProviderConfig..."
    cat <<EOF | kubectl apply -f - > /dev/null
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
      namespace: default
      key: data
    source: Secret
  serviceAccountSecret:
    secretRef:
      name: sa-provider-secret
      namespace: default
      key: credentials
    source: Secret
EOF
    echo "✓ ProviderConfig created"
  fi
fi

# =============================================================================
# INSTALL GIT HOOKS
# =============================================================================

[ -f ".devcontainer/install-hooks.sh" ] && bash .devcontainer/install-hooks.sh

# =============================================================================
# SUMMARY
# =============================================================================

echo ""
echo "✅ Setup complete!"
echo ""
echo "🔧 Installed tools:"
echo "   - kind:       $(kind version 2>/dev/null | head -n1)"
echo "   - kubectl:    $(kubectl version --client -o json 2>/dev/null | jq -r '.clientVersion.gitVersion')"
echo "   - crossplane: $(crossplane --version 2>/dev/null)"
echo "   - terraform:  $(terraform version -json 2>/dev/null | jq -r '.terraform_version')"
echo "   - k9s:        ${K9S_VERSION}"
echo ""
echo "📚 Next steps:"
echo ""
echo "   👨‍💻 Developer workflow (run code locally):"
echo "   ----------------------------------------"
echo "   make run                                    # Run local controller"
echo "   kubectl apply -f examples/subaccount.yaml   # Test it"
echo ""
echo "   🧪 Evaluator workflow (try published provider):"
echo "   -----------------------------------------------"
echo "   kubectl crossplane install provider ghcr.io/sap/crossplane-provider-btp:latest"
echo "   kubectl wait --for=condition=Healthy provider.pkg.crossplane.io --all --timeout=300s"
echo "   kubectl apply -f examples/subaccount.yaml"
echo ""
echo "   📊 Monitor resources:"
echo "   --------------------"
echo "   k9s                                         # Interactive UI"
echo ""