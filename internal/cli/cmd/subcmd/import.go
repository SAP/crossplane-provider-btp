package subcmd

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"strings"

	xpv1 "github.com/crossplane/crossplane-runtime/apis/common/v1"
	"github.com/crossplane/crossplane-runtime/pkg/meta"
	"github.com/fatih/color"
	"github.com/sap/crossplane-provider-btp/apis"
	v1alpha1env "github.com/sap/crossplane-provider-btp/apis/environment/v1alpha1"
	providerv1alpha1 "github.com/sap/crossplane-provider-btp/apis/v1alpha1"
	"github.com/sap/crossplane-provider-btp/btp"
	"github.com/sap/crossplane-provider-btp/internal/cli/adapters"
	cli "github.com/sap/crossplane-provider-btp/internal/cli/pkg/credentialManager"
	"github.com/sap/crossplane-provider-btp/internal/cli/pkg/utils"
	cpconfig "github.com/sap/crossplane-provider-btp/internal/crossplaneimport/config"
	"github.com/sap/crossplane-provider-btp/internal/crossplaneimport/importer"
	"github.com/sap/crossplane-provider-btp/internal/crossplaneimport/resource"
	"github.com/spf13/cobra"
	"gopkg.in/alecthomas/kingpin.v2"
	v1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/client-go/tools/clientcmd"
	"sigs.k8s.io/controller-runtime/pkg/client"

	// Import adapters so they register themselves
	_ "github.com/sap/crossplane-provider-btp/internal/cli/adapters/v1alpha1"
)

var (
	preview    bool
	configPath string
	// singleImportKind is used to specify a single resource kind to import
	singleImportKind string
	singleImportSa   string
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

		// if no config config path is provided, fallback to default
		if configPath == "" {
			configPath = "./config.yaml"
		}

		// Create registry adapter for configuration parsing
		registryAdapter := importer.NewRegistryAdapter()
		cfg, err := cpconfig.LoadAndValidateCLIConfigWithRegistry(configPath, registryAdapter)
		kingpin.FatalIfError(err, "Failed to load configuration")

		// Create Kubernetes client
		k8sConfig, err := clientcmd.BuildConfigFromFlags("", kubeConfigPath)
		kingpin.FatalIfError(err, "Failed to build Kubernetes config")

		// Create scheme with all necessary types
		scheme := runtime.NewScheme()
		err = apis.AddToScheme(scheme)
		kingpin.FatalIfError(err, "Failed to add APIs to scheme")
		err = v1.AddToScheme(scheme)
		kingpin.FatalIfError(err, "Failed to add core v1 to scheme")
		k8sClient, err := client.New(k8sConfig, client.Options{Scheme: scheme})
		kingpin.FatalIfError(err, "Failed to create Kubernetes client")

		if singleImportKind != "" {
			importSingleResource(cfg, k8sClient, ctx, singleImportKind, singleImportSa, preview)
		} else {
			// Create BTP client by loading credentials from environment file
			btpClient, err := createBTPClient()
			kingpin.FatalIfError(err, "Failed to create BTP client")

			importFromConfig(btpClient, cfg, k8sClient, ctx, preview)
		}
	},
}

func importSingleResource(cfg *cpconfig.ImportConfig, k8sClient client.Client, ctx context.Context, kind string, saName string, preview bool) {
	//TODO: make dependant on kind
	requiredTool := "CloudManagement"
	tooling := cfg.FindTooling(saName, requiredTool)
	if tooling == nil {
		kingpin.FatalIfError(fmt.Errorf("no tooling found for kind %s with service account %s", requiredTool, saName), "Failed to find tooling configuration")
	}
	// lookup secret from cached tooling
	localCisSecret := v1.Secret{}
	err := k8sClient.Get(ctx, client.ObjectKey{Name: tooling.SecretReference.Name, Namespace: tooling.SecretReference.Namespace}, &localCisSecret)
	kingpin.FatalIfError(err, "Failed to get local secret %s in namespace %s", tooling.SecretReference.Name, tooling.SecretReference.Namespace)

	cisBinding := localCisSecret.Data[providerv1alpha1.RawBindingKey]
	if cisBinding == nil {
		kingpin.FatalIfError(err, "Failed to get local secret %s in namespace %s", tooling.SecretReference.Name, tooling.SecretReference.Namespace)
	}

	btpClient, err := btp.NewBTPClient(cisBinding, []byte("{}"))
	kingpin.FatalIfError(err, "failed to create BTP client")

	res, _, err := btpClient.ProvisioningServiceClient.GetEnvironmentInstances(ctx).Authorization("").Execute()
	if err != nil {
		kingpin.FatalIfError(err, "failed to get environment instances from BTP")
	}

	resInstance := res.EnvironmentInstances[0]

	params := map[string]string{}
	err = json.Unmarshal([]byte(*resInstance.Parameters), &params)
	kingpin.FatalIfError(err, "failed to unmarshal parameters from environment instance")

	env := v1alpha1env.CloudFoundryEnvironment{
		TypeMeta: metav1.TypeMeta{
			APIVersion: v1alpha1env.SchemeGroupVersion.String(),
			Kind:       v1alpha1env.CfEnvironmentKind,
		},
		ObjectMeta: metav1.ObjectMeta{
			Name: saName + "-cf",
		},
		Spec: v1alpha1env.CfEnvironmentSpec{
			ResourceSpec: xpv1.ResourceSpec{
				ProviderConfigReference: &xpv1.Reference{
					Name: cfg.ProviderConfigRefName,
				},
				ManagementPolicies: XPManagementPolicies(cfg),
			},
			ForProvider: v1alpha1env.CfEnvironmentParameters{
				Managers:        []string{},
				Landscape:       *resInstance.LandscapeLabel,
				OrgName:         params["instance_name"],
				EnvironmentName: *resInstance.Name,
			},
			SubaccountGuid: *resInstance.SubaccountGUID,
			//TODO: add
			//SubaccountRef:      &v1.Reference{},
			CloudManagementRef: &xpv1.Reference{
				//TODO: save in state and read from it
				Name: tooling.Subaccount + "-cloud-management",
			},
		},
		Status: v1alpha1env.EnvironmentStatus{},
	}
	meta.SetExternalName(&env, *resInstance.Name)

	err = k8sClient.Create(ctx, &env)
	kingpin.FatalIfError(err, "Failed to create CloudFoundryEnvironment resource")

}

func importFromConfig(btpClient *btp.Client, cfg *cpconfig.ImportConfig, k8sClient client.Client, ctx context.Context, preview bool) {

	// Wrap the BTP client
	wrappedBTPClient := resource.NewBTPClientWrapper(btpClient)

	// Create importer with the new structure
	importerInstance := importer.NewImporter(wrappedBTPClient, k8sClient)

	// Run the import process
	err := importerInstance.RunImportProcess(ctx, cfg, preview)
	kingpin.FatalIfError(err, "%s", errImportResources)

	if !preview {
		fmt.Println("✅ Resource(s) successfully imported")
		fmt.Println("\n If you want to revert the import run:")
		fmt.Println(suggestionColorLocal("kubectl delete <RESOURCE TYPE> -l import.xpbtp.crossplane.io/transaction-id=<TRANSACTION_ID>"))
		fmt.Println("(The transaction ID is displayed in the import process output above)")
	}
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
	ImportCMD.Flags().StringVarP(&singleImportKind, "kind", "k", "", "Kind of single resource to import (e.g. Cloudfoundry). If not set, all resources will be imported based on config.")
	ImportCMD.Flags().StringVarP(&singleImportSa, "subaccount", "s", "", "Subaccount of the resource to import (e.g. my-subaccount). If not set, all resources will be imported based on config.")
}

func XPManagementPolicies(cfg *cpconfig.ImportConfig) []xpv1.ManagementAction {
	// Set management policy - only use standard Crossplane ManagementActions
	switch cfg.ManagementPolicy {
	case "Observe":
		return []xpv1.ManagementAction{xpv1.ManagementActionObserve}
	case "*":
		return []xpv1.ManagementAction{xpv1.ManagementActionAll}
	case "Create":
		return []xpv1.ManagementAction{xpv1.ManagementActionCreate}
	case "Update":
		return []xpv1.ManagementAction{xpv1.ManagementActionUpdate}
	case "Delete":
		return []xpv1.ManagementAction{xpv1.ManagementActionDelete}
	case "LateInitialize":
		return []xpv1.ManagementAction{xpv1.ManagementActionLateInitialize}
	default:
		// Default to Observe for unknown policies
		return []xpv1.ManagementAction{xpv1.ManagementActionObserve}
	}
}
