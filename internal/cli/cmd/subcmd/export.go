package subcmd

import (
	"context"
	"fmt"
	"os"
	"strings"

	"github.com/sap/crossplane-provider-btp/apis"
	cli "github.com/sap/crossplane-provider-btp/internal/cli/pkg/credentialManager"
	cpconfig "github.com/sap/crossplane-provider-btp/internal/crossplaneimport/config"
	"github.com/sap/crossplane-provider-btp/internal/crossplaneimport/importer"
	"github.com/spf13/cobra"
	"gopkg.in/alecthomas/kingpin.v2"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/client-go/tools/clientcmd"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/yaml"

	accountv1alpha1 "github.com/sap/crossplane-provider-btp/apis/account/v1alpha1"
	env1alpha1 "github.com/sap/crossplane-provider-btp/apis/environment/v1alpha1"
)

var ExportCMD = &cobra.Command{
	// TODO: better naming and description
	Use:   "export",
	Short: "Export previously imported BTP resources",
	Long:  `Export the BTP resources that where imported using the xpbtp import command. This will create a YAML file with all resources that were imported into the cluster.`,
	Run: func(cmd *cobra.Command, args []string) {
		fmt.Println(strings.Repeat("-", 52))
		fmt.Println("| Export Run: " + cli.RetrieveTransactionID() + " |")
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

		// Kind to type mapping
		kindToType := map[string]func() client.Object{
			"Subaccount":              func() client.Object { return &accountv1alpha1.Subaccount{} },
			"CloudManagement":         func() client.Object { return &accountv1alpha1.CloudManagement{} },
			"ServiceManager":          func() client.Object { return &accountv1alpha1.ServiceManager{} },
			"Entitlement":             func() client.Object { return &accountv1alpha1.Entitlement{} },
			"CloudFoundryEnvironment": func() client.Object { return &env1alpha1.CloudFoundryEnvironment{} },
		}

		// Helper to collect objects by kind/name from a list
		collectObjects := func(entries []struct{ Kind, Name, Namespace string }) []client.Object {
			var objs []client.Object
			for _, ent := range entries {
				newObjFunc, ok := kindToType[ent.Kind]
				if !ok {
					fmt.Printf("Warning: kind %s is not supported for export\n", ent.Kind)
					continue
				}
				obj := newObjFunc()
				key := client.ObjectKey{Name: ent.Name}
				if ent.Namespace != "" {
					key.Namespace = ent.Namespace
				}
				err := k8sClient.Get(ctx, key, obj)
				if err != nil {
					fmt.Printf("Warning: could not get %s/%s: %v\n", ent.Kind, ent.Name, err)
					continue
				}
				objs = append(objs, obj)
			}
			return objs
		}

		// Prepare entries from imported and tooling
		importedEntries := make([]struct{ Kind, Name, Namespace string }, 0, len(cfg.Imported))
		for _, imp := range cfg.Imported {
			importedEntries = append(importedEntries, struct{ Kind, Name, Namespace string }{imp.Kind, imp.Name, imp.Namespace})
		}
		toolingEntries := make([]struct{ Kind, Name, Namespace string }, 0, len(cfg.Tooling))
		for _, t := range cfg.Tooling {
			toolingEntries = append(toolingEntries, struct{ Kind, Name, Namespace string }{t.Kind, t.Name, ""})
		}

		objects := collectObjects(importedEntries)
		objects = append(objects, collectObjects(toolingEntries)...)

		if len(objects) == 0 {
			fmt.Println("No resources found to export.")
			return
		}

		// Marshal all objects to YAML (cleaning metadata and status)
		var allYaml []byte
		for _, obj := range objects {
			uObj := &unstructured.Unstructured{}
			err := k8sClient.Scheme().Convert(obj, uObj, nil)
			if err != nil {
				fmt.Printf("Warning: could not convert object %s/%s to unstructured: %v\n", obj.GetObjectKind().GroupVersionKind().Kind, obj.GetName(), err)
				continue
			}

			metadata := uObj.Object["metadata"].(map[string]interface{})
			for _, field := range []string{"managedFields", "creationTimestamp", "resourceVersion", "uid", "generation", "finalizers", "ownerReferences"} {
				delete(metadata, field)
			}
			if annotations, ok := metadata["annotations"].(map[string]interface{}); ok {
				delete(annotations, "crossplane.io/external-name")
				if len(annotations) == 0 {
					delete(metadata, "annotations")
				} else {
					metadata["annotations"] = annotations
				}
			}
			for k := range metadata {
				if k != "name" && k != "namespace" && k != "labels" && k != "annotations" {
					delete(metadata, k)
				}
			}
			uObj.Object["metadata"] = metadata
			delete(uObj.Object, "status")

			b, err := yaml.Marshal(uObj.Object)
			if err != nil {
				fmt.Printf("Warning: could not marshal object %s/%s: %v\n", obj.GetObjectKind().GroupVersionKind().Kind, obj.GetName(), err)
				continue
			}
			allYaml = append(allYaml, b...)
			allYaml = append(allYaml, []byte("---\n")...)
		}

		exportFile := "exported-resources.yaml"
		err = os.WriteFile(exportFile, allYaml, 0644)
		if err != nil {
			fmt.Printf("Failed to write export file: %v\n", err)
			return
		}
		fmt.Printf("Exported %d resources to %s\n", len(objects), exportFile)

	},
}

// AddToolingCMD adds the tooling command to the root command
func AddExportCMD(rootCmd *cobra.Command) {
	rootCmd.AddCommand(ExportCMD)
	ExportCMD.Flags().StringVarP(&configPath, "config", "c", "", "Path to your Import-Config (default ./config.yaml)")
}
