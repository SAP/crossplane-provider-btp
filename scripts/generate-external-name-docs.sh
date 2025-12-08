#!/bin/bash
# Copyright 2024 The Crossplane Authors. All rights reserved.
#
# Licensed under the Apache License, Version 2.0 (the "License");
# you may not use this file except in compliance with the License.
# You may obtain a copy of the License at
#
#     http://www.apache.org/licenses/LICENSE-2.0
#
# Unless required by applicable law or agreed to in writing, software
# distributed under the License is distributed on an "AS IS" BASIS,
# WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
# See the License for the specific language governing permissions and
# limitations under the License.

set -e

# Configuration
APIS_DIR="apis"
DOCS_FILE="docs/user/external-name.md"
MARKER="## Generated Data Below"
TEMP_FILE=$(mktemp)
TEMP_CONTENT=$(mktemp)
TEMP_DIR=$(mktemp -d)

# Function to extract external-name configuration from a file
extract_external_name_config() {
    local file="$1"
    awk '
    BEGIN { in_comment = 0; comment_block = ""; resource_name = "" }
    
    # Found the External-Name Configuration marker
    /^[[:space:]]*\/\/[[:space:]]*External-Name[[:space:]]+Configuration:/ {
        in_comment = 1
        next
    }
    
    # If in comment block, collect lines
    in_comment == 1 {
        if (/^[[:space:]]*\/\/[[:space:]]*$/) {
            # Empty comment line - end of External-Name block
            # Now look for the type definition
            in_comment = 2
            next
        } else if (/^[[:space:]]*\/\//) {
            # Regular comment line - collect it
            line = $0
            sub(/^[[:space:]]*\/\/[[:space:]]*/, "", line)
            comment_block = comment_block line "\n"
        }
    }
    
    # After the External-Name block, look for type definition
    in_comment == 2 {
        if (/^type[[:space:]]+[A-Za-z0-9_]+[[:space:]]+struct/) {
            # Extract resource name
            resource_name = $2
            print resource_name "|||" comment_block
            exit
        }
    }
    ' "$file"
}

# Function to format the comment content with proper indentation
format_comment_content() {
    local content="$1"
    echo "$content" | awk '
    BEGIN { in_how_to_find = 0 }
    {
        # Check if this is the "How to find:" line
        if ($0 ~ /^[[:space:]]*-[[:space:]]*How to find:/) {
            print $0
            in_how_to_find = 1
            print ""
            next
        }
        
        # If we are in "How to find" section and line starts with - (but not "How to find:")
        if (in_how_to_find == 1 && $0 ~ /^[[:space:]]*-[[:space:]]/ && $0 !~ /How to find:/) {
            # Add proper indentation (2 spaces before the -)
            sub(/^[[:space:]]*-/, "  -")
            print $0
            next
        }
        
        # Any other line that does not start with - ends the "How to find" section
        if (in_how_to_find == 1 && $0 !~ /^[[:space:]]*-/) {
            in_how_to_find = 0
        }
        
        # Print the line as-is
        print $0
    }
    '
}

# Main script
echo "Searching for External-Name Configuration comments in *_types.go files..."

# Find all *_types.go files and extract configurations
while IFS= read -r file; do
    result=$(extract_external_name_config "$file")
    if [ -n "$result" ]; then
        # Extract resource name (first line, everything before |||)
        resource_name=$(echo "$result" | head -1 | cut -d'|' -f1)
        # Extract comment content (everything after |||, removing the resource name line)
        comment_content=$(echo "$result" | sed '1s/^[^|]*|||//')
        
        # Format the comment content with proper indentation
        formatted_content=$(format_comment_content "$comment_content")
        
        # Save each resource to a separate file for sorting
        printf "%s" "$formatted_content" > "$TEMP_DIR/$resource_name.txt"
        echo "  Found: $resource_name in $file"
    fi
done < <(find "$APIS_DIR" -type f -name "*_types.go")

# Generate the documentation content
echo "" > "$TEMP_CONTENT"

resource_count=$(ls -1 "$TEMP_DIR"/*.txt 2>/dev/null | wc -l | tr -d ' ')
if [ "$resource_count" -eq 0 ]; then
    echo "No External-Name Configuration comments found."
else
    echo "Generating documentation for $resource_count resource(s)..."
    
    # Sort resources alphabetically by filename and generate markdown
    first=true
    for resource_file in $(ls -1 "$TEMP_DIR"/*.txt | sort); do
        resource_name=$(basename "$resource_file" .txt)
        # Add blank line before each resource except the first one
        if [ "$first" = false ]; then
            echo "" >> "$TEMP_CONTENT"
        fi
        first=false
        echo "### $resource_name" >> "$TEMP_CONTENT"
        echo "" >> "$TEMP_CONTENT"
        cat "$resource_file" >> "$TEMP_CONTENT"
        echo "" >> "$TEMP_CONTENT"
    done
fi

# Update the documentation file
if [ ! -f "$DOCS_FILE" ]; then
    echo "Error: Documentation file $DOCS_FILE not found!"
    rm -rf "$TEMP_FILE" "$TEMP_CONTENT" "$TEMP_DIR"
    exit 1
fi

# Extract content before the marker
awk -v marker="$MARKER" '
    {
        print
        if ($0 ~ marker) {
            exit
        }
    }
' "$DOCS_FILE" > "$TEMP_FILE"

# Append the generated content
cat "$TEMP_CONTENT" >> "$TEMP_FILE"

# Replace the original file
mv "$TEMP_FILE" "$DOCS_FILE"
rm -rf "$TEMP_CONTENT" "$TEMP_DIR"

echo "Documentation updated successfully in $DOCS_FILE"
