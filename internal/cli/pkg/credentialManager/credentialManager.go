package credentialManager

import (
	"bufio"
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/sap/crossplane-provider-btp/internal/cli/adapters"
	"github.com/sap/crossplane-provider-btp/internal/cli/pkg/utils"
	"github.com/sap/crossplane-provider-btp/internal/crossplaneimport/client"
	"k8s.io/apimachinery/pkg/runtime"
)

const (
	// error messages
	errGetCredentials       = "Could not get credentials"
	errUnmarshalCredentials = "Failed to unmarshal credentials JSON"
	errParseCredEnvironment = "Could not parse CredEnvironment"
)

const (
	// CredEnvironment attribute names
	fieldTransactionID  = "TRANSACTION_ID"
	fieldKubeConfigPath = "KUBECONFIGPATH"
	fieldConfigPath     = "CONFIGPATH"
	fieldCredentials    = "CREDENTIALS"
)

// CreateEnvironment sets up the environment for the CLI by fetching credentials from Kubernetes
func CreateEnvironment(kubeConfigPath string, envFilePath string, ctx context.Context, providerConfigName string, clientAdapter *adapters.BTPClientAdapter, scheme *runtime.Scheme) error {
	// If no kubeConfigPath is provided fall back to ~/.kube/config
	if kubeConfigPath == "" {
		kubeConfigPath = getKubeConfigFallBackPath()
	}

	creds, err := clientAdapter.GetCredentials(ctx, kubeConfigPath, client.ProviderConfigRef{Name: providerConfigName}, scheme)
	if err != nil {
		return fmt.Errorf("%s: %w", errGetCredentials, err)
	}

	err = storeCredentialsToFile(creds, kubeConfigPath, envFilePath)
	if err != nil {
		return fmt.Errorf("failed to store credentials: %w", err)
	}

	fmt.Println("Import Environment created ...")
	return nil
}

func storeCredentials(creds client.Credentials, kubeConfigPath string, configPath string) error {
	jsonCreds, err := json.Marshal(creds)
	if err != nil {
		return fmt.Errorf("%s: %w", errUnmarshalCredentials, err)
	}

	env := map[string]string{
		fieldTransactionID:  "pending",
		fieldKubeConfigPath: kubeConfigPath,
		fieldConfigPath:     configPath,
		fieldCredentials:    string(jsonCreds),
	}

	return utils.StoreKeyValues(env)
}

func storeCredentialsToFile(creds client.Credentials, kubeConfigPath string, envFilePath string) error {
	jsonCreds, err := json.Marshal(creds)
	if err != nil {
		return fmt.Errorf("%s: %w", errUnmarshalCredentials, err)
	}

	env := map[string]string{
		fieldTransactionID:  "pending",
		fieldKubeConfigPath: kubeConfigPath,
		fieldConfigPath:     "", // We don't store the config path when using custom env file
		fieldCredentials:    string(jsonCreds),
	}

	return utils.StoreKeyValuesToFile(env, envFilePath)
}

func getKubeConfigFallBackPath() string {
	// Get the home directory
	homeDir, err := os.UserHomeDir()
	if err != nil {
		return ""
	}

	// Define the .kube/config path
	kubeConfigPath := filepath.Join(homeDir, ".kube", "config")

	// Get the absolute path
	absPath, err := filepath.Abs(kubeConfigPath)
	if err != nil {
		return ""
	}

	return absPath
}

func RetrieveCredentials() client.Credentials {
	envFilePath := utils.ConfigName

	// Check if the environment file exists
	if _, err := os.Stat(envFilePath); os.IsNotExist(err) {
		panic(fmt.Sprintf("environment file '%s' not found. Please run 'xpbtp init' successfully to create it.", envFilePath))
	}

	// Read file content to check if it's empty
	fileInfo, err := os.Stat(envFilePath)
	if err != nil {
		panic(fmt.Sprintf("failed to get file info for '%s': %v", envFilePath, err))
	}

	if fileInfo.Size() == 0 {
		panic(fmt.Sprintf("environment file '%s' is empty. Please run 'xpbtp init' successfully to populate it.", envFilePath))
	}

	file := utils.OpenFile(utils.ConfigName)
	defer file.Close()

	config := make(map[string]string)

	// Read the file line by line
	scanner := bufio.NewScanner(file)
	for scanner.Scan() {
		line := scanner.Text()
		// Split the line by "="
		parts := strings.SplitN(line, "=", 2)
		if len(parts) == 2 {
			key := parts[0]
			value := parts[1]
			// Store key-value pairs in the map
			config[key] = value
		}
	}

	err = scanner.Err()
	if err != nil {
		panic(fmt.Sprintf("%s: %v", errParseCredEnvironment, err))
	}

	// Check if credentials field exists and is not empty
	credentialsJSON, exists := config[fieldCredentials]
	if !exists {
		panic(fmt.Sprintf("credentials field not found in environment file '%s'. Please run 'xpbtp init' successfully to populate it.", envFilePath))
	}

	if strings.TrimSpace(credentialsJSON) == "" {
		panic(fmt.Sprintf("credentials field is empty in environment file '%s'. Please run 'xpbtp init' successfully to populate it.", envFilePath))
	}

	// map values and return Credentials
	var credentials adapters.BTPCredentials
	err = json.Unmarshal([]byte(credentialsJSON), &credentials)
	if err != nil {
		panic(fmt.Sprintf("Failed to unmarshal credentials JSON from '%s': %v", envFilePath, err))
	}
	return &credentials
}

func RetrieveKubeConfigPath() string {
	file := utils.OpenFile(utils.ConfigName)
	defer file.Close()

	// Read the file line by line
	scanner := bufio.NewScanner(file)
	for scanner.Scan() {
		line := scanner.Text()
		// Split the line by "="
		parts := strings.SplitN(line, "=", 2)
		if len(parts) == 2 && parts[0] == fieldKubeConfigPath {
			return parts[1]
		}
	}
	return getKubeConfigFallBackPath()
}

func RetrieveTransactionID() string {
	file := utils.OpenFile(utils.ConfigName)
	defer file.Close()

	// Read the file line by line
	scanner := bufio.NewScanner(file)
	for scanner.Scan() {
		line := scanner.Text()
		// Split the line by "="
		parts := strings.SplitN(line, "=", 2)
		if len(parts) == 2 && parts[0] == fieldTransactionID {
			return parts[1]
		}
	}
	return ""
}

func RetrieveConfigPath() string {
	file := utils.OpenFile(utils.ConfigName)
	defer file.Close()

	// Read the file line by line
	scanner := bufio.NewScanner(file)
	for scanner.Scan() {
		line := scanner.Text()
		// Split the line by "="
		parts := strings.SplitN(line, "=", 2)
		if len(parts) == 2 && parts[0] == fieldConfigPath {
			return parts[1]
		}
	}
	return ""
}
