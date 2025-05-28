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
func CreateEnvironment(kubeConfigPath string, configPath string, ctx context.Context, providerConfigNamespace string, providerConfigName string, clientAdapter *adapters.BTPClientAdapter, scheme *runtime.Scheme) error {
	// If no kubeConfigPath is provided fall back to ~/.kube/config
	if kubeConfigPath == "" {
		kubeConfigPath = getKubeConfigFallBackPath()
	}

	creds, err := clientAdapter.GetCredentials(ctx, kubeConfigPath, client.ProviderConfigRef{Name: providerConfigName, Namespace: providerConfigNamespace}, scheme)
	if err != nil {
		return fmt.Errorf("%s: %w", errGetCredentials, err)
	}

	err = storeCredentials(creds, kubeConfigPath, configPath)
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

	err := scanner.Err()
	if err != nil {
		panic(fmt.Sprintf("%s: %v", errParseCredEnvironment, err))
	}

	// map values and return Credentials
	var credentials adapters.BTPCredentials
	err = json.Unmarshal([]byte(config[fieldCredentials]), &credentials)
	if err != nil {
		panic(fmt.Sprintf("%s: %v", errUnmarshalCredentials, err))
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
