package utils

import (
	"bufio"
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"os"
	"regexp"
	"strings"
	"time"
)

const ConfigName = ".xpbtp"

// NormalizeToRFC1123 normalizes a string to be RFC1123 compliant
func NormalizeToRFC1123(name string) string {
	// Convert to lowercase
	normalized := strings.ToLower(name)

	// Replace invalid characters with hyphens
	reg := regexp.MustCompile(`[^a-z0-9-]`)
	normalized = reg.ReplaceAllString(normalized, "-")

	// Remove leading/trailing hyphens
	normalized = strings.Trim(normalized, "-")

	// Ensure it doesn't exceed 63 characters
	if len(normalized) > 63 {
		normalized = normalized[:63]
	}

	// Ensure it doesn't end with a hyphen
	normalized = strings.TrimSuffix(normalized, "-")

	return normalized + "-"
}

// IsFullMatch checks if a pattern matches the full string
func IsFullMatch(pattern, text string) bool {
	if pattern == ".*" {
		return true
	}

	matched, err := regexp.MatchString("^"+pattern+"$", text)
	if err != nil {
		return false
	}
	return matched
}

// PrintLine prints a formatted line with consistent spacing
func PrintLine(label, value string, maxWidth int) {
	padding := maxWidth - len(label)
	if padding < 0 {
		padding = 0
	}
	fmt.Printf("%s:%s %s\n", label, strings.Repeat(" ", padding), value)
}

// UpdateTransactionID generates a new transaction ID
func UpdateTransactionID() {
	// This would typically update a global transaction ID
	// For now, we'll just generate a random ID
}

// GenerateTransactionID generates a random transaction ID
func GenerateTransactionID() string {
	timestamp := time.Now().Format("20060102-150405")
	randomBytes := make([]byte, 4)
	rand.Read(randomBytes)
	randomHex := hex.EncodeToString(randomBytes)
	return fmt.Sprintf("%s-%s", timestamp, randomHex)
}

// StoreKeyValues stores key-value pairs in the config file
func StoreKeyValues(env map[string]string) error {
	file, err := os.Create(ConfigName)
	if err != nil {
		return fmt.Errorf("could not create config file: %w", err)
	}
	defer file.Close()

	for key, value := range env {
		_, err := file.WriteString(fmt.Sprintf("%s=%s\n", key, value))
		if err != nil {
			return fmt.Errorf("could not write to config file: %w", err)
		}
	}

	return nil
}

// StoreKeyValuesToFile stores key-value pairs in a specified file
func StoreKeyValuesToFile(env map[string]string, filePath string) error {
	file, err := os.Create(filePath)
	if err != nil {
		return fmt.Errorf("could not create file %s: %w", filePath, err)
	}
	defer file.Close()

	for key, value := range env {
		_, err := file.WriteString(fmt.Sprintf("%s=%s\n", key, value))
		if err != nil {
			return fmt.Errorf("could not write to file %s: %w", filePath, err)
		}
	}

	return nil
}

// OpenFile opens the config file for reading
func OpenFile(filename string) *os.File {
	file, err := os.Open(filename)
	if err != nil {
		// If file doesn't exist, create it
		file, err = os.Create(filename)
		if err != nil {
			panic(fmt.Sprintf("Could not create config file: %v", err))
		}
	}
	return file
}

// ReadKeyValue reads a specific key from the config file
func ReadKeyValue(key string) (string, error) {
	file := OpenFile(ConfigName)
	defer file.Close()

	scanner := bufio.NewScanner(file)
	for scanner.Scan() {
		line := scanner.Text()
		parts := strings.SplitN(line, "=", 2)
		if len(parts) == 2 && parts[0] == key {
			return parts[1], nil
		}
	}

	if err := scanner.Err(); err != nil {
		return "", fmt.Errorf("error reading config file: %w", err)
	}

	return "", fmt.Errorf("key %s not found", key)
}
