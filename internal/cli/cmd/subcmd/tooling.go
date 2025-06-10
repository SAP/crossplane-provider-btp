package subcmd

import (
	"context"
	"fmt"
	"strings"

	xpv1 "github.com/crossplane/crossplane-runtime/apis/common/v1"
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

var ToolingCMD = &cobra.Command{
	// TODO: better naming and description
	Use:   "tooling",
	Short: "Tooling BTP resources",
	Long:  `Tooling the BTP resources you defined in your config.yaml. Make sure to first run xpbtp init first.`,
	Run: func(cmd *cobra.Command, args []string) {
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

		// if no xpbtp config path is provided, fallback to default
		if configPath == "" {
			configPath = "./config.yaml"
		}

		// Create registry adapter for configuration parsing
		registryAdapter := importer.NewRegistryAdapter()
		cfg, err := cpconfig.LoadAndValidateCLIConfigWithRegistry(configPath, registryAdapter)
		kingpin.FatalIfError(err, "Failed to load configuration")

		saName := "dcom-demo"

		//TODO: move into function
		err = k8sClient.Create(ctx, ServiceManager(saName))
		kingpin.FatalIfError(err, "Failed to create ServiceManager resource")
		cfg.AddTooling(saName, "ServiceManager", xpv1.SecretReference{Name: saName + "-service-manager", Namespace: "default"})
		err = cpconfig.SaveCLIConfig(configPath, cfg)
		kingpin.FatalIfError(err, "Failed to save configuration")

		err = k8sClient.Create(ctx, CloudManagement(saName))
		kingpin.FatalIfError(err, "Failed to create CloudManagement resource")
		cfg.AddTooling(saName, "CloudManagement", xpv1.SecretReference{Name: saName + "-cloud-management", Namespace: "default"})
		err = cpconfig.SaveCLIConfig(configPath, cfg)
		kingpin.FatalIfError(err, "Failed to save configuration")

		err = k8sClient.Create(ctx, CISEntitlement(saName))
		kingpin.FatalIfError(err, "Failed to create CISEntitlement resource")
		cfg.AddTooling(saName, "Entitlement", xpv1.SecretReference{Name: saName + "-cis-entitlement", Namespace: "default"})
		err = cpconfig.SaveCLIConfig(configPath, cfg)
		kingpin.FatalIfError(err, "Failed to save configuration")

	},
}

// AddToolingCMD adds the tooling command to the root command
func AddToolingCMD(rootCmd *cobra.Command) {
	rootCmd.AddCommand(ToolingCMD)
	ToolingCMD.Flags().StringVarP(&configPath, "config", "c", "", "Path to your Import-Config (default ./config.yaml)")
}

func ServiceManager(saName string) *v1beta1.ServiceManager {
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
				// TODO: read from CLI
				SubaccountGuid: "a4f88d21-c1f0-486e-b828-bccf776bc9a3",
				//SubaccountRef: &xpv1.Reference{
				//	Name: "mirza-subaccount-config-block", // Replace with your subaccount name
				//},
			},
			ResourceSpec: xpv1.ResourceSpec{
				WriteConnectionSecretToReference: &xpv1.SecretReference{
					Name:      saName + "-sm-binding",
					Namespace: "default", // Adjust namespace as needed
				},
				ProviderConfigReference: &xpv1.Reference{
					//TODO: read from config
					Name: "default-canary", // Replace with your provider config name
				},
			},
		},
	}

	return serviceManager
}

func CISEntitlement(saName string) *v1alpha1.Entitlement {
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
				ServiceName:     "cis",
				ServicePlanName: "local",
				Enable:          internal.Ptr(true),
				// TODO: read from CLI
				SubaccountGuid: "a4f88d21-c1f0-486e-b828-bccf776bc9a3",
				//SubaccountRef: &xpv1.Reference{
				//	Name: "mirza-subaccount-config-block", // Replace with your subaccount name
				//},
			},
			ResourceSpec: xpv1.ResourceSpec{
				ProviderConfigReference: &xpv1.Reference{
					//TODO: read from config
					Name: "default-canary", // Replace with your provider config name
				},
			},
		},
	}

	return ent
}

func CloudManagement(saName string) *v1alpha1.CloudManagement {
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
				SubaccountGuid: "a4f88d21-c1f0-486e-b828-bccf776bc9a3",
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
					//TODO: read from config
					Name: "default-canary", // Replace with your provider config name
				},
			},
		},
	}

	return cis
}
