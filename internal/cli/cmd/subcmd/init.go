package subcmd

import (
	"context"
	"fmt"

	"github.com/fatih/color"
	v1alpha1 "github.com/sap/crossplane-provider-btp/apis/v1alpha1"
	v1beta1 "github.com/sap/crossplane-provider-btp/apis/v1beta1"
	"github.com/sap/crossplane-provider-btp/internal/cli/adapters"
	"github.com/sap/crossplane-provider-btp/internal/cli/pkg/credentialManager"
	"github.com/spf13/cobra"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/runtime"
)

var (
	successColor    = color.New(color.FgGreen).SprintFunc()
	suggestionColor = color.New(color.FgCyan).SprintFunc()
	kubeConfigPath  string
	xpbtpConfigPath string
	envFilePath     string
)

var (
	errParseConfig = "Could not parse config file"
)

var InitCMD = &cobra.Command{
	Use:   "init",
	Short: "Initializes environment",
	Long:  `Creates an env-file for storing authentication details`,
	RunE: func(cmd *cobra.Command, args []string) error {
		ctx := context.TODO()

		// Create schemes
		scheme := runtime.NewScheme()
		err := v1beta1.SchemeBuilder.AddToScheme(scheme)
		if err != nil {
			return fmt.Errorf("%s: %w", errAddv1beta1Scheme, err)
		}
		err = v1alpha1.SchemeBuilder.AddToScheme(scheme)
		if err != nil {
			return fmt.Errorf("%s: %w", errAddv1alpha1Scheme, err)
		}
		err = corev1.AddToScheme(scheme)
		if err != nil {
			return fmt.Errorf("%s: %w", errAddCorev1Scheme, err)
		}

		// Create adapters
		clientAdapter := &adapters.BTPClientAdapter{}
		configParser := &adapters.BTPConfigParser{}

		// if no xpbtp config path is provided, fallback to default
		if xpbtpConfigPath == "" {
			xpbtpConfigPath = "./config.yaml"
		}

		// if no env file path is provided, fallback to default
		if envFilePath == "" {
			envFilePath = "./.xpbtp_env.yaml"
		}

		// Parse the xpbtp configuration file to extract providerConfigRef.Name
		providerConfigRef, _, err := configParser.ParseConfig(xpbtpConfigPath)
		if err != nil {
			return fmt.Errorf("%s: %w", errParseConfig, err)
		}

		// Extract the provider config name from the parsed configuration
		providerConfigName := providerConfigRef.GetProviderConfigRef().Name
		if providerConfigName == "" {
			return fmt.Errorf("providerConfigRef.name is missing or empty in configuration file %s", xpbtpConfigPath)
		}

		// Create environment with the extracted provider config name
		err = credentialManager.CreateEnvironment(kubeConfigPath, envFilePath, ctx, providerConfigName, clientAdapter, scheme)
		if err != nil {
			return fmt.Errorf("failed to create environment: %w", err)
		}

		fmt.Println(successColor("\nReady..."))
		fmt.Println("\nStart your import with:")
		fmt.Println(suggestionColor("xpbtp import [--preview | -p]"))
		return nil
	},
}

// AddInitCMD adds the init command to the root command
func AddInitCMD(rootCmd *cobra.Command) {
	rootCmd.AddCommand(InitCMD)
	InitCMD.Flags().StringVar(&xpbtpConfigPath, "config", "", "Path to the xpbtp configuration file (default ./config.yaml)")
	InitCMD.Flags().StringVar(&kubeConfigPath, "kubeconfig", "", "Path to your Kubernetes kubeconfig file (optional, defaults to standard client-go behavior)")
	InitCMD.Flags().StringVar(&envFilePath, "env-file", "", "Path where the fetched BTP credentials and environment settings will be stored (default ./.xpbtp_env.yaml)")
}
