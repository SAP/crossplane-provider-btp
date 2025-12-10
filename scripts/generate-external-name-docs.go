// Copyright 2025 The Crossplane Authors. All rights reserved.
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//     http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

// This tool scans all *_types.go files in the apis directory, extracts External-Name
// Configuration comments from resource type definitions, and generates comprehensive
// documentation in docs/user/external-name.md.
//
// The External-Name Configuration comment format should be:
//
//	// External-Name Configuration:
//	//   - Follow Standard: yes|no
//	//   - Format: <description of the identifier format>
//	//   - How to find:
//	//     - UI: <navigation path in the UI>
//	//     - CLI: <CLI command> (field: <field_name>)
//
// Usage:
//
//	go run scripts/generate-external-name-docs.go
//
// Or via Make:
//
//	make docs.generate-external-name
package main

import (
	"bufio"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
)

const (
	// apisDir is the root directory containing API type definitions
	apisDir = "apis"
	// docsFile is the path to the external-name documentation file
	docsFile = "docs/user/external-name.md"
	// marker is the text marker in the docs file after which generated content is inserted
	marker = "## Generated Data Below"
	// File permissions
	filePermissions = 0644
)

// ResourceConfig holds the external-name configuration extracted from a resource type.
// It contains the resource name and the formatted configuration content.
type ResourceConfig struct {
	// Name is the resource type name (e.g., "GlobalAccount", "Subaccount")
	Name string
	// Content is the formatted external-name configuration documentation
	Content string
}

func main() {
	if err := run(); err != nil {
		fmt.Fprintf(os.Stderr, "Error: %v\n", err)
		os.Exit(1)
	}
}

// run orchestrates the documentation generation process.
// It searches for External-Name Configuration comments in all *_types.go files,
// extracts the configuration details, sorts them alphabetically, and updates
// the documentation file with the generated content.
func run() error {
	fmt.Println("Searching for External-Name Configuration comments in *_types.go files...")

	// Find all *_types.go files and extract configurations
	configs, err := extractConfigurations()
	if err != nil {
		return fmt.Errorf("failed to extract configurations: %w", err)
	}

	if len(configs) == 0 {
		fmt.Println("No External-Name Configuration comments found.")
		return nil
	}

	fmt.Printf("Generating documentation for %d resource(s)...\n", len(configs))

	// Sort configurations by resource name
	sort.Slice(configs, func(i, j int) bool {
		return configs[i].Name < configs[j].Name
	})

	// Generate the documentation content
	generatedContent := generateDocumentation(configs)

	// Update the documentation file
	if err := updateDocsFile(generatedContent); err != nil {
		return fmt.Errorf("failed to update documentation file: %w", err)
	}

	fmt.Printf("Documentation updated successfully in %s\n", docsFile)
	return nil
}

func extractConfigurations() ([]ResourceConfig, error) {
	var configs []ResourceConfig

	err := filepath.Walk(apisDir, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}

		if info.IsDir() || !strings.HasSuffix(path, "_types.go") {
			return nil
		}

		config, err := extractFromFile(path)
		if err != nil {
			return fmt.Errorf("failed to extract from %s: %w", path, err)
		}

		if config != nil {
			fmt.Printf("  Found: %s in %s\n", config.Name, path)
			configs = append(configs, *config)
		}

		return nil
	})

	return configs, err
}

func extractFromFile(filePath string) (*ResourceConfig, error) {
	file, err := os.Open(filePath)
	if err != nil {
		return nil, err
	}
	defer file.Close()

	scanner := bufio.NewScanner(file)

	const (
		stateSearching = iota
		stateInComment
		stateAfterComment
	)

	state := stateSearching
	var commentLines []string
	var resourceName string

	// Regex to match the External-Name Configuration marker
	markerRegex := regexp.MustCompile(`^\s*//\s*External-Name\s+Configuration:`)
	// Regex to match comment lines
	commentRegex := regexp.MustCompile(`^\s*//\s*(.*)$`)
	// Regex to match type definition
	typeRegex := regexp.MustCompile(`^type\s+([A-Za-z0-9_]+)\s+struct`)

	for scanner.Scan() {
		line := scanner.Text()

		switch state {
		case stateSearching:
			if markerRegex.MatchString(line) {
				state = stateInComment
			}

		case stateInComment:
			if matches := commentRegex.FindStringSubmatch(line); matches != nil {
				content := matches[1]
				// Empty comment line ends the External-Name block
				if strings.TrimSpace(content) == "" {
					state = stateAfterComment
				} else {
					commentLines = append(commentLines, content)
				}
			} else {
				// Non-comment line, stop collecting
				state = stateAfterComment
			}

		case stateAfterComment:
			if matches := typeRegex.FindStringSubmatch(line); matches != nil {
				resourceName = matches[1]
				// Found the resource, we're done
				return &ResourceConfig{
					Name:    resourceName,
					Content: formatCommentContent(commentLines),
				}, nil
			}
		}
	}

	if err := scanner.Err(); err != nil {
		return nil, err
	}

	return nil, nil
}

func formatCommentContent(lines []string) string {
	var result []string
	inHowToFind := false

	for _, line := range lines {
		trimmed := strings.TrimSpace(line)

		// Check if this is the "How to find:" line
		if strings.HasPrefix(trimmed, "- How to find:") {
			result = append(result, line)
			inHowToFind = true
			result = append(result, "")
			continue
		}

		// If we are in "How to find" section and line starts with -
		if inHowToFind && strings.HasPrefix(trimmed, "-") && !strings.HasPrefix(trimmed, "- How to find:") {
			// Add proper indentation (2 spaces before the -)
			indented := "  " + trimmed
			result = append(result, indented)
			continue
		}

		// Any other line that does not start with - ends the "How to find" section
		if inHowToFind && !strings.HasPrefix(trimmed, "-") {
			inHowToFind = false
		}

		result = append(result, line)
	}

	return strings.Join(result, "\n")
}

func generateDocumentation(configs []ResourceConfig) string {
	var sb strings.Builder

	for i, config := range configs {
		// Add blank line before each resource except the first one
		if i > 0 {
			sb.WriteString("\n")
		}
		sb.WriteString(fmt.Sprintf("### %s\n\n", config.Name))
		sb.WriteString(config.Content)
		sb.WriteString("\n")
	}

	return sb.String()
}

func updateDocsFile(generatedContent string) error {
	// Read the existing file
	content, err := os.ReadFile(docsFile)
	if err != nil {
		return fmt.Errorf("documentation file %s not found: %w", docsFile, err)
	}

	// Find the marker and extract content before it
	lines := strings.Split(string(content), "\n")
	var beforeMarker []string
	markerFound := false

	for _, line := range lines {
		beforeMarker = append(beforeMarker, line)
		if strings.Contains(line, marker) {
			markerFound = true
			break
		}
	}

	if !markerFound {
		return fmt.Errorf("marker '%s' not found in %s", marker, docsFile)
	}

	// Combine the content before marker with generated content
	var result strings.Builder
	result.WriteString(strings.Join(beforeMarker, "\n"))
	result.WriteString("\n\n")
	result.WriteString(generatedContent)

	// Write the updated content back to the file
	if err := os.WriteFile(docsFile, []byte(result.String()), filePermissions); err != nil {
		return fmt.Errorf("failed to write file: %w", err)
	}

	return nil
}
