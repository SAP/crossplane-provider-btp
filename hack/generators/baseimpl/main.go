/*
Copyright 2025 The Crossplane Authors.

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

    http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
*/

// Package main provides a code generator that reads base type definitions
// and generates scope-specific (cluster and namespaced) variants.
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
		clusterCtrlDir    = flag.String("cluster-ctrl-dir", "internal/controller/cluster", "Cluster controller output root directory")
		namespacedCtrlDir = flag.String("namespaced-ctrl-dir", "internal/controller/namespaced", "Namespaced controller output root directory")
		modulePath        = flag.String("module", "github.com/sap/crossplane-provider-btp", "Go module path")
		providerName      = flag.String("provider-name", "btp", "Provider name used in kubebuilder categories")
		skipProviderConfig = flag.Bool("skip-providerconfig", false, "Skip generating ProviderConfig types and controller")
	)
	flag.Parse()

	log.SetOutput(os.Stdout)
	log.SetFlags(log.Ltime | log.Lshortfile)

	gen := &Generator{
		BaseDir:           *baseDir,
		ClusterDir:        *clusterDir,
		NamespacedDir:     *namespacedDir,
		ClusterCtrlDir:    *clusterCtrlDir,
		NamespacedCtrlDir: *namespacedCtrlDir,
		ModulePath:        *modulePath,
		ProviderName:      *providerName,
		SkipProviderConfig: *skipProviderConfig,
	}

	if err := gen.Generate(); err != nil {
		log.Fatalf("generation failed: %v", err)
	}

	log.Println("Code generation completed successfully")
}
