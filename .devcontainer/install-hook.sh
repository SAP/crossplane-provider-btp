#!/bin/bash
# Install git hooks to prevent credential commits

echo "🔒 Installing pre-commit hook..."
cat > .git/hooks/pre-commit << 'EOF'
#!/bin/bash
# Pre-commit hook to detect potential credential leaks

echo "🔍 [Pre-commit] Scanning staged files for credentials..."

# Patterns to detect in file content
PATTERNS=(
  "clientsecret"
  "client_secret"
  "password.*:.*[\"'].*[\"']"
  "\"password\":"
  "credentials.*:.*{" 
  "BEGIN.*PRIVATE KEY"
  "authentication.sap.hana.ondemand.com"
  "grant_type.*client_credentials"
  "uaa.*clientid"
)

# File patterns to block (regex)
BLOCKED_PATTERNS=(
  "^\.env$"
  "^\.env\.local$"
  "^\.env\..*$"
  "credentials\.json$"
  "service-key\.json$"
  "cis-binding\.json$"
  ".*-credentials\.yaml$"
  ".*-credentials\.json$"
  ".*-secret\.yaml$"
)

# Check blocked file patterns
STAGED_FILES=$(git diff --cached --name-only)
for pattern in "${BLOCKED_PATTERNS[@]}"; do
  if echo "$STAGED_FILES" | grep -qE "$pattern"; then
    MATCHED_FILE=$(echo "$STAGED_FILES" | grep -E "$pattern" | head -1)
    echo "❌ ERROR: Attempting to commit blocked file: $MATCHED_FILE"
    echo "   Pattern matched: $pattern"
    echo "   This file should NEVER be committed to git!"
    echo "   Add it to .gitignore if not already there."
    exit 1
  fi
done

# Scan staged files for credential patterns
for pattern in "${PATTERNS[@]}"; do
  if git diff --cached | grep -iE "$pattern" > /dev/null; then
    echo "❌ ERROR: Potential credential detected in staged files!"
    echo "   Pattern matched: $pattern"
    echo ""
    echo "   Matched lines:"
    git diff --cached | grep -iE "$pattern" --color=always | head -5
    echo ""
    echo "   ⚠️  If this is a false positive, you can:"
    echo "      1. Remove the sensitive data"
    echo "      2. Use git commit --no-verify (NOT RECOMMENDED)"
    exit 1
  fi
done

echo "✅ No credentials detected in staged files"
EOF

chmod +x .git/hooks/pre-commit
echo "✅ Pre-commit hook installed"

# =============================================================================
# PRE-PUSH HOOK - Stronger protection (scans commits being pushed)
# =============================================================================

echo "🔒 Installing pre-push hook (last line of defense)..."
cat > .git/hooks/pre-push << 'EOF'
#!/bin/bash
# Pre-push hook - final check before credentials reach GitHub

echo ""
echo "🔍 [Pre-push] Final security scan before push..."
echo "   This is your LAST CHANCE to prevent credential leaks!"
echo ""

# Get the remote name and URL
remote="$1"
url="$2"

# Patterns to detect (same as pre-commit)
PATTERNS=(
  "clientsecret"
  "client_secret"
  "password.*:.*[\"'].*[\"']"
  "\"password\":"
  "credentials.*:.*{" 
  "BEGIN.*PRIVATE KEY"
  "authentication.sap.hana.ondemand.com"
  "grant_type.*client_credentials"
  "uaa.*clientid"
)

# File patterns to block
BLOCKED_PATTERNS=(
  "^\.env$"
  "credentials\.json$"
  "service-key\.json$"
  ".*-credentials\.(yaml|json)$"
)

# Empty tree hash for comparing against new branches
EMPTY_TREE=$(git hash-object -t tree /dev/null)

# Read stdin for ref updates
while read local_ref local_sha remote_ref remote_sha; do
  # Skip deletes
  if [ "$local_sha" = "0000000000000000000000000000000000000000" ]; then
    continue
  fi
  
  # Determine range to scan
  if [ "$remote_sha" = "0000000000000000000000000000000000000000" ]; then
    # New branch - scan all commits from empty tree
    range="$EMPTY_TREE..$local_sha"
    echo "   Scanning new branch: $(echo $local_ref | sed 's/refs\/heads\///')"
  else
    # Existing branch - scan commits being pushed
    range="$remote_sha..$local_sha"
    echo "   Scanning commits: $remote_sha..$local_sha"
  fi
  
  # Scan the diff of commits being pushed for content patterns
  for pattern in "${PATTERNS[@]}"; do
    if git diff "$range" | grep -iE "$pattern" > /dev/null 2>&1; then
      echo ""
      echo "❌❌❌ CRITICAL: Credential detected in commits being pushed! ❌❌❌"
      echo ""
      echo "   Pattern matched: $pattern"
      echo "   Remote: $remote ($url)"
      echo ""
      echo "   🚨 PUSH BLOCKED 🚨"
      echo ""
      echo "   Matched content:"
      git diff "$range" | grep -iE "$pattern" --color=always -B2 -A2 | head -10
      echo ""
      echo "   Action required:"
      echo "   1. DO NOT use --no-verify"
      echo "   2. Remove credentials from your commits"
      echo "   3. Use 'git reset' or 'git rebase' to clean history"
      echo "   4. Store credentials in .env (which is gitignored)"
      echo ""
      exit 1
    fi
  done
  
  # Check for blocked filenames
  FILES_IN_RANGE=$(git diff-tree --no-commit-id --name-only -r "$range")
  for pattern in "${BLOCKED_PATTERNS[@]}"; do
    if echo "$FILES_IN_RANGE" | grep -qE "$pattern"; then
      MATCHED=$(echo "$FILES_IN_RANGE" | grep -E "$pattern")
      echo ""
      echo "❌❌❌ CRITICAL: Blocked file detected in commits! ❌❌❌"
      echo ""
      echo "   Files that should never be committed:"
      echo "$MATCHED"
      echo ""
      echo "   🚨 PUSH BLOCKED 🚨"
      echo ""
      exit 1
    fi
  done
done

echo "✅ Pre-push scan complete - no credentials detected"
echo "   Pushing to: $url"
echo ""
EOF

chmod +x .git/hooks/pre-push
echo "✅ Pre-push hook installed"

echo ""
echo "🛡️  Local protection enabled:"
echo "   • Pre-commit: Scans staged files"
echo "   • Pre-push: Scans commits before they reach GitHub"
echo ""
echo "⚠️  Remember: These can be bypassed with --no-verify"
echo "   NEVER use --no-verify unless you know what you're doing!"
echo ""