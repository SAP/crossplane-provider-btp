package subcmd

import (
	"context"
	"fmt"
	"strings"

	xpv1 "github.com/crossplane/crossplane-runtime/apis/common/v1"
	"github.com/google/uuid"
	"github.com/sap/crossplane-provider-btp/apis"
	"github.com/sap/crossplane-provider-btp/apis/account/v1alpha1"
	"github.com/sap/crossplane-provider-btp/apis/account/v1beta1"
	"github.com/sap/crossplane-provider-btp/internal"
	cli "github.com/sap/crossplane-provider-btp/internal/cli/pkg/credentialManager"
	cpconfig "github.com/sap/crossplane-provider-btp/internal/crossplaneimport/config"
	"github.com/sap/crossplane-provider-btp/internal/crossplaneimport/importer"
	"github.com/spf13/cobra"
	"gopkg.in/alecthomas/kingpin.v2"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/client-go/tools/clientcmd"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

var (
	flagSaName string
	flagSaID   string
)

var ToolingCMD = &cobra.Command{
	// TODO: better naming and description
	Use:   "tooling",
	Short: "Tooling BTP resources",
	Long:  `Tooling the BTP resources you defined in your config.yaml. Make sure to first run xpbtp init first.`,
	Run: func(cmd *cobra.Command, args []string) {
		if flagSaName == "" || flagSaID == "" {
			kingpin.FatalIfError(fmt.Errorf("subaccount name and ID must be provided"), "Subaccount name and ID are required")
		}

		fmt.Println(strings.Repeat("-", 52))
		fmt.Println("| Tooling Run: " + cli.RetrieveTransactionID() + " |")
		fmt.Println(strings.Repeat("-", 52))

		ctx := context.TODO()
		kubeConfigPath := cli.RetrieveKubeConfigPath()

		// Create Kubernetes client
		k8sConfig, err := clientcmd.BuildConfigFromFlags("", kubeConfigPath)
		kingpin.FatalIfError(err, "Failed to build Kubernetes config")

		// Create scheme with all necessary types
		scheme := runtime.NewScheme()
		err = apis.AddToScheme(scheme)
		kingpin.FatalIfError(err, "Failed to add APIs to scheme")

		k8sClient, err := client.New(k8sConfig, client.Options{Scheme: scheme})
		kingpin.FatalIfError(err, "Failed to create Kubernetes client")

		// if no config path is provided, fallback to default
		if configPath == "" {
			configPath = "./config.yaml"
		}

		// Create registry adapter for configuration parsing
		registryAdapter := importer.NewRegistryAdapter()
		cfg, err := cpconfig.LoadAndValidateCLIConfigWithRegistry(configPath, registryAdapter)
		kingpin.FatalIfError(err, "Failed to load configuration")

		transactionID := uuid.New().String()
		fmt.Printf("Starting import process. Transaction ID: %s, ProviderConfigRef: %s, Default ManagementPolicy: %s\n", transactionID, cfg.ProviderConfigRefName, cfg.ManagementPolicy)
		fmt.Printf("Create Tooling for Subaccount %s...\n", flagSaName)

		//TODO: move into function
		sm := ServiceManager(flagSaName, flagSaID, cfg.ProviderConfigRefName)
		err = k8sClient.Create(ctx, sm)
		kingpin.FatalIfError(err, "Failed to create ServiceManager resource")
		cfg.AddTooling(sm.Name, flagSaName, "ServiceManager", flagSaID, xpv1.SecretReference{Name: flagSaName + "-sm-binding", Namespace: "default"})
		err = cpconfig.SaveCLIConfig(configPath, cfg)
		kingpin.FatalIfError(err, "Failed to save configuration")

		cis := CloudManagement(flagSaName, flagSaID, cfg.ProviderConfigRefName)
		err = k8sClient.Create(ctx, cis)
		kingpin.FatalIfError(err, "Failed to create CloudManagement resource")
		cfg.AddTooling(cis.Name, flagSaName, "CloudManagement", flagSaID, xpv1.SecretReference{Name: flagSaName + "-cis-binding", Namespace: "default"})
		err = cpconfig.SaveCLIConfig(configPath, cfg)
		kingpin.FatalIfError(err, "Failed to save configuration")

		ent := CISEntitlement(flagSaName, flagSaID, cfg.ProviderConfigRefName)
		err = k8sClient.Create(ctx, ent)
		kingpin.FatalIfError(err, "Failed to create CISEntitlement resource")
		cfg.AddTooling(ent.Name, flagSaName, "Entitlement", flagSaID, xpv1.SecretReference{Name: flagSaName + "-cis-entitlement", Namespace: "default"})
		err = cpconfig.SaveCLIConfig(configPath, cfg)
		kingpin.FatalIfError(err, "Failed to save configuration")

		fmt.Printf("Tooling Resources created successfully in cluster, please wait for them to reconcile before proceeding with imports\n")
	},
}

// AddToolingCMD adds the tooling command to the root command
func AddToolingCMD(rootCmd *cobra.Command) {
	rootCmd.AddCommand(ToolingCMD)
	ToolingCMD.Flags().StringVarP(&configPath, "config", "c", "", "Path to your Import-Config (default ./config.yaml)")
	ToolingCMD.Flags().StringVarP(&flagSaName, "subaccount", "s", "", "Subaccount of the resource for which to create the tooling (e.g. my-subaccount)")
	ToolingCMD.Flags().StringVarP(&flagSaID, "subaccount-id", "i", "", "Subaccount ID of the resource for which to create the tooling (e.g. a4f88d21-c1f0-486e-b828-bccf776bc9a3)")
}

func ServiceManager(saName, saID, providerConfigRef string) *v1beta1.ServiceManager {
	serviceManager := &v1beta1.ServiceManager{
		TypeMeta: metav1.TypeMeta{
			APIVersion: "account.btp.sap.com/v1beta1",
			Kind:       "ServiceManager",
		},
		ObjectMeta: metav1.ObjectMeta{
			// TODO: set something useful here
			Name: saName + "-service-manager",
		},
		Spec: v1beta1.ServiceManagerSpec{
			ForProvider: v1beta1.ServiceManagerParameters{
				SubaccountGuid: saID,
				//SubaccountRef: &xpv1.Reference{
				//	Name: "mirza-subaccount-config-block", // Replace with your subaccount name
				//},
			},
			ResourceSpec: xpv1.ResourceSpec{
				WriteConnectionSecretToReference: &xpv1.SecretReference{
					Name:      saName + "-sm-binding",
					Namespace: "default",
				},
				ProviderConfigReference: &xpv1.Reference{
					Name: providerConfigRef,
				},
			},
		},
	}
	fmt.Printf("- ServiceManager: %s\n", serviceManager.Name)
	return serviceManager
}

func CISEntitlement(saName, saID, providerConfigRef string) *v1alpha1.Entitlement {
	serviceName := "cis"
	servicePlanName := "local"

	ent := &v1alpha1.Entitlement{
		TypeMeta: metav1.TypeMeta{
			APIVersion: "account.btp.sap.com/v1alpha1",
			Kind:       "Entitlement",
		},
		ObjectMeta: metav1.ObjectMeta{
			// TODO: set something useful here
			Name: saName + "-cis-entitlement",
		},
		Spec: v1alpha1.EntitlementSpec{
			ForProvider: v1alpha1.EntitlementParameters{
				ServiceName:     serviceName,
				ServicePlanName: servicePlanName,
				Enable:          internal.Ptr(true),
				// TODO: read from CLI
				SubaccountGuid: saID,
				//SubaccountRef: &xpv1.Reference{
				//	Name: "mirza-subaccount-config-block", // Replace with your subaccount name
				//},
			},
			ResourceSpec: xpv1.ResourceSpec{
				ProviderConfigReference: &xpv1.Reference{
					Name: providerConfigRef,
				},
			},
		},
	}

	fmt.Printf("- Entitlement: %s with ServiceName: %s and PlanName: %s\n", ent.Name, serviceName, servicePlanName)
	return ent
}

func CloudManagement(saName, saID, providerConfigRef string) *v1alpha1.CloudManagement {
	cis := &v1alpha1.CloudManagement{
		TypeMeta: metav1.TypeMeta{
			APIVersion: "account.btp.sap.com/v1alpha1",
			Kind:       "CloudManagement",
		},
		ObjectMeta: metav1.ObjectMeta{
			// TODO: set something useful here
			Name: saName + "-cloud-management",
		},
		Spec: v1alpha1.CloudManagementSpec{
			ForProvider: v1alpha1.CloudManagementParameters{
				// TODO: read from CLI
				SubaccountGuid: saID,
				//SubaccountRef: &xpv1.Reference{
				//	Name: "mirza-subaccount-config-block", // Replace with your subaccount name
				//},
				ServiceManagerRef: &xpv1.Reference{
					//TODO: make dynamic
					Name: saName + "-service-manager", // Replace with your ServiceManager name
				},
			},
			ResourceSpec: xpv1.ResourceSpec{
				WriteConnectionSecretToReference: &xpv1.SecretReference{
					Name:      saName + "-cis-binding",
					Namespace: "default", // Adjust namespace as needed
				},
				ProviderConfigReference: &xpv1.Reference{
					Name: providerConfigRef,
				},
			},
		},
	}

	fmt.Printf("- CloudManagement: %s\n", cis.Name)
	return cis
}
