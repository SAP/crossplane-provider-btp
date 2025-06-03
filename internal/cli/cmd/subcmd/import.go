package subcmd

import (
	"context"
	"fmt"
	"os"
	"strings"

	"github.com/fatih/color"
	"github.com/sap/crossplane-provider-btp/apis"
	"github.com/sap/crossplane-provider-btp/btp"
	"github.com/sap/crossplane-provider-btp/internal/cli/adapters"
	cli "github.com/sap/crossplane-provider-btp/internal/cli/pkg/credentialManager"
	"github.com/sap/crossplane-provider-btp/internal/cli/pkg/utils"
	cpconfig "github.com/sap/crossplane-provider-btp/internal/crossplaneimport/config"
	"github.com/sap/crossplane-provider-btp/internal/crossplaneimport/importer"
	"github.com/sap/crossplane-provider-btp/internal/crossplaneimport/resource"
	"github.com/spf13/cobra"
	"gopkg.in/alecthomas/kingpin.v2"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/client-go/tools/clientcmd"
	"sigs.k8s.io/controller-runtime/pkg/client"

	// Import adapters so they register themselves
	_ "github.com/sap/crossplane-provider-btp/internal/cli/adapters/v1alpha1"
)

var (
	preview    bool
	configPath string
)

var (
	// error messages
	errImportResources = "Could not import resources"
)

// Define colors for this file
var suggestionColorLocal = color.New(color.FgCyan).SprintFunc()

var ImportCMD = &cobra.Command{
	Use:   "import",
	Short: "Import BTP resources",
	Long:  `Import the BTP resources you defined in your config.yaml. Make sure to first run xpbtp init first.`,
	Run: func(cmd *cobra.Command, args []string) {
		utils.UpdateTransactionID()
		fmt.Println(strings.Repeat("-", 52))
		fmt.Println("| Import Run: " + cli.RetrieveTransactionID() + " |")
		fmt.Println(strings.Repeat("-", 52))

		ctx := context.TODO()
		kubeConfigPath := cli.RetrieveKubeConfigPath()

		// Load configuration using the new parser
		if configPath == "" {
			configPath = cli.RetrieveConfigPath()
		}

		// Create registry adapter for configuration parsing
		registryAdapter := importer.NewRegistryAdapter()
		cfg, err := cpconfig.LoadAndValidateCLIConfigWithRegistry(configPath, registryAdapter)
		kingpin.FatalIfError(err, "Failed to load configuration")

		// Create BTP client by loading credentials from environment file
		btpClient, err := createBTPClient()
		kingpin.FatalIfError(err, "Failed to create BTP client")

		// Wrap the BTP client
		wrappedBTPClient := resource.NewBTPClientWrapper(btpClient)

		// Create Kubernetes client
		k8sConfig, err := clientcmd.BuildConfigFromFlags("", kubeConfigPath)
		kingpin.FatalIfError(err, "Failed to build Kubernetes config")

		// Create scheme with all necessary types
		scheme := runtime.NewScheme()
		err = apis.AddToScheme(scheme)
		kingpin.FatalIfError(err, "Failed to add APIs to scheme")

		k8sClient, err := client.New(k8sConfig, client.Options{Scheme: scheme})
		kingpin.FatalIfError(err, "Failed to create Kubernetes client")

		// Create importer with the new structure
		importerInstance := importer.NewImporter(wrappedBTPClient, k8sClient)

		// Run the import process
		err = importerInstance.RunImportProcess(ctx, cfg, preview)
		kingpin.FatalIfError(err, "%s", errImportResources)

		if !preview {
			fmt.Println("✅ Resource(s) successfully imported")
			fmt.Println("\n If you want to revert the import run:")
			fmt.Println(suggestionColorLocal("kubectl delete <RESOURCE TYPE> -l import.xpbtp.crossplane.io/transaction-id=<TRANSACTION_ID>"))
			fmt.Println("(The transaction ID is displayed in the import process output above)")
		}
	},
}

// createBTPClient creates a BTP client by loading credentials from the environment file
func createBTPClient() (*btp.Client, error) {
	// Check if environment file exists
	envFilePath := "./.xpbtp_env.yaml"
	if _, err := os.Stat(envFilePath); os.IsNotExist(err) {
		return nil, fmt.Errorf("environment file not found. Please run 'xpbtp init' first to set up credentials")
	}

	// Retrieve credentials from the environment file created by init command
	credentials := cli.RetrieveCredentials()
	if credentials == nil {
		return nil, fmt.Errorf("no credentials found. Please run 'xpbtp init' first to set up credentials")
	}

	// Cast to BTPCredentials to access the credential data
	btpCreds, ok := credentials.(*adapters.BTPCredentials)
	if !ok {
		return nil, fmt.Errorf("invalid credentials type: expected *adapters.BTPCredentials, got %T. Please run 'xpbtp init' to refresh credentials", credentials)
	}

	// Get the auth data from credentials
	authData := btpCreds.GetAuthData()
	cisSecretData, hasCIS := authData["cisSecret"]
	serviceAccountSecretData, hasSA := authData["serviceAccountSecret"]

	if !hasCIS {
		return nil, fmt.Errorf("CIS secret data not found in credentials. Please run 'xpbtp init' to refresh credentials")
	}
	if !hasSA {
		return nil, fmt.Errorf("service account secret data not found in credentials. Please run 'xpbtp init' to refresh credentials")
	}

	if len(cisSecretData) == 0 {
		return nil, fmt.Errorf("CIS secret data is empty. Please run 'xpbtp init' to refresh credentials")
	}
	if len(serviceAccountSecretData) == 0 {
		return nil, fmt.Errorf("service account secret data is empty. Please run 'xpbtp init' to refresh credentials")
	}

	// Create BTP client using the credential data
	btpClient, err := btp.NewBTPClient(cisSecretData, serviceAccountSecretData)
	if err != nil {
		return nil, fmt.Errorf("failed to create BTP client: %w. Please verify your credentials by running 'xpbtp init'", err)
	}

	return btpClient, nil
}

func AddImportCMD(rootCmd *cobra.Command) {
	rootCmd.AddCommand(ImportCMD)
	ImportCMD.Flags().BoolVarP(&preview, "preview", "p", false, "Get a detailed overview on importable resources")
	ImportCMD.Flags().StringVarP(&configPath, "config", "c", "", "Path to your Import-Config (default ./config.yaml)")
}
