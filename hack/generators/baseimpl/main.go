package main

import (
	"flag"
	"log"
	"os"
)

func main() {
	var (
		baseDir           = flag.String("base-dir", "apis/base", "Base types root directory")
		clusterDir        = flag.String("cluster-dir", "apis/cluster", "Cluster-scoped output root directory")
		namespacedDir     = flag.String("namespaced-dir", "apis/namespaced", "Namespaced output root directory")
		logicCtrlDir      = flag.String("logic-ctrl-dir", "internal/controller/logic", "Logic controller root directory")
		clusterCtrlDir    = flag.String("cluster-ctrl-dir", "internal/controller/cluster", "Cluster controller output root directory")
		namespacedCtrlDir = flag.String("namespaced-ctrl-dir", "internal/controller/namespaced", "Namespaced controller output root directory")
		modulePath        = flag.String("module", "github.com/sap/crossplane-provider-btp", "Go module path")
		providerName      = flag.String("provider-name", "btp", "Provider name used in kubebuilder categories")
	)
	flag.Parse()

	log.SetOutput(os.Stdout)
	log.SetFlags(log.Ltime | log.Lshortfile)

	gen := &Generator{
		BaseDir:           *baseDir,
		ClusterDir:        *clusterDir,
		NamespacedDir:     *namespacedDir,
		LogicCtrlDir:      *logicCtrlDir,
		ClusterCtrlDir:    *clusterCtrlDir,
		NamespacedCtrlDir: *namespacedCtrlDir,
		ModulePath:        *modulePath,
		ProviderName:      *providerName,
	}

	if err := gen.Generate(); err != nil {
		log.Fatalf("generation failed: %v", err)
	}

	log.Println("Code generation completed successfully")
}
