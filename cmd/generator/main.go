/*
Copyright 2021 Upbound Inc.
*/

package main

import (
	"fmt"
	"os"
	"path/filepath"

	"github.com/crossplane/upjet/v2/pkg/pipeline"

	"github.com/sap/crossplane-provider-btp/config"
)

func main() {
	if len(os.Args) < 2 || os.Args[1] == "" {
		panic("root directory is required to be given as argument")
	}
	rootDir := os.Args[1]
	absRootDir, err := filepath.Abs(rootDir)
	if err != nil {
		panic(fmt.Sprintf("cannot calculate the absolute path with %s", rootDir))
	}

	// need to overide the rootgroup as we as want to control the name of the CRD groups
	provider := config.GetProvider()
	rg := provider.RootGroup
	fmt.Println(rg)

	pipeline.Run(provider, nil, absRootDir)
}
